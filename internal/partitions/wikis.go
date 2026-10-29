package partitions

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
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/wiki"
)

// The agent kinds of each scope of wiki (section 2 of the wikis spec, and
// rules K4 and K10 of the skill packs spec).
var (
	partitionKinds     = []string{wiki.Feature, wiki.Principle, wiki.Rule}
	orchestrationKinds = []string{wiki.Feature, wiki.Principle, wiki.Rule}
	publicKinds        = []string{wiki.Rule, wiki.Principle}
	// releaseKinds are the documents a release copies (rule L2).
	releaseKinds = []string{wiki.Feature, wiki.Principle, wiki.Rule}
)

// The scopes of a wiki.
const (
	scopePartition     = "partition"
	scopeOrchestration = "orchestration"
	scopePublic        = "public"
)

// wikiOf is one wiki of the vault: a partition's feature map, the
// orchestration feature map, or the public wiki.
type wikiOf struct {
	ref   string
	scope string
	name  string
	// author writes its agent documents: the partition agent, or the orchestration agent.
	// The public wiki of a project without an orchestration agent has none.
	author string
	// dir is its directory, relative to the vault, with slashes.
	dir   string
	files *wiki.Files
}

// label names the wiki for a person or an agent: "public", "orchestration",
// or the partition.
func (w *wikiOf) label() string { return w.name }

// openWikis opens the wiki of every partition, the public wiki, and the
// orchestration feature map when the topology names an orchestration agent
// (rule L1, and K4 of the skill packs spec).
func (m *Manager) openWikis() error {
	for _, name := range m.topo.PartitionAgents() {
		s, err := wiki.Open(m.vault.FeatureMap(name), partitionKinds)
		if err != nil {
			return fmt.Errorf("agent %q: %w", name, err)
		}
		m.wikis[api.FeatureMapRef(name)] = &wikiOf{
			ref: api.FeatureMapRef(name), scope: scopePartition, name: name, author: name,
			dir: m.vault.Rel(m.vault.FeatureMap(name)), files: s,
		}
	}
	s, err := wiki.Open(m.vault.PublicWiki(), publicKinds)
	if err != nil {
		return fmt.Errorf("the public wiki: %w", err)
	}
	m.wikis[api.PublicWikiRef] = &wikiOf{
		ref: api.PublicWikiRef, scope: scopePublic, name: "public", author: m.topo.Orchestration,
		dir: m.vault.Rel(m.vault.PublicWiki()), files: s,
	}
	if m.topo.Orchestration == "" {
		return nil
	}
	dir := m.vault.OrchestrationFeatureMap()
	if s, err = wiki.Open(dir, orchestrationKinds); err != nil {
		return fmt.Errorf("the orchestration feature map: %w", err)
	}
	m.wikis[api.OrchestrationWikiRef] = &wikiOf{
		ref: api.OrchestrationWikiRef, scope: scopeOrchestration, name: api.OrchestrationWikiRef, author: m.topo.Orchestration,
		dir: m.vault.Rel(dir), files: s,
	}
	return nil
}

// wikiRefs returns the refs of every wiki: the public wiki first, then the
// orchestration feature map, then the partitions in name order (rule N1b).
func (m *Manager) wikiRefs() []string {
	out := []string{api.PublicWikiRef}
	if _, ok := m.wikis[api.OrchestrationWikiRef]; ok {
		out = append(out, api.OrchestrationWikiRef)
	}
	for _, name := range m.topo.PartitionAgents() {
		out = append(out, api.FeatureMapRef(name))
	}
	return out
}

// linkableRefs returns the other wikis whose agent documents w can link
// (rule N2, and K5 of the skill packs spec): no wiki links the
// orchestration feature map, which links all the others.
func (m *Manager) linkableRefs(w *wikiOf) []string {
	var out []string
	switch w.scope {
	case scopePartition:
		return []string{api.PublicWikiRef}
	case scopeOrchestration:
		out = append(out, api.PublicWikiRef)
	}
	for _, name := range m.topo.PartitionAgents() {
		out = append(out, api.FeatureMapRef(name))
	}
	return out
}

// readableRefs returns the wikis that agent reads, its own first (rules A1
// to A3, and K8 of the skill packs spec).
func (m *Manager) readableRefs(agent string) []string {
	var out []string
	if _, ok := m.partitionAgents[agent]; ok {
		out = append(out, api.FeatureMapRef(agent))
	}
	if _, ok := m.wikis[api.OrchestrationWikiRef]; ok && agent == m.topo.Orchestration {
		out = append(out, api.OrchestrationWikiRef)
	}
	out = append(out, api.PublicWikiRef)
	for _, sys := range m.topo.VisiblePartitions(agent) {
		out = append(out, api.FeatureMapRef(sys))
	}
	return out
}

// check returns what the rules of w need from the other wikis.
func (m *Manager) check(w *wikiOf) wiki.Check {
	c := wiki.Check{Kinds: w.files.Kinds(), Taken: map[string]bool{}}
	linkable := map[string]bool{}
	for _, ref := range m.linkableRefs(w) {
		for id, d := range m.wikis[ref].files.Wiki() {
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
		for id := range other.files.Wiki() {
			c.Taken[strings.ToLower(id)] = true
			known[id] = true
		}
	}
	c.Linkable = func(id string) bool { return linkable[id] }
	c.Known = func(id string) bool { return known[id] }
	if w.scope == scopePartition {
		o := m.partitionAgents[w.name]
		mp := o.featureMap.Map()
		c.Interfaces = map[string]bool{}
		for _, group := range [][]featuremap.Interface{mp.Partition.Interfaces.Provides, mp.Partition.Interfaces.Consumes} {
			for _, in := range group {
				c.Interfaces[in.Name] = true
			}
		}
		c.Exists = o.featureMap.Exists
	}
	return c
}

// locate finds the document named id, looking first in the wiki from, then
// in the public wiki, then in the partitions (rule N1b).
func (m *Manager) locate(id, from string) (*wikiOf, *wiki.Document) {
	refs := m.wikiRefs()
	if from != "" {
		refs = append([]string{from}, refs...)
	}
	for _, ref := range refs {
		if d := m.wikis[ref].files.Wiki()[id]; d != nil {
			return m.wikis[ref], d
		}
	}
	return nil, nil
}

// ChangeWiki applies one change to agent's own feature map, or with public
// set, to the public wiki, which only the orchestration agent changes (rules
// T1 and T2, and decision 2 of the skill packs spec).
func (m *Manager) ChangeWiki(agent string, public bool, writes []wiki.Write) (api.ChangeResult, error) {
	var w *wikiOf
	if !public {
		if own, ok := m.wikis[api.OrchestrationWikiRef]; ok && agent == m.topo.Orchestration {
			w = own
		} else if _, err := m.partitionAgent(agent); err != nil {
			return api.ChangeResult{}, err
		} else {
			w = m.wikis[api.FeatureMapRef(agent)]
		}
	} else {
		if err := m.orchestration(agent, "changes the public wiki"); err != nil {
			return api.ChangeResult{}, err
		}
		w = m.wikis[api.PublicWikiRef]
	}
	m.wikiMu.Lock()
	defer m.wikiMu.Unlock()
	m.commitUserEdits(fmt.Sprintf("user: wiki edits before %s's change", agent), w)
	res, err := w.files.Change(writes, m.check(w), m.now())
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
			written, err := other.files.Relink(res.Renamed, removed)
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
	verb := "change the wiki"
	switch w.scope {
	case scopePartition:
		o := m.partitionAgents[agent]
		out.Notes = append(out.Notes, m.followFeatureChange(o, res, removed)...)
		if err := m.project(o); err != nil {
			out.Notes = append(out.Notes, "the map could not be written from the wiki: "+err.Error())
		}
		files = append(files, m.mapPath(agent))
	case scopeOrchestration:
		verb = "change the orchestration feature map"
	default:
		verb = "change the public wiki"
	}
	if note := m.commitVault(agent, fmt.Sprintf("%s: %s: %s", agent, verb, describeWrites(writes)), files...); note != "" {
		out.Notes = append(out.Notes, note)
	}
	m.markSeen(agent, changed)
	for _, c := range changed {
		m.changed(m.wikiChange(c))
	}
	return out, nil
}

// followFeatureChange follows a change of o's wiki in its private
// documents and test results, and notes what the partition agent must fix by hand
// (rules E7 to E10 of the feature wiki and tests specs).
func (m *Manager) followFeatureChange(o *partitionAgent, res wiki.Result, removed map[string]string) []string {
	notes := featuremap.RewriteIDs(o.featureMap.Root(), res.Renamed, removed)
	o.runs.Lock()
	resultsPath := o.featureMap.PrivatePath(testmap.ResultsFile)
	if results, err := testmap.LoadResults(resultsPath); err == nil && results.FollowChange(res.Renamed, res.Removed) {
		if err := testmap.SaveResults(resultsPath, results); err != nil {
			notes = append(notes, "the test results could not follow the change: "+err.Error())
		}
	}
	o.runs.Unlock()
	priv, _ := o.featureMap.Private()
	for _, id := range slices.Sorted(maps.Keys(removed)) {
		if _, ok := priv.CodeMap.Features[id]; ok {
			notes = append(notes, fmt.Sprintf("%s still names the removed feature %q; move its code to another feature or delete the entry (P2)", featuremap.CodeMapFile, id))
		}
	}
	return notes
}

// docFiles returns the vault paths of the documents ids of w.
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
	var dirs []string
	for _, w := range ws {
		dirs = append(dirs, w.dir)
		if w.scope == scopePartition {
			// hikari writes the map beside the documents.
			dirs = append(dirs, excludeSpec+path.Join(w.dir, vault.MapName))
		}
	}
	m.commitVault("", message, dirs...)
}

// wikiChange is the change of a wiki, with the partition it belongs to.
func (m *Manager) wikiChange(w *wikiOf) Change {
	c := Change{Wiki: w.ref}
	if w.scope == scopePartition {
		c.Partition = w.name
	}
	return c
}

// featureHealth returns the health of every feature of every partition, by
// id, from each partition's test report.
func (m *Manager) featureHealth() map[string]string {
	out := map[string]string{}
	for _, name := range m.topo.PartitionAgents() {
		o := m.partitionAgents[name]
		priv, _ := o.featureMap.Private()
		ids := featureIDs(o.featureMap.Map())
		rep, _ := m.testReport(o, priv, ids)
		for _, id := range ids {
			_, out[id] = featuremap.TestsOf(rep.Features[id])
		}
	}
	return out
}

// support sums the health of the partition features that a document of the
// public wiki or of the orchestration feature map links (rule H2, and K6 of
// the skill packs spec). It is "" for a document that links no feature.
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

// orchestrationHealth is the health of a feature of the orchestration
// feature map: its support, and untested when it links no partition
// feature (rule K6).
func orchestrationHealth(d *wiki.Document, health map[string]string) string {
	if s := support(d, health); s != "" {
		return s
	}
	return "untested"
}

// health sets the health or the support of a document of w, by its kind.
func (w *wikiOf) health(kind string, d *wiki.Document, health map[string]string) (feature, sup string) {
	switch {
	case w.scope == scopePartition && kind == wiki.Feature:
		return health[d.ID], ""
	case w.scope == scopeOrchestration && kind == wiki.Feature:
		return orchestrationHealth(d, health), ""
	case w.scope != scopePartition && (kind == wiki.Rule || kind == wiki.Principle):
		return "", support(d, health)
	}
	return "", ""
}

func hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// wikiView returns one wiki for the UI.
func (m *Manager) wikiView(w *wikiOf, health map[string]string) api.WikiView {
	v := api.WikiView{Ref: w.ref, Scope: w.scope, Name: w.name, Documents: []api.WikiDoc{}, Problems: w.files.Problems()}
	docs := w.files.Wiki()
	c := m.check(w)
	v.Warnings = docs.Warnings(c)
	for _, id := range docs.IDs() {
		d := docs[id]
		doc := api.WikiDoc{
			ID: id, Kind: d.KindOf(), Name: d.Name, Description: d.Description, Links: d.Neighbors(),
			Status: d.Status, Created: d.Created,
			File: filepath.ToSlash(path.Join(w.dir, id+".md")), Hash: hash(w.files.Raw(id)),
		}
		if doc.Links == nil {
			doc.Links = []string{}
		}
		doc.Health, doc.Support = w.health(doc.Kind, d, health)
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

// WikiViews returns the wikis refs, for the UI, from one computation of
// the features' health. A ref that names no wiki is skipped.
func (m *Manager) WikiViews(refs []string) []api.WikiView {
	if len(refs) == 0 {
		return nil
	}
	health := m.featureHealth()
	var out []api.WikiView
	for _, ref := range refs {
		if w, ok := m.wikis[ref]; ok {
			out = append(out, m.wikiView(w, health))
		}
	}
	return out
}

// GetDocument returns a document of agent's readable wikis, with its
// backlinks in them: a reader sees only the backlinks from documents it can
// read (rule T4, and K11 of the skill packs spec).
func (m *Manager) GetDocument(agent, id string) (api.DocumentHit, error) {
	readable := m.readableRefs(agent)
	var w *wikiOf
	var d *wiki.Document
	for _, ref := range readable {
		if doc := m.wikis[ref].files.Wiki()[id]; doc != nil {
			w, d = m.wikis[ref], doc
			break
		}
	}
	if w == nil {
		if other, _ := m.locate(id, ""); other != nil {
			return api.DocumentHit{}, fmt.Errorf("%w: %q is in the wiki of %s, which you cannot read; ask the orchestration agent", ErrNotFound, id, other.label())
		}
		return api.DocumentHit{}, fmt.Errorf("%w: no document %q", ErrNotFound, id)
	}
	hit := m.hit(w, id, d, m.featureHealth())
	hit.Document = w.files.Raw(id)
	for _, ref := range readable {
		other := m.wikis[ref]
		docs := other.files.Wiki()
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
	h.Health, h.Support = w.health(h.Kind, d, health)
	if w.scope == scopePartition && h.Kind == wiki.Feature {
		for _, f := range m.view(m.partitionAgents[w.name]).Features {
			if f.ID == id {
				h.State = f.State
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
		docs := w.files.Wiki()
		for _, id := range docs.IDs() {
			text := strings.ToLower(id + "\n" + w.files.Raw(id))
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
	for id := range w.files.Wiki() {
		out[id] = hash(w.files.Raw(id))
	}
	return out
}

// noticeRefs are the wikis whose changes agent hears about: its readable
// wikis without its visible partitions' (rule E1).
func (m *Manager) noticeRefs(agent string) []string {
	var out []string
	visible := m.topo.VisiblePartitions(agent)
	for _, ref := range m.readableRefs(agent) {
		if w := m.wikis[ref]; w.scope == scopePartition && slices.Contains(visible, w.name) {
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
		docs := w.files.Wiki()
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

// IsUserPath reports whether a path of the project repository holds what
// you change in the vault: a document of a wiki, or a file of a can-edit/
// or a granted/ (rule C5 of the wikis spec, and V10 of the vault layout
// spec). hikari up commits these as yours.
func IsUserPath(vaultRel, p string) bool {
	rel, ok := strings.CutPrefix(p, vaultRel+"/")
	if !ok {
		return false
	}
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) >= 3 && parts[0] == vault.PublicDir:
		return parts[1] == vault.Wiki
	case len(parts) >= 4 && parts[0] == vault.PartitionsDir:
		if parts[2] == vault.CanEditDir {
			return true
		}
		// Tags and pins written by hikari release while hikari was stopped
		// are yours too (rule TG6 and PN8 of the releases spec).
		return len(parts) >= 5 && parts[2] == vault.ReadOnlyDir &&
			(parts[3] == vault.GrantedDir || parts[3] == vault.FeaturesDir && len(parts) == 5 && strings.HasSuffix(parts[4], ".md") ||
				parts[3] == vault.ReleasesDir && len(parts) == 6 && parts[5] == vault.TagsFile) ||
			len(parts) == 4 && parts[2] == vault.ReadOnlyDir && parts[3] == vault.PinsFile
	case len(parts) >= 3 && parts[0] == vault.OrchestrationDir:
		return parts[1] == vault.CanEditDir || len(parts) >= 4 && parts[1] == vault.ReadOnlyDir &&
			(parts[2] == vault.GrantedDir || parts[2] == vault.FeaturesDir && len(parts) == 4 && strings.HasSuffix(parts[3], ".md"))
	}
	return false
}

// WriteDocument writes a document that the user gives in full, in the UI
// (rules U6, U8, and C4).
func (m *Manager) WriteDocument(ref, name, text string) error {
	return m.userChange(ref, "write "+name, name, func(w *wikiOf) error { return w.files.WriteDoc(name, text, m.check(w)) })
}

// RemoveDocument removes a plan or a note, in the UI.
func (m *Manager) RemoveDocument(ref, name string) error {
	return m.userChange(ref, "remove "+name, name, func(w *wikiOf) error { return w.files.RemoveUserDoc(name) })
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
	if w.scope == scopePartition {
		m.project(m.partitionAgents[w.name])
	}
	m.commitVault("", fmt.Sprintf("user: %s in %s", what, w.label()), path.Join(w.dir, id+".md"))
	m.changed(m.wikiChange(w))
	return nil
}
