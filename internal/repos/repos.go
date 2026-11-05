// Package repos keeps one view of each repository of the topology: your
// branch against the default branch, every branch, and the pull requests of
// its forge with their checks (section 2 of the repositories, forges, and
// issues spec). It reads git on the host and the forge over the network,
// and it offers the actions that are yours: refresh, push, open a pull
// request, and merge.
package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/coverage"
	"github.com/nathanaday/hikari/internal/forge"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

// The intervals of Watch (rule RP4).
const (
	GitInterval   = 5 * time.Second
	ForgeInterval = time.Minute
	FetchInterval = 5 * time.Minute
	fetchTimeout  = 60 * time.Second
	forgeTimeout  = 45 * time.Second
)

// Options are what New takes beyond the topology and the repos.
type Options struct {
	// NewForge opens the forge of a remote; nil uses forge.New.
	NewForge func(forge.Remote) (forge.Forge, error)
	// PartitionsOf names the partitions with a writable folder in a repo.
	PartitionsOf func(repo string) []string
	// OnChange is told of each changed view.
	OnChange func(api.RepoView)
	Now      func() time.Time
}

type entry struct {
	repo  *gitrepo.Repo
	forge forge.Forge
	// forgeErr says why the repo has no usable forge, or "".
	forgeErr string
	view     api.RepoView
	// shown is the JSON of the view as last published.
	shown []byte
	// prs maps a head branch to its open pull request.
	prs map[string]int
	// coveredAt is the head whose uncovered paths the view holds, and
	// driftAt the heads of your branch and the default branch whose drift
	// it holds, so that git lists them again only when a branch moved.
	coveredAt string
	driftAt   string
}

// Manager holds the views.
type Manager struct {
	topo *topology.Topology
	opts Options
	mu   sync.Mutex
	// each serializes the refreshes of one repo.
	each    map[string]*sync.Mutex
	entries map[string]*entry
}

// New opens a manager for topo over the repos that partitions.Manager
// prepared, by name.
func New(topo *topology.Topology, repos map[string]*gitrepo.Repo, opts Options) *Manager {
	if opts.NewForge == nil {
		opts.NewForge = forge.New
	}
	if opts.PartitionsOf == nil {
		opts.PartitionsOf = func(string) []string { return nil }
	}
	if opts.OnChange == nil {
		opts.OnChange = func(api.RepoView) {}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	m := &Manager{topo: topo, opts: opts, each: map[string]*sync.Mutex{}, entries: map[string]*entry{}}
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		r := repos[name]
		if r == nil {
			r = gitrepo.Open(topo.Repos[name].Path)
		}
		e := &entry{repo: r, prs: map[string]int{}, view: api.RepoView{Name: name, Branch: topo.Repos[name].Branch, Project: name == topology.ProjectRepo, Branches: []api.BranchView{}, PullRequests: []api.PullRequestView{}}}
		if url := r.RemoteURL(gitrepo.Origin); url != "" {
			e.view.URL = url
			if remote, ok := forge.Parse(url); ok {
				e.view.Forge = &remote
				f, err := opts.NewForge(remote)
				if err != nil {
					e.forgeErr = err.Error()
					e.view.Error = e.forgeErr
				} else {
					e.forge = f
				}
			}
		}
		m.each[name] = &sync.Mutex{}
		m.entries[name] = e
		m.refreshGit(name)
	}
	return m
}

// Names lists the repositories, sorted.
func (m *Manager) Names() []string { return slices.Sorted(maps.Keys(m.entries)) }

// Views returns every view, by name.
func (m *Manager) Views() []api.RepoView {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []api.RepoView{}
	for _, name := range m.Names() {
		out = append(out, copyView(m.entries[name].view))
	}
	return out
}

// copyView returns a view whose slices are its own.
func copyView(v api.RepoView) api.RepoView {
	v.Branches = slices.Clone(v.Branches)
	v.PullRequests = slices.Clone(v.PullRequests)
	v.Partitions = slices.Clone(v.Partitions)
	v.Uncovered = slices.Clone(v.Uncovered)
	v.Drift = slices.Clone(v.Drift)
	return v
}

// View returns one view.
func (m *Manager) View(name string) (api.RepoView, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[name]
	if !ok {
		return api.RepoView{}, false
	}
	return copyView(e.view), true
}

// Forge returns the forge of a repository, when it has one that hikari can
// reach.
func (m *Manager) Forge(name string) (forge.Forge, bool) {
	e, ok := m.entries[name]
	if !ok || e.forge == nil {
		return nil, false
	}
	return e.forge, true
}

// Tracker returns the issue tracker of a repository, when its forge has
// one.
func (m *Manager) Tracker(name string) (forge.Tracker, bool) {
	f, ok := m.Forge(name)
	if !ok {
		return nil, false
	}
	return forge.TrackerOf(f)
}

// TrackerRepos lists the repositories whose forge has an issue tracker, the
// project repository first.
func (m *Manager) TrackerRepos() []string {
	var out []string
	for _, name := range m.Names() {
		if _, ok := m.Tracker(name); ok {
			out = append(out, name)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int {
		switch {
		case a == topology.ProjectRepo:
			return -1
		case b == topology.ProjectRepo:
			return 1
		}
		return 0
	})
	return out
}

// Watch refreshes the views until ctx ends: git every few seconds, the
// forge every minute, and a fetch of origin every few minutes (rule RP4).
func (m *Manager) Watch(ctx context.Context) {
	for _, name := range m.Names() {
		go m.refreshForge(ctx, name)
	}
	git := time.NewTicker(GitInterval)
	defer git.Stop()
	remote := time.NewTicker(ForgeInterval)
	defer remote.Stop()
	fetch := time.NewTicker(FetchInterval)
	defer fetch.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-git.C:
			for _, name := range m.Names() {
				m.refreshGit(name)
			}
		case <-remote.C:
			for _, name := range m.Names() {
				go m.refreshForge(ctx, name)
			}
		case <-fetch.C:
			for _, name := range m.Names() {
				go func() {
					m.fetch(ctx, name)
					m.refreshGit(name)
				}()
			}
		}
	}
}

// Refresh fetches origin, reads git, and reads the forge, now.
func (m *Manager) Refresh(ctx context.Context, name string) error {
	if _, ok := m.entries[name]; !ok {
		return fmt.Errorf("%w: no repository %q", ErrNotFound, name)
	}
	m.fetch(ctx, name)
	m.refreshGit(name)
	m.refreshForge(ctx, name)
	v, _ := m.View(name)
	if v.Error != "" {
		return errors.New(v.Error)
	}
	return nil
}

// ErrNotFound names a repository that the topology lacks.
var ErrNotFound = errors.New("not found")

// Push pushes your branch to origin, never forced (rule RP5).
func (m *Manager) Push(ctx context.Context, name string) error {
	e, ok := m.entries[name]
	if !ok {
		return fmt.Errorf("%w: no repository %q", ErrNotFound, name)
	}
	if !e.repo.HasRemote(gitrepo.Origin) {
		return fmt.Errorf("%s has no origin to push to", name)
	}
	if err := e.repo.Push(m.topo.Repos[name].Branch, gitrepo.Origin); err != nil {
		return err
	}
	m.refreshGit(name)
	go m.refreshForge(ctx, name)
	return nil
}

// OpenPullRequest opens a pull request from your branch to the default
// branch, or from head to base when they are set (rule RP5). Your branch
// must be on origin.
func (m *Manager) OpenPullRequest(ctx context.Context, name string, req api.NewPullRequestRequest) (api.PullRequestView, error) {
	e, ok := m.entries[name]
	if !ok {
		return api.PullRequestView{}, fmt.Errorf("%w: no repository %q", ErrNotFound, name)
	}
	if e.forge == nil {
		return api.PullRequestView{}, m.noForge(e)
	}
	v, _ := m.View(name)
	head, base := req.Head, req.Base
	if head == "" {
		head = v.Branch
	}
	if base == "" {
		base = v.Default
	}
	if base == "" {
		return api.PullRequestView{}, fmt.Errorf("%s has no default branch that hikari knows; name the base", name)
	}
	if strings.TrimSpace(req.Title) == "" {
		return api.PullRequestView{}, errors.New("a pull request needs a title")
	}
	for _, b := range v.Branches {
		if b.Name == head && b.Local {
			switch {
			case !b.Remote:
				return api.PullRequestView{}, fmt.Errorf("the branch %s is not on origin yet; push it first", head)
			case b.Unpushed > 0:
				return api.PullRequestView{}, fmt.Errorf("the branch %s has %d unpushed commit(s); push it first", head, b.Unpushed)
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, forgeTimeout)
	defer cancel()
	pr, err := e.forge.CreatePullRequest(ctx, forge.NewPullRequest{Title: strings.TrimSpace(req.Title), Body: req.Body, Head: head, Base: base, Draft: req.Draft})
	if err != nil {
		return api.PullRequestView{}, err
	}
	m.refreshForge(ctx, name)
	return api.PullRequestView{PullRequest: pr, Checks: forge.Summarize(nil)}, nil
}

// Merge merges a pull request (rule RP5).
func (m *Manager) Merge(ctx context.Context, name string, number int, method string) error {
	e, ok := m.entries[name]
	if !ok {
		return fmt.Errorf("%w: no repository %q", ErrNotFound, name)
	}
	if e.forge == nil {
		return m.noForge(e)
	}
	ctx, cancel := context.WithTimeout(ctx, forgeTimeout)
	defer cancel()
	if err := e.forge.Merge(ctx, number, method); err != nil {
		return err
	}
	m.refreshForge(ctx, name)
	return nil
}

func (m *Manager) noForge(e *entry) error {
	if e.forgeErr != "" {
		return errors.New(e.forgeErr)
	}
	if e.view.URL == "" {
		return fmt.Errorf("%s has no origin", e.view.Name)
	}
	return fmt.Errorf("%w: %s", forge.ErrUnknownForge, e.view.URL)
}

// fetch fetches origin when the repo has one, and records the error.
func (m *Manager) fetch(ctx context.Context, name string) {
	e := m.entries[name]
	if !e.repo.HasRemote(gitrepo.Origin) {
		return
	}
	err := e.repo.FetchQuiet(ctx, gitrepo.Origin, fetchTimeout)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		e.view.GitError = err.Error()
	} else {
		e.view.GitError, e.view.Fetched = "", m.opts.Now().UTC()
	}
}

// refreshGit reads the branches of a repo from git, and publishes the view
// when it changed.
func (m *Manager) refreshGit(name string) {
	lock := m.each[name]
	lock.Lock()
	defer lock.Unlock()
	e := m.entries[name]
	repo, yours := e.repo, m.topo.Repos[name].Branch
	hasOrigin := repo.HasRemote(gitrepo.Origin)
	local, _ := repo.Branches()
	var remote []gitrepo.Ref
	if hasOrigin {
		remote, _ = repo.RemoteBranches(gitrepo.Origin)
	}
	m.mu.Lock()
	def := e.view.Default
	m.mu.Unlock()
	if gitDefault := repo.DefaultBranch(gitrepo.Origin); def == "" || !hasOrigin {
		def = gitDefault
	}
	// The default branch as a ref: origin's when it has it, else local.
	defRef := ""
	if slices.ContainsFunc(remote, func(r gitrepo.Ref) bool { return r.Name == def }) {
		defRef = "refs/remotes/origin/" + def
	} else if slices.ContainsFunc(local, func(r gitrepo.Ref) bool { return r.Name == def }) {
		defRef = "refs/heads/" + def
	}
	byName := map[string]*api.BranchView{}
	order := []string{}
	add := func(ref gitrepo.Ref) *api.BranchView {
		b, ok := byName[ref.Name]
		if !ok {
			b = &api.BranchView{Name: ref.Name}
			byName[ref.Name] = b
			order = append(order, ref.Name)
		}
		return b
	}
	for _, ref := range local {
		b := add(ref)
		b.Local, b.SHA = true, ref.SHA
	}
	for _, ref := range remote {
		b := add(ref)
		b.Remote = true
		if !b.Local {
			b.SHA = ref.SHA
		}
	}
	m.mu.Lock()
	prs := maps.Clone(e.prs)
	m.mu.Unlock()
	for _, n := range order {
		b := byName[n]
		full := "refs/heads/" + n
		if !b.Local {
			full = "refs/remotes/origin/" + n
		}
		if c, err := repo.LastCommit(full); err == nil {
			b.SHA, b.Subject, b.Author, b.Time = c.SHA, c.Subject, c.Author, c.Time
		}
		b.Yours, b.Default = n == yours, n == def
		if defRef != "" && !b.Default {
			b.Ahead, b.Behind, _ = repo.AheadBehind(full, defRef)
			b.Merged = b.Ahead == 0 && repo.IsAncestor(full, defRef)
		}
		if b.Local && hasOrigin {
			if b.Remote {
				b.Unpushed, _, _ = repo.AheadBehind(full, "refs/remotes/origin/"+n)
			} else {
				b.Unpushed, _ = repo.Unpushed(n, gitrepo.Origin)
			}
		}
		b.PullRequest = prs[n]
	}
	branches := make([]api.BranchView, 0, len(order))
	// Yours first, then the default branch, then the rest by last commit.
	slices.SortStableFunc(order, func(a, b string) int {
		x, y := byName[a], byName[b]
		switch {
		case x.Yours != y.Yours:
			return boolCmp(y.Yours, x.Yours)
		case x.Default != y.Default:
			return boolCmp(y.Default, x.Default)
		}
		return y.Time.Compare(x.Time)
	})
	for _, n := range order {
		branches = append(branches, *byName[n])
	}
	head, _ := repo.Head()
	changed, err := repo.Changed()
	dirty := err == nil && len(changed) > 0
	uncovered, coveredAt := m.uncovered(e, name, head)
	drift, driftAt := m.drift(e, name, yours, defRef, byName)
	m.mu.Lock()
	e.view.Default, e.view.Branches, e.view.Head, e.view.Dirty = def, branches, head, dirty
	e.view.Uncovered, e.coveredAt = uncovered, coveredAt
	e.view.Drift, e.driftAt = drift, driftAt
	m.mu.Unlock()
	m.publish(name)
}

// uncovered lists the paths of your branch that no partition owns (the
// uncovered code of the topology), reading git only when the head moved.
// The project repository holds the topology and the vault, which no
// partition owns by design, so it is left out.
func (m *Manager) uncovered(e *entry, name, head string) ([]string, string) {
	if name == topology.ProjectRepo || head == "" {
		return nil, ""
	}
	m.mu.Lock()
	at, kept := e.coveredAt, e.view.Uncovered
	m.mu.Unlock()
	if at == head {
		return kept, at
	}
	out, err := coverage.Uncovered(e.repo, head, coverage.WritableFolders(m.topo, name))
	if err != nil {
		return nil, head
	}
	return out, head
}

// drift lists, for each partition with a folder in the repository, the
// paths in its folders that the default branch changed since your branch
// diverged from it (the default-branch drift of a partition). It reads
// git only when your branch or the default branch moved.
func (m *Manager) drift(e *entry, name, yours, defRef string, byName map[string]*api.BranchView) ([]api.PartitionDrift, string) {
	y, ok := byName[yours]
	if !ok || !y.Local || defRef == "" || y.Default {
		return nil, ""
	}
	// The default branch's head is the ref's, which can be origin's and
	// not the local branch's.
	d, err := e.repo.LastCommit(defRef)
	if err != nil {
		return nil, ""
	}
	at := y.SHA + ".." + d.SHA
	m.mu.Lock()
	kept, keptAt := e.view.Drift, e.driftAt
	m.mu.Unlock()
	if keptAt == at {
		return kept, at
	}
	var out []api.PartitionDrift
	for _, p := range m.topo.PartitionAgents() {
		var specs []string
		for _, f := range m.topo.Agents[p].Partition.Folders {
			if f.Repo == name {
				specs = append(specs, f.Path)
			}
		}
		if len(specs) == 0 {
			continue
		}
		paths, err := e.repo.ChangedBetween("refs/heads/"+yours, defRef, specs...)
		if err != nil || len(paths) == 0 {
			continue
		}
		out = append(out, api.PartitionDrift{Partition: p, Paths: paths})
	}
	return out, at
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// refreshForge reads the pull requests and their checks, and publishes the
// view when it changed.
func (m *Manager) refreshForge(ctx context.Context, name string) {
	e := m.entries[name]
	if e.forge == nil {
		return
	}
	lock := m.each[name]
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(ctx, forgeTimeout)
	defer cancel()
	fail := func(err error) {
		m.mu.Lock()
		e.view.Error = err.Error()
		m.mu.Unlock()
		m.publish(name)
	}
	def, err := e.forge.DefaultBranch(ctx)
	if err != nil {
		fail(err)
		return
	}
	prs, err := e.forge.PullRequests(ctx)
	if err != nil {
		fail(err)
		return
	}
	views := make([]api.PullRequestView, 0, len(prs))
	heads := map[string]int{}
	for _, pr := range prs {
		v := api.PullRequestView{PullRequest: pr, Checks: forge.Summarize(nil)}
		if pr.State == forge.Open {
			if c, err := e.forge.Checks(ctx, pr.SHA); err == nil {
				v.Checks = c
			}
			if _, taken := heads[pr.Head]; !taken {
				heads[pr.Head] = pr.Number
			}
		}
		views = append(views, v)
	}
	m.mu.Lock()
	e.prs = heads
	e.view.PullRequests, e.view.Error, e.view.Synced = views, "", m.opts.Now().UTC()
	if def != "" {
		e.view.Default = def
	}
	// The published views share nothing with the entry's slices.
	branches := slices.Clone(e.view.Branches)
	for i := range branches {
		branches[i].PullRequest = heads[branches[i].Name]
		branches[i].Default = branches[i].Name == e.view.Default
	}
	e.view.Branches = branches
	m.mu.Unlock()
	m.publish(name)
}

// publish fills the partitions and tells OnChange when the view changed.
func (m *Manager) publish(name string) {
	e := m.entries[name]
	m.mu.Lock()
	e.view.Partitions = m.opts.PartitionsOf(name)
	if e.view.Partitions == nil {
		e.view.Partitions = []string{}
	}
	data, err := json.Marshal(e.view)
	if err != nil || string(data) == string(e.shown) {
		m.mu.Unlock()
		return
	}
	e.shown = data
	v := copyView(e.view)
	m.mu.Unlock()
	m.opts.OnChange(v)
}

// Summary writes the views for an agent (rule RP7): each repository's
// branch, how far it is ahead, what is unpushed, and the open pull requests
// with their checks.
func Summary(views []api.RepoView) string {
	var b strings.Builder
	for _, v := range views {
		fmt.Fprintf(&b, "- %s: your branch is %s", v.Name, v.Branch)
		if v.Forge != nil {
			fmt.Fprintf(&b, " (%s on %s)", v.Forge.Slug(), v.Forge.Kind)
		}
		for _, br := range v.Branches {
			if !br.Yours {
				continue
			}
			if v.Default != "" {
				fmt.Fprintf(&b, "; %d ahead of %s, %d behind", br.Ahead, v.Default, br.Behind)
			}
			switch {
			case !br.Remote && v.URL != "":
				b.WriteString("; not pushed yet")
			case br.Unpushed > 0:
				fmt.Fprintf(&b, "; %d unpushed", br.Unpushed)
			case v.URL != "":
				b.WriteString("; pushed")
			}
			if br.Merged && v.Default != "" {
				b.WriteString("; merged into " + v.Default)
			}
		}
		if v.Dirty {
			b.WriteString("; uncommitted changes in the working tree")
		}
		if n := len(v.Uncovered); n > 0 {
			fmt.Fprintf(&b, "; %d %s that no partition owns (%s)", n, plural(n, "path"), listed(v.Uncovered, 5))
		}
		b.WriteString(".")
		for _, d := range v.Drift {
			fmt.Fprintf(&b, "\n  - %s: %d %s that %s changed since your branch diverged (%s)", d.Partition, len(d.Paths), plural(len(d.Paths), "path"), v.Default, listed(d.Paths, 5))
		}
		open := 0
		for _, pr := range v.PullRequests {
			if pr.State != forge.Open {
				continue
			}
			open++
			fmt.Fprintf(&b, "\n  - PR #%d %q: %s -> %s, checks %s", pr.Number, pr.Title, pr.Head, pr.Base, pr.Checks.State)
			if pr.Review != "" {
				b.WriteString(", review " + pr.Review)
			}
			if pr.Mergeable == "conflicting" {
				b.WriteString(", conflicts with " + pr.Base)
			}
			if pr.Draft {
				b.WriteString(", draft")
			}
		}
		if open == 0 && v.Forge != nil {
			b.WriteString("\n  - no open pull request")
		}
		if v.Error != "" {
			b.WriteString("\n  - the forge could not be read: " + v.Error)
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "The project has no repositories.\n"
	}
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// listed joins at most n paths, and counts the rest.
func listed(paths []string, n int) string {
	if len(paths) <= n {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(paths[:n], ", "), len(paths)-n)
}
