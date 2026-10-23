package systems

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/featurewiki"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
)

const projectTopology = `
name: test
x-hikari:
  project: {branch: hikari}
  hubs:
    eng: {orchestrator: lead, members: [orders]}
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {system: {repo: project, scope: app}}}
`

// newProject makes a project repository with one owner, orders, whose
// scope app is in the project repository, and starts a Manager on it.
func newProject(t *testing.T) (dir string, m *Manager) {
	t.Helper()
	f := newFixture(t) // sets up git for the test
	dir = filepath.Join(f.dir, "project")
	write(t, dir, ".gitignore", ".env\n/.hikari/\n")
	write(t, dir, "compose.yaml", projectTopology)
	write(t, dir, "app/create.py", "def create(): pass\n")
	write(t, dir, "app/tests/test_create.py", "def test(): pass\n")
	write(t, dir, "store/shared/policies/writing.md", "Write plainly.\n")
	git(t, dir, "init", "--quiet", "--initial-branch=main")
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "--quiet", "-m", "initial")
	topo, err := topology.Parse([]byte(projectTopology), dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err = New(topo)
	if err != nil {
		t.Fatal(err)
	}
	return dir, m
}

// lastCommit returns the author name, the subject, and the files of HEAD.
func lastCommit(t *testing.T, dir string) (author, subject string, files []string) {
	t.Helper()
	out := git(t, dir, "show", "--name-only", "--format=%an|%s", "HEAD")
	head, rest, _ := strings.Cut(out, "\n")
	author, subject, _ = strings.Cut(head, "|")
	return author, subject, strings.Fields(rest)
}

func TestProjectStoreCommits(t *testing.T) {
	dir, m := newProject(t)
	if got := git(t, dir, "branch", "--show-current"); got != "hikari" {
		t.Errorf("PR8 branch = %q", got)
	}
	// PR12 and PR18: hikari's own files, committed by hikari.
	log := git(t, dir, "log", "--format=%an|%s")
	for _, want := range []string{"hikari (hikari)|hikari: add store/README.md", "hikari (hikari)|hikari: add store/.gitignore"} {
		if !strings.Contains(log, want) {
			t.Errorf("PR18 log lacks %q:\n%s", want, log)
		}
	}
	if got := git(t, dir, "show", "HEAD:store/.gitignore"); got != "systems/*/staging/\nsystems/*/releases/" {
		t.Errorf("PR12 ignore file = %q", got)
	}

	must(t, m.SetSystemInfo("orders", sysmap.SystemPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]sysmap.Interface{{Name: "create_order", Kind: "function"}}),
	}))
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: set the system info" || !slices.Equal(files, []string{"store/systems/orders/map.yaml"}) {
		t.Errorf("PR15 set_system_info commit = %s | %s | %v", a, s, files)
	}
	res, err := m.ChangeFeatures("orders", []featurewiki.Write{{
		Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.",
		Interfaces: []string{"create_order"}, Body: body("A customer places an order."),
	}})
	must(t, err)
	if len(res.Notes) > 0 {
		t.Errorf("notes = %v", res.Notes)
	}
	a, s, files := lastCommit(t, dir)
	if a != "orders (hikari)" || s != "orders: change features: create create-order" || !slices.Equal(files, []string{"store/systems/orders/features/create-order.md", "store/systems/orders/map.yaml"}) {
		t.Errorf("PR15 change_features commit = %s | %s | %v", a, s, files)
	}

	must(t, m.DefineTerm("lead", "eng", vocab.Term{Term: "order", Definition: "A request to buy items."}))
	if a, s, files := lastCommit(t, dir); a != "lead (hikari)" || s != "lead: define order in the #eng vocabulary" || !slices.Equal(files, []string{"store/shared/vocabulary/eng.yaml"}) {
		t.Errorf("PR15 vocabulary commit = %s | %s | %v", a, s, files)
	}

	// The scope's own commit stays separate from the store's.
	write(t, dir, "app/.hikari/private/layout.yaml", "version: 1\nparts: [{path: create.py, description: Order creation.}]\nfeatures:\n  create-order: {code: [create.py]}\n")
	write(t, dir, "app/.hikari/private/tests.yaml", "version: 1\nsuites:\n  unit: {command: pytest}\ntests:\n  tests/test_create.py: [\"[[create-order]]\"]\n")
	head, err := m.MarkCurrent("orders")
	must(t, err)
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: mark the map current at "+head[:7] || !slices.Contains(files, "store/systems/orders/map.yaml") {
		t.Errorf("PR15 mark_map_current commit = %s | %s | %v", a, s, files)
	}
	if got := git(t, dir, "log", "-1", "--format=%s", head); got != "orders: changes described by the system map" {
		t.Errorf("the described commit is the scope's: %q", got)
	}

	// PR16: a test run changes the map but makes no commit.
	before := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "app/.hikari/private/reports/unit.xml", `<testsuites><testsuite name="pytest"><testcase classname="tests.test_create.T" name="test_a" file="tests/test_create.py"/></testsuite></testsuites>`)
	if _, err := m.RecordRun("orders", "unit"); err != nil {
		t.Fatal(err)
	}
	if git(t, dir, "rev-parse", "HEAD") != before {
		t.Error("PR16 a test run made a commit")
	}
	if !strings.Contains(git(t, dir, "status", "--porcelain"), "store/systems/orders/map.yaml") {
		t.Error("the run's health is not in map.yaml")
	}

	// PR17, PR19, PR20: at stop, the agent paths are committed; your shared
	// files are not.
	write(t, dir, "store/shared/policies/writing.md", "Write plainly and briefly.\n")
	if errs := m.CommitAll(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: store changes left when hikari stopped" || !slices.Equal(files, []string{"store/systems/orders/map.yaml"}) {
		t.Errorf("PR17 stop commit = %s | %s | %v", a, s, files)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "M store/shared/policies/writing.md" {
		t.Errorf("PR19 status after stop = %q", got)
	}

	// PR22: the summary counts the store's commits.
	summary := strings.Join(m.Summary(), "\n")
	if !strings.Contains(summary, "the store has 7 commit(s) on hikari in the project repository") {
		t.Errorf("PR22 summary:\n%s", summary)
	}
	// PR27: the project repository holds a system, so the UI lists it.
	if repos := m.Repos(); len(repos) != 1 || repos[0].Name != topology.ProjectRepo || !slices.Equal(repos[0].Systems, []string{"orders"}) {
		t.Errorf("PR27 repos = %+v", repos)
	}
}

func TestProjectWithoutSystemsIsNotListed(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.dir, "empty")
	src := "name: e\nx-hikari:\n  project: {branch: hikari}\n  hubs: {h: {orchestrator: lead}}\nservices:\n  lead: {x-hikari: {}}\n"
	write(t, dir, ".gitignore", ".env\n/.hikari/\n")
	write(t, dir, "compose.yaml", src)
	git(t, dir, "init", "--quiet", "--initial-branch=main")
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "--quiet", "-m", "initial")
	topo, err := topology.Parse([]byte(src), dir)
	must(t, err)
	m, err := New(topo)
	must(t, err)
	if repos := m.Repos(); len(repos) != 0 {
		t.Errorf("PR27 repos = %+v", repos)
	}
	if errs := m.CommitAll(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("a fresh project is left dirty: %q", got)
	}
}
