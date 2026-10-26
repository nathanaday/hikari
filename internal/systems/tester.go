package systems

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/e2e"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/wiki"
)

// testedKinds are the kinds of a hub's documents that a tester's tests
// link (rule TT1 of the tester spec).
var testedKinds = []string{wiki.Story, wiki.Rule, wiki.Principle}

// storyIDs returns the stories and rules of the hub that o tests, or nil
// when o is no tester.
func (m *Manager) storyIDs(o *owner) []string {
	hub := m.topo.Agents[o.name].Tests
	if hub == "" {
		return nil
	}
	var out []string
	docs := m.wikis[api.HubWikiRef(hub)].store.Wiki()
	for _, id := range docs.IDs() {
		if slices.Contains(testedKinds, docs[id].KindOf()) {
			out = append(out, id)
		}
	}
	return out
}

// lockHash is the hash of o's releases.lock, which is the code of every
// story and rule its tests link (rule TT3).
func lockHash(o *owner) string {
	h, _ := sysmap.ContentHash(o.store.Root(), []string{e2e.LockFile})
	return h
}

// codeHash returns the function that gives the hash of the code of an id
// that o's tests link: a feature's code, or for a tester's story, its lock.
func (m *Manager) codeHash(o *owner, priv sysmap.Private) func(id string) string {
	stories := m.storyIDs(o)
	root := o.store.Root()
	return func(id string) string {
		if slices.Contains(stories, id) {
			return lockHash(o)
		}
		h, _ := sysmap.ContentHash(root, priv.Code(id))
		return h
	}
}

// testerDocs reads the plan and the lock of a tester, and returns the
// problems that block its runs (rules PL6 and LK2).
func (m *Manager) testerDocs(o *owner) (e2e.Plan, string, e2e.Lock, []string) {
	var problems []string
	var plan e2e.Plan
	var body string
	if data, err := os.ReadFile(filepath.Join(o.store.Root(), e2e.PlanFile)); err == nil {
		if plan, body, err = e2e.ParsePlan(string(data)); err != nil {
			problems = append(problems, err.Error())
		}
	}
	lock, err := e2e.LoadLock(filepath.Join(o.store.Root(), e2e.LockFile))
	if err != nil {
		problems = append(problems, err.Error())
	}
	// The lock names each system by id (rule ID8 of the ids spec).
	for i, r := range lock.Releases {
		lock.Releases[i].System = m.store.SystemName(r.System)
	}
	for _, r := range lock.Releases {
		man, ok := m.release(r.System, r.Deliverable, r.Version)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("LK2: %s pins %s/%s %s, which is not a release; pin again", e2e.LockFile, r.System, r.Deliverable, r.Version))
		case man.Product.Digest != r.Digest:
			problems = append(problems, fmt.Sprintf("LK2: %s pins %s/%s %s with the digest %s, but the release has %s; pin again", e2e.LockFile, r.System, r.Deliverable, r.Version, r.Digest, man.Product.Digest))
		}
	}
	return plan, body, lock, problems
}

// release returns the manifest of one release.
func (m *Manager) release(system, deliverable, version string) (store.Manifest, bool) {
	releases, _ := m.store.Releases(system)
	for _, r := range releases {
		if r.Deliverable == deliverable && r.Version == version {
			return r, true
		}
	}
	return store.Manifest{}, false
}

// testerProblems returns the problems of a tester's plan and lock, for
// the private errors, or nil for an owner that is no tester.
func (m *Manager) testerProblems(o *owner) []string {
	if m.topo.Agents[o.name].Tests == "" {
		return nil
	}
	_, _, _, problems := m.testerDocs(o)
	return problems
}

// PinReleases writes the tester's lock from the releases it names (rule
// LK1). It returns the lock.
func (m *Manager) PinReleases(agent string, pins []e2e.LockedRelease) (e2e.Lock, error) {
	o, err := m.owner(agent)
	if err != nil {
		return e2e.Lock{}, err
	}
	hub := m.topo.Agents[agent].Tests
	if hub == "" {
		return e2e.Lock{}, fmt.Errorf("%w: only a tester pins releases", ErrForbidden)
	}
	readable := m.topo.ReadableReleases(agent)
	lock := e2e.Lock{Version: 1}
	var errs []string
	seen := map[string]bool{}
	for _, p := range pins {
		key := p.System + "/" + p.Deliverable
		switch {
		case seen[key]:
			errs = append(errs, fmt.Sprintf("%s is pinned twice", key))
			continue
		case !slices.Contains(readable, p.System):
			errs = append(errs, fmt.Sprintf("you cannot read the releases of %q; you read %s", p.System, list(readable)))
			continue
		}
		seen[key] = true
		man, ok := m.release(p.System, p.Deliverable, p.Version)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s has no release %s; see %s", key, p.Version, api.ReleasesPath(p.System)))
			continue
		}
		lock.Releases = append(lock.Releases, e2e.LockedRelease{System: p.System, Deliverable: p.Deliverable, Version: p.Version, Digest: man.Product.Digest})
	}
	if len(errs) > 0 {
		return e2e.Lock{}, fmt.Errorf("LK1: %s", strings.Join(errs, "; "))
	}
	disk := e2e.Lock{Version: lock.Version}
	for _, r := range lock.Releases {
		r.System = m.store.SystemID(r.System)
		disk.Releases = append(disk.Releases, r)
	}
	if err := os.WriteFile(filepath.Join(o.store.Root(), e2e.LockFile), disk.Render(), 0o644); err != nil {
		return e2e.Lock{}, err
	}
	m.changed(Change{System: agent, Wiki: api.HubWikiRef(hub)})
	return e2e.ParseLock(lock.Render())
}

// newerReleases lists the pinned releases of a tester that have a newer
// version (rule LK3).
func (m *Manager) newerReleases(o *owner) []string {
	_, _, lock, _ := m.testerDocs(o)
	var out []string
	for _, r := range lock.Releases {
		versions, _ := m.store.Versions(r.System, r.Deliverable)
		latest := r.Version
		for _, v := range versions {
			if store.CompareVersions(v, latest) > 0 {
				latest = v
			}
		}
		if latest != r.Version {
			out = append(out, fmt.Sprintf("%s/%s %s is newer than your pinned %s", r.System, r.Deliverable, latest, r.Version))
		}
	}
	return out
}

// testPlan returns the plan of hub with its findings, the lock, the gaps
// with their approvals, and the E2E health of each story and rule (rule
// PL7), or nil when the hub has no tester.
func (m *Manager) testPlan(hub string, health map[string]string) *api.TestPlan {
	tester := m.topo.Tester(hub)
	if tester == "" {
		return nil
	}
	o := m.owners[tester]
	plan, body, lock, problems := m.testerDocs(o)
	approvals, err := e2e.LoadApprovals(m.approvalsPath(hub))
	if err != nil {
		problems = append(problems, err.Error())
	}
	priv, _ := o.store.Private()
	stories := m.storyIDs(o)
	rep, _ := m.testReport(o, priv, append(o.wiki.Wiki().Features(), stories...))
	docs := m.wikis[api.HubWikiRef(hub)].store.Wiki()
	covered := map[string]bool{}
	var key []string
	out := &api.TestPlan{Hub: hub, Tester: tester, Plan: plan, Body: body, Lock: lock, Problems: problems, Stories: []api.StoryHealth{}, Gaps: []api.GapView{}}
	bench, err := e2e.LoadBenchResults(m.benchPath(hub))
	if err != nil {
		problems = append(problems, err.Error())
		out.Problems = problems
	}
	lh := lockHash(o)
	for _, id := range slices.Sorted(maps.Keys(plan.Bench)) {
		proc := plan.Bench[id]
		bv := api.BenchView{ID: id, Procedure: proc}
		if r, ok := bench[id]; ok {
			bv.Result = &api.BenchResultView{BenchResult: r, Current: r.Current(proc, lh)}
		}
		out.Bench = append(out.Bench, bv)
	}
	for _, id := range stories {
		d := docs[id]
		fh := rep.Features[id]
		if fh.Passed+fh.Failed+fh.Pending > 0 {
			covered[id] = true
		}
		if d.Key {
			key = append(key, id)
		}
		fh.Health = joinBench(fh.Health, plan.BenchTouching(id), out.Bench)
		sh := api.StoryHealth{ID: id, Kind: d.KindOf(), Name: d.Name, Key: d.Key, Health: fh.Health, Support: support(d, health), Gaps: plan.GapsTouching(id), Bench: plan.BenchTouching(id)}
		sh.Passed, sh.Failed, sh.Pending = fh.Passed, fh.Failed, fh.Pending
		if fh.Health == "failing" {
			sh.Suspects = m.suspects(d)
		}
		out.Stories = append(out.Stories, sh)
	}
	for _, id := range slices.Sorted(func(yield func(string) bool) {
		for k := range plan.Gaps {
			if !yield(k) {
				return
			}
		}
	}) {
		out.Gaps = append(out.Gaps, api.GapView{ID: id, Gap: plan.Gaps[id], Approved: approvals.Approved(id, plan.Gaps[id])})
	}
	touchable := map[string]bool{}
	for _, id := range stories {
		touchable[id] = true
	}
	for _, sys := range m.topo.TestedSystems(tester) {
		for _, id := range m.wikis[api.SystemWikiRef(sys)].store.Wiki().Features() {
			touchable[id] = true
		}
	}
	storySet := map[string]bool{}
	for _, id := range stories {
		storySet[id] = true
	}
	out.Warnings = append(plan.Warnings(e2e.Context{Lock: lock, Stories: storySet, Touchable: touchable, Key: key, Covered: covered}), m.environmentFindings(o, plan, lock)...)
	env := m.hubEnvironment(context.Background(), hub)
	out.Environment = &env
	return out
}

// joinBench joins a story's E2E health with the results of the procedures
// that touch it (rule BH1).
func joinBench(health string, ids []string, bench []api.BenchView) string {
	seen := map[string]bool{health: health != "untested"}
	for _, b := range bench {
		if !slices.Contains(ids, b.ID) || b.Result == nil {
			continue
		}
		switch {
		case !b.Result.Current:
			seen["pending"] = true
		case b.Result.Passed:
			seen["passing"] = true
		default:
			seen["failing"] = true
		}
	}
	for _, h := range []string{"failing", "pending", "passing"} {
		if seen[h] {
			return h
		}
	}
	return "untested"
}

func (m *Manager) benchPath(hub string) string {
	return filepath.Join(m.store.HubDir(hub), "bench.yaml")
}

// RecordBench records your result of a bench procedure of hub's plan
// (rules BR1 and BR3).
func (m *Manager) RecordBench(hub, id string, passed bool, note string) error {
	tester := m.topo.Tester(hub)
	if tester == "" {
		return fmt.Errorf("%w: #%s has no tester", ErrNotFound, hub)
	}
	o := m.owners[tester]
	plan, _, _, _ := m.testerDocs(o)
	proc, ok := plan.Bench[id]
	if !ok {
		return fmt.Errorf("%w: the plan of #%s has no bench procedure %q", ErrNotFound, hub, id)
	}
	results, err := e2e.LoadBenchResults(m.benchPath(hub))
	if err != nil {
		return err
	}
	results[id] = e2e.BenchResult{Passed: passed, Note: strings.TrimSpace(note), At: m.now().UTC().Truncate(time.Second), Lock: lockHash(o), Procedure: proc.Digest()}
	if err := results.Save(m.benchPath(hub)); err != nil {
		return err
	}
	outcome := "fail"
	if passed {
		outcome = "pass"
	}
	m.commitStore("", fmt.Sprintf("user: record %s as %s in #%s", id, outcome, hub), m.store.Rel(m.benchPath(hub)))
	m.changed(Change{Wiki: api.HubWikiRef(hub)})
	return nil
}

// suspects returns the systems whose features a story links (rule EH3).
func (m *Manager) suspects(d *wiki.Document) []string {
	var out []string
	for _, id := range d.Neighbors() {
		for _, name := range m.topo.Owners() {
			if f := m.wikis[api.SystemWikiRef(name)].store.Wiki()[id]; f != nil && f.KindOf() == wiki.Feature && !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

func (m *Manager) approvalsPath(hub string) string {
	return filepath.Join(m.store.HubDir(hub), "approvals.yaml")
}

// GetTestPlan returns the plan of hub for its tester or its orchestrator
// (rule PL7).
func (m *Manager) GetTestPlan(agent, hub string) (api.TestPlan, error) {
	h, ok := m.topo.Hubs[hub]
	if !ok {
		return api.TestPlan{}, fmt.Errorf("%w: no hub %q", ErrNotFound, hub)
	}
	if h.Orchestrator != agent && m.topo.Agents[agent].Tests != hub {
		return api.TestPlan{}, fmt.Errorf("%w: only the tester and the orchestrator of #%s read its test plan", ErrForbidden, hub)
	}
	p := m.testPlan(hub, m.featureHealth())
	if p == nil {
		return api.TestPlan{}, fmt.Errorf("%w: #%s has no tester", ErrNotFound, hub)
	}
	return *p, nil
}

// SetApproval approves or withdraws the approval of a gap of hub's plan,
// for the user (rules GA1 to GA3).
func (m *Manager) SetApproval(hub, gap string, approve bool) error {
	p := m.testPlan(hub, nil)
	if p == nil {
		return fmt.Errorf("%w: #%s has no tester", ErrNotFound, hub)
	}
	g, ok := p.Plan.Gaps[gap]
	if !ok {
		return fmt.Errorf("%w: the plan of #%s has no gap %q", ErrNotFound, hub, gap)
	}
	approvals, err := e2e.LoadApprovals(m.approvalsPath(hub))
	if err != nil {
		return err
	}
	verb := "approve"
	if approve {
		approvals[gap] = g.Digest()
	} else {
		delete(approvals, gap)
		verb = "withdraw"
	}
	if err := approvals.Save(m.approvalsPath(hub)); err != nil {
		return err
	}
	m.commitStore("", fmt.Sprintf("user: %s %s in #%s", verb, gap, hub), m.store.Rel(m.approvalsPath(hub)))
	m.changed(Change{Wiki: api.HubWikiRef(hub)})
	return nil
}
