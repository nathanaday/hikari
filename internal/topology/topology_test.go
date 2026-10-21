package topology

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// company: ceo orchestrates eng-lead. eng: eng-lead orchestrates dev.
const tree = `
agents:
  ceo: {description: runs the company}
  eng-lead: {}
  dev: {}
hubs:
  company: {orchestrator: ceo, members: [eng-lead]}
  eng: {orchestrator: eng-lead, members: [dev]}
`

func TestParse(t *testing.T) {
	topo, err := Parse([]byte(tree), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Participants("eng"); !slices.Equal(got, []string{"eng-lead", "dev", "user"}) {
		t.Errorf("Participants(eng) = %v", got)
	}
	if got := topo.HubsOf("eng-lead"); !slices.Equal(got, []string{"company", "eng"}) {
		t.Errorf("HubsOf(eng-lead) = %v", got)
	}
	if got := topo.HubsOf(User); !slices.Equal(got, []string{"company", "eng"}) {
		t.Errorf("the user must be in every hub, got %v", got)
	}
	if got := topo.Contacts("eng-lead"); !slices.Equal(got, []string{"ceo", "dev", "user"}) {
		t.Errorf("Contacts(eng-lead) = %v", got)
	}
	if topo.CanDirect("dev", "ceo") {
		t.Error("dev and ceo share no hub")
	}
	if !topo.CanDirect(User, "dev") {
		t.Error("the user can reach every agent")
	}
	if topo.InHub("dev", "company") {
		t.Error("dev is not in company")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"empty":            ``,
		"old format":       "nodes: {a: {}}\nedges: []",
		"no hubs":          "agents: {a: {}}",
		"reserved name":    "agents: {user: {}}\nhubs: {h: {orchestrator: user}}",
		"bad name":         "agents: {Alice: {}}\nhubs: {h: {orchestrator: Alice}}",
		"no orchestrator":  "agents: {a: {}}\nhubs: {h: {members: [a]}}",
		"unknown member":   "agents: {a: {}}\nhubs: {h: {orchestrator: a, members: [b]}}",
		"duplicate member": "agents: {a: {}}\nhubs: {h: {orchestrator: a, members: [a]}}",
		"name clash":       "agents: {a: {}}\nhubs: {a: {orchestrator: a}}",
		"unreachable":      "agents: {a: {}, b: {}}\nhubs: {h: {orchestrator: a}}",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src), t.TempDir()); err == nil {
				t.Errorf("expected an error for %q", src)
			}
		})
	}
}

// fakeRepo makes a directory that looks like a git repo with the given
// subdirectories, and returns its path.
func fakeRepo(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range append(dirs, ".git") {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func systemsTopology(repo, agents string) string {
	return `
repos:
  shop: {path: ` + repo + `, branch: hikari/work}
agents:
  lead: {}
` + agents + `
hubs:
  h: {orchestrator: lead, members: [orders, inventory]}
`
}

const twoOwners = `
  orders:
    system: {repo: shop, scope: services/orders, read: [proto], test: make test, container_only: [node_modules]}
    packages: [python3, g++]
    path: [/usr/local/go/bin]
  inventory:
    system: {repo: shop, scope: ./services/inventory/}
`

func TestSystems(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	topo, err := Parse([]byte(systemsTopology(repo, twoOwners)), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Owners(); !slices.Equal(got, []string{"inventory", "orders"}) {
		t.Errorf("Owners() = %v", got)
	}
	if got := topo.Agents["inventory"].System.Scope; got != "services/inventory" {
		t.Errorf("scope is not cleaned: %q", got)
	}
	if got := topo.OrchestratedHubs("lead"); !slices.Equal(got, []string{"h"}) {
		t.Errorf("OrchestratedHubs(lead) = %v", got)
	}
	if got := topo.VisibleSystems("lead"); !slices.Equal(got, []string{"inventory", "orders"}) {
		t.Errorf("VisibleSystems(lead) = %v", got)
	}
	if got := topo.VisibleSystems("orders"); len(got) != 0 {
		t.Errorf("an owner sees no systems, got %v", got)
	}
}

func TestRelativeRepoPath(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "code", "shop")
	for _, d := range []string{".git", "services/orders", "services/inventory"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	topo, err := Parse([]byte(systemsTopology("code/shop", `
  orders: {system: {repo: shop, scope: services/orders}}
  inventory: {system: {repo: shop, scope: services/inventory}}
`)), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Repos["shop"].Path; got != repo {
		t.Errorf("repo path = %q, want %q", got, repo)
	}
}

func TestSystemErrors(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	valid := `
  orders: {system: {repo: shop, scope: services/orders}}
  inventory: {system: {repo: shop, scope: services/inventory}}
`
	cases := map[string]struct{ src, want string }{
		"T1 not a repo": {
			strings.Replace(systemsTopology(repo, valid), repo, t.TempDir(), 1),
			"is not the top directory of a git repo",
		},
		"T2 bad branch": {
			strings.Replace(systemsTopology(repo, valid), "hikari/work", "bad..branch", 1),
			"is not a valid branch name",
		},
		"T3 unknown repo": {
			systemsTopology(repo, strings.Replace(valid, "repo: shop, scope: services/orders", "repo: nope, scope: services/orders", 1)),
			`unknown repo "nope"`,
		},
		"T4 escaping scope": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/orders", "scope: ../elsewhere", 1)),
			"must be a relative path inside the repo",
		},
		"T4 missing scope": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/orders", "scope: services/missing", 1)),
			`scope "services/missing" is not a directory`,
		},
		"T4 absolute container_only": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/orders}", "scope: services/orders, container_only: [/abs]}", 1)),
			"container_only path",
		},
		"T5 nested scopes": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/inventory", "scope: services", 1)),
			"overlapping scopes",
		},
		"T5 whole-repo scope": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/inventory", "scope: .", 1)),
			"overlapping scopes",
		},
		"T6 read into another scope": {
			systemsTopology(repo, strings.Replace(valid, "scope: services/orders}", "scope: services/orders, read: [services/inventory]}", 1)),
			`read path "services/inventory" overlaps the scope of "inventory"`,
		},
		"T7 orchestrator owns a system": {
			strings.Replace(systemsTopology(repo, valid), "lead: {}", "lead: {system: {repo: shop, scope: proto}}", 1),
			"an orchestrator cannot own a system",
		},
		"T8 bad package": {
			systemsTopology(repo, strings.Replace(valid, "inventory: {system", "inventory: {packages: [Bad_Pkg], system", 1)),
			`invalid package name "Bad_Pkg"`,
		},
		"T8 relative path entry": {
			systemsTopology(repo, strings.Replace(valid, "inventory: {system", "inventory: {path: [bin], system", 1)),
			`path entry "bin" is not an absolute path`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestUnusedRepo(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	src := systemsTopology(repo, `
  orders: {system: {repo: shop, scope: services/orders}}
  inventory: {system: {repo: shop, scope: services/inventory}}
`)
	src = strings.Replace(src, "repos:\n", "repos:\n  spare: {path: "+repo+", branch: main}\n", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err == nil || !strings.Contains(err.Error(), `repo "spare" is not used by any system`) {
		t.Errorf("error = %v", err)
	}
}

func TestEveryViolationIsReported(t *testing.T) {
	src := "agents: {Bad: {}, ok: {packages: [X]}}\nhubs: {h: {orchestrator: ok}}"
	_, err := Parse([]byte(src), t.TempDir())
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`invalid agent name "Bad"`, `agent "Bad" is in no hub`, `invalid package name "X"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}
