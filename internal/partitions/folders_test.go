package partitions

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/wiki"
)

// A partition with a folder in each of two repos, and a read-only folder,
// and a second partition in the first repo (rule F19).
const twoRepoTopology = `
name: two
x-hikari:
  repos:
    shop: {path: shop, branch: hikari/work}
    protos: {path: protos, branch: hikari/protos}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  orders:
    x-hikari:
      partition:
        folders:
          - {repo: shop, path: services/orders}
          - {repo: shop, path: libs/money, read_only: true}
          - {repo: protos, path: orders, at: protos}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`

type twoRepos struct {
	dir, shop, protos string
	m                 *Manager
}

func newTwoRepos(t *testing.T) *twoRepos {
	t.Helper()
	f := newFixture(t) // sets up git for the test
	r := &twoRepos{dir: filepath.Join(f.dir, "two")}
	r.shop, r.protos = filepath.Join(r.dir, "shop"), filepath.Join(r.dir, "protos")
	for repo, files := range map[string]map[string]string{
		r.shop: {
			"services/orders/create.py":   "def create(): pass\n",
			"services/inventory/stock.py": "def reserve(): pass\n",
			"libs/money/money.py":         "def add(): pass\n",
		},
		r.protos: {"orders/order.proto": "message Order {}\n", "README.md": "# protos\n"},
	} {
		for name, content := range files {
			write(t, repo, name, content)
		}
		git(t, repo, "init", "--quiet", "--initial-branch=main")
		git(t, repo, "add", "--all")
		git(t, repo, "commit", "--quiet", "-m", "initial")
	}
	topo, err := topology.Parse([]byte(twoRepoTopology), r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.m, err = New(topo); err != nil {
		t.Fatal(err)
	}
	return r
}

// describe gives orders one feature in each repo, and marks the map
// current.
func (r *twoRepos) describe(t *testing.T) []string {
	t.Helper()
	_, err := r.m.ChangeWiki("orders", false, []wiki.Write{
		{Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.", Body: body("A customer places an order.")},
		{Op: "create", ID: "order-message", Name: "Order message", Description: "The order on the wire.", Body: body("Other services read an order.")},
	})
	must(t, err)
	write(t, r.shop, "services/orders/.hikari/private/code-map.yaml", `version: 1
parts: [{path: shop/services/orders, description: Orders.}, {path: protos, description: The messages.}]
features:
  create-order: {code: [shop/services/orders/create.py]}
  order-message: {code: [protos/order.proto]}
`)
	commits, err := r.m.MarkCurrent("orders")
	must(t, err)
	return commits
}

func TestFoldersOfTwoRepos(t *testing.T) {
	r := newTwoRepos(t)
	if _, err := os.Stat(filepath.Join(r.shop, "services/orders/.hikari/private/code-map.yaml")); err != nil {
		t.Errorf("F6: the private documents live in the first writable folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.protos, "orders/.hikari")); err == nil {
		t.Error("F6: only the first writable folder holds private documents")
	}
	commits := r.describe(t)
	shopHead, protosHead := git(t, r.shop, "rev-parse", "HEAD"), git(t, r.protos, "rev-parse", "HEAD")
	if want := []string{"shop@" + shopHead, "protos@" + protosHead}; !slices.Equal(commits, want) {
		t.Errorf("F8: described commits %v, want %v", commits, want)
	}
	if v, _ := r.m.OwnView("orders"); v.State != featuremap.Current || len(v.PrivateErrors) != 0 {
		t.Fatalf("after MarkCurrent: %s, %v", v.State, v.PrivateErrors)
	}
	doc, _ := os.ReadFile(filepath.Join(r.dir, "vault", "partitions", "orders", "read-only", "features", "create-order.md"))
	if !strings.Contains(string(doc), "described_commits: [shop@"+shopHead+", protos@"+protosHead+"]") {
		t.Errorf("F8: the document records a commit of each repo:\n%s", doc)
	}

	// F7: a change in each repo makes the feature of that repo stale, with
	// workspace paths; a change in the read-only folder makes nothing
	// stale.
	write(t, r.shop, "services/orders/create.py", "def create(): return 1\n")
	write(t, r.protos, "orders/order.proto", "message Order { string id = 1; }\n")
	write(t, r.shop, "libs/money/money.py", "def add(): return 1\n")
	tc := r.m.TurnContext("orders")
	stale := map[string][]string{}
	for _, f := range tc.StaleFeatures {
		stale[f.ID] = f.Paths
	}
	if !slices.Equal(stale["create-order"], []string{"shop/services/orders/create.py"}) || !slices.Equal(stale["order-message"], []string{"protos/order.proto"}) || len(tc.UnmappedPaths) != 0 {
		t.Errorf("F7: stale %v, unmapped %v", stale, tc.UnmappedPaths)
	}
	r.m.checkDirty(r.m.partitionAgents["orders"])
	if v := r.m.PartitionView("orders"); !v.Dirty || len(v.Repos) != 2 || !v.Repos[0].Dirty || !v.Repos[1].Dirty || v.Repos[1].Branch != "hikari/protos" {
		t.Errorf("F15: repos %+v", v.Repos)
	}

	// F9: an apply makes one commit in each repo whose folders changed,
	// with the agent as author and one message, and leaves the read-only
	// folder alone.
	got, err := r.m.Commit("orders", "Give orders an id")
	if err != nil || len(got) != 2 || !strings.HasPrefix(got[0], "shop@") || !strings.HasPrefix(got[1], "protos@") {
		t.Fatalf("F9: apply = %v, %v", got, err)
	}
	for _, repo := range []string{r.shop, r.protos} {
		if log := git(t, repo, "log", "-1", "--format=%an|%s"); log != "orders (hikari)|Give orders an id" {
			t.Errorf("F9: %s: %q", repo, log)
		}
	}
	if files := git(t, r.shop, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "libs/money") || strings.Contains(files, "inventory") {
		t.Errorf("F9: the commit takes only the writable folders:\n%s", files)
	}
	if status := git(t, r.shop, "status", "--porcelain"); !strings.Contains(status, "libs/money/money.py") {
		t.Errorf("the read-only folder's change stays: %q", status)
	}
	git(t, r.shop, "checkout", "--quiet", "--", "libs/money/money.py")

	// F9: a change in one repo commits in that repo only.
	write(t, r.protos, "orders/order.proto", "message Order { string id = 2; }\n")
	protosBefore, shopBefore := git(t, r.protos, "rev-list", "--count", "HEAD"), git(t, r.shop, "rev-list", "--count", "HEAD")
	got, err = r.m.Commit("orders", "")
	if err != nil || len(got) != 1 || !strings.HasPrefix(got[0], "protos@") {
		t.Fatalf("F9: apply of one repo = %v, %v", got, err)
	}
	if git(t, r.shop, "rev-list", "--count", "HEAD") != shopBefore || git(t, r.protos, "rev-list", "--count", "HEAD") == protosBefore {
		t.Error("F9: no empty commit in the unchanged repo")
	}
	if got, err := r.m.Commit("orders", "nothing"); err != nil || len(got) != 0 {
		t.Errorf("F9: an apply without changes = %v, %v", got, err)
	}
	if sum := r.m.Summary(); !slices.Contains(sum, `orders made 2 commit(s) on hikari/protos in repo "protos"`) {
		t.Errorf("summary = %v", sum)
	}

	// F12: the orchestration agent sees the folders by place, with no host
	// path.
	p, err := r.m.GetPartition("lead", "orders")
	if err != nil || len(p.Folders) != 3 || p.Folders[2].Place != "protos" || !p.Folders[1].ReadOnly {
		t.Errorf("F12: %+v, %v", p.Folders, err)
	}
	if self := r.m.Self("orders"); len(self.Repos) != 2 || self.Folders[0].Place != "shop/services/orders" {
		t.Errorf("F11: %+v", self)
	}
	if repos := r.m.Repos(); len(repos) != 2 || repos[0].Name != "protos" || !slices.Equal(repos[0].Partitions, []string{"orders"}) || !slices.Equal(repos[1].Partitions, []string{"inventory", "orders"}) {
		t.Errorf("repos = %+v", repos)
	}
}

// TestOneCommitMap reads a map and a document of the format before, with
// one described commit, as a partition of the repo of its first writable
// folder (rule F8).
func TestOneCommitMap(t *testing.T) {
	r := newTwoRepos(t)
	r.describe(t)
	shopHead := git(t, r.shop, "rev-parse", "HEAD")
	features := filepath.Join(r.dir, "vault", "partitions", "orders", "read-only", "features")
	mapFile := filepath.Join(features, "map.yaml")
	data, _ := os.ReadFile(mapFile)
	old := strings.Replace(string(data), "version: 5", "version: 4", 1)
	start := strings.Index(old, "  described_state:\n")
	end := strings.Index(old[start:], "    updated:")
	old = old[:start] + "  described_state:\n    commit: " + shopHead + "\n" + old[start+end:]
	if err := os.WriteFile(mapFile, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	docFile := filepath.Join(features, "create-order.md")
	doc, _ := os.ReadFile(docFile)
	oldDoc := replaceSpan(string(doc), "described_commits: [", "]", "described_commit: "+shopHead)
	if err := os.WriteFile(docFile, []byte(oldDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	if errs := r.m.CommitAll(); len(errs) != 0 {
		t.Fatal(errs)
	}
	topo, err := topology.Parse([]byte(twoRepoTopology), r.dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := m.OwnView("orders")
	if !slices.Equal(v.Partition.DescribedState.Commits, []string{"shop@" + shopHead}) {
		t.Errorf("F8: the one commit belongs to shop: %v", v.Partition.DescribedState.Commits)
	}
	if !slices.ContainsFunc(m.Warnings(), func(w string) bool { return strings.Contains(w, "converted") }) {
		t.Errorf("F8: a note says the map was converted: %v", m.Warnings())
	}
	if !slices.ContainsFunc(v.Changed, func(p string) bool { return strings.Contains(p, `no commit of repo "protos"`) }) {
		t.Errorf("F7: a repo without a described commit asks for a description: %v", v.Changed)
	}
	if d := m.wikis["partition:orders"].files.Wiki()["create-order"]; !slices.Equal(d.DescribedCommits, []string{"shop@" + shopHead}) {
		t.Errorf("F8: the document's one commit belongs to shop: %v", d.DescribedCommits)
	}
	commits, err := m.MarkCurrent("orders")
	if err != nil || len(commits) != 2 {
		t.Fatalf("MarkCurrent = %v, %v", commits, err)
	}
	if data, _ := os.ReadFile(mapFile); !strings.Contains(string(data), "commits:") || strings.Contains(string(data), "commit: ") {
		t.Errorf("F8: the next change writes the new form:\n%s", data)
	}
}

// replaceSpan replaces the text from the first from up to the next to,
// both included, with with.
func replaceSpan(s, from, to, with string) string {
	i := strings.Index(s, from)
	j := strings.Index(s[i:], to)
	return s[:i] + with + s[i+j+len(to):]
}
