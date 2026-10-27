package topology

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// renamed has a renamed project, repo, partition agent, and orchestration agent. The
// references in the file name ids (rule ID5).
func renamed(repo string) string {
	return `name: shop
x-hikari:
  name: Shop Floor
  repos:
    shop: {name: storefront, path: ` + repo + `, branch: hikari}
  orchestration: lead
services:
  lead: {x-hikari: {name: chief}}
  orders:
    environment: {ANTHROPIC_API_KEY: key}
    x-hikari:
      name: intake
      partition: {repo: shop, scope: orders}
      grants: {releases: [inventory]}
  inventory:
    x-hikari:
      partition: {repo: shop, scope: inventory}
  cache: {image: redis, x-hikari: {serves: orders}}
`
}

func TestNames(t *testing.T) {
	repo := fakeRepo(t, "orders", "inventory")
	topo, err := Parse([]byte(renamed(repo)), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if topo.ID != "shop" || topo.Name != "Shop Floor" {
		t.Errorf("project id %q, name %q", topo.ID, topo.Name)
	}
	if got := topo.AgentNames(); !slices.Equal(got, []string{"chief", "intake", "inventory"}) {
		t.Errorf("agents are keyed by name: %v", got)
	}
	if a := topo.Agents["intake"]; a.ID != "orders" || a.Name != "intake" {
		t.Errorf("intake: id %q, name %q", a.ID, a.Name)
	}
	if a := topo.Agents["inventory"]; a.ID != "inventory" || a.Name != "inventory" {
		t.Errorf("an agent without a name field is named by its id: %+v", a)
	}
	if topo.Orchestration != "chief" {
		t.Errorf("x-hikari.orchestration resolves to the agent's name, got %q", topo.Orchestration)
	}
	if r := topo.Repos["storefront"]; r.ID != "shop" || r.Path != repo {
		t.Errorf("storefront: %+v", r)
	}
	if got := topo.Agents["intake"].Partition.Repo; got != "storefront" {
		t.Errorf("partition.repo resolves to the repo's name, got %q", got)
	}
	if got := topo.Agents["intake"].Grants.Releases; !slices.Equal(got, []string{"inventory"}) {
		t.Errorf("grants: %v", got)
	}
	if got := topo.Services["cache"].Serves; got != "intake" {
		t.Errorf("serves resolves to the agent's name, got %q", got)
	}
	if got := topo.Sidecars("intake"); !slices.Equal(got, []string{"cache"}) {
		t.Errorf("Sidecars(intake) = %v", got)
	}
	if _, ok := topo.Services["orders"]; !ok {
		t.Errorf("services stay keyed by service key: %v", slices.Sorted(maps.Keys(topo.Services)))
	}
	if !topo.ServiceOf("intake").Credentials() {
		t.Error("ServiceOf finds the service of a renamed agent by its id")
	}
	if got, ok := topo.AgentOfService("orders"); !ok || got != "intake" {
		t.Errorf("AgentOfService(orders) = %q, %v", got, ok)
	}
	if got := topo.Participants(); !slices.Equal(got, []string{"chief", "intake", "inventory", "user"}) {
		t.Errorf("Participants() = %v", got)
	}
	if got := topo.VisiblePartitions("chief"); !slices.Equal(got, []string{"intake", "inventory"}) {
		t.Errorf("VisibleSystems(chief) = %v", got)
	}

	st := topo.VaultOf()
	if got := st.PartitionDir("intake"); got != filepath.Join(topo.Vault, "systems", "orders") {
		t.Errorf("SystemDir(intake) = %s", got)
	}
	if got := st.PublicWiki(); got != filepath.Join(topo.Vault, "shared", "wiki") {
		t.Errorf("PublicWiki() = %s", got)
	}
	if got := st.VocabularyFile(); got != filepath.Join(topo.Vault, "shared", "vocabulary", "terms.yaml") {
		t.Errorf("VocabularyFile() = %s", got)
	}
	if st.PartitionName("orders") != "intake" {
		t.Error("the vault maps ids back to names")
	}
}

func TestNamesDefaultToIDs(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	topo, err := Parse([]byte(partitionsTopology(repo, twoPartitionAgents)), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if topo.Name != topo.ID {
		t.Errorf("project name %q, id %q", topo.Name, topo.ID)
	}
	for name, a := range topo.Agents {
		if a.ID != name || a.Name != name {
			t.Errorf("agent %q: %+v", name, a)
		}
	}
}

func TestLinkedRepoCloneIsKeyedByID(t *testing.T) {
	dir := fakeRepo(t)
	src := `name: p
x-hikari:
  project: {branch: hikari}
  repos:
    app: {name: web, url: https://example.com/app.git}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  dev: {x-hikari: {partition: {repo: app, scope: src}}}
`
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Repos["web"].Path; got != filepath.Join(dir, CloneDir, "app") {
		t.Errorf("the clone of web is %s, want repos/app (rule ID10)", got)
	}
	if !slices.Equal(topo.Missing, []string{"web"}) {
		t.Errorf("Missing = %v, want the repo's name", topo.Missing)
	}
}

func TestNameErrors(t *testing.T) {
	orch := "  orchestration: a"
	two := "  a: {x-hikari: {}}\n  b: {x-hikari: {}}"
	cases := map[string]struct{ hikari, services, want string }{
		"ID2 agent name":  {orch, "  a: {x-hikari: {name: Alpha}}\n  b: {x-hikari: {}}", `agent "a": invalid name "Alpha"`},
		"ID2 long name":   {orch, "  a: {x-hikari: {name: " + strings.Repeat("a", 33) + "}}\n  b: {x-hikari: {}}", "invalid name"},
		"ID2 project":     {"  name: \"" + strings.Repeat("x", 65) + "\"\n" + orch, two, "x-hikari.name"},
		"ID2 empty":       {"  name: \" \"\n" + orch, two, "x-hikari.name"},
		"ID3 sidecar":     {orch, two + "\n  db: {x-hikari: {serves: a, name: db}}", "its x-hikari holds only serves"},
		"ID4 user":        {orch, "  a: {x-hikari: {name: user}}\n  b: {x-hikari: {}}", `the name "user" is reserved`},
		"ID4 same name":   {orch, "  a: {x-hikari: {name: c}}\n  b: {x-hikari: {name: c}}", `agent "a" and agent "b" have the same name "c"`},
		"ID4 other's id":  {orch, "  a: {x-hikari: {name: b}}\n  b: {x-hikari: {name: z}}", `agent "a" is named "b", which is the id of agent "b"`},
		"ID5 name as ref": {"  orchestration: bee", "  a: {x-hikari: {}}\n  b: {x-hikari: {name: bee}}", `"bee" is not an agent's service id`},
		"ID4 repo name":   {"  repos: {r: {name: project, path: /nonexistent, branch: b}}\n" + orch, two, `the name "project" is reserved`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(compose(c.hikari, c.services)), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}
