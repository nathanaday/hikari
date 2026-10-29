package partitions

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

const projectTopology = `
name: test
x-hikari:
  project: {branch: hikari}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {partition: {folders: [{repo: project, path: app}]}}}
`

// newProject makes a project repository with one partition agent, orders, whose
// folder app is in the project repository, and starts a Manager on it.
func newProject(t *testing.T) (dir string, m *Manager) {
	t.Helper()
	f := newFixture(t) // sets up git for the test
	dir = filepath.Join(f.dir, "project")
	write(t, dir, ".gitignore", ".env\n/.hikari/\n")
	write(t, dir, "topology.yaml", projectTopology)
	write(t, dir, "app/create.py", "def create(): pass\n")
	write(t, dir, "app/tests/test_create.py", "def test(): pass\n")
	write(t, dir, "vault/public/policies/writing.md", "Write plainly.\n")
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

func TestProjectVaultCommits(t *testing.T) {
	dir, m := newProject(t)
	if got := git(t, dir, "branch", "--show-current"); got != "hikari" {
		t.Errorf("PR8 branch = %q", got)
	}
	// PR12 and PR18: hikari's own files, committed by hikari.
	log := git(t, dir, "log", "--format=%an|%s")
	for _, want := range []string{"hikari (hikari)|hikari: add vault/README.md", "hikari (hikari)|hikari: add vault/.gitignore"} {
		if !strings.Contains(log, want) {
			t.Errorf("PR18 log lacks %q:\n%s", want, log)
		}
	}
	if got := git(t, dir, "show", "HEAD:vault/.gitignore"); got != "partitions/*/staging/\npartitions/*/read-only/releases/*/*/product/\npartitions/*/read-only/releases/*/tags/" {
		t.Errorf("RL2 ignore file = %q", got)
	}

	must(t, m.SetPartitionInfo("orders", featuremap.PartitionPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]featuremap.Interface{{Name: "create_order", Kind: "function"}}),
	}))
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: set the partition info" || !slices.Equal(files, []string{"vault/partitions/orders/read-only/features/map.yaml"}) {
		t.Errorf("PR15 set_partition_info commit = %s | %s | %v", a, s, files)
	}
	res, err := m.ChangeWiki("orders", false, []wiki.Write{{
		Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.",
		Interfaces: []string{"create_order"}, Body: body("A customer places an order."),
	}})
	must(t, err)
	if len(res.Notes) > 0 {
		t.Errorf("notes = %v", res.Notes)
	}
	a, s, files := lastCommit(t, dir)
	if a != "orders (hikari)" || s != "orders: change the wiki: create create-order" || !slices.Equal(files, []string{"vault/partitions/orders/read-only/features/create-order.md", "vault/partitions/orders/read-only/features/map.yaml"}) {
		t.Errorf("C2 change_wiki commit = %s | %s | %v", a, s, files)
	}

	must(t, m.DefineTerm("lead", vocab.Term{Term: "order", Definition: "A request to buy items."}))
	if a, s, files := lastCommit(t, dir); a != "lead (hikari)" || s != "lead: define order in the vocabulary" || !slices.Equal(files, []string{"vault/public/vocabulary/terms.yaml"}) {
		t.Errorf("PR15 vocabulary commit = %s | %s | %v", a, s, files)
	}

	// The folder's own commit stays separate from the vault's.
	write(t, dir, "app/.hikari/private/code-map.yaml", "version: 1\nparts: [{path: project/app/create.py, description: Order creation.}]\nfeatures:\n  create-order: {code: [project/app/create.py]}\n")
	write(t, dir, "app/.hikari/private/tests.yaml", "version: 1\nsuites:\n  unit: {command: pytest}\ntests:\n  project/app/tests/test_create.py: [\"[[create-order]]\"]\n")
	commits, err := m.MarkCurrent("orders")
	must(t, err)
	head := strings.TrimPrefix(commits[0], "project@")
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: mark the map current at project@"+head[:7] || !slices.Contains(files, "vault/partitions/orders/read-only/features/map.yaml") {
		t.Errorf("PR15 mark_map_current commit = %s | %s | %v", a, s, files)
	}
	if got := git(t, dir, "log", "-1", "--format=%s", head); got != "orders: changes described by the feature map" {
		t.Errorf("the described commit is the folder's: %q", got)
	}

	// PR16: a test run changes the map but makes no commit.
	before := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "app/.hikari/private/reports/unit.xml", `<testsuites><testsuite name="pytest"><testcase classname="tests.test_create.T" name="test_a" file="project/app/tests/test_create.py"/></testsuite></testsuites>`)
	if _, err := m.RecordRun("orders", "unit"); err != nil {
		t.Fatal(err)
	}
	if git(t, dir, "rev-parse", "HEAD") != before {
		t.Error("PR16 a test run made a commit")
	}
	if !strings.Contains(git(t, dir, "status", "--porcelain"), "vault/partitions/orders/read-only/features/map.yaml") {
		t.Error("the run's health is not in map.yaml")
	}

	// PR17, PR19, PR20: at stop, the agent paths are committed; your shared
	// files are not.
	write(t, dir, "vault/public/policies/writing.md", "Write plainly and briefly.\n")
	if errs := m.CommitAll(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: vault changes left when hikari stopped" || !slices.Equal(files, []string{"vault/partitions/orders/read-only/features/map.yaml"}) {
		t.Errorf("PR17 stop commit = %s | %s | %v", a, s, files)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "M vault/public/policies/writing.md" {
		t.Errorf("PR19 status after stop = %q", got)
	}

	// PR22: the summary counts the vault's commits.
	summary := strings.Join(m.Summary(), "\n")
	if !strings.Contains(summary, "the vault has 7 commit(s) on hikari in the project repository") {
		t.Errorf("PR22 summary:\n%s", summary)
	}
	// PR27: the project repository holds a partition, so the UI lists it.
	if repos := m.Repos(); len(repos) != 1 || repos[0].Name != topology.ProjectRepo || !slices.Equal(repos[0].Partitions, []string{"orders"}) {
		t.Errorf("PR27 repos = %+v", repos)
	}
}

func TestProjectWithoutPartitionsIsNotListed(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.dir, "empty")
	src := "name: e\nx-hikari:\n  project: {branch: hikari}\n  orchestration: lead\nservices:\n  lead: {x-hikari: {}}\n"
	write(t, dir, ".gitignore", ".env\n/.hikari/\n")
	write(t, dir, "topology.yaml", src)
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

func TestProjectWikiAuthors(t *testing.T) {
	dir, m := newProject(t)
	must(t, m.SetPartitionInfo("orders", featuremap.PartitionPatch{Summary: ptr("Takes orders.")}))
	_, err := m.ChangeWiki("orders", false, []wiki.Write{{Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.", Body: body("A customer places an order.")}})
	must(t, err)

	// C1: your edit of a document comes first, as yours, when an agent
	// changes the wiki.
	feature := filepath.Join(dir, "vault/partitions/orders/read-only/features/create-order.md")
	data, _ := os.ReadFile(feature)
	os.WriteFile(feature, append(data, []byte("\n## Notes\n\nMine.\n")...), 0o644)
	write(t, dir, "vault/partitions/orders/read-only/features/Roadmap.md", "---\nkind: plan\n---\nRefunds.\n")
	m.wikis[api.FeatureMapRef("orders")].files.Reload()
	_, err = m.ChangeWiki("orders", false, []wiki.Write{{Op: "modify", ID: "create-order", Body: body("A customer places an order, and it is stored.")}})
	must(t, err)
	log := git(t, dir, "log", "-2", "--format=%an|%s")
	if log != "orders (hikari)|orders: change the wiki: modify create-order\nTest User|user: wiki edits before orders's change" {
		t.Errorf("C1, C2: log =\n%s", log)
	}
	if got := git(t, dir, "show", "--name-only", "--format=", "HEAD~1"); got != "vault/partitions/orders/read-only/features/Roadmap.md\nvault/partitions/orders/read-only/features/create-order.md" {
		t.Errorf("C1: your commit holds %q", got)
	}

	// K9: a change of the orchestration feature map is the orchestration
	// agent's.
	_, err = m.ChangeWiki("lead", false, []wiki.Write{{Op: "create", ID: "order-flow", Name: "An order flows", Description: "A customer orders.", Body: body("Through [[create-order]].")}})
	must(t, err)
	if a, s, files := lastCommit(t, dir); a != "lead (hikari)" || s != "lead: change the orchestration feature map: create order-flow" || !slices.Equal(files, []string{"vault/orchestration/read-only/features/order-flow.md"}) {
		t.Errorf("K9: %s | %s | %v", a, s, files)
	}

	// C4: a change in the UI is committed at once, as yours.
	must(t, m.WriteDocument(api.PublicWikiRef, "Ideas", "More.\n"))
	if a, s, files := lastCommit(t, dir); a != "Test User" || s != "user: write Ideas in public" || !slices.Equal(files, []string{"vault/public/wiki/Ideas.md"}) {
		t.Errorf("C4: %s | %s | %v", a, s, files)
	}

	// C3: at stop, what is left in the wikis is yours.
	write(t, dir, "vault/public/wiki/Later.md", "Later.\n")
	write(t, dir, "vault/orchestration/read-only/features/Soon.md", "Soon.\n")
	if errs := m.CommitAll(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if a, s, files := lastCommit(t, dir); a != "Test User" || s != "user: wiki edits left when hikari stopped" || !slices.Equal(files, []string{"vault/orchestration/read-only/features/Soon.md", "vault/public/wiki/Later.md"}) {
		t.Errorf("C3: %s | %s | %v", a, s, files)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("C3: left %q", got)
	}
}

func TestIsUserPath(t *testing.T) {
	for p, want := range map[string]bool{
		"vault/partitions/orders/read-only/features/a.md":      true,
		"vault/public/wiki/Ideas.md":                           true,
		"vault/partitions/orders/can-edit/notes/a.txt":         true,
		"vault/partitions/orders/read-only/granted/spec.pdf":   true,
		"vault/orchestration/can-edit/plan.md":                 true,
		"vault/orchestration/read-only/granted/brief.md":       true,
		"vault/orchestration/read-only/features/a.md":          true,
		"vault/orchestration/read-only/features/x/a.md":        false,
		"vault/partitions/orders/read-only/features/map.yaml":  false,
		"vault/partitions/orders/read-only/releases/x/1.0.0/a": false,
		"vault/partitions/orders/staging/lib/a.py":             false,
		"vault/public/policies/p.md":                           false,
		"vault/public/vocabulary/terms.yaml":                   false,
		"vault/skills/style/SKILL.md":                          false,
		"vault/hubs/eng/wiki/Ideas.md":                         false,
		"topology.yaml":                                        false,
		"vault/public/wiki":                                    false,
	} {
		if got := IsUserPath("vault", p); got != want {
			t.Errorf("C5, V10 IsUserPath(%q) = %v", p, got)
		}
	}
}

// TestProjectCanEditCommits checks rule V10: an apply commits the agent's
// changes in its can-edit/ as the agent, and at stop your changes in
// can-edit/ and granted/ are committed as yours.
func TestProjectCanEditCommits(t *testing.T) {
	dir, m := newProject(t)
	write(t, dir, "vault/partitions/orders/can-edit/notes.md", "Agent notes.\n")
	if _, err := m.Commit("orders", "orders: keep notes"); err != nil {
		t.Fatal(err)
	}
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: keep notes" || !slices.Equal(files, []string{"vault/partitions/orders/can-edit/notes.md"}) {
		t.Errorf("V10 apply commit = %s | %s | %v", a, s, files)
	}
	write(t, dir, "vault/partitions/orders/read-only/granted/brief.md", "Read this.\n")
	write(t, dir, "vault/orchestration/can-edit/plan.md", "The plan.\n")
	if _, err := m.Commit("lead", ""); err != nil {
		t.Fatal(err)
	}
	if a, _, files := lastCommit(t, dir); a != "lead (hikari)" || !slices.Equal(files, []string{"vault/orchestration/can-edit/plan.md"}) {
		t.Errorf("V10 the orchestration agent's turn commits its can-edit/: %s | %v", a, files)
	}
	if errs := m.CommitAll(); len(errs) > 0 {
		t.Fatal(errs)
	}
	log := git(t, dir, "log", "--format=%an|%s", "--", "vault/partitions/orders/read-only/granted")
	if log != "Test User|user: vault edits left when hikari stopped" {
		t.Errorf("V10 your granted file at stop: %q", log)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("V10: left %q", got)
	}
}
