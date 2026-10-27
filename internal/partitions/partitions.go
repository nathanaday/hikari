// Package partitions connects the topology to the repos, the feature maps, the
// wikis, and the vocabulary. It enforces who may read and change each of
// them: a partition agent reads and changes only its own map, the orchestration agent reads
// the orchestration agent's view of every partition, and only the orchestration agent changes
// the vocabulary and the public wiki.
package partitions

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
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/version"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
	"gopkg.in/yaml.v3"
)

var (
	ErrForbidden = errors.New("not allowed")
	ErrNotFound  = errors.New("not found")
)

// Change names what changed: a feature map, the vocabulary, or a wiki.
type Change struct {
	Partition  string
	Vocabulary bool
	// Wiki is the ref of a wiki that changed.
	Wiki string
}

type partitionAgent struct {
	name       string
	repoName   string
	scope      string
	branch     string
	repo       *gitrepo.Repo
	featureMap *featuremap.Files
	wiki       *wiki.Files

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
	topo            *topology.Topology
	vault           vault.Vault
	repos           map[string]*gitrepo.Repo
	start           map[string]string
	partitionAgents map[string]*partitionAgent
	vocab           *vocab.File
	warnings        []string
	now             func() time.Time
	// projectRepo is the project repository, or nil. vaultRel is the vault's
	// path in it.
	projectRepo *gitrepo.Repo
	vaultRel    string

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

// New prepares every repo of topo (rules G1 to G5) and loads the feature maps,
// the wikis, and the vocabulary.
func New(topo *topology.Topology) (*Manager, error) {
	m := &Manager{
		topo:            topo,
		vault:           topo.VaultOf(),
		repos:           map[string]*gitrepo.Repo{},
		start:           map[string]string{},
		partitionAgents: map[string]*partitionAgent{},
		now:             time.Now,
		seen:            map[string]int{},
		seenDocs:        map[string]map[string]map[string]string{},
		wikis:           map[string]*wikiOf{},
		onChange:        func(Change) {},
	}
	for name, r := range topo.Repos {
		repo := gitrepo.Open(r.Path)
		start, err := repo.Prepare(r.Branch)
		if err != nil {
			return nil, fmt.Errorf("repo %q: %w", name, err)
		}
		m.repos[name], m.start[name] = repo, start
	}
	if err := m.vault.Prepare(topo.PartitionAgents()); err != nil {
		return nil, fmt.Errorf("vault %s: %w", m.vault.Root, err)
	}
	if topo.Project != nil {
		m.projectRepo, m.vaultRel = m.repos[topology.ProjectRepo], topo.VaultInProject()
		if _, err := m.vault.WriteIgnore(); err != nil {
			return nil, fmt.Errorf("vault %s: %w", m.vault.Root, err)
		}
		m.commitOwnFiles()
	}
	for _, name := range topo.PartitionAgents() {
		s := topo.Agents[name].Partition
		r := topo.Repos[s.Repo]
		scopeRoot := filepath.Join(r.Path, s.Scope)
		maps, _, err := featuremap.Open(scopeRoot, m.vault.MapFile(name), name, s.Test)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		repo := m.repos[s.Repo]
		privatePath := path.Join(filepath.ToSlash(s.Scope), featuremap.Dir, featuremap.PrivateDir, featuremap.CodeMapFile)
		if repo.IsIgnored(privatePath) {
			m.warnings = append(m.warnings, fmt.Sprintf("repo %q ignores %s, so the private documents of %q are not versioned with its code", s.Repo, privatePath, name))
		}
		m.warnings = append(m.warnings, maps.Notes()...)
		m.partitionAgents[name] = &partitionAgent{name: name, repoName: s.Repo, scope: s.Scope, branch: r.Branch, repo: repo, featureMap: maps}
	}
	if err := m.openWikis(); err != nil {
		return nil, err
	}
	for _, name := range topo.PartitionAgents() {
		m.partitionAgents[name].wiki = m.wikis[api.FeatureMapRef(name)].files
	}
	for _, name := range topo.PartitionAgents() {
		o := m.partitionAgents[name]
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
	vs, err := vocab.Open(m.vault.VocabularyFile())
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

// vaultPath returns a vault path, relative to the vault, as a path of the
// project repository.
func (m *Manager) vaultPath(rel string) string { return path.Join(m.vaultRel, rel) }

// mapPath is the vault path of a partition agent's map.
func (m *Manager) mapPath(partitionAgent string) string {
	return m.vault.Rel(m.vault.MapFile(partitionAgent))
}

// vocabularyPath is the vault path of the vocabulary.
func (m *Manager) vocabularyPath() string { return m.vault.Rel(m.vault.VocabularyFile()) }

// commitVault commits the vault paths of agent in the project repository
// (rules PR14 to PR17). It returns a note when the commit failed, and does
// nothing outside a project repository.
func (m *Manager) commitVault(agent, message string, paths ...string) string {
	if m.projectRepo == nil {
		return ""
	}
	specs := make([]string, len(paths))
	for i, p := range paths {
		specs[i] = m.vaultPath(p)
	}
	if _, err := m.projectRepo.CommitPaths(m.topo.Project.Branch, specs, agent, message); err != nil {
		log.Printf("vault commit for %s: %v", agent, err)
		return "the vault change could not be committed: " + err.Error()
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

// commitOwnFiles commits the vault files that hikari writes itself (rule
// PR18).
func (m *Manager) commitOwnFiles() {
	for _, f := range []string{"README.md", vault.IgnoreFile} {
		verb := "update"
		if !m.projectRepo.Tracked(m.vaultPath(f)) {
			verb = "add"
		}
		m.commitVault("hikari", fmt.Sprintf("hikari: %s %s", verb, m.vaultPath(f)), f)
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

// Vault returns the vault.
func (m *Manager) Vault() vault.Vault { return m.vault }

// checkDeliverables checks every granted deliverable at startup (rules T15
// and T16): it exists and is valid, and its name is not a skill's name.
func (m *Manager) checkDeliverables() error {
	var errs []error
	checked := map[string]bool{}
	for _, name := range m.topo.PartitionAgents() {
		for _, d := range m.topo.Agents[name].Grants.Deliverables {
			if checked[d] {
				continue
			}
			checked[d] = true
			if _, err := m.vault.LoadDeliverable(d); err != nil {
				errs = append(errs, err)
			}
			if slices.Contains(api.BuiltinSkills, d) {
				errs = append(errs, fmt.Errorf("deliverable %q has the name of hikari's own skill", d))
			} else if exists(filepath.Join(m.vault.Shared(vault.Skills), d, vault.SkillFile)) {
				errs = append(errs, fmt.Errorf("deliverable %q has the name of a shared skill; rename one of them", d))
			}
		}
	}
	return errors.Join(errs...)
}

// checkAgents refuses a shared agent whose name starts with hikari-, which
// is reserved for hikari's own agents (rule T17).
func (m *Manager) checkAgents() error {
	entries, _ := os.ReadDir(m.vault.Shared(vault.Agents))
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
func (m *Manager) project(o *partitionAgent) error {
	w := o.wiki.Wiki()
	neighbors := w.Neighbors()
	priv, _ := o.featureMap.Private()
	rep, _ := m.testReport(o, priv, w.Features())
	// With no results file, this machine has run no tests, which is no
	// evidence: the map keeps the health that another machine recorded
	// (rule SY10 of the linked repositories spec).
	recorded := map[string]featuremap.Feature{}
	if m.noLocalResults(o) {
		for _, f := range o.featureMap.Map().Features {
			recorded[f.ID] = f
		}
	}
	entries := make([]featuremap.FeatureEntry, 0, len(w))
	for _, id := range w.Features() {
		d := w[id]
		tests, health := featuremap.TestsOf(rep.Features[id])
		if f, ok := recorded[id]; ok && !f.Tests.Ran.IsZero() {
			tests, health = f.Tests, f.Health
		}
		entries = append(entries, featuremap.FeatureEntry{
			ID: id, Name: d.Name, Description: d.Description, Interfaces: d.Interfaces, Related: neighbors[id],
			Created: d.Created, Tests: tests, Health: health,
		})
	}
	err := o.featureMap.Update(func(mp *featuremap.Map) error { mp.Project(entries); return nil })
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
func (m *Manager) noLocalResults(o *partitionAgent) bool {
	if _, err := os.Stat(o.featureMap.PrivatePath(testmap.ResultsFile)); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return slices.ContainsFunc(o.featureMap.Map().Features, func(f featuremap.Feature) bool { return !f.Tests.Ran.IsZero() })
}

// testReport computes the health of the features ids of o from its test
// map and its results. The string is the problem of the results file.
func (m *Manager) testReport(o *partitionAgent, priv featuremap.Private, ids []string) (testmap.Report, string) {
	results, err := testmap.LoadResults(o.featureMap.PrivatePath(testmap.ResultsFile))
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	root := o.featureMap.Root()
	hashes := testmap.Hashes{
		File: func(rel string) string { h, _ := featuremap.ContentHash(root, []string{rel}); return h },
		Code: func(id string) string { h, _ := featuremap.ContentHash(root, priv.Code(id)); return h },
	}
	return testmap.Compute(priv.Tests, testmap.NewMatcher(priv.Tests, o.featureMap), results, ids, hashes), problem
}

// Deliverables returns the deliverables that agent holds, with their
// descriptions, for its prompt.
func (m *Manager) Deliverables(agent string) []api.DeliverableInfo {
	var out []api.DeliverableInfo
	for _, name := range m.topo.Agents[agent].Grants.Deliverables {
		info := api.DeliverableInfo{Name: name}
		if d, err := m.vault.LoadDeliverable(name); err != nil {
			info.Description = "(invalid: " + err.Error() + ")"
		} else {
			info.Description = d.Description
		}
		out = append(out, info)
	}
	return out
}

// holds checks that agent is a partition agent that holds deliverable.
func (m *Manager) holds(agent, deliverable string) (*partitionAgent, error) {
	o, err := m.partitionAgent(agent)
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

// releaseProblems lists what blocks a release of agent's partition: a map
// that is not current, or invalid private documents.
func (m *Manager) releaseProblems(o *partitionAgent) (featuremap.PartitionAgentView, []string) {
	v := m.partitionAgentView(o)
	var problems []string
	if v.State != featuremap.Current {
		problems = append(problems, fmt.Sprintf("the map is %s; update it and call mark_map_current first", v.State))
	}
	if len(v.PrivateErrors) > 0 {
		problems = append(problems, "the private documents have problems: "+strings.Join(v.PrivateErrors, "; "))
	}
	return v, problems
}

// CheckRelease checks the staged product of a deliverable against the
// deliverable and the state of the partition (rule R15). It changes nothing.
func (m *Manager) CheckRelease(agent, deliverable string) (vault.ProductCheck, error) {
	o, err := m.holds(agent, deliverable)
	if err != nil {
		return vault.ProductCheck{}, err
	}
	_, problems := m.releaseProblems(o)
	check, err := m.vault.Check(agent, deliverable)
	if err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return vault.ProductCheck{}, errors.New(strings.Join(problems, "\n"))
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

// changed reports c, and also every partition whose view depends on it: a
// partition's flags and providers come from the vocabulary and from the maps
// of every other partition.
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
	if c.Partition != "" || c.Vocabulary {
		for _, name := range m.peers(c.Partition) {
			fn(Change{Partition: name})
		}
	}
}

// peers returns partition first, then every other partition agent.
func (m *Manager) peers(partition string) []string {
	var out []string
	if partition != "" {
		out = append(out, partition)
	}
	for _, name := range m.topo.PartitionAgents() {
		if name != partition {
			out = append(out, name)
		}
	}
	return out
}

// Watch reloads hand edits of the map files and the feature wikis, and
// checks each scope for uncommitted changes, until ctx ends.
func (m *Manager) Watch(ctx context.Context) {
	go m.watchDirty(ctx, 2*time.Second)
	for _, o := range m.partitionAgents {
		go o.featureMap.Watch(ctx, time.Second, func() {
			m.project(o)
			m.changed(Change{Partition: o.name})
		})
	}
	go m.watchWikis(ctx)
}

// watchWikis reloads every wiki each second, and after a change of a
// partition's wiki writes the map from it (rules W8 and D4).
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
				reloaded := w.files.Reload()
				if reloaded && w.scope == "partition" {
					m.project(m.partitionAgents[w.name])
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
// reports each partition whose answer changed (rule N4).
func (m *Manager) watchDirty(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, name := range m.topo.PartitionAgents() {
				if o := m.partitionAgents[name]; m.checkDirty(o) {
					m.changed(Change{Partition: name})
				}
			}
		}
	}
}

// checkDirty asks git whether o's scope has uncommitted changes, records
// the answer, and reports whether it changed. A failed check keeps the last
// answer.
func (m *Manager) checkDirty(o *partitionAgent) bool {
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

// Repos returns the repositories of the topology with their partitions, for
// the UI (rule N2).
func (m *Manager) Repos() []api.RepoInfo {
	out := []api.RepoInfo{}
	for _, name := range slices.Sorted(maps.Keys(m.topo.Repos)) {
		info := api.RepoInfo{Name: name, Branch: m.topo.Repos[name].Branch, Partitions: []string{}}
		for _, partitionAgent := range m.topo.PartitionAgents() {
			if m.partitionAgents[partitionAgent].repoName == name {
				info.Partitions = append(info.Partitions, partitionAgent)
			}
		}
		slices.Sort(info.Partitions)
		if name == topology.ProjectRepo && len(info.Partitions) == 0 {
			continue
		}
		out = append(out, info)
	}
	return out
}

// CommitAll commits the changes left in every partition agent's scope, so that the
// next run does not find uncommitted changes. hikari calls it at shutdown.
func (m *Manager) CommitAll() []error {
	var errs []error
	for _, name := range m.topo.PartitionAgents() {
		o := m.partitionAgents[name]
		if _, err := o.repo.Commit(o.branch, o.scope, name, name+": changes left when hikari stopped"); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		if note := m.commitVault(name, name+": vault changes left when hikari stopped", m.mapPath(name)); note != "" {
			errs = append(errs, fmt.Errorf("%s: %s", name, note))
		}
	}
	var all []*wikiOf
	for _, ref := range m.wikiRefs() {
		all = append(all, m.wikis[ref])
	}
	m.commitUserEdits("user: wiki edits left when hikari stopped", all...)
	if orch := m.topo.Orchestration; orch != "" {
		if note := m.commitVault(orch, orch+": vault changes left when hikari stopped", m.vocabularyPath()); note != "" {
			errs = append(errs, fmt.Errorf("%s: %s", orch, note))
		}
	}
	if m.projectRepo != nil {
		m.commitOwnFiles()
	}
	return errs
}

// Summary returns one line for each partition agent with its commits since start
// (rule G11).
func (m *Manager) Summary() []string {
	var out []string
	for _, name := range m.topo.PartitionAgents() {
		o := m.partitionAgents[name]
		n, err := o.repo.CountAuthored(m.start[o.repoName], name)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		out = append(out, fmt.Sprintf("%s made %d commit(s) on %s in repo %q", name, n, o.branch, o.repoName))
	}
	if m.projectRepo != nil {
		n, err := m.projectRepo.CountOn(m.start[topology.ProjectRepo], m.vaultRel)
		if err != nil {
			out = append(out, fmt.Sprintf("the vault: %v", err))
		} else {
			out = append(out, fmt.Sprintf("the vault has %d commit(s) on %s in the project repository", n, m.topo.Project.Branch))
		}
	}
	return out
}

// Self returns the partition details of agent, for its container.
func (m *Manager) Self(agent string) *api.SelfPartition {
	o, ok := m.partitionAgents[agent]
	if !ok {
		return nil
	}
	s := m.topo.Agents[agent].Partition
	return &api.SelfPartition{
		Repo:    o.repoName,
		Branch:  o.branch,
		Scope:   o.scope,
		Read:    s.Read,
		WorkDir: api.WorkDir(o.scope),
	}
}

func (m *Manager) partitionAgent(agent string) (*partitionAgent, error) {
	o, ok := m.partitionAgents[agent]
	if !ok {
		return nil, fmt.Errorf("%w: %q has no partition", ErrForbidden, agent)
	}
	return o, nil
}

// changedPaths returns the paths in o's scope that changed since the map's
// described commit.
func (m *Manager) changedPaths(o *partitionAgent, mp *featuremap.Map) []string {
	commit := mp.Partition.DescribedState.Commit
	if commit == "" {
		return nil
	}
	if !o.repo.HasCommit(commit) {
		return []string{fmt.Sprintf("(the described commit %s no longer exists; describe the whole partition again)", commit)}
	}
	changed, err := o.repo.ChangedSince(commit, o.scope, featuremap.Dir)
	if err != nil {
		return []string{fmt.Sprintf("(cannot compare with commit %s: %v)", commit, err)}
	}
	return changed
}

// view returns the public view of o's map.
func (m *Manager) view(o *partitionAgent) featuremap.View {
	return m.partitionAgentView(o).View
}

// partitionAgentView returns what the partition agent sees of its partition.
func (m *Manager) partitionAgentView(o *partitionAgent) featuremap.PartitionAgentView {
	mp := o.featureMap.Map()
	priv, _ := o.featureMap.Private()
	changed := m.changedPaths(o, mp)
	w := o.wiki.Wiki()
	ids := make([]string, len(mp.Features))
	for i, f := range mp.Features {
		ids[i] = f.ID
	}
	rep, resultsProblem := m.testReport(o, priv, ids)
	described := func(id string) bool { return w[id] != nil && w[id].DescribedCommit != "" }
	paths := func(id string) []string { return append(priv.Code(id), rep.Features[id].Files...) }
	st := featuremap.FeatureStates(ids, described, paths, rep.TestFiles, changed)
	warnings := w.Warnings(m.check(m.wikis[api.FeatureMapRef(o.name)]))
	if resultsProblem != "" {
		warnings = append(warnings, resultsProblem)
	}
	for _, key := range rep.Shadowed {
		warnings = append(warnings, fmt.Sprintf("%s: the key %q decides no test, because a more specific key decides each test it matches; remove it", testmap.MapFile, key))
	}
	for _, id := range ids {
		if len(priv.CodeMap.Features[id].Code) == 0 {
			warnings = append(warnings, fmt.Sprintf("feature %q has no code entry in %s; add one so that hikari can tell when its code changes", id, featuremap.CodeMapFile))
		}
	}
	o.mu.Lock()
	projectErr := o.projectErr
	o.mu.Unlock()
	for _, p := range o.wiki.Problems() {
		warnings = append(warnings, "the wiki: "+p)
	}
	var problems []string
	for _, p := range []string{o.featureMap.Error(), projectErr} {
		if p != "" {
			problems = append(problems, p)
		}
	}
	return featuremap.PartitionAgentView{
		View:                featuremap.NewView(mp, changed, rep.Features, st.ByID),
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
		PrivateErrors:       o.featureMap.PrivateErrors(mp),
	}
}

// OwnView returns what agent sees of its own map.
func (m *Manager) OwnView(agent string) (featuremap.PartitionAgentView, error) {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return featuremap.PartitionAgentView{}, err
	}
	return m.partitionAgentView(o), nil
}

// checkPrivate returns an error that lists the problems of o's private
// documents, if there are any.
func checkPrivate(o *partitionAgent, mp *featuremap.Map) error {
	if errs := o.featureMap.PrivateErrors(mp); len(errs) > 0 {
		return fmt.Errorf("fix your private documents in %s/%s first:\n%s", featuremap.Dir, featuremap.PrivateDir, strings.Join(errs, "\n"))
	}
	return nil
}

func (m *Manager) update(agent string, fn func(o *partitionAgent, mp *featuremap.Map) error) error {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return err
	}
	if err := o.featureMap.Update(func(mp *featuremap.Map) error { return fn(o, mp) }); err != nil {
		return err
	}
	m.changed(Change{Partition: agent})
	return nil
}

// SetPartitionInfo changes the partition section, then writes the features from
// the wiki again: a wiki that names interfaces the map lost, as after an old
// map is deleted (rule Q4), projects once the interfaces return.
func (m *Manager) SetPartitionInfo(agent string, p featuremap.PartitionPatch) error {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return err
	}
	if err := o.featureMap.Update(func(mp *featuremap.Map) error { mp.ApplyPartition(p); return nil }); err != nil {
		return err
	}
	defer m.changed(Change{Partition: agent})
	err = m.project(o)
	m.commitVault(agent, agent+": set the partition info", m.mapPath(agent))
	if err != nil {
		return fmt.Errorf("the partition info is saved, but the map could not be written from the wiki: %w", err)
	}
	return nil
}

// RecordRun records the report of one run of a suite (rules J1 to J8): it
// reads the report, keeps the result of each test with the hashes it
// depends on, deletes the report, and writes the map with the new health.
func (m *Manager) RecordRun(agent, suite string) (api.RunResult, error) {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return api.RunResult{}, err
	}
	o.runs.Lock()
	defer o.runs.Unlock()
	if err := checkPrivate(o, o.featureMap.Map()); err != nil {
		return api.RunResult{}, err
	}
	priv, _ := o.featureMap.Private()
	if _, ok := priv.Tests.Suites[suite]; !ok {
		return api.RunResult{}, fmt.Errorf("%w: %s has no suite %q; its suites are %s", ErrNotFound, testmap.MapFile, suite, list(slices.Sorted(maps.Keys(priv.Tests.Suites))))
	}
	rel := path.Join(testmap.ReportsDir, suite+".xml")
	report := o.featureMap.PrivatePath(filepath.FromSlash(rel))
	where := path.Join(api.PrivateDir, rel)
	info, err := os.Stat(report)
	if err != nil {
		return api.RunResult{}, fmt.Errorf("J2: no report at %s; make the command of suite %q write a JUnit XML report there, then run it again (the hikari-partition skill's references/tests.md lists the flags of each runner)", where, suite)
	}
	if info.Size() > testmap.MaxReportBytes {
		return api.RunResult{}, fmt.Errorf("J6: the report %s has %d bytes; at most %d", where, info.Size(), testmap.MaxReportBytes)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		return api.RunResult{}, err
	}
	cases, err := testmap.ParseReport(data, api.WorkDir(o.scope), o.featureMap)
	if err != nil {
		return api.RunResult{}, fmt.Errorf("J2: %s: %w", where, err)
	}
	ids := o.wiki.Wiki().Features()
	before, _ := m.testReport(o, priv, ids)
	root := o.featureMap.Root()
	code := map[string]string{}
	for _, id := range ids {
		code[id], _ = featuremap.ContentHash(root, priv.Code(id))
	}
	run := testmap.NewRun(cases, m.now(), code, func(rel string) string { h, _ := featuremap.ContentHash(root, []string{rel}); return h })
	resultsPath := o.featureMap.PrivatePath(testmap.ResultsFile)
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
	matcher := testmap.NewMatcher(priv.Tests, o.featureMap)
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
	m.changed(Change{Partition: agent})
	return out, nil
}

// MarkCurrent commits the pending changes in agent's scope, then records the
// commit as the code that the map describes. It returns that commit.
func (m *Manager) MarkCurrent(agent string) (string, error) {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return "", err
	}
	if err := checkPrivate(o, o.featureMap.Map()); err != nil {
		return "", err
	}
	if _, err := o.repo.Commit(o.branch, o.scope, agent, agent+": changes described by the feature map"); err != nil {
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
	w := m.wikis[api.FeatureMapRef(agent)]
	m.commitUserEdits(fmt.Sprintf("user: wiki edits before %s's change", agent), w)
	if err := o.wiki.SetDescribed(head, now); err != nil {
		return "", err
	}
	if err := m.update(agent, func(_ *partitionAgent, mp *featuremap.Map) error { mp.MarkCurrent(head, now); return nil }); err != nil {
		return "", err
	}
	m.commitVault(agent, fmt.Sprintf("%s: mark the map current at %.7s", agent, head), m.mapPath(agent), w.dir)
	m.markSeen(agent, []*wikiOf{w})
	return head, nil
}

// Commit commits the changes in agent's scope. An empty message makes the
// automatic commit at the end of a turn (rule G13). It returns the new
// commit, or "" when the scope had no changes.
func (m *Manager) Commit(agent, message string) (string, error) {
	o, err := m.partitionAgent(agent)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		message = agent + ": changes left at the end of a turn"
	}
	commit, err := o.repo.Commit(o.branch, o.scope, agent, message)
	if err == nil && commit != "" {
		m.checkDirty(o)
		m.changed(Change{Partition: agent})
	}
	return commit, err
}

// PublishRelease publishes the staged product of a deliverable as a
// release (rules R16 and R17). The map must be current and the private
// documents valid.
func (m *Manager) PublishRelease(agent string, req api.PublishRequest) (vault.Manifest, error) {
	o, err := m.holds(agent, req.Deliverable)
	if err != nil {
		return vault.Manifest{}, err
	}
	v, problems := m.releaseProblems(o)
	if len(problems) > 0 {
		return vault.Manifest{}, errors.New(strings.Join(problems, "\n"))
	}
	public, err := yaml.Marshal(v.View)
	if err != nil {
		return vault.Manifest{}, err
	}
	rel, err := m.vault.Publish(vault.PublishInput{
		Partition: agent, Deliverable: req.Deliverable, Version: req.Version, Notes: req.Notes,
		Certificates: req.Certificates, SourceCommit: v.Partition.DescribedState.Commit, Map: public,
		Wiki: o.wiki.Files(releaseKinds...), Hikari: version.Version, Now: m.now(),
	})
	if err != nil {
		return vault.Manifest{}, err
	}
	m.changed(Change{Partition: agent})
	return rel, nil
}

// TurnContext tells the driver what changed before agent's next turn. It
// marks the vocabulary as seen.
func (m *Manager) TurnContext(agent string) api.TurnContext {
	var tc api.TurnContext
	if o, ok := m.partitionAgents[agent]; ok {
		tc.NoLocalResults = m.noLocalResults(o)
		v := m.partitionAgentView(o)
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

// visible returns the partitions that agent may read as the orchestration agent.
func (m *Manager) visible(agent string) ([]string, error) {
	if err := m.orchestration(agent, "reads the feature maps"); err != nil {
		return nil, err
	}
	return m.topo.VisiblePartitions(agent), nil
}

// orchestration returns an error unless agent is the orchestration agent, the only
// agent that does what.
func (m *Manager) orchestration(agent, what string) error {
	switch o := m.topo.Orchestration; {
	case o == "":
		return fmt.Errorf("%w: only the orchestration agent %s, and the project has none", ErrForbidden, what)
	case o != agent:
		return fmt.Errorf("%w: only %q, the orchestration agent, %s", ErrForbidden, o, what)
	}
	return nil
}

func (m *Manager) visiblePartition(agent, name string) (*partitionAgent, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(names, name) {
		return nil, fmt.Errorf("%w: %q is not a partition; the partitions are %s", ErrNotFound, name, list(names))
	}
	return m.partitionAgents[name], nil
}

// ListPartitions returns a summary of each partition visible to agent.
func (m *Manager) ListPartitions(agent string) ([]api.PartitionSummary, error) {
	names, err := m.visible(agent)
	if err != nil {
		return nil, err
	}
	out := []api.PartitionSummary{}
	for _, name := range names {
		v := m.view(m.partitionAgents[name])
		health := map[string]int{}
		for _, f := range v.Features {
			health[f.Health]++
		}
		out = append(out, api.PartitionSummary{Name: name, Summary: v.Partition.Summary, State: v.State, Features: health})
	}
	return out, nil
}

// GetPartition returns the orchestration agent's view of one partition visible to agent,
// with its interface flags and the providers of what it consumes.
func (m *Manager) GetPartition(agent, name string) (api.OrchestrationPartition, error) {
	o, err := m.visiblePartition(agent, name)
	if err != nil {
		return api.OrchestrationPartition{}, err
	}
	names, _ := m.visible(agent)
	known := m.vocabularyNames()
	providers := m.providers(names)
	v := m.view(o)
	releases, _ := m.vault.Releases(name)
	return api.OrchestrationPartition{
		View:      v,
		Releases:  releases,
		Flags:     vocab.Check(interfaceNames(v.Partition.Interfaces.Provides), interfaceNames(v.Partition.Interfaces.Consumes), known, keys(providers)),
		Providers: providersOf(v.Partition.Interfaces.Consumes, providers, name),
	}, nil
}

// changeVocab applies fn to the vocabulary and commits it with a message
// that says what changed: verb and name.
func (m *Manager) changeVocab(agent, verb, name string, fn func(*vocab.File) error) error {
	if err := m.orchestration(agent, "changes the vocabulary"); err != nil {
		return err
	}
	if err := fn(m.vocab); err != nil {
		return err
	}
	m.commitVault(agent, fmt.Sprintf("%s: %s %s in the vocabulary", agent, verb, name), m.vocabularyPath())
	m.changed(Change{Vocabulary: true})
	return nil
}

func (m *Manager) DefineTerm(agent string, t vocab.Term) error {
	return m.changeVocab(agent, "define", t.Term, func(s *vocab.File) error { return s.DefineTerm(t) })
}

func (m *Manager) DefineInterface(agent string, in vocab.Interface) error {
	return m.changeVocab(agent, "define", in.Name, func(s *vocab.File) error { return s.DefineInterface(in) })
}

func (m *Manager) RemoveEntry(agent, name string) error {
	return m.changeVocab(agent, "remove", name, func(s *vocab.File) error { return s.Remove(name) })
}

// Vocabulary returns the project's vocabulary, which every agent reads.
func (m *Manager) Vocabulary() vocab.Vocabulary {
	v, _ := m.vocab.Get()
	return v
}

// Partitions returns the full view of every partition, for the UI.
func (m *Manager) Partitions() []api.PartitionView {
	out := []api.PartitionView{}
	for _, name := range m.topo.PartitionAgents() {
		out = append(out, m.PartitionView(name))
	}
	return out
}

// PartitionView returns the full view of one partition with its repo details and
// its interface flags.
func (m *Manager) PartitionView(name string) api.PartitionView {
	o := m.partitionAgents[name]
	v := m.partitionAgentView(o)
	priv, _ := o.featureMap.Private()
	providers := m.providers(m.topo.PartitionAgents())
	releases, _ := m.vault.Releases(name)
	o.mu.Lock()
	dirty := o.dirty
	o.mu.Unlock()
	grants := m.topo.Agents[name].Grants
	return api.PartitionView{
		PartitionAgentView: v,
		Dirty:              dirty,
		Grants:             api.Grants{Releases: nonNil(grants.Releases), Deliverables: nonNil(grants.Deliverables)},
		Private:            priv,
		Releases:           releases,
		PartitionAgent:     name,
		Repo:               o.repoName,
		Scope:              o.scope,
		Branch:             o.branch,
		Flags:              vocab.Check(interfaceNames(v.Partition.Interfaces.Provides), interfaceNames(v.Partition.Interfaces.Consumes), m.vocabularyNames(), keys(providers)),
		Providers:          providersOf(v.Partition.Interfaces.Consumes, providers, name),
	}
}

func (m *Manager) vocabularyNames() map[string]bool {
	known := map[string]bool{}
	for _, name := range m.Vocabulary().Names() {
		known[name] = true
	}
	return known
}

// providers maps each interface name to the partitions among names that
// provide it.
func (m *Manager) providers(names []string) map[string][]string {
	out := map[string][]string{}
	for _, name := range names {
		for _, in := range m.partitionAgents[name].featureMap.Map().Partition.Interfaces.Provides {
			out[in.Name] = append(out[in.Name], name)
		}
	}
	return out
}

func providersOf(consumes []featuremap.Interface, providers map[string][]string, self string) map[string][]string {
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

func interfaceNames(ins []featuremap.Interface) []string {
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
