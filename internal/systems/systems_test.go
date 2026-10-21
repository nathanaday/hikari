package systems

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
)

// The tree of the tests: lead orchestrates #product, whose members are
// eng-lead and writer. eng-lead orchestrates #eng, whose members are the
// owners orders and inventory.
const testTopology = `
repos:
  shop: {path: shop, branch: hikari/work}
agents:
  lead: {}
  writer: {}
  eng-lead: {}
  orders: {system: {repo: shop, scope: services/orders, test: python3 -m unittest}}
  inventory: {system: {repo: shop, scope: services/inventory}}
hubs:
  product: {orchestrator: lead, members: [eng-lead, writer]}
  eng: {orchestrator: eng-lead, members: [orders, inventory]}
`

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
		Layout:   ptr([]sysmap.LayoutEntry{{Path: "create.py", Description: "Order creation."}}),
	}))
	must(t, f.m.UpsertFeature("orders", sysmap.FeatureInput{
		ID: "create-order", Name: "Create an order", Description: "Stores a new order.",
		Interfaces: []string{"POST /orders", "inventory.reserve"},
		Code:       []string{"create.py"}, TestRefs: []string{"tests/test_create.py"},
	}))
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
	for _, scope := range []string{"services/orders", "services/inventory"} {
		if _, err := os.Stat(filepath.Join(f.repo, scope, ".hikari", "map.yaml")); err != nil {
			t.Errorf("G4: no map in %s: %v", scope, err)
		}
	}
	v, err := f.m.OwnView("orders")
	if err != nil || v.State != sysmap.Unbuilt || v.System.TestCommand != "python3 -m unittest" {
		t.Errorf("new map: %+v, %v", v, err)
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
	topo, err := topology.Parse([]byte(testTopology), dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	if w := m.Warnings(); len(w) != 2 || !strings.Contains(w[0], "is not versioned with its code") {
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

func TestOwnerWorkflow(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	must(t, f.m.RecordTests("orders", []sysmap.TestResult{{Feature: "create-order", Passed: 3}}))

	write(t, f.repo, "services/orders/cancel.py", "def cancel(): pass\n")
	commit, err := f.m.MarkCurrent("orders")
	if err != nil {
		t.Fatal(err)
	}
	if got := git(t, f.repo, "log", "-1", "--format=%an|%s"); got != "orders (hikari)|orders: changes described by the system map" {
		t.Errorf("MarkCurrent commit: %q", got)
	}
	v, _ := f.m.OwnView("orders")
	if v.State != sysmap.Current || v.System.DescribedState.Commit != commit || v.Features[0].Health != sysmap.Passing {
		t.Errorf("after MarkCurrent: state %s, commit %s, health %s", v.State, v.System.DescribedState.Commit, v.Features[0].Health)
	}

	// The map change itself is not a code change.
	if tc := f.m.TurnContext("orders"); tc.MapState != sysmap.Current || len(tc.Changed) != 0 {
		t.Errorf("turn context after MarkCurrent: %+v", tc)
	}

	// A host edit makes the map stale and the feature stale.
	write(t, f.repo, "services/orders/create.py", "def create(): return 1\n")
	tc := f.m.TurnContext("orders")
	if tc.MapState != sysmap.Stale || !slices.Equal(tc.Changed, []string{"create.py"}) {
		t.Errorf("turn context after a host edit: %+v", tc)
	}
	if v, _ := f.m.OwnView("orders"); v.Features[0].Health != sysmap.Stale {
		t.Errorf("S9 health after an edit: %s", v.Features[0].Health)
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
	if !strings.Contains(files, "services/orders/create.py") || !strings.Contains(files, "services/orders/.hikari/map.yaml") || strings.Contains(files, "inventory") {
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
	if hit, err := f.m.GetFeature("eng-lead", "orders", "create-order"); err != nil || hit.Feature.Name != "Create an order" {
		t.Errorf("get feature = %+v, %v", hit, err)
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
	if _, err := os.Stat(filepath.Join(f.dir, "vocabulary", "eng.yaml")); err != nil {
		t.Errorf("V1 vocabulary file: %v", err)
	}
}

func TestChangesNotifyListeners(t *testing.T) {
	f := newFixture(t)
	var got []Change
	f.m.OnChange(func(c Change) { got = append(got, c) })
	f.describeOrders(t)
	must(t, f.m.DefineTerm("eng-lead", "eng", vocab.Term{Term: "order", Definition: "x"}))
	if len(got) != 3 || got[0].System != "orders" || got[2].Hub != "eng" {
		t.Errorf("changes = %+v", got)
	}
}

func TestSelf(t *testing.T) {
	f := newFixture(t)
	sys, orchestrates := f.m.Self("orders")
	if sys == nil || sys.WorkDir != "/workspace/services/orders" || sys.TestCommand != "python3 -m unittest" || len(orchestrates) != 0 {
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
