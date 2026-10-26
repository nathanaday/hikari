package systems

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/wiki"
)

// The agent kinds of each scope of wiki (section 2 of the wikis spec).
var (
	systemKinds = []string{wiki.Feature, wiki.Principle, wiki.Rule}
	hubKinds    = []string{wiki.Story, wiki.Rule, wiki.Principle}
	// releaseKinds are the documents a release copies (rule L2).
	releaseKinds = []string{wiki.Feature, wiki.Principle, wiki.Rule}
)

// wikiOf is one wiki of the store: a system's or a hub's.
type wikiOf struct {
	ref   string
	scope string // "system" or "hub"
	name  string
	// author writes its agent documents: the owner, or the hub's
	// orchestrator.
	author string
	// dir is its directory, relative to the store, with slashes.
	dir   string
	store *wiki.Store
}

// label names the wiki for a person or an agent: "#shop", or the system.
func (w *wikiOf) label() string {
	if w.scope == "hub" {
		return "#" + w.name
	}
	return w.name
}

// openWikis opens the wiki of every system and hub (rule L1).
func (m *Manager) openWikis() error {
	for _, name := range m.topo.Owners() {
		s, err := wiki.Open(m.store.SystemWiki(name), systemKinds)
		if err != nil {
			return fmt.Errorf("agent %q: %w", name, err)
		}
		m.wikis[api.SystemWikiRef(name)] = &wikiOf{
			ref: api.SystemWikiRef(name), scope: "system", name: name, author: name,
			dir: m.store.Rel(m.store.SystemWiki(name)), store: s,
		}
	}
	for _, hub := range m.topo.HubNames() {
		s, err := wiki.Open(m.store.HubWiki(hub), hubKinds)
		if err != nil {
			return fmt.Errorf("hub %q: %w", hub, err)
		}
		m.wikis[api.HubWikiRef(hub)] = &wikiOf{
			ref: api.HubWikiRef(hub), scope: "hub", name: hub, author: m.topo.Hubs[hub].Orchestrator,
			dir: m.store.Rel(m.store.HubWiki(hub)), store: s,
		}
	}
	return nil
}

// wikiRefs returns the refs of every wiki: the hubs first, then the
// systems, each in name order (rule N1b).
func (m *Manager) wikiRefs() []string {
	var out []string
	for _, hub := range m.topo.HubNames() {
		out = append(out, api.HubWikiRef(hub))
	}
	for _, name := range m.topo.Owners() {
		out = append(out, api.SystemWikiRef(name))
	}
	return out
}

// linkableRefs returns the other wikis whose agent documents w can link
// (rule N2).
func (m *Manager) linkableRefs(w *wikiOf) []string {
	var out []string
	if w.scope == "system" {
		for _, hub := range m.topo.MemberHubs(w.name) {
			out = append(out, api.HubWikiRef(hub))
		}
		return out
	}
	for _, member := range m.topo.Hubs[w.name].Members {
		if _, ok := m.owners[member]; ok {
			out = append(out, api.SystemWikiRef(member))
		}
	}
	for _, hub := range append(m.topo.ChildHubs(w.name), m.topo.ParentHubs(w.name)...) {
		out = append(out, api.HubWikiRef(hub))
	}
	return out
}

// readableRefs returns the wikis that agent reads (rules A1 to A3).
func (m *Manager) readableRefs(agent string) []string {
	var out []string
	if _, ok := m.owners[agent]; ok {
		out = append(out, api.SystemWikiRef(agent))
	}
	for _, hub := range m.topo.ReadableHubWikis(agent) {
		out = append(out, api.HubWikiRef(hub))
	}
	for _, sys := range m.topo.VisibleSystems(agent) {
		out = append(out, api.SystemWikiRef(sys))
	}
	return out
}

// check returns what the rules of w need from the other wikis.
func (m *Manager) check(w *wikiOf) wiki.Check {
	c := wiki.Check{Kinds: w.store.Kinds(), Taken: map[string]bool{}}
	linkable := map[string]bool{}
	for _, ref := range m.linkableRefs(w) {
		for id, d := range m.wikis[ref].store.Wiki() {
			if !d.User() {
				linkable[id] = true
			}
		}
	}
	known := map[string]bool{}
	for ref, other := range m.wikis {
		if ref == w.ref {
			continue
		}
		for id := range other.store.Wiki() {
			c.Taken[strings.ToLower(id)] = true
			known[id] = true
		}
	}
	c.Linkable = func(id string) bool { return linkable[id] }
	c.Known = func(id string) bool { return known[id] }
	if w.scope == "system" {
		o := m.owners[w.name]
		mp := o.store.Map()
		c.Interfaces = map[string]bool{}
		for _, group := range [][]sysmap.Interface{mp.System.Interfaces.Provides, mp.System.Interfaces.Consumes} {
			for _, in := range group {
				c.Interfaces[in.Name] = true
			}
		}
		c.Exists = o.store.Exists
	}
	return c
}

// locate finds the document named id, looking first in the wiki from, then
// in the hubs, then in the systems (rule N1b).
func (m *Manager) locate(id, from string) (*wikiOf, *wiki.Document) {
	refs := m.wikiRefs()
	if from != "" {
		refs = append([]string{from}, refs...)
	}
	for _, ref := range refs {
		if d := m.wikis[ref].store.Wiki()[id]; d != nil {
			return m.wikis[ref], d
		}
	}
	return nil, nil
}

// ChangeWiki applies one change to agent's own wiki, or with hub set, to
// the wiki of a hub that agent orchestrates (rules T1 and T2).
func (m *Manager) ChangeWiki(agent, hub string, writes []wiki.Write) (api.ChangeResult, error) {
	var w *wikiOf
	if hub == "" {
		if _, err := m.owner(agent); err != nil {
			return api.ChangeResult{}, fmt.Errorf("%w; an orchestrator changes a hub's wiki with change_hub_wiki", err)
		}
		w = m.wikis[api.SystemWikiRef(agent)]
	} else {
		h, ok := m.topo.Hubs[hub]
		switch {
		case !ok:
			return api.ChangeResult{}, fmt.Errorf("%w: no hub %q", ErrNotFound, hub)
		case h.Orchestrator != agent:
			return api.ChangeResult{}, fmt.Errorf("%w: only %q, the orchestrator of #%s, changes its wiki", ErrForbidden, h.Orchestrator, hub)
		}
		w = m.wikis[api.HubWikiRef(hub)]
	}
	m.wikiMu.Lock()
	defer m.wikiMu.Unlock()
	m.commitUserEdits(fmt.Sprintf("user: wiki edits before %s's change", agent), w)
	res, err := w.store.Change(writes, m.check(w), m.now())
	if err != nil {
		return api.ChangeResult{}, err
	}
	out := api.ChangeResult{Result: res}
	removed := map[string]string{}
	for _, id := range res.Removed {
		removed[id] = res.Redirects[id]
	}
	files := m.docFiles(w, res.Written)
	for old := range res.Renamed {
		files = append(files, path.Join(w.dir, old+".md"))
	}
	for id := range removed {
		files = append(files, path.Join(w.dir, id+".md"))
	}
	changed := []*wikiOf{w}
	if len(res.Renamed) > 0 || len(removed) > 0 {
		for _, ref := range m.wikiRefs() {
			other := m.wikis[ref]
			if other == w {
				continue
			}
			written, err := other.store.Relink(res.Renamed, removed)
			if err != nil {
				out.Notes = append(out.Notes, fmt.Sprintf("the links in %s could not be rewritten: %v", other.label(), err))
			}
			for _, id := range written {
				out.Rewritten = append(out.Rewritten, other.label()+"/"+id)
			}
			if len(written) > 0 {
				files = append(files, m.docFiles(other, written)...)
				changed = append(changed, other)
			}
		}
	}
	if tester := m.topo.Tester(w.name); w.scope == "hub" && tester != "" && (len(res.Renamed) > 0 || len(removed) > 0) {
		// TT2: the tester's test map follows the hub's stories.
		out.Notes = append(out.Notes, m.followFeatureChange(m.owners[tester], res, removed)...)
		changed = append(changed, m.wikis[api.SystemWikiRef(tester)])
	}
	verb := "change the wiki"
	if w.scope == "system" {
		o := m.owners[agent]
		out.Notes = append(out.Notes, m.followFeatureChange(o, res, removed)...)
		if err := m.project(o); err != nil {
			out.Notes = append(out.Notes, "the map could not be written from the wiki: "+err.Error())
		}
		files = append(files, m.mapPath(agent))
	} else {
		verb = fmt.Sprintf("change the #%s wiki", hub)
	}
	if note := m.commitStore(agent, fmt.Sprintf("%s: %s: %s", agent, verb, describeWrites(writes)), files...); note != "" {
		out.Notes = append(out.Notes, note)
	}
	m.markSeen(agent, changed)
	for _, c := range changed {
		m.changed(m.wikiChange(c))
	}
	return out, nil
}

// followFeatureChange follows a change of o's wiki in its private
// documents and test results, and notes what the owner must fix by hand
// (rules E7 to E10 of the feature wiki and tests specs).
func (m *Manager) followFeatureChange(o *owner, res wiki.Result, removed map[string]string) []string {
	notes := sysmap.RewriteIDs(o.store.Root(), res.Renamed, removed)
	o.runs.Lock()
	resultsPath := o.store.PrivatePath(testmap.ResultsFile)
	if results, err := testmap.LoadResults(resultsPath); err == nil && results.FollowChange(res.Renamed, res.Removed) {
		if err := testmap.SaveResults(resultsPath, results); err != nil {
			notes = append(notes, "the test results could not follow the change: "+err.Error())
		}
	}
	o.runs.Unlock()
	priv, _ := o.store.Private()
	for _, id := range slices.Sorted(maps.Keys(removed)) {
		if _, ok := priv.Layout.Features[id]; ok {
			notes = append(notes, fmt.Sprintf("%s still names the removed feature %q; move its code to another feature or delete the entry (P2)", sysmap.LayoutFile, id))
		}
	}
	return notes
}

// docFiles returns the store paths of the documents ids of w.
func (m *Manager) docFiles(w *wikiOf, ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = path.Join(w.dir, id+".md")
	}
	return out
}

// commitUserEdits commits what the wikis hold that no tool wrote: the
// user's edits, with the user's git identity (rules C1 and C3).
func (m *Manager) commitUserEdits(message string, ws ...*wikiOf) {
	dirs := make([]string, len(ws))
	for i, w := range ws {
		dirs[i] = w.dir
	}
	m.commitStore("", message, dirs...)
}

// wikiChange is the change of a wiki, with the system it belongs to.
func (m *Manager) wikiChange(w *wikiOf) Change {
	c := Change{Wiki: w.ref}
	if w.scope == "system" {
		c.System = w.name
	}
	return c
}

// featureHealth returns the health of every feature of every system, by id.
func (m *Manager) featureHealth() map[string]string {
	out := map[string]string{}
	for _, name := range m.topo.Owners() {
		for _, f := range m.view(m.owners[name]).Features {
			out[f.ID] = f.Health
		}
	}
	return out
}

// support sums the health of the features that a hub's story or rule links
// (rule H2). It is "" for a document that links no feature.
func support(d *wiki.Document, health map[string]string) string {
	var seen []string
	for _, id := range d.Neighbors() {
		if h, ok := health[id]; ok {
			seen = append(seen, h)
		}
	}
	if len(seen) == 0 {
		return ""
	}
	for _, h := range []string{"failing", "pending", "passing"} {
		if slices.Contains(seen, h) {
			return h
		}
	}
	return "untested"
}

func hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// wikiView returns one wiki for the UI.
func (m *Manager) wikiView(w *wikiOf, health map[string]string) api.WikiView {
	v := api.WikiView{Ref: w.ref, Scope: w.scope, Name: w.name, Documents: []api.WikiDoc{}, Problems: w.store.Problems()}
	e2eHealth := map[string]string{}
	if w.scope == "hub" {
		v.Tests = m.testPlan(w.name, health)
		if v.Tests != nil {
			for _, sh := range v.Tests.Stories {
				e2eHealth[sh.ID] = sh.Health
			}
		}
	}
	docs := w.store.Wiki()
	c := m.check(w)
	v.Warnings = docs.Warnings(c)
	for _, id := range docs.IDs() {
		d := docs[id]
		doc := api.WikiDoc{
			ID: id, Kind: d.KindOf(), Name: d.Name, Description: d.Description, Links: d.Neighbors(),
			Key: d.Key, Status: d.Status, Created: d.Created,
			File: filepath.ToSlash(path.Join(w.dir, id+".md")), Hash: hash(w.store.Raw(id)),
		}
		if doc.Links == nil {
			doc.Links = []string{}
		}
		switch {
		case doc.Kind == wiki.Feature:
			doc.Health = health[id]
		case w.scope == "hub" && (doc.Kind == wiki.Story || doc.Kind == wiki.Rule):
			doc.Support = support(d, health)
			doc.Health = e2eHealth[id]
		case w.scope == "hub" && doc.Kind == wiki.Principle:
			doc.Health = e2eHealth[id]
		}
		v.Documents = append(v.Documents, doc)
	}
	return v
}

// Wikis returns every wiki, for the UI.
func (m *Manager) Wikis() []api.WikiView {
	health := m.featureHealth()
	out := []api.WikiView{}
	for _, ref := range m.wikiRefs() {
		out = append(out, m.wikiView(m.wikis[ref], health))
	}
	return out
}

// WikiView returns one wiki, for the UI.
func (m *Manager) WikiView(ref string) (api.WikiView, bool) {
	w, ok := m.wikis[ref]
	if !ok {
		return api.WikiView{}, false
	}
	return m.wikiView(w, m.featureHealth()), true
}

// HubRefsOf returns the hub wikis whose support depends on a system's
// health: the hubs the system's owner is a member of.
func (m *Manager) HubRefsOf(system string) []string {
	var out []string
	for _, hub := range m.topo.MemberHubs(system) {
		out = append(out, api.HubWikiRef(hub))
	}
	return out
}

// GetDocument returns a document of agent's readable wikis, with its
// backlinks in them (rule T4).
func (m *Manager) GetDocument(agent, id string) (api.DocumentHit, error) {
	readable := m.readableRefs(agent)
	var w *wikiOf
	var d *wiki.Document
	for _, ref := range readable {
		if doc := m.wikis[ref].store.Wiki()[id]; doc != nil {
			w, d = m.wikis[ref], doc
			break
		}
	}
	if w == nil {
		if other, _ := m.locate(id, ""); other != nil {
			return api.DocumentHit{}, fmt.Errorf("%w: %q is in the wiki of %s, which you cannot read; ask your orchestrator", ErrNotFound, id, other.label())
		}
		return api.DocumentHit{}, fmt.Errorf("%w: no document %q", ErrNotFound, id)
	}
	hit := m.hit(w, id, d, m.featureHealth())
	hit.Document = w.store.Raw(id)
	for _, ref := range readable {
		other := m.wikis[ref]
		docs := other.store.Wiki()
		for _, oid := range docs.IDs() {
			if oid != id && slices.Contains(docs[oid].Neighbors(), id) {
				hit.Backlinks = append(hit.Backlinks, api.DocRef{Wiki: other.label(), ID: oid, Kind: docs[oid].KindOf(), Name: docs[oid].Name})
			}
		}
	}
	return hit, nil
}

func (m *Manager) hit(w *wikiOf, id string, d *wiki.Document, health map[string]string) api.DocumentHit {
	h := api.DocumentHit{Wiki: w.label(), ID: id, Kind: d.KindOf(), Name: d.Name, Description: d.Description}
	switch {
	case h.Kind == wiki.Feature:
		h.Health = health[id]
		for _, f := range m.view(m.owners[w.name]).Features {
			if f.ID == id {
				h.State = f.State
			}
		}
	case w.scope == "hub" && (h.Kind == wiki.Story || h.Kind == wiki.Rule):
		h.Support = support(d, health)
		if p := m.testPlan(w.name, health); p != nil {
			for _, sh := range p.Stories {
				if sh.ID == id {
					h.Health = sh.Health
				}
			}
		}
	}
	return h
}

// SearchWiki returns the documents of agent's readable wikis that match
// query (rule T5).
func (m *Manager) SearchWiki(agent, query string) ([]api.DocumentHit, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	health := m.featureHealth()
	out := []api.DocumentHit{}
	for _, ref := range m.readableRefs(agent) {
		w := m.wikis[ref]
		docs := w.store.Wiki()
		for _, id := range docs.IDs() {
			text := strings.ToLower(id + "\n" + w.store.Raw(id))
			if strings.Contains(text, q) {
				out = append(out, m.hit(w, id, docs[id], health))
			}
		}
	}
	return out, nil
}

// markSeen records the documents of ws as agent last saw them, so that its
// own change is not a notice (rule E1).
func (m *Manager) markSeen(agent string, ws []*wikiOf) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range ws {
		if m.seenDocs[agent] == nil {
			continue
		}
		if _, ok := m.seenDocs[agent][w.ref]; ok {
			m.seenDocs[agent][w.ref] = m.snapshot(w)
		}
	}
}

// snapshot returns the hash of each document of w.
func (m *Manager) snapshot(w *wikiOf) map[string]string {
	out := map[string]string{}
	for id := range w.store.Wiki() {
		out[id] = hash(w.store.Raw(id))
	}
	return out
}

// noticeRefs are the wikis whose changes agent hears about: its readable
// wikis without its visible systems' (rule E1).
func (m *Manager) noticeRefs(agent string) []string {
	var out []string
	visible := m.topo.VisibleSystems(agent)
	for _, ref := range m.readableRefs(agent) {
		if w := m.wikis[ref]; w.scope == "system" && slices.Contains(visible, w.name) {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// wikiChanges returns what changed in agent's notice wikis since its last
// turn, and records what it saw. m.mu must be held.
func (m *Manager) wikiChanges(agent string) []api.WikiChange {
	var out []api.WikiChange
	for _, ref := range m.noticeRefs(agent) {
		w := m.wikis[ref]
		now := m.snapshot(w)
		before := m.seenDocs[agent][ref]
		docs := w.store.Wiki()
		var changes []api.DocChange
		for _, id := range docs.IDs() {
			if before[id] != now[id] {
				changes = append(changes, api.DocChange{ID: id, Kind: docs[id].KindOf()})
			}
		}
		for _, id := range slices.Sorted(maps.Keys(before)) {
			if _, ok := now[id]; !ok {
				changes = append(changes, api.DocChange{ID: id, Removed: true})
			}
		}
		if len(changes) > 0 {
			out = append(out, api.WikiChange{Wiki: w.label(), Docs: changes})
		}
		m.seenDocs[agent][ref] = now
	}
	return out
}

// isProjectWiki reports whether a path of the project repository is in a
// wiki folder of the store (rule C5).
func IsWikiPath(storeRel, p string) bool {
	rel, ok := strings.CutPrefix(p, storeRel+"/")
	if !ok {
		return false
	}
	parts := strings.Split(rel, "/")
	return len(parts) >= 4 && (parts[0] == "systems" || parts[0] == "hubs") && parts[2] == store.Wiki
}

// WriteDocument writes a document that the user gives in full, in the UI
// (rules U6, U8, and C4).
func (m *Manager) WriteDocument(ref, name, text string) error {
	return m.userChange(ref, "write "+name, name, func(w *wikiOf) error { return w.store.WriteDoc(name, text, m.check(w)) })
}

// RemoveDocument removes a plan or a note, in the UI.
func (m *Manager) RemoveDocument(ref, name string) error {
	return m.userChange(ref, "remove "+name, name, func(w *wikiOf) error { return w.store.RemoveUserDoc(name) })
}

// SetKey marks or unmarks a story of a hub as key, in the UI (rule K3).
func (m *Manager) SetKey(ref, id string, key bool) error {
	verb := "mark key"
	if !key {
		verb = "unmark key"
	}
	return m.userChange(ref, verb+" "+id, id, func(w *wikiOf) error { return w.store.SetKey(id, key) })
}

// userChange applies one change that the user makes in the UI and commits
// it with the user's git identity.
func (m *Manager) userChange(ref, what, id string, fn func(*wikiOf) error) error {
	w, ok := m.wikis[ref]
	if !ok {
		return fmt.Errorf("%w: no wiki %q", ErrNotFound, ref)
	}
	m.wikiMu.Lock()
	defer m.wikiMu.Unlock()
	if err := fn(w); err != nil {
		return err
	}
	if w.scope == "system" {
		m.project(m.owners[w.name])
	}
	m.commitStore("", fmt.Sprintf("user: %s in %s", what, w.label()), path.Join(w.dir, id+".md"))
	m.changed(m.wikiChange(w))
	return nil
}
