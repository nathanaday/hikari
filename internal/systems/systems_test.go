package systems

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featurewiki"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
)

// The tree of the tests: lead orchestrates #product, whose members are
// eng-lead and writer. eng-lead orchestrates #eng, whose members are the
// owners orders and inventory.
const testTopology = `
name: test
x-hikari:
  repos:
    shop: {path: shop, branch: hikari/work}
  hubs:
    product: {orchestrator: lead, members: [eng-lead, writer]}
    eng: {orchestrator: eng-lead, members: [orders, inventory]}
services:
  lead: {x-hikari: {}}
  writer: {x-hikari: {}}
  eng-lead: {x-hikari: {}}
  orders: {x-hikari: {system: {repo: shop, scope: services/orders, test: python3 -m unittest}, grants: {deliverables: [python-library]}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
`

const pythonLibrary = `version: 1
description: A Python library that other systems install from the release.
layout:
  require: [README.md, "**/*.py"]
  forbid: ["**/__pycache__/**"]
`

// defineDeliverable writes a deliverable into the store under dir.
func defineDeliverable(t *testing.T, dir, name, yaml string) {
	t.Helper()
	d := filepath.Join(dir, "store", "shared", "deliverables", name)
	write(t, d, "deliverable.yaml", yaml)
	write(t, d, "SKILL.md", "# Make the library\n")
}

type fixture struct {
	dir  string
	repo string
	m    *Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	for name, content := range map[string]string{
		"services/orders/create.py":            "def create(): pass\n",
		"services/orders/tests/test_create.py": "def test(): pass\n",
		"services/inventory/stock.py":          "def reserve(): pass\n",
	} {
		write(t, repo, name, content)
	}
	git(t, repo, "init", "--quiet", "--initial-branch=main")
	git(t, repo, "add", "--all")
	git(t, repo, "commit", "--quiet", "-m", "initial")
	defineDeliverable(t, dir, "python-library", pythonLibrary)

	topo, err := topology.Parse([]byte(testTopology), dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{dir: dir, repo: repo, m: m}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func ptr[T any](v T) *T { return &v }

// describeOrders gives orders a small valid map.
func (f *fixture) describeOrders(t *testing.T) {
	t.Helper()
	must(t, f.m.SetSystemInfo("orders", sysmap.SystemPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]sysmap.Interface{{Name: "POST /orders", Kind: "http", Description: "Creates an order."}}),
		Consumes: ptr([]sysmap.Interface{{Name: "inventory.reserve", Kind: "function"}}),
	}))
	_, err := f.m.ChangeFeatures("orders", []featurewiki.Write{{
		Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.",
		Interfaces: []string{"POST /orders", "inventory.reserve"}, Body: body("A customer places an order, and the shop stores it."),
	}})
	must(t, err)
	write(t, f.repo, "services/orders/.hikari/private/layout.yaml", `version: 1
parts: [{path: create.py, description: Order creation.}]
features:
  create-order: {code: [create.py]}
`)
	write(t, f.repo, "services/orders/.hikari/private/tests.yaml", `version: 1
suites:
  unit: {command: python3 -m pytest}
tests:
  tests/test_create.py: ["[[create-order]]"]
`)
}

// runOrders writes a JUnit report of orders' suite unit, with one test in
// tests/test_create.py for each outcome, and records it.
func (f *fixture) runOrders(t *testing.T, outcomes ...string) api.RunResult {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<testsuites><testsuite name="pytest">`)
	for i, o := range outcomes {
		fmt.Fprintf(&b, `<testcase classname="tests.test_create.CreateTest" name="test_%d" file="tests/test_create.py">`, i)
		switch o {
		case testmap.Failed:
			b.WriteString(`<failure message="AssertionError: no order stored"/>`)
		case testmap.Skipped:
			b.WriteString(`<skipped/>`)
		}
		b.WriteString(`</testcase>`)
	}
	b.WriteString(`</testsuite></testsuites>`)
	write(t, f.repo, "services/orders/.hikari/private/reports/unit.xml", b.String())
	res, err := f.m.RecordRun("orders", "unit")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// body makes a document body with the given Purpose.
func body(purpose string) *string {
	s := "## Purpose\n\n" + purpose + "\n"
	return &s
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewPreparesReposAndMaps(t *testing.T) {
	f := newFixture(t)
	if got := git(t, f.repo, "branch", "--show-current"); got != "hikari/work" {
		t.Errorf("G2: checked out %q", got)
	}
	st := filepath.Join(f.dir, "store")
	for _, p := range []string{
		"README.md", "shared/vocabulary", "shared/policies", "shared/skills", "shared/agents",
		"systems/orders/map.yaml", "systems/orders/staging", "systems/orders/releases",
		"systems/inventory/map.yaml",
	} {
		if _, err := os.Stat(filepath.Join(st, p)); err != nil {
			t.Errorf("L1/L3: store lacks %s: %v", p, err)
		}
	}
	for _, scope := range []string{"services/orders", "services/inventory"} {
		if _, err := os.Stat(filepath.Join(f.repo, scope, ".hikari", "private", "tests.yaml")); err != nil {
			t.Errorf("L5: no private documents in %s: %v", scope, err)
		}
		if _, err := os.Stat(filepath.Join(f.repo, scope, ".hikari", "map.yaml")); err == nil {
			t.Errorf("L3: the map must not be written into the repo in %s", scope)
		}
	}
	if len(f.m.Warnings()) != 0 {
		t.Errorf("no warnings expected: %v", f.m.Warnings())
	}
	v, err := f.m.OwnView("orders")
	if err != nil || v.State != sysmap.Unbuilt || len(v.PrivateErrors) != 0 {
		t.Errorf("new map: %+v, %v", v, err)
	}
	if p := f.m.SystemView("orders").Private; p.Tests.Suites["all"].Command != "python3 -m unittest" {
		t.Errorf("the test map must start with the topology's test command: %+v", p.Tests)
	}
}

func TestNewWarnsWhenTheMapIsIgnored(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	write(t, repo, "services/orders/a.py", "x")
	write(t, repo, "services/inventory/b.py", "x")
	write(t, repo, ".gitignore", ".hikari/\n")
	git(t, repo, "init", "--quiet", "--initial-branch=main")
	git(t, repo, "add", "--all")
	git(t, repo, "-c", "user.name=T", "-c", "user.email=t@e", "commit", "--quiet", "-m", "initial")
	defineDeliverable(t, dir, "python-library", pythonLibrary)
	topo, err := topology.Parse([]byte(testTopology), dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	if w := m.Warnings(); len(w) != 2 || !strings.Contains(w[0], "are not versioned with its code") {
		t.Errorf("G5 warnings = %v", w)
	}
}

func TestNewRefusesUncommittedChanges(t *testing.T) {
	f := newFixture(t)
	git(t, f.repo, "checkout", "--quiet", "main")
	// The maps that New wrote are uncommitted changes now.
	topo, _ := topology.Parse([]byte(testTopology), f.dir)
	if _, err := New(topo); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("G1: New = %v", err)
	}
}

func TestPrivateDocumentsGateResultsAndMarking(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	_, err := f.m.ChangeFeatures("orders", []featurewiki.Write{{Op: "create", ID: "untested-thing", Name: "Untested thing", Description: "Nothing tests it.", Body: body("It exists.")}})
	must(t, err)
	if _, err := f.m.RecordRun("orders", "e2e"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "its suites are unit") {
		t.Errorf("J2: a suite that the test map lacks: %v", err)
	}
	if _, err := f.m.RecordRun("orders", "unit"); err == nil || !strings.Contains(err.Error(), "no report at /hikari/private/reports/unit.xml") {
		t.Errorf("J2: no report: %v", err)
	}
	write(t, f.repo, "services/orders/.hikari/private/reports/unit.xml", "<testsuites/>")
	if _, err := f.m.RecordRun("orders", "unit"); err == nil || !strings.Contains(err.Error(), "no test case") {
		t.Errorf("J2: an empty report: %v", err)
	}
	write(t, f.repo, "services/orders/.hikari/private/layout.yaml", "version: 1\nparts: []\nfeatures:\n  ghost: {code: [create.py]}\n")
	if _, err := f.m.MarkCurrent("orders"); err == nil || !strings.Contains(err.Error(), "P2") {
		t.Errorf("mark current with invalid documents: %v", err)
	}
	if _, err := f.m.RecordRun("orders", "unit"); err == nil || !strings.Contains(err.Error(), "P2") {
		t.Errorf("record with invalid documents: %v", err)
	}
	if v, _ := f.m.OwnView("orders"); len(v.PrivateErrors) != 1 {
		t.Errorf("the owner view must list the problem: %v", v.PrivateErrors)
	}
	if tc := f.m.TurnContext("orders"); len(tc.PrivateErrors) != 1 {
		t.Errorf("the turn context must list the problem: %+v", tc)
	}
	if got := git(t, f.repo, "log", "-1", "--format=%s"); got != "initial" {
		t.Errorf("a refused mark_map_current must not commit; HEAD is %q", got)
	}
}

func TestOwnerWorkflow(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	run := f.runOrders(t, testmap.Passed, testmap.Passed, testmap.Passed)
	if run.Passed != 3 || run.Unmapped != 0 || len(run.Changed) != 1 || run.Changed[0] != (api.HealthChange{Feature: "create-order", From: testmap.Pending, To: testmap.Passing}) {
		t.Errorf("J1: %+v", run)
	}
	if _, err := os.Stat(filepath.Join(f.repo, "services/orders/.hikari/private/reports/unit.xml")); err == nil {
		t.Error("J1: the report is deleted once read")
	}

	write(t, f.repo, "services/orders/cancel.py", "def cancel(): pass\n")
	commit, err := f.m.MarkCurrent("orders")
	if err != nil {
		t.Fatal(err)
	}
	if got := git(t, f.repo, "log", "-1", "--format=%an|%s"); got != "orders (hikari)|orders: changes described by the system map" {
		t.Errorf("MarkCurrent commit: %q", got)
	}
	v, _ := f.m.OwnView("orders")
	if v.State != sysmap.Current || v.System.DescribedState.Commit != commit || v.Features[0].Health != testmap.Passing || v.Features[0].Tests.Passed != 3 {
		t.Errorf("after MarkCurrent: state %s, commit %s, health %s", v.State, v.System.DescribedState.Commit, v.Features[0].Health)
	}

	// The map change itself is not a code change.
	if tc := f.m.TurnContext("orders"); tc.MapState != sysmap.Current || len(tc.StaleFeatures) != 0 || len(tc.UnmappedPaths) != 0 || len(tc.UndescribedFeatures) != 0 {
		t.Errorf("turn context after MarkCurrent: %+v", tc)
	}
	if v.Features[0].State != sysmap.Current {
		t.Errorf("S20: state after MarkCurrent: %s", v.Features[0].State)
	}

	// A host edit makes the map stale and the feature stale, and an edit
	// outside every feature is unmapped.
	write(t, f.repo, "services/orders/create.py", "def create(): return 1\n")
	write(t, f.repo, "services/orders/README.md", "# orders\n")
	tc := f.m.TurnContext("orders")
	if tc.MapState != sysmap.Stale || len(tc.StaleFeatures) != 1 || tc.StaleFeatures[0].ID != "create-order" || !slices.Equal(tc.StaleFeatures[0].Paths, []string{"create.py"}) {
		t.Errorf("S20: turn context after a host edit: %+v", tc)
	}
	if !slices.Equal(tc.UnmappedPaths, []string{"README.md"}) {
		t.Errorf("S21: unmapped paths: %v", tc.UnmappedPaths)
	}
	if v, _ := f.m.OwnView("orders"); v.Features[0].Health != testmap.Pending || v.Features[0].State != sysmap.Stale || !slices.Equal(v.PendingFeatures, []string{"create-order"}) {
		t.Errorf("S26, S20 after an edit: health %s, state %s, pending %v", v.Features[0].Health, v.Features[0].State, v.PendingFeatures)
	}
	if tc.PendingFeatures == nil || tc.PendingFeatures[0] != "create-order" {
		t.Errorf("P20: pending features in the turn context: %+v", tc)
	}

	// The automatic commit takes the edit and the map file.
	commit, err = f.m.Commit("orders", "")
	if err != nil || commit == "" {
		t.Fatalf("automatic commit = %q, %v", commit, err)
	}
	if got := git(t, f.repo, "log", "-1", "--format=%s"); got != "orders: changes left at the end of a turn" {
		t.Errorf("G13 message: %q", got)
	}
	files := git(t, f.repo, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "services/orders/create.py") || strings.Contains(files, "map.yaml") || strings.Contains(files, "inventory") {
		t.Errorf("G7 automatic commit files:\n%s", files)
	}
	if commit, err := f.m.Commit("orders", "nothing to do"); err != nil || commit != "" {
		t.Errorf("G12 commit without changes = %q, %v", commit, err)
	}
	if got := f.m.Summary(); !slices.Contains(got, `orders made 2 commit(s) on hikari/work in repo "shop"`) {
		t.Errorf("G11 summary = %v", got)
	}
}

func TestAccess(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	forbidden := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: error = %v, want ErrForbidden", name, err)
		}
	}
	notFound := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}

	// A1: an owner changes only its own map; non-owners have no map.
	_, err := f.m.OwnView("eng-lead")
	forbidden("A1 orchestrator has no own map", err)
	forbidden("A1 writer cannot commit", func() error { _, err := f.m.Commit("writer", "x"); return err }())

	// A2: eng-lead sees its direct members' systems; lead sees none (D2).
	if s, err := f.m.ListSystems("eng-lead"); err != nil || len(s) != 2 {
		t.Errorf("A2 eng-lead systems = %+v, %v", s, err)
	}
	if s, err := f.m.ListSystems("lead"); err != nil || len(s) != 0 {
		t.Errorf("A2 lead systems = %+v, %v", s, err)
	}
	_, err = f.m.GetSystem("lead", "orders")
	notFound("A2 lead reads orders", err)

	// A3: no other agent reads any map.
	_, err = f.m.ListSystems("orders")
	forbidden("A3 owner lists systems", err)
	_, err = f.m.GetSystem("inventory", "orders")
	forbidden("A3 owner reads a peer", err)
	_, err = f.m.SearchFeatures("writer", "order")
	forbidden("A3 member searches", err)

	// V3: vocabularies.
	forbidden("V3 member defines a term", f.m.DefineTerm("orders", "eng", vocab.Term{Term: "x", Definition: "y"}))
	forbidden("V3 orchestrator of another hub", f.m.DefineTerm("lead", "eng", vocab.Term{Term: "x", Definition: "y"}))
	must(t, f.m.DefineTerm("eng-lead", "eng", vocab.Term{Term: "order", Definition: "A request to buy."}))
	if v, err := f.m.GetVocabulary("orders", "eng"); err != nil || len(v.Terms) != 1 {
		t.Errorf("V3 member reads = %+v, %v", v, err)
	}
	_, err = f.m.GetVocabulary("orders", "product")
	forbidden("V3 non-participant reads", err)
	if _, err := f.m.GetVocabulary("user", "eng"); err != nil {
		t.Errorf("the user reads every vocabulary: %v", err)
	}
}

func TestOrchestratorView(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	must(t, f.m.SetSystemInfo("inventory", sysmap.SystemPatch{
		Provides: ptr([]sysmap.Interface{{Name: "inventory.reserve", Kind: "function", Description: "Reserves stock."}}),
	}))
	must(t, f.m.DefineInterface("eng-lead", "eng", vocab.Interface{Name: "inventory.reserve", Kind: "function", Description: "Reserves stock."}))

	s, err := f.m.GetSystem("eng-lead", "orders")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(s)
	for _, leak := range []string{"create.py", "tests/", "python3", "private"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("the orchestrator view contains %q:\n%s", leak, data)
		}
	}
	if !slices.Equal(s.Providers["inventory.reserve"], []string{"inventory"}) {
		t.Errorf("providers = %v", s.Providers)
	}
	if len(s.Flags) != 1 || s.Flags[0] != (vocab.Flag{Interface: "POST /orders", Problem: vocab.NotInVocabulary}) {
		t.Errorf("V6/V7 flags = %+v", s.Flags)
	}
	if len(s.Features) != 1 || s.Features[0].ID != "create-order" {
		t.Errorf("features = %+v", s.Features)
	}

	hits, err := f.m.SearchFeatures("eng-lead", "STORES")
	if err != nil || len(hits) != 1 || hits[0].System != "orders" {
		t.Errorf("search = %+v, %v", hits, err)
	}
	if hit, err := f.m.GetFeature("eng-lead", "orders", "create-order"); err != nil || hit.Feature.Name != "Create an order" || !strings.Contains(hit.Document, "## Purpose") {
		t.Errorf("Q3: get feature = %+v, %v", hit, err)
	}
	if hits, err := f.m.SearchFeatures("eng-lead", "customer places"); err != nil || len(hits) != 1 {
		t.Errorf("Q3: search matches the document: %+v, %v", hits, err)
	}
	if _, err := f.m.GetFeature("eng-lead", "orders", "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown feature: %v", err)
	}

	// The UI view flags the same way from the owner's hubs.
	uv := f.m.SystemView("orders")
	if uv.Owner != "orders" || uv.Scope != "services/orders" || uv.Branch != "hikari/work" || len(uv.Flags) != 1 {
		t.Errorf("UI view: %+v", uv)
	}
}

func TestTurnContextReportsVocabularyChangesOnce(t *testing.T) {
	f := newFixture(t)
	if tc := f.m.TurnContext("orders"); len(tc.VocabularyChanged) != 0 {
		t.Errorf("first turn: %+v", tc)
	}
	must(t, f.m.DefineTerm("eng-lead", "eng", vocab.Term{Term: "order", Definition: "A request to buy."}))
	if tc := f.m.TurnContext("orders"); !slices.Equal(tc.VocabularyChanged, []string{"eng"}) {
		t.Errorf("after a change: %+v", tc)
	}
	if tc := f.m.TurnContext("orders"); len(tc.VocabularyChanged) != 0 {
		t.Errorf("second turn: %+v", tc)
	}
	if tc := f.m.TurnContext("writer"); len(tc.VocabularyChanged) != 0 {
		t.Errorf("writer is not in #eng: %+v", tc)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "store", "shared", "vocabulary", "eng.yaml")); err != nil {
		t.Errorf("V1 vocabulary file: %v", err)
	}
}

func TestChangesNotifyDependentSystems(t *testing.T) {
	f := newFixture(t)
	var got []Change
	f.m.OnChange(func(c Change) { got = append(got, c) })

	// A map change reaches the system and its peer in #eng, whose providers
	// may depend on it.
	must(t, f.m.SetSystemInfo("orders", sysmap.SystemPatch{Summary: ptr("Takes orders.")}))
	if want := []Change{{System: "orders"}, {System: "inventory"}}; !slices.Equal(got, want) {
		t.Errorf("map change: %+v, want %+v", got, want)
	}

	// A vocabulary change reaches the vocabulary and the flags of the hub's
	// systems.
	got = nil
	must(t, f.m.DefineTerm("eng-lead", "eng", vocab.Term{Term: "order", Definition: "x"}))
	if want := []Change{{Hub: "eng"}, {System: "orders"}, {System: "inventory"}}; !slices.Equal(got, want) {
		t.Errorf("vocabulary change: %+v, want %+v", got, want)
	}
	got = nil
	must(t, f.m.DefineTerm("lead", "product", vocab.Term{Term: "plan", Definition: "x"}))
	if want := []Change{{Hub: "product"}}; !slices.Equal(got, want) {
		t.Errorf("#product has no systems: %+v", got)
	}
}

func TestSelf(t *testing.T) {
	f := newFixture(t)
	sys, orchestrates := f.m.Self("orders")
	if sys == nil || sys.WorkDir != "/workspace/services/orders" || len(orchestrates) != 0 {
		t.Errorf("Self(orders) = %+v, %v", sys, orchestrates)
	}
	if sys, orchestrates := f.m.Self("eng-lead"); sys != nil || !slices.Equal(orchestrates, []string{"eng"}) {
		t.Errorf("Self(eng-lead) = %+v, %v", sys, orchestrates)
	}
}

func TestCommitAllLeavesTheRepoClean(t *testing.T) {
	f := newFixture(t)
	write(t, f.repo, "services/orders/half.py", "unfinished\n")
	if errs := f.m.CommitAll(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if got := git(t, f.repo, "status", "--porcelain"); got != "" {
		t.Errorf("status after CommitAll = %q", got)
	}
	topo, _ := topology.Parse([]byte(testTopology), f.dir)
	if _, err := New(topo); err != nil {
		t.Errorf("the next run cannot start: %v", err)
	}
}

func TestPrepareKeepsAnExistingReadme(t *testing.T) {
	f := newFixture(t)
	readme := filepath.Join(f.dir, "store", "README.md")
	os.WriteFile(readme, []byte("mine\n"), 0o644)
	f.m.CommitAll()
	topo, _ := topology.Parse([]byte(testTopology), f.dir)
	if _, err := New(topo); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(readme); string(data) != "mine\n" {
		t.Errorf("the README was overwritten: %q", data)
	}
}

func TestPublishRelease(t *testing.T) {
	f := newFixture(t)
	staging := filepath.Join(f.dir, "store", "systems", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	publish := func(agent, deliverable, version string) (store.Manifest, []string, error) {
		return f.m.PublishRelease(agent, api.PublishRequest{Deliverable: deliverable, Version: version, Notes: "First."})
	}

	if _, _, err := publish("orders", "python-library", "1.0.0"); err == nil || !strings.Contains(err.Error(), "the map is unbuilt") {
		t.Errorf("R16: publishing an unbuilt map: %v", err)
	}
	if _, err := f.m.CheckRelease("orders", "python-library"); err == nil || !strings.Contains(err.Error(), "the map is unbuilt") {
		t.Errorf("R15: checking with an unbuilt map: %v", err)
	}
	if _, _, err := publish("eng-lead", "python-library", "1.0.0"); !errors.Is(err, ErrForbidden) {
		t.Errorf("only owners publish: %v", err)
	}
	if _, _, err := publish("inventory", "python-library", "1.0.0"); !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), "hold no deliverable") {
		t.Errorf("an owner without deliverables: %v", err)
	}
	if _, _, err := publish("orders", "docs", "1.0.0"); !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), `do not hold the deliverable "docs"; you hold python-library`) {
		t.Errorf("a deliverable the owner does not hold: %v", err)
	}

	f.describeOrders(t)
	f.runOrders(t, testmap.Passed)
	if _, err := f.m.MarkCurrent("orders"); err != nil {
		t.Fatal(err)
	}
	if check, err := f.m.CheckRelease("orders", "python-library"); err != nil || check.Files != 2 {
		t.Errorf("R15 check = %+v, %v", check, err)
	}
	write(t, staging, "orders/__pycache__/create.pyc", "x")
	if _, err := f.m.CheckRelease("orders", "python-library"); err == nil || !strings.Contains(err.Error(), "layout.forbid") {
		t.Errorf("R15: the layout rules apply: %v", err)
	}
	os.RemoveAll(filepath.Join(staging, "orders", "__pycache__"))

	var changes []Change
	f.m.OnChange(func(c Change) { changes = append(changes, c) })
	rel, hubs, err := publish("orders", "python-library", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	head := git(t, f.repo, "rev-parse", "HEAD")
	if rel.SourceCommit != head || !slices.Equal(hubs, []string{"eng"}) || !slices.Equal(rel.Hubs, []string{"eng"}) || rel.Hikari == "" {
		t.Errorf("release %+v, hubs %v; HEAD is %s", rel, hubs, head)
	}
	copied, _ := os.ReadFile(filepath.Join(f.dir, "store", "systems", "orders", "releases", "python-library", "1.0.0", "map.yaml"))
	if !strings.Contains(string(copied), "health: passing") || !strings.Contains(string(copied), "state: current") || strings.Contains(string(copied), "create.py") {
		t.Errorf("the copied map must be the public view:\n%s", copied)
	}
	if doc, err := os.ReadFile(filepath.Join(f.dir, "store", "systems", "orders", "releases", "python-library", "1.0.0", "features", "create-order.md")); err != nil || !strings.Contains(string(doc), "described_commit: "+head) {
		t.Errorf("L11: the release holds the wiki: %v\n%s", err, doc)
	}
	if len(changes) == 0 || changes[0].System != "orders" {
		t.Errorf("a release must notify listeners: %+v", changes)
	}
	if v := f.m.SystemView("orders"); len(v.Releases) != 1 || v.Releases[0].Version != "1.0.0" || v.Releases[0].Deliverable != "python-library" {
		t.Errorf("the UI view must list the release: %+v", v.Releases)
	}
	if s, _ := f.m.GetSystem("eng-lead", "orders"); len(s.Releases) != 1 {
		t.Errorf("the orchestrator view must list the release: %+v", s.Releases)
	}
	if d := f.m.Deliverables("orders"); len(d) != 1 || d[0].Name != "python-library" || !strings.HasPrefix(d[0].Description, "A Python library") {
		t.Errorf("Deliverables = %+v", d)
	}
}

func TestDeliverableChecksAtStartup(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	newRepo := func(t *testing.T) string {
		dir := t.TempDir()
		repo := filepath.Join(dir, "shop")
		write(t, repo, "services/orders/create.py", "x\n")
		write(t, repo, "services/inventory/stock.py", "x\n")
		git(t, repo, "init", "--quiet", "--initial-branch=main")
		git(t, repo, "add", "--all")
		git(t, repo, "commit", "--quiet", "-m", "initial")
		return dir
	}
	start := func(t *testing.T, dir string) (*Manager, error) {
		topo, err := topology.Parse([]byte(testTopology), dir)
		if err != nil {
			t.Fatal(err)
		}
		return New(topo)
	}

	t.Run("T15 missing", func(t *testing.T) {
		dir := newRepo(t)
		if _, err := start(t, dir); err == nil || !strings.Contains(err.Error(), `deliverable "python-library" does not exist`) {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("T15 invalid", func(t *testing.T) {
		dir := newRepo(t)
		defineDeliverable(t, dir, "python-library", "version: 1\n")
		if _, err := start(t, dir); err == nil || !strings.Contains(err.Error(), "description is required") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("T16 shared skill", func(t *testing.T) {
		dir := newRepo(t)
		defineDeliverable(t, dir, "python-library", pythonLibrary)
		write(t, filepath.Join(dir, "store", "shared", "skills", "python-library"), "SKILL.md", "# mine\n")
		if _, err := start(t, dir); err == nil || !strings.Contains(err.Error(), "has the name of a shared skill") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestChangeFeatures(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	features := filepath.Join(f.dir, "store", "systems", "orders", "features")
	if _, err := os.Stat(filepath.Join(features, "create-order.md")); err != nil {
		t.Fatalf("L10, W7: the document is in the store: %v", err)
	}
	var changes []Change
	f.m.OnChange(func(c Change) { changes = append(changes, c) })

	res, err := f.m.ChangeFeatures("orders", []featurewiki.Write{
		{Op: "create", ID: "cancel-order", Name: "Cancel an order", Description: "Cancel a placed order.", Body: body("After [[create-order]] placed an order, the customer can cancel it.")},
	})
	if err != nil || len(res.Applied) != 1 || len(changes) == 0 {
		t.Fatalf("change = %+v, %v; changes %v", res, err, changes)
	}
	v, _ := f.m.OwnView("orders")
	if len(v.Features) != 2 || v.Features[0].ID != "cancel-order" || !slices.Equal(v.Features[0].Related, []string{"create-order"}) || !slices.Equal(v.Features[1].Related, []string{"cancel-order"}) {
		t.Errorf("Q1, K3: the map lists both directions: %+v", v.Features)
	}
	if v.Features[0].State != sysmap.Undescribed || !slices.Equal(v.UndescribedFeatures, []string{"cancel-order", "create-order"}) {
		t.Errorf("S20: new features are undescribed: %+v", v)
	}
	if !slices.ContainsFunc(v.Warnings, func(w string) bool { return strings.Contains(w, `"cancel-order" has no code entry`) }) {
		t.Errorf("the owner view warns about a feature without code: %v", v.Warnings)
	}

	// The rules refuse as a whole, and name the rule.
	_, err = f.m.ChangeFeatures("orders", []featurewiki.Write{
		{Op: "create", ID: "ship-order", Name: "Ship", Description: "Ship it.", Body: body("Ships.")},
		{Op: "create", ID: "bad", Name: "Bad", Description: "Bad.", Interfaces: []string{"GET /nope"}, Body: body("Uses an unknown interface.")},
	})
	if err == nil || !strings.Contains(err.Error(), "W3") {
		t.Errorf("W3: %v", err)
	}
	if v, _ := f.m.OwnView("orders"); len(v.Features) != 2 {
		t.Error("a refused change must not change the wiki")
	}

	// Ids are unique across systems.
	_, err = f.m.ChangeFeatures("inventory", []featurewiki.Write{{Op: "create", ID: "create-order", Name: "Create", Description: "Dup.", Body: body("Dup.")}})
	if err == nil || !strings.Contains(err.Error(), "W1") || !strings.Contains(err.Error(), "another system") {
		t.Errorf("W1 across systems: %v", err)
	}

	// A rename keeps test results and rewrites links in the wiki and the
	// private documents.
	f.runOrders(t, testmap.Passed, testmap.Passed, testmap.Passed)
	res, err = f.m.ChangeFeatures("orders", []featurewiki.Write{{Op: "rename", ID: "create-order", To: "place-order"}})
	if err != nil {
		t.Fatal(err)
	}
	v, _ = f.m.OwnView("orders")
	if len(v.Features) != 2 || v.Features[1].ID != "place-order" || v.Features[1].Tests.Passed != 3 || v.Features[1].Health != testmap.Passing || !slices.Equal(v.Features[0].Related, []string{"place-order"}) {
		t.Errorf("E5, E7, K4 after a rename: %+v", v.Features)
	}
	if len(res.Notes) != 0 || len(v.PrivateErrors) != 0 {
		t.Errorf("E7: a rename leaves nothing to fix: notes %v, errors %v", res.Notes, v.PrivateErrors)
	}
	if priv, _ := sysmap.LoadPrivate(filepath.Join(f.repo, "services/orders")); priv.Tests.Tests["tests/test_create.py"][0] != "[[place-order]]" || priv.Layout.Features["place-order"].Code == nil {
		t.Errorf("E7: private documents after a rename: %+v", priv)
	}
	if _, err := os.Stat(filepath.Join(features, "create-order.md")); err == nil {
		t.Error("K4: the old file is gone")
	}
	doc, _ := os.ReadFile(filepath.Join(features, "cancel-order.md"))
	if !strings.Contains(string(doc), "[[place-order]]") {
		t.Errorf("K4: links follow the rename:\n%s", doc)
	}
	hit, err := f.m.GetFeature("eng-lead", "orders", "cancel-order")
	if err != nil || hit.Document != string(doc) {
		t.Errorf("Q3: the document as on disk: %v", err)
	}

	// A remove drops the feature; its tests become unmapped, and the code
	// map is left to the owner.
	res, err = f.m.ChangeFeatures("orders", []featurewiki.Write{{Op: "remove", ID: "place-order"}})
	if err != nil {
		t.Fatal(err)
	}
	v, _ = f.m.OwnView("orders")
	if len(v.Features) != 1 || len(v.Features[0].Related) != 0 {
		t.Errorf("after a remove: %+v", v.Features)
	}
	if len(v.UnmappedTests) != 1 || len(v.UnmappedTests[0].Tests) != 3 {
		t.Errorf("E8: the removed feature's tests are unmapped: %+v", v.UnmappedTests)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], `layout.yaml still names the removed feature "place-order"`) {
		t.Errorf("E10: notes %v", res.Notes)
	}
	if _, err := f.m.ChangeFeatures("eng-lead", nil); !errors.Is(err, ErrForbidden) {
		t.Errorf("only owners change features: %v", err)
	}
}

func TestMarkCurrentDescribesEveryDocument(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	commit, err := f.m.MarkCurrent("orders")
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := os.ReadFile(filepath.Join(f.dir, "store", "systems", "orders", "features", "create-order.md"))
	if !strings.Contains(string(doc), "described_commit: "+commit) || !strings.Contains(string(doc), "described_at: ") {
		t.Errorf("S22: the document records the commit:\n%s", doc)
	}
	if v, _ := f.m.OwnView("orders"); v.Features[0].State != sysmap.Current {
		t.Errorf("S20: %s", v.Features[0].State)
	}
}

func TestHandEditsOfTheWikiReachTheMap(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	o := f.m.owners["orders"]
	path := filepath.Join(f.dir, "store", "systems", "orders", "features", "create-order.md")
	doc, _ := os.ReadFile(path)
	edited := strings.Replace(string(doc), "description: Stores a new order.", "description: Stores a new order for a customer.", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	if !o.wiki.Reload() {
		t.Fatal("the edit was not seen")
	}
	must(t, f.m.project(o))
	if v, _ := f.m.OwnView("orders"); v.Features[0].Description != "Stores a new order for a customer." || v.Error != "" {
		t.Errorf("W8: %+v", v)
	}
	// An edit that names an unknown interface keeps the wiki but the map
	// cannot follow; the owner view says so.
	os.WriteFile(path, []byte(strings.Replace(edited, "interfaces: [", "interfaces: [GET /nope, ", 1)), 0o644)
	o.wiki.Reload()
	if err := f.m.project(o); err == nil {
		t.Fatal("S2 must refuse the projection")
	}
	if v, _ := f.m.OwnView("orders"); !strings.Contains(v.Error, "S2") {
		t.Errorf("W8: the owner view reports the problem: %q", v.Error)
	}
}

func TestOldMapsAreDeleted(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	git(t, f.repo, "add", "--all")
	git(t, f.repo, "commit", "--quiet", "-m", "skeletons")
	mapPath := filepath.Join(f.dir, "store", "systems", "orders", "map.yaml")
	os.WriteFile(mapPath, []byte("version: 2\nsystem:\n  name: orders\nfeatures:\n  - {id: x, name: X}\n"), 0o644)
	topo, err := topology.Parse([]byte(testTopology), f.dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(m.Warnings(), func(w string) bool { return strings.Contains(w, "version 2") }) {
		t.Errorf("Q4: no warning: %v", m.Warnings())
	}
	if v, _ := m.OwnView("orders"); v.State != sysmap.Unbuilt || len(v.Features) != 0 {
		t.Errorf("Q4: the system starts unbuilt: %+v", v)
	}
	// The wiki names interfaces that the deleted map held; once the owner
	// gives them again, the features return.
	must(t, m.SetSystemInfo("orders", sysmap.SystemPatch{
		Provides: ptr([]sysmap.Interface{{Name: "POST /orders", Kind: "http"}}),
		Consumes: ptr([]sysmap.Interface{{Name: "inventory.reserve", Kind: "function"}}),
	}))
	if v, _ := m.OwnView("orders"); len(v.Features) != 1 || v.Features[0].ID != "create-order" || v.Error != "" {
		t.Errorf("Q4: set_system_info writes the features again: %+v", v)
	}
}

func TestSharedAgentPrefixIsRefused(t *testing.T) {
	f := newFixture(t)
	git(t, f.repo, "add", "--all")
	git(t, f.repo, "commit", "--quiet", "-m", "skeletons")
	write(t, filepath.Join(f.dir, "store", "shared", "agents"), "hikari-feature-survey.md", "---\nname: x\n---\n")
	topo, _ := topology.Parse([]byte(testTopology), f.dir)
	if _, err := New(topo); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("T17: %v", err)
	}
}

func TestTestHealth(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	run := f.runOrders(t, testmap.Passed, testmap.Failed, testmap.Skipped)
	if run.Passed != 1 || run.Failed != 1 || run.Skipped != 1 {
		t.Errorf("J1: %+v", run)
	}
	v, _ := f.m.OwnView("orders")
	if v.Features[0].Health != testmap.Failing || v.Features[0].Tests != (sysmap.Tests{Passed: 1, Failed: 1, Pending: 1, Ran: v.Features[0].Tests.Ran}) {
		t.Errorf("S25, Q5: %+v", v.Features[0])
	}
	if len(v.FailingTests) != 1 || v.FailingTests[0].ID != "tests.test_create.CreateTest > test_1" || v.FailingTests[0].Message != "AssertionError: no order stored" {
		t.Errorf("E12: failing tests %+v", v.FailingTests)
	}
	tc := f.m.TurnContext("orders")
	if len(tc.FailingFeatures) != 1 || tc.FailingFeatures[0].ID != "create-order" || tc.FailingFeatures[0].Tests[0] != "tests.test_create.CreateTest > test_1" {
		t.Errorf("P20: %+v", tc.FailingFeatures)
	}
	public, _ := os.ReadFile(filepath.Join(f.dir, "store/systems/orders/map.yaml"))
	if !strings.Contains(string(public), "health: failing") || !strings.Contains(string(public), "pending: 1") {
		t.Errorf("Q6: the map file holds the new health:\n%s", public)
	}
	for _, leak := range []string{"test_create", "AssertionError", "content_hash"} {
		if strings.Contains(string(public), leak) {
			t.Errorf("Q5: the map holds %q:\n%s", leak, public)
		}
	}
	hit, _ := f.m.GetFeature("eng-lead", "orders", "create-order")
	if hit.Feature.Health != testmap.Failing {
		t.Errorf("E13: orchestrators see the health: %+v", hit.Feature)
	}

	// Results and reports stay out of commits (rule L13).
	write(t, f.repo, "services/orders/.hikari/private/reports/stray.xml", "<x/>")
	if _, err := f.m.Commit("orders", ""); err != nil {
		t.Fatal(err)
	}
	if files := git(t, f.repo, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "results.yaml") || strings.Contains(files, "reports/") || !strings.Contains(files, ".hikari/private/.gitignore") {
		t.Errorf("L13: committed files:\n%s", files)
	}

	// A test that no key covers is unmapped, and a key that matches no test
	// is stale and makes its features pending.
	write(t, f.repo, "services/orders/.hikari/private/tests.yaml", `version: 1
suites:
  unit: {command: python3 -m pytest}
tests:
  tests/test_gone.py: ["[[create-order]]"]
`)
	v, _ = f.m.OwnView("orders")
	if len(v.UnmappedTests) != 1 || v.UnmappedTests[0].File != "tests/test_create.py" || len(v.UnmappedTests[0].Tests) != 3 {
		t.Errorf("C1: unmapped %+v", v.UnmappedTests)
	}
	if !slices.Equal(v.StaleKeys, []string{"tests/test_gone.py"}) || v.Features[0].Health != testmap.Pending {
		t.Errorf("S30: stale keys %v, health %s", v.StaleKeys, v.Features[0].Health)
	}
}
