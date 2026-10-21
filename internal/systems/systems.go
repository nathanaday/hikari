// Package systems connects the topology to the repos, the system maps, and
// the hub vocabularies. It enforces who may read and change each of them:
// an owner reads and changes only its own map, an orchestrator reads the
// orchestrator view of the systems in the hubs it orchestrates, and only a
// hub's orchestrator changes the hub's vocabulary.
package systems

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
)

var (
	ErrForbidden = errors.New("not allowed")
	ErrNotFound  = errors.New("not found")
)

// Change names what changed: a system map or a hub vocabulary.
type Change struct {
	System string
	Hub    string
}

type owner struct {
	name     string
	repoName string
	scope    string
	branch   string
	repo     *gitrepo.Repo
	store    *sysmap.Store
}

type Manager struct {
	topo     *topology.Topology
	repos    map[string]*gitrepo.Repo
	start    map[string]string
	owners   map[string]*owner
	vocabs   map[string]*vocab.Store
	warnings []string
	now      func() time.Time

	mu       sync.Mutex
	seen     map[string]map[string]int
	onChange func(Change)
}

// New prepares every repo of topo (rules G1 to G5) and loads the system maps
// and the hub vocabularies.
func New(topo *topology.Topology) (*Manager, error) {
	m := &Manager{
		topo:     topo,
		repos:    map[string]*gitrepo.Repo{},
		start:    map[string]string{},
		owners:   map[string]*owner{},
		vocabs:   map[string]*vocab.Store{},
		now:      time.Now,
		seen:     map[string]map[string]int{},
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
	for _, name := range topo.Owners() {
		s := topo.Agents[name].System
		r := topo.Repos[s.Repo]
		store, _, err := sysmap.Open(filepath.Join(r.Path, s.Scope), name, s.Test)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		repo := m.repos[s.Repo]
		mapPath := path.Join(filepath.ToSlash(s.Scope), sysmap.Dir, sysmap.File)
		if repo.IsIgnored(mapPath) {
			m.warnings = append(m.warnings, fmt.Sprintf("repo %q ignores %s, so the map of %q is not versioned with its code", s.Repo, mapPath, name))
		}
		m.owners[name] = &owner{name: name, repoName: s.Repo, scope: s.Scope, branch: r.Branch, repo: repo, store: store}
	}
	for _, hub := range topo.HubNames() {
		store, err := vocab.Open(filepath.Join(topo.Dir, "vocabulary", hub+".yaml"), hub)
		if err != nil {
			return nil, err
		}
		m.vocabs[hub] = store
	}
	for _, agent := range topo.AgentNames() {
		m.seen[agent] = map[string]int{}
		for _, hub := range topo.HubsOf(agent) {
			_, v := m.vocabs[hub].Get()
			m.seen[agent][hub] = v
		}
	}
	return m, nil
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
// system's flags and providers come from the vocabularies of its hubs and
// from the maps of its peers in those hubs.
func (m *Manager) changed(c Change) {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	var systems []string
	if c.System != "" {
		systems = m.peers(c.System)
	}
	if c.Hub != "" {
		fn(Change{Hub: c.Hub})
		for _, member := range m.topo.Hubs[c.Hub].Members {
			if _, ok := m.owners[member]; ok && !slices.Contains(systems, member) {
				systems = append(systems, member)
			}
		}
	}
	for _, name := range systems {
		fn(Change{System: name})
	}
}

// peers returns system and the other owners that are members of a hub with
// it.
func (m *Manager) peers(system string) []string {
	out := []string{system}
	for _, hub := range m.topo.HubNames() {
		members := m.topo.Hubs[hub].Members
		if !slices.Contains(members, system) {
			continue
		}
		for _, member := range members {
			if _, ok := m.owners[member]; ok && !slices.Contains(out, member) {
				out = append(out, member)
			}
		}
	}
	return out
}

// Watch reloads hand edits of the map files until ctx ends.
func (m *Manager) Watch(ctx context.Context) {
	for _, o := range m.owners {
		go o.store.Watch(ctx, time.Second, func() { m.changed(Change{System: o.name}) })
	}
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
	return out
}

// Self returns the system details of agent, for its container.
func (m *Manager) Self(agent string) (*api.SelfSystem, []string) {
	orchestrates := m.topo.OrchestratedHubs(agent)
	o, ok := m.owners[agent]
	if !ok {
		return nil, orchestrates
	}
	s := m.topo.Agents[agent].System
	return &api.SelfSystem{
		Repo:    o.repoName,
		Branch:  o.branch,
		Scope:   o.scope,
		Read:    s.Read,
		WorkDir: api.WorkDir(o.scope),
	}, orchestrates
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

// ownerView returns what the owner sees of its map.
func (m *Manager) ownerView(o *owner) sysmap.OwnerView {
	mp := o.store.Map()
	priv, _ := o.store.Private()
	hash := func(id string) string {
		h, _ := sysmap.ContentHash(o.store.Root(), priv.Paths(id))
		return h
	}
	changed := m.changedPaths(o, mp)
	return sysmap.OwnerView{
		View:          sysmap.NewView(mp, priv, changed, hash),
		Changed:       changed,
		Error:         o.store.Error(),
		PrivateErrors: o.store.PrivateErrors(mp),
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

func (m *Manager) SetSystemInfo(agent string, p sysmap.SystemPatch) error {
	return m.update(agent, func(_ *owner, mp *sysmap.Map) error { mp.ApplySystem(p); return nil })
}

// UpsertFeatures adds or replaces features as one change, so that features
// in the batch can refer to each other.
func (m *Manager) UpsertFeatures(agent string, ins []sysmap.FeatureInput) error {
	if len(ins) == 0 {
		return errors.New("give at least one feature")
	}
	return m.update(agent, func(_ *owner, mp *sysmap.Map) error {
		for _, in := range ins {
			mp.Upsert(in)
		}
		return nil
	})
}

func (m *Manager) RemoveFeature(agent, id string) error {
	return m.update(agent, func(_ *owner, mp *sysmap.Map) error { return mp.Remove(id) })
}

// RecordTests stores test results. The private documents must be valid, and
// the test plan must list tests for each feature.
func (m *Manager) RecordTests(agent string, results []sysmap.TestResult) error {
	return m.update(agent, func(o *owner, mp *sysmap.Map) error {
		if err := checkPrivate(o, mp); err != nil {
			return err
		}
		priv, err := o.store.Private()
		if err != nil {
			return err
		}
		for _, r := range results {
			if !priv.Tested(r.Feature) {
				return fmt.Errorf("%s/%s/%s lists no tests for feature %q; add them before you record results", sysmap.Dir, sysmap.PrivateDir, sysmap.TestFile, r.Feature)
			}
		}
		return mp.RecordTests(results, m.now(), o.store.FeatureHash)
	})
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
	head, err := o.repo.Head()
	if err != nil {
		return "", err
	}
	return head, m.update(agent, func(_ *owner, mp *sysmap.Map) error { mp.MarkCurrent(head, m.now()); return nil })
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
		m.changed(Change{System: agent})
	}
	return commit, err
}

// TurnContext tells the driver what changed before agent's next turn. It
// marks the vocabularies as seen.
func (m *Manager) TurnContext(agent string) api.TurnContext {
	var tc api.TurnContext
	if o, ok := m.owners[agent]; ok {
		v := m.ownerView(o)
		tc.MapState, tc.Changed, tc.PrivateErrors = v.State, v.Changed, v.PrivateErrors
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, hub := range m.topo.HubsOf(agent) {
		_, version := m.vocabs[hub].Get()
		if version != m.seen[agent][hub] {
			tc.VocabularyChanged = append(tc.VocabularyChanged, hub)
			m.seen[agent][hub] = version
		}
	}
	return tc
}

// visible returns the systems that agent may read as an orchestrator.
func (m *Manager) visible(agent string) ([]string, error) {
	if len(m.topo.OrchestratedHubs(agent)) == 0 {
		return nil, fmt.Errorf("%w: only orchestrators can read system maps", ErrForbidden)
	}
	return m.topo.VisibleSystems(agent), nil
}

func (m *Manager) visibleSystem(agent, name string) (*owner, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(names, name) {
		return nil, fmt.Errorf("%w: %q is not a system in the hubs that %q orchestrates; visible systems are %s",
			ErrNotFound, name, agent, list(names))
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
	known := m.vocabularyNames(m.topo.OrchestratedHubs(agent))
	providers := m.providers(names)
	v := m.view(o)
	return api.OrchestratorSystem{
		View:      v,
		Flags:     vocab.Check(interfaceNames(v.System.Interfaces.Provides), interfaceNames(v.System.Interfaces.Consumes), known, keys(providers)),
		Providers: providersOf(v.System.Interfaces.Consumes, providers, name),
	}, nil
}

// SearchFeatures returns the features of visible systems that match query.
func (m *Manager) SearchFeatures(agent, query string) ([]api.FeatureHit, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := []api.FeatureHit{}
	for _, name := range names {
		for _, f := range m.view(m.owners[name]).Features {
			text := strings.ToLower(strings.Join(append([]string{f.ID, f.Name, f.Description}, f.Interfaces...), "\n"))
			if strings.Contains(text, q) {
				out = append(out, api.FeatureHit{System: name, Feature: f})
			}
		}
	}
	return out, nil
}

// GetFeature returns the orchestrator view of one feature.
func (m *Manager) GetFeature(agent, system, id string) (api.FeatureHit, error) {
	o, err := m.visibleSystem(agent, system)
	if err != nil {
		return api.FeatureHit{}, err
	}
	for _, f := range m.view(o).Features {
		if f.ID == id {
			return api.FeatureHit{System: system, Feature: f}, nil
		}
	}
	return api.FeatureHit{}, fmt.Errorf("%w: system %q has no feature %q", ErrNotFound, system, id)
}

func (m *Manager) orchestratedVocab(agent, hub string) (*vocab.Store, error) {
	h, ok := m.topo.Hubs[hub]
	if !ok {
		return nil, fmt.Errorf("%w: no hub %q", ErrNotFound, hub)
	}
	if h.Orchestrator != agent {
		return nil, fmt.Errorf("%w: only %q, the orchestrator of #%s, can change its vocabulary", ErrForbidden, h.Orchestrator, hub)
	}
	return m.vocabs[hub], nil
}

func (m *Manager) changeVocab(agent, hub string, fn func(*vocab.Store) error) error {
	s, err := m.orchestratedVocab(agent, hub)
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	m.changed(Change{Hub: hub})
	return nil
}

func (m *Manager) DefineTerm(agent, hub string, t vocab.Term) error {
	return m.changeVocab(agent, hub, func(s *vocab.Store) error { return s.DefineTerm(t) })
}

func (m *Manager) DefineInterface(agent, hub string, in vocab.Interface) error {
	return m.changeVocab(agent, hub, func(s *vocab.Store) error { return s.DefineInterface(in) })
}

func (m *Manager) RemoveEntry(agent, hub, name string) error {
	return m.changeVocab(agent, hub, func(s *vocab.Store) error { return s.Remove(name) })
}

// GetVocabulary returns the vocabulary of a hub that agent is in.
func (m *Manager) GetVocabulary(agent, hub string) (vocab.Vocabulary, error) {
	if _, ok := m.topo.Hubs[hub]; !ok {
		return vocab.Vocabulary{}, fmt.Errorf("%w: no hub %q", ErrNotFound, hub)
	}
	if !m.topo.InHub(agent, hub) {
		return vocab.Vocabulary{}, fmt.Errorf("%w: %q is not in #%s", ErrForbidden, agent, hub)
	}
	v, _ := m.vocabs[hub].Get()
	return v, nil
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
// the interface flags seen from the hubs its owner is a member of.
func (m *Manager) SystemView(name string) api.SystemView {
	o := m.owners[name]
	v := m.ownerView(o)
	priv, _ := o.store.Private()
	var hubs, peers []string
	for _, hub := range m.topo.HubNames() {
		if slices.Contains(m.topo.Hubs[hub].Members, name) {
			hubs = append(hubs, hub)
			for _, member := range m.topo.Hubs[hub].Members {
				if _, ok := m.owners[member]; ok && !slices.Contains(peers, member) {
					peers = append(peers, member)
				}
			}
		}
	}
	providers := m.providers(peers)
	return api.SystemView{
		OwnerView: v,
		Private:   priv,
		Owner:     name,
		Repo:      o.repoName,
		Scope:     o.scope,
		Branch:    o.branch,
		Flags:     vocab.Check(interfaceNames(v.System.Interfaces.Provides), interfaceNames(v.System.Interfaces.Consumes), m.vocabularyNames(hubs), keys(providers)),
		Providers: providersOf(v.System.Interfaces.Consumes, providers, name),
	}
}

// Vocabularies returns every hub vocabulary, for the UI.
func (m *Manager) Vocabularies() []vocab.Vocabulary {
	out := []vocab.Vocabulary{}
	for _, hub := range m.topo.HubNames() {
		v, _ := m.vocabs[hub].Get()
		out = append(out, v)
	}
	return out
}

// Vocabulary returns the vocabulary of one hub, for the UI.
func (m *Manager) Vocabulary(hub string) vocab.Vocabulary {
	v, _ := m.vocabs[hub].Get()
	return v
}

func (m *Manager) vocabularyNames(hubs []string) map[string]bool {
	known := map[string]bool{}
	for _, hub := range hubs {
		v, _ := m.vocabs[hub].Get()
		for _, name := range v.Names() {
			known[name] = true
		}
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
