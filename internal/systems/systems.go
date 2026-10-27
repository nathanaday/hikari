// Package systems connects the topology to the repos, the system maps, the
// wikis, and the vocabulary. It enforces who may read and change each of
// them: an owner reads and changes only its own map, the orchestrator reads
// the orchestrator view of every system, and only the orchestrator changes
// the vocabulary and the public wiki.
package systems

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
	"gopkg.in/yaml.v3"
)

var (
	ErrForbidden = errors.New("not allowed")
	ErrNotFound  = errors.New("not found")
)

// Change names what changed: a system map, the vocabulary, or a wiki.
type Change struct {
	System     string
	Vocabulary bool
	// Wiki is the ref of a wiki that changed.
	Wiki string
}

type owner struct {
	name     string
	repoName string
	scope    string
	branch   string
	repo     *gitrepo.Repo
	store    *sysmap.Store
	wiki     *wiki.Store

	mu sync.Mutex
	// projectErr is the problem of the last projection of the wiki into
	// the map, when a hand edit of the wiki broke a rule of the map.
	projectErr string
	// runs serializes the recording of test runs.
	runs sync.Mutex
	// dirty says whether the scope had uncommitted changes at the last
	// check (rule N4).
	dirty bool
}

type Manager struct {
	topo     *topology.Topology
	store    store.Store
	repos    map[string]*gitrepo.Repo
	start    map[string]string
	owners   map[string]*owner
	vocab    *vocab.Store
	warnings []string
	now      func() time.Time
	// projectRepo is the project repository, or nil. storeRel is the store's
	// path in it.
	projectRepo *gitrepo.Repo
	storeRel    string

	// wikis holds every wiki by ref. wikiMu lets one wiki change run at a
	// time, because a change reads and rewrites the other wikis (decision
	// 9 of the wikis spec).
	wikis  map[string]*wikiOf
	wikiMu sync.Mutex

	mu sync.Mutex
	// seen holds the version of the vocabulary that each agent last saw.
	seen map[string]int
	// seenDocs holds, for each agent and each wiki it hears about, the
	// hash of each document as of its last turn (rule E1).
	seenDocs map[string]map[string]map[string]string
	onChange func(Change)
}

// New prepares every repo of topo (rules G1 to G5) and loads the system maps,
// the wikis, and the vocabulary.
func New(topo *topology.Topology) (*Manager, error) {
	m := &Manager{
		topo:     topo,
		store:    topo.StoreOf(),
		repos:    map[string]*gitrepo.Repo{},
		start:    map[string]string{},
		owners:   map[string]*owner{},
		now:      time.Now,
		seen:     map[string]int{},
		seenDocs: map[string]map[string]map[string]string{},
		wikis:    map[string]*wikiOf{},
		onChange: func(Change) {},
	}
	for name, r := range topo.Repos {
		repo := gitrepo.Open(r.Path)
		start, err := repo.Prepare(r.Branch)
		if err != nil {
			return nil, fmt.Errorf("repo %q: %w", name, err)
		}
		m.repos[name], m.start[name] = repo, start
	}
	if err := m.store.Prepare(topo.Owners()); err != nil {
		return nil, fmt.Errorf("store %s: %w", m.store.Root, err)
	}
	if topo.Project != nil {
		m.projectRepo, m.storeRel = m.repos[topology.ProjectRepo], topo.StoreInProject()
		if _, err := m.store.WriteIgnore(); err != nil {
			return nil, fmt.Errorf("store %s: %w", m.store.Root, err)
		}
		m.commitOwnFiles()
	}
	for _, name := range topo.Owners() {
		s := topo.Agents[name].System
		r := topo.Repos[s.Repo]
		scopeRoot := filepath.Join(r.Path, s.Scope)
		maps, _, err := sysmap.Open(scopeRoot, m.store.MapFile(name), name, s.Test)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		repo := m.repos[s.Repo]
		privatePath := path.Join(filepath.ToSlash(s.Scope), sysmap.Dir, sysmap.PrivateDir, sysmap.LayoutFile)
		if repo.IsIgnored(privatePath) {
			m.warnings = append(m.warnings, fmt.Sprintf("repo %q ignores %s, so the private documents of %q are not versioned with its code", s.Repo, privatePath, name))
		}
		m.warnings = append(m.warnings, maps.Notes()...)
		m.owners[name] = &owner{name: name, repoName: s.Repo, scope: s.Scope, branch: r.Branch, repo: repo, store: maps}
	}
	if err := m.openWikis(); err != nil {
		return nil, err
	}
	for _, name := range topo.Owners() {
		m.owners[name].wiki = m.wikis[api.SystemWikiRef(name)].store
	}
	for _, name := range topo.Owners() {
		o := m.owners[name]
		m.checkDirty(o)
		if err := m.project(o); err != nil {
			m.warnings = append(m.warnings, fmt.Sprintf("agent %q: the map does not match the feature wiki: %v", name, err))
		}
	}
	if err := m.checkDeliverables(); err != nil {
		return nil, err
	}
	if err := m.checkAgents(); err != nil {
		return nil, err
	}
	vs, err := vocab.Open(m.store.VocabularyFile())
	if err != nil {
		return nil, err
	}
	m.vocab = vs
	for _, agent := range topo.AgentNames() {
		_, m.seen[agent] = vs.Get()
		m.seenDocs[agent] = map[string]map[string]string{}
		for _, ref := range m.noticeRefs(agent) {
			m.seenDocs[agent][ref] = m.snapshot(m.wikis[ref])
		}
	}
	return m, nil
}

// storePath returns a store path, relative to the store, as a path of the
// project repository.
func (m *Manager) storePath(rel string) string { return path.Join(m.storeRel, rel) }

// mapPath is the store path of an owner's map.
func (m *Manager) mapPath(owner string) string { return m.store.Rel(m.store.MapFile(owner)) }

// vocabularyPath is the store path of the vocabulary.
func (m *Manager) vocabularyPath() string { return m.store.Rel(m.store.VocabularyFile()) }

// commitStore commits the store paths of agent in the project repository
// (rules PR14 to PR17). It returns a note when the commit failed, and does
// nothing outside a project repository.
func (m *Manager) commitStore(agent, message string, paths ...string) string {
	if m.projectRepo == nil {
		return ""
	}
	specs := make([]string, len(paths))
	for i, p := range paths {
		specs[i] = m.storePath(p)
	}
	if _, err := m.projectRepo.CommitPaths(m.topo.Project.Branch, specs, agent, message); err != nil {
		log.Printf("store commit for %s: %v", agent, err)
		return "the store change could not be committed: " + err.Error()
	}
	return ""
}

// CommitAsUser commits paths of the project repository, relative to it,
// with the user's identity, on the project branch: the topology editor's
// saves during a run (rule TC4 of the topology API spec). It shares the
// repository's lock with the agents' commits.
func (m *Manager) CommitAsUser(paths []string, message string) (string, error) {
	if m.projectRepo == nil {
		return "", errors.New("the topology has no project repository")
	}
	return m.projectRepo.CommitPaths(m.topo.Project.Branch, paths, "", message)
}

// commitOwnFiles commits the store files that hikari writes itself (rule
// PR18).
func (m *Manager) commitOwnFiles() {
	for _, f := range []string{"README.md", store.IgnoreFile} {
		verb := "update"
		if !m.projectRepo.Tracked(m.storePath(f)) {
			verb = "add"
		}
		m.commitStore("hikari", fmt.Sprintf("hikari: %s %s", verb, m.storePath(f)), f)
	}
}

// describeWrites says what a change of the wiki does, for its commit
// message: at most four writes, then a count.
func describeWrites(writes []wiki.Write) string {
	var parts []string
	for _, w := range writes {
		part := w.Op + " " + w.ID
		if w.Op == "rename" {
			part += " to " + w.To
		}
		parts = append(parts, part)
	}
	if len(parts) > 4 {
		return strings.Join(parts[:4], ", ") + fmt.Sprintf(", and %d more", len(parts)-4)
	}
	return strings.Join(parts, ", ")
}

// Store returns the shared store.
func (m *Manager) Store() store.Store { return m.store }

// checkDeliverables checks every granted deliverable at startup (rules T15
// and T16): it exists and is valid, and its name is not a skill's name.
func (m *Manager) checkDeliverables() error {
	var errs []error
	checked := map[string]bool{}
	for _, name := range m.topo.Owners() {
		for _, d := range m.topo.Agents[name].Grants.Deliverables {
			if checked[d] {
				continue
			}
			checked[d] = true
			if _, err := m.store.LoadDeliverable(d); err != nil {
				errs = append(errs, err)
			}
			if slices.Contains(api.BuiltinSkills, d) {
				errs = append(errs, fmt.Errorf("deliverable %q has the name of hikari's own skill", d))
			} else if exists(filepath.Join(m.store.Shared(store.Skills), d, store.SkillFile)) {
				errs = append(errs, fmt.Errorf("deliverable %q has the name of a shared skill; rename one of them", d))
			}
		}
	}
	return errors.Join(errs...)
}

// checkAgents refuses a shared agent whose name starts with hikari-, which
// is reserved for hikari's own agents (rule T17).
func (m *Manager) checkAgents() error {
	entries, _ := os.ReadDir(m.store.Shared(store.Agents))
	var errs []error
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), api.AgentPrefix) {
			errs = append(errs, fmt.Errorf("shared agent %q: the prefix %s is reserved for hikari's own agents; rename it", e.Name(), api.AgentPrefix))
		}
	}
	return errors.Join(errs...)
}

// project writes the features of o's wiki into its map, with the health of
// each feature (rules Q1 and Q6).
func (m *Manager) project(o *owner) error {
	w := o.wiki.Wiki()
	neighbors := w.Neighbors()
	priv, _ := o.store.Private()
	rep, _ := m.testReport(o, priv, w.Features())
	// With no results file, this machine has run no tests, which is no
	// evidence: the map keeps the health that another machine recorded
	// (rule SY10 of the linked repositories spec).
	recorded := map[string]sysmap.Feature{}
	if m.noLocalResults(o) {
		for _, f := range o.store.Map().Features {
			recorded[f.ID] = f
		}
	}
	entries := make([]sysmap.FeatureEntry, 0, len(w))
	for _, id := range w.Features() {
		d := w[id]
		tests, health := sysmap.TestsOf(rep.Features[id])
		if f, ok := recorded[id]; ok && !f.Tests.Ran.IsZero() {
			tests, health = f.Tests, f.Health
		}
		entries = append(entries, sysmap.FeatureEntry{
			ID: id, Name: d.Name, Description: d.Description, Interfaces: d.Interfaces, Related: neighbors[id],
			Created: d.Created, Tests: tests, Health: health,
		})
	}
	err := o.store.Update(func(mp *sysmap.Map) error { mp.Project(entries); return nil })
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		o.projectErr = err.Error()
	} else {
		o.projectErr = ""
	}
	return err
}

// noLocalResults reports whether o's map records test runs, but this
// machine has no results file: a fresh clone of the project.
func (m *Manager) noLocalResults(o *owner) bool {
	if _, err := os.Stat(o.store.PrivatePath(testmap.ResultsFile)); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return slices.ContainsFunc(o.store.Map().Features, func(f sysmap.Feature) bool { return !f.Tests.Ran.IsZero() })
}

// testReport computes the health of the features ids of o from its test
// map and its results. The string is the problem of the results file.
func (m *Manager) testReport(o *owner, priv sysmap.Private, ids []string) (testmap.Report, string) {
	results, err := testmap.LoadResults(o.store.PrivatePath(testmap.ResultsFile))
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	root := o.store.Root()
	hashes := testmap.Hashes{
		File: func(rel string) string { h, _ := sysmap.ContentHash(root, []string{rel}); return h },
		Code: func(id string) string { h, _ := sysmap.ContentHash(root, priv.Code(id)); return h },
	}
	return testmap.Compute(priv.Tests, testmap.NewMatcher(priv.Tests, o.store), results, ids, hashes), problem
}

// Deliverables returns the deliverables that agent holds, with their
// descriptions, for its prompt.
func (m *Manager) Deliverables(agent string) []api.DeliverableInfo {
	var out []api.DeliverableInfo
	for _, name := range m.topo.Agents[agent].Grants.Deliverables {
		info := api.DeliverableInfo{Name: name}
		if d, err := m.store.LoadDeliverable(name); err != nil {
			info.Description = "(invalid: " + err.Error() + ")"
		} else {
			info.Description = d.Description
		}
		out = append(out, info)
	}
	return out
}

// holds checks that agent is an owner that holds deliverable.
func (m *Manager) holds(agent, deliverable string) (*owner, error) {
	o, err := m.owner(agent)
	if err != nil {
		return nil, err
	}
	granted := m.topo.Agents[agent].Grants.Deliverables
	switch {
	case len(granted) == 0:
		return nil, fmt.Errorf("%w: you hold no deliverable, so you cannot publish; ask the user to grant you one", ErrForbidden)
	case !slices.Contains(granted, deliverable):
		return nil, fmt.Errorf("%w: you do not hold the deliverable %q; you hold %s", ErrForbidden, deliverable, strings.Join(granted, ", "))
	}
	return o, nil
}

// releaseProblems lists what blocks a release of agent's system: a map
// that is not current, or invalid private documents.
func (m *Manager) releaseProblems(o *owner) (sysmap.OwnerView, []string) {
	v := m.ownerView(o)
	var problems []string
	if v.State != sysmap.Current {
		problems = append(problems, fmt.Sprintf("the map is %s; update it and call mark_map_current first", v.State))
	}
	if len(v.PrivateErrors) > 0 {
		problems = append(problems, "the private documents have problems: "+strings.Join(v.PrivateErrors, "; "))
	}
	return v, problems
}

// CheckRelease checks the staged product of a deliverable against the
// deliverable and the state of the system (rule R15). It changes nothing.
func (m *Manager) CheckRelease(agent, deliverable string) (store.ProductCheck, error) {
	o, err := m.holds(agent, deliverable)
	if err != nil {
		return store.ProductCheck{}, err
	}
	_, problems := m.releaseProblems(o)
	check, err := m.store.Check(agent, deliverable)
	if err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return store.ProductCheck{}, errors.New(strings.Join(problems, "\n"))
	}
	return check, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Warnings returns problems found by New that do not stop hikari.
func (m *Manager) Warnings() []string { return m.warnings }

// OnChange sets the function that New's maps and vocabularies call after
// each change.
func (m *Manager) OnChange(fn func(Change)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// changed reports c, and also every system whose view depends on it: a
// system's flags and providers come from the vocabulary and from the maps
// of every other system.
func (m *Manager) changed(c Change) {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if c.Wiki != "" {
		fn(Change{Wiki: c.Wiki})
	}
	if c.Vocabulary {
		fn(Change{Vocabulary: true})
	}
	if c.System != "" || c.Vocabulary {
		for _, name := range m.peers(c.System) {
			fn(Change{System: name})
		}
	}
}

// peers returns system first, then every other owner.
func (m *Manager) peers(system string) []string {
	var out []string
	if system != "" {
		out = append(out, system)
	}
	for _, name := range m.topo.Owners() {
		if name != system {
			out = append(out, name)
		}
	}
	return out
}

// Watch reloads hand edits of the map files and the feature wikis, and
// checks each scope for uncommitted changes, until ctx ends.
func (m *Manager) Watch(ctx context.Context) {
	go m.watchDirty(ctx, 2*time.Second)
	for _, o := range m.owners {
		go o.store.Watch(ctx, time.Second, func() {
			m.project(o)
			m.changed(Change{System: o.name})
		})
	}
	go m.watchWikis(ctx)
}

// watchWikis reloads every wiki each second, and after a change of a
// system's wiki writes the map from it (rules W8 and D4).
func (m *Manager) watchWikis(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, ref := range m.wikiRefs() {
				w := m.wikis[ref]
				m.wikiMu.Lock()
				reloaded := w.store.Reload()
				if reloaded && w.scope == "system" {
					m.project(m.owners[w.name])
				}
				m.wikiMu.Unlock()
				if reloaded {
					m.changed(m.wikiChange(w))
				}
			}
		}
	}
}

// watchDirty checks every scope for uncommitted changes every interval, and
// reports each system whose answer changed (rule N4).
func (m *Manager) watchDirty(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, name := range m.topo.Owners() {
				if o := m.owners[name]; m.checkDirty(o) {
					m.changed(Change{System: name})
				}
			}
		}
	}
}

// checkDirty asks git whether o's scope has uncommitted changes, records
// the answer, and reports whether it changed. A failed check keeps the last
// answer.
func (m *Manager) checkDirty(o *owner) bool {
	dirty, err := o.repo.Dirty(o.scope)
	if err != nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	changed := o.dirty != dirty
	o.dirty = dirty
	return changed
}

// Repos returns the repositories of the topology with their systems, for
// the UI (rule N2).
func (m *Manager) Repos() []api.RepoInfo {
	out := []api.RepoInfo{}
	for _, name := range slices.Sorted(maps.Keys(m.topo.Repos)) {
		info := api.RepoInfo{Name: name, Branch: m.topo.Repos[name].Branch, Systems: []string{}}
		for _, owner := range m.topo.Owners() {
			if m.owners[owner].repoName == name {
				info.Systems = append(info.Systems, owner)
			}
		}
		slices.Sort(info.Systems)
		if name == topology.ProjectRepo && len(info.Systems) == 0 {
			continue
		}
		out = append(out, info)
	}
	return out
}

// CommitAll commits the changes left in every owner's scope, so that the
// next run does not find uncommitted changes. hikari calls it at shutdown.
func (m *Manager) CommitAll() []error {
	var errs []error
	for _, name := range m.topo.Owners() {
		o := m.owners[name]
		if _, err := o.repo.Commit(o.branch, o.scope, name, name+": changes left when hikari stopped"); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		if note := m.commitStore(name, name+": store changes left when hikari stopped", m.mapPath(name)); note != "" {
			errs = append(errs, fmt.Errorf("%s: %s", name, note))
		}
	}
	var all []*wikiOf
	for _, ref := range m.wikiRefs() {
		all = append(all, m.wikis[ref])
	}
	m.commitUserEdits("user: wiki edits left when hikari stopped", all...)
	if orch := m.topo.Orchestrator; orch != "" {
		if note := m.commitStore(orch, orch+": store changes left when hikari stopped", m.vocabularyPath()); note != "" {
			errs = append(errs, fmt.Errorf("%s: %s", orch, note))
		}
	}
	if m.projectRepo != nil {
		m.commitOwnFiles()
	}
	return errs
}

// Summary returns one line for each owner with its commits since start
// (rule G11).
func (m *Manager) Summary() []string {
	var out []string
	for _, name := range m.topo.Owners() {
		o := m.owners[name]
		n, err := o.repo.CountAuthored(m.start[o.repoName], name)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		out = append(out, fmt.Sprintf("%s made %d commit(s) on %s in repo %q", name, n, o.branch, o.repoName))
	}
	if m.projectRepo != nil {
		n, err := m.projectRepo.CountOn(m.start[topology.ProjectRepo], m.storeRel)
		if err != nil {
			out = append(out, fmt.Sprintf("the store: %v", err))
		} else {
			out = append(out, fmt.Sprintf("the store has %d commit(s) on %s in the project repository", n, m.topo.Project.Branch))
		}
	}
	return out
}

// Self returns the system details of agent, for its container.
func (m *Manager) Self(agent string) *api.SelfSystem {
	o, ok := m.owners[agent]
	if !ok {
		return nil
	}
	s := m.topo.Agents[agent].System
	return &api.SelfSystem{
		Repo:    o.repoName,
		Branch:  o.branch,
		Scope:   o.scope,
		Read:    s.Read,
		WorkDir: api.WorkDir(o.scope),
	}
}

func (m *Manager) owner(agent string) (*owner, error) {
	o, ok := m.owners[agent]
	if !ok {
		return nil, fmt.Errorf("%w: %q owns no system", ErrForbidden, agent)
	}
	return o, nil
}

// changedPaths returns the paths in o's scope that changed since the map's
// described commit.
func (m *Manager) changedPaths(o *owner, mp *sysmap.Map) []string {
	commit := mp.System.DescribedState.Commit
	if commit == "" {
		return nil
	}
	if !o.repo.HasCommit(commit) {
		return []string{fmt.Sprintf("(the described commit %s no longer exists; describe the whole system again)", commit)}
	}
	changed, err := o.repo.ChangedSince(commit, o.scope, sysmap.Dir)
	if err != nil {
		return []string{fmt.Sprintf("(cannot compare with commit %s: %v)", commit, err)}
	}
	return changed
}

// view returns the public view of o's map.
func (m *Manager) view(o *owner) sysmap.View {
	return m.ownerView(o).View
}

// ownerView returns what the owner sees of its system.
func (m *Manager) ownerView(o *owner) sysmap.OwnerView {
	mp := o.store.Map()
	priv, _ := o.store.Private()
	changed := m.changedPaths(o, mp)
	w := o.wiki.Wiki()
	ids := make([]string, len(mp.Features))
	for i, f := range mp.Features {
		ids[i] = f.ID
	}
	rep, resultsProblem := m.testReport(o, priv, ids)
	described := func(id string) bool { return w[id] != nil && w[id].DescribedCommit != "" }
	paths := func(id string) []string { return append(priv.Code(id), rep.Features[id].Files...) }
	st := sysmap.FeatureStates(ids, described, paths, rep.TestFiles, changed)
	warnings := w.Warnings(m.check(m.wikis[api.SystemWikiRef(o.name)]))
	if resultsProblem != "" {
		warnings = append(warnings, resultsProblem)
	}
	for _, key := range rep.Shadowed {
		warnings = append(warnings, fmt.Sprintf("%s: the key %q decides no test, because a more specific key decides each test it matches; remove it", testmap.MapFile, key))
	}
	for _, id := range ids {
		if len(priv.Layout.Features[id].Code) == 0 {
			warnings = append(warnings, fmt.Sprintf("feature %q has no code entry in %s; add one so that hikari can tell when its code changes", id, sysmap.LayoutFile))
		}
	}
	o.mu.Lock()
	projectErr := o.projectErr
	o.mu.Unlock()
	for _, p := range o.wiki.Problems() {
		warnings = append(warnings, "the wiki: "+p)
	}
	var problems []string
	for _, p := range []string{o.store.Error(), projectErr} {
		if p != "" {
			problems = append(problems, p)
		}
	}
	return sysmap.OwnerView{
		View:                sysmap.NewView(mp, changed, rep.Features, st.ByID),
		Changed:             changed,
		StaleFeatures:       st.Stale,
		UndescribedFeatures: st.Undescribed,
		UnmappedPaths:       st.Unmapped,
		FailingTests:        rep.Failing,
		PendingFeatures:     rep.Pending(),
		UnmappedTests:       rep.Unmapped,
		StaleKeys:           rep.StaleKeys,
		Warnings:            warnings,
		Error:               strings.Join(problems, "; "),
		PrivateErrors:       o.store.PrivateErrors(mp),
	}
}

// OwnView returns what agent sees of its own map.
func (m *Manager) OwnView(agent string) (sysmap.OwnerView, error) {
	o, err := m.owner(agent)
	if err != nil {
		return sysmap.OwnerView{}, err
	}
	return m.ownerView(o), nil
}

// checkPrivate returns an error that lists the problems of o's private
// documents, if there are any.
func checkPrivate(o *owner, mp *sysmap.Map) error {
	if errs := o.store.PrivateErrors(mp); len(errs) > 0 {
		return fmt.Errorf("fix your private documents in %s/%s first:\n%s", sysmap.Dir, sysmap.PrivateDir, strings.Join(errs, "\n"))
	}
	return nil
}

func (m *Manager) update(agent string, fn func(o *owner, mp *sysmap.Map) error) error {
	o, err := m.owner(agent)
	if err != nil {
		return err
	}
	if err := o.store.Update(func(mp *sysmap.Map) error { return fn(o, mp) }); err != nil {
		return err
	}
	m.changed(Change{System: agent})
	return nil
}

// SetSystemInfo changes the system section, then writes the features from
// the wiki again: a wiki that names interfaces the map lost, as after an old
// map is deleted (rule Q4), projects once the interfaces return.
func (m *Manager) SetSystemInfo(agent string, p sysmap.SystemPatch) error {
	o, err := m.owner(agent)
	if err != nil {
		return err
	}
	if err := o.store.Update(func(mp *sysmap.Map) error { mp.ApplySystem(p); return nil }); err != nil {
		return err
	}
	defer m.changed(Change{System: agent})
	err = m.project(o)
	m.commitStore(agent, agent+": set the system info", m.mapPath(agent))
	if err != nil {
		return fmt.Errorf("the system info is saved, but the map could not be written from the wiki: %w", err)
	}
	return nil
}

// RecordRun records the report of one run of a suite (rules J1 to J8): it
// reads the report, keeps the result of each test with the hashes it
// depends on, deletes the report, and writes the map with the new health.
func (m *Manager) RecordRun(agent, suite string) (api.RunResult, error) {
	o, err := m.owner(agent)
	if err != nil {
		return api.RunResult{}, err
	}
	o.runs.Lock()
	defer o.runs.Unlock()
	if err := checkPrivate(o, o.store.Map()); err != nil {
		return api.RunResult{}, err
	}
	priv, _ := o.store.Private()
	if _, ok := priv.Tests.Suites[suite]; !ok {
		return api.RunResult{}, fmt.Errorf("%w: %s has no suite %q; its suites are %s", ErrNotFound, testmap.MapFile, suite, list(slices.Sorted(maps.Keys(priv.Tests.Suites))))
	}
	rel := path.Join(testmap.ReportsDir, suite+".xml")
	report := o.store.PrivatePath(filepath.FromSlash(rel))
	where := path.Join(api.PrivateDir, rel)
	info, err := os.Stat(report)
	if err != nil {
		return api.RunResult{}, fmt.Errorf("J2: no report at %s; make the command of suite %q write a JUnit XML report there, then run it again (the hikari-system skill's references/tests.md lists the flags of each runner)", where, suite)
	}
	if info.Size() > testmap.MaxReportBytes {
		return api.RunResult{}, fmt.Errorf("J6: the report %s has %d bytes; at most %d", where, info.Size(), testmap.MaxReportBytes)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		return api.RunResult{}, err
	}
	cases, err := testmap.ParseReport(data, api.WorkDir(o.scope), o.store)
	if err != nil {
		return api.RunResult{}, fmt.Errorf("J2: %s: %w", where, err)
	}
	ids := o.wiki.Wiki().Features()
	before, _ := m.testReport(o, priv, ids)
	root := o.store.Root()
	code := map[string]string{}
	for _, id := range ids {
		code[id], _ = sysmap.ContentHash(root, priv.Code(id))
	}
	run := testmap.NewRun(cases, m.now(), code, func(rel string) string { h, _ := sysmap.ContentHash(root, []string{rel}); return h })
	resultsPath := o.store.PrivatePath(testmap.ResultsFile)
	results, _ := testmap.LoadResults(resultsPath)
	results.Runs[suite] = run
	if err := testmap.SaveResults(resultsPath, results); err != nil {
		return api.RunResult{}, err
	}
	if err := os.Remove(report); err != nil {
		return api.RunResult{}, err
	}
	if err := m.project(o); err != nil {
		return api.RunResult{}, err
	}
	after, _ := m.testReport(o, priv, ids)
	out := api.RunResult{Suite: suite}
	matcher := testmap.NewMatcher(priv.Tests, o.store)
	for _, c := range cases {
		switch c.Outcome {
		case testmap.Passed:
			out.Passed++
		case testmap.Failed:
			out.Failed++
		case testmap.Skipped:
			out.Skipped++
		}
		if _, ok := matcher.Key(c.ID, c.File); !ok {
			out.Unmapped++
		}
	}
	for _, id := range ids {
		from, to := before.Features[id].Health, after.Features[id].Health
		if from != to {
			out.Changed = append(out.Changed, api.HealthChange{Feature: id, From: from, To: to})
		}
	}
	m.changed(Change{System: agent})
	return out, nil
}

// MarkCurrent commits the pending changes in agent's scope, then records the
// commit as the code that the map describes. It returns that commit.
func (m *Manager) MarkCurrent(agent string) (string, error) {
	o, err := m.owner(agent)
	if err != nil {
		return "", err
	}
	if err := checkPrivate(o, o.store.Map()); err != nil {
		return "", err
	}
	if _, err := o.repo.Commit(o.branch, o.scope, agent, agent+": changes described by the system map"); err != nil {
		return "", err
	}
	m.checkDirty(o)
	head, err := o.repo.Head()
	if err != nil {
		return "", err
	}
	now := m.now()
	m.wikiMu.Lock()
	defer m.wikiMu.Unlock()
	w := m.wikis[api.SystemWikiRef(agent)]
	m.commitUserEdits(fmt.Sprintf("user: wiki edits before %s's change", agent), w)
	if err := o.wiki.SetDescribed(head, now); err != nil {
		return "", err
	}
	if err := m.update(agent, func(_ *owner, mp *sysmap.Map) error { mp.MarkCurrent(head, now); return nil }); err != nil {
		return "", err
	}
	m.commitStore(agent, fmt.Sprintf("%s: mark the map current at %.7s", agent, head), m.mapPath(agent), w.dir)
	m.markSeen(agent, []*wikiOf{w})
	return head, nil
}

// Commit commits the changes in agent's scope. An empty message makes the
// automatic commit at the end of a turn (rule G13). It returns the new
// commit, or "" when the scope had no changes.
func (m *Manager) Commit(agent, message string) (string, error) {
	o, err := m.owner(agent)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		message = agent + ": changes left at the end of a turn"
	}
	commit, err := o.repo.Commit(o.branch, o.scope, agent, message)
	if err == nil && commit != "" {
		m.checkDirty(o)
		m.changed(Change{System: agent})
	}
	return commit, err
}

// PublishRelease publishes the staged product of a deliverable as a
// release (rules R16 and R17). The map must be current and the private
// documents valid.
func (m *Manager) PublishRelease(agent string, req api.PublishRequest) (store.Manifest, error) {
	o, err := m.holds(agent, req.Deliverable)
	if err != nil {
		return store.Manifest{}, err
	}
	v, problems := m.releaseProblems(o)
	if len(problems) > 0 {
		return store.Manifest{}, errors.New(strings.Join(problems, "\n"))
	}
	public, err := yaml.Marshal(v.View)
	if err != nil {
		return store.Manifest{}, err
	}
	rel, err := m.store.Publish(store.PublishInput{
		System: agent, Deliverable: req.Deliverable, Version: req.Version, Notes: req.Notes,
		Certificates: req.Certificates, SourceCommit: v.System.DescribedState.Commit, Map: public,
		Wiki: o.wiki.Files(releaseKinds...), Hikari: version.Version, Now: m.now(),
	})
	if err != nil {
		return store.Manifest{}, err
	}
	m.changed(Change{System: agent})
	return rel, nil
}

// TurnContext tells the driver what changed before agent's next turn. It
// marks the vocabulary as seen.
func (m *Manager) TurnContext(agent string) api.TurnContext {
	var tc api.TurnContext
	if o, ok := m.owners[agent]; ok {
		tc.NoLocalResults = m.noLocalResults(o)
		v := m.ownerView(o)
		tc.MapState, tc.PrivateErrors = v.State, v.PrivateErrors
		tc.StaleFeatures, tc.UndescribedFeatures, tc.UnmappedPaths = v.StaleFeatures, v.UndescribedFeatures, v.UnmappedPaths
		tc.FailingFeatures, tc.PendingFeatures = failingFeatures(v.FailingTests), v.PendingFeatures
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tc.WikiChanged = m.wikiChanges(agent)
	if _, version := m.vocab.Get(); version != m.seen[agent] {
		tc.VocabularyChanged = true
		m.seen[agent] = version
	}
	return tc
}

// failingFeatures groups failing tests by the features they touch.
func failingFeatures(tests []testmap.FailingTest) []api.FailingFeature {
	byID := map[string][]string{}
	for _, t := range tests {
		for _, id := range t.Features {
			byID[id] = append(byID[id], t.ID)
		}
	}
	var out []api.FailingFeature
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		out = append(out, api.FailingFeature{ID: id, Tests: byID[id]})
	}
	return out
}

// visible returns the systems that agent may read as the orchestrator.
func (m *Manager) visible(agent string) ([]string, error) {
	if err := m.orchestrator(agent, "reads the system maps"); err != nil {
		return nil, err
	}
	return m.topo.VisibleSystems(agent), nil
}

// orchestrator returns an error unless agent is the orchestrator, the only
// agent that does what.
func (m *Manager) orchestrator(agent, what string) error {
	switch o := m.topo.Orchestrator; {
	case o == "":
		return fmt.Errorf("%w: only the orchestrator %s, and the project has none", ErrForbidden, what)
	case o != agent:
		return fmt.Errorf("%w: only %q, the orchestrator, %s", ErrForbidden, o, what)
	}
	return nil
}

func (m *Manager) visibleSystem(agent, name string) (*owner, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(names, name) {
		return nil, fmt.Errorf("%w: %q is not a system; the systems are %s", ErrNotFound, name, list(names))
	}
	return m.owners[name], nil
}

// ListSystems returns a summary of each system visible to agent.
func (m *Manager) ListSystems(agent string) ([]api.SystemSummary, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	out := []api.SystemSummary{}
	for _, name := range names {
		v := m.view(m.owners[name])
		health := map[string]int{}
		for _, f := range v.Features {
			health[f.Health]++
		}
		out = append(out, api.SystemSummary{Name: name, Summary: v.System.Summary, State: v.State, Features: health})
	}
	return out, nil
}

// GetSystem returns the orchestrator view of one system visible to agent,
// with its interface flags and the providers of what it consumes.
func (m *Manager) GetSystem(agent, name string) (api.OrchestratorSystem, error) {
	o, err := m.visibleSystem(agent, name)
	if err != nil {
		return api.OrchestratorSystem{}, err
	}
	names, _ := m.visible(agent)
	known := m.vocabularyNames()
	providers := m.providers(names)
	v := m.view(o)
	releases, _ := m.store.Releases(name)
	return api.OrchestratorSystem{
		View:      v,
		Releases:  releases,
		Flags:     vocab.Check(interfaceNames(v.System.Interfaces.Provides), interfaceNames(v.System.Interfaces.Consumes), known, keys(providers)),
		Providers: providersOf(v.System.Interfaces.Consumes, providers, name),
	}, nil
}

// changeVocab applies fn to the vocabulary and commits it with a message
// that says what changed: verb and name.
func (m *Manager) changeVocab(agent, verb, name string, fn func(*vocab.Store) error) error {
	if err := m.orchestrator(agent, "changes the vocabulary"); err != nil {
		return err
	}
	if err := fn(m.vocab); err != nil {
		return err
	}
	m.commitStore(agent, fmt.Sprintf("%s: %s %s in the vocabulary", agent, verb, name), m.vocabularyPath())
	m.changed(Change{Vocabulary: true})
	return nil
}

func (m *Manager) DefineTerm(agent string, t vocab.Term) error {
	return m.changeVocab(agent, "define", t.Term, func(s *vocab.Store) error { return s.DefineTerm(t) })
}

func (m *Manager) DefineInterface(agent string, in vocab.Interface) error {
	return m.changeVocab(agent, "define", in.Name, func(s *vocab.Store) error { return s.DefineInterface(in) })
}

func (m *Manager) RemoveEntry(agent, name string) error {
	return m.changeVocab(agent, "remove", name, func(s *vocab.Store) error { return s.Remove(name) })
}

// Vocabulary returns the project's vocabulary, which every agent reads.
func (m *Manager) Vocabulary() vocab.Vocabulary {
	v, _ := m.vocab.Get()
	return v
}

// Systems returns the full view of every system, for the UI.
func (m *Manager) Systems() []api.SystemView {
	out := []api.SystemView{}
	for _, name := range m.topo.Owners() {
		out = append(out, m.SystemView(name))
	}
	return out
}

// SystemView returns the full view of one system with its repo details and
// its interface flags.
func (m *Manager) SystemView(name string) api.SystemView {
	o := m.owners[name]
	v := m.ownerView(o)
	priv, _ := o.store.Private()
	providers := m.providers(m.topo.Owners())
	releases, _ := m.store.Releases(name)
	o.mu.Lock()
	dirty := o.dirty
	o.mu.Unlock()
	grants := m.topo.Agents[name].Grants
	return api.SystemView{
		OwnerView: v,
		Dirty:     dirty,
		Grants:    api.Grants{Releases: nonNil(grants.Releases), Deliverables: nonNil(grants.Deliverables)},
		Private:   priv,
		Releases:  releases,
		Owner:     name,
		Repo:      o.repoName,
		Scope:     o.scope,
		Branch:    o.branch,
		Flags:     vocab.Check(interfaceNames(v.System.Interfaces.Provides), interfaceNames(v.System.Interfaces.Consumes), m.vocabularyNames(), keys(providers)),
		Providers: providersOf(v.System.Interfaces.Consumes, providers, name),
	}
}

func (m *Manager) vocabularyNames() map[string]bool {
	known := map[string]bool{}
	for _, name := range m.Vocabulary().Names() {
		known[name] = true
	}
	return known
}

// providers maps each interface name to the systems among names that
// provide it.
func (m *Manager) providers(names []string) map[string][]string {
	out := map[string][]string{}
	for _, name := range names {
		for _, in := range m.owners[name].store.Map().System.Interfaces.Provides {
			out[in.Name] = append(out[in.Name], name)
		}
	}
	return out
}

func providersOf(consumes []sysmap.Interface, providers map[string][]string, self string) map[string][]string {
	out := map[string][]string{}
	for _, in := range consumes {
		for _, p := range providers[in.Name] {
			if p != self {
				out[in.Name] = append(out[in.Name], p)
			}
		}
	}
	return out
}

func interfaceNames(ins []sysmap.Interface) []string {
	out := make([]string, len(ins))
	for i, in := range ins {
		out[i] = in.Name
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return slices.Clone(s)
}

func keys[V any](m map[string]V) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

func list(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
