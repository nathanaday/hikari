package topology

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// renamed has a renamed project, repo, hub, owner, and orchestrator. The
// references in the file name ids (rule ID5).
func renamed(repo string) string {
	return `name: shop
x-hikari:
  name: Shop Floor
  repos:
    shop: {name: storefront, path: ` + repo + `, branch: hikari}
  hubs:
    eng: {name: platform, orchestrator: lead, members: [orders, inventory]}
    ops: {orchestrator: inventory-lead, members: [lead]}
services:
  lead: {x-hikari: {name: chief}}
  inventory-lead: {x-hikari: {}}
  orders:
    environment: {ANTHROPIC_API_KEY: key}
    x-hikari:
      name: intake
      system: {repo: shop, scope: orders}
      grants: {releases: [inventory]}
  inventory:
    x-hikari:
      system: {repo: shop, scope: inventory}
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
	if got := topo.AgentNames(); !slices.Equal(got, []string{"chief", "intake", "inventory", "inventory-lead"}) {
		t.Errorf("agents are keyed by name: %v", got)
	}
	if a := topo.Agents["intake"]; a.ID != "orders" || a.Name != "intake" {
		t.Errorf("intake: id %q, name %q", a.ID, a.Name)
	}
	if a := topo.Agents["inventory"]; a.ID != "inventory" || a.Name != "inventory" {
		t.Errorf("an agent without a name field is named by its id: %+v", a)
	}
	if got := topo.HubNames(); !slices.Equal(got, []string{"ops", "platform"}) {
		t.Errorf("hubs are keyed by name: %v", got)
	}
	h := topo.Hubs["platform"]
	if h.ID != "eng" || h.Orchestrator != "chief" || !slices.Equal(h.Members, []string{"intake", "inventory"}) {
		t.Errorf("platform: %+v", h)
	}
	if got := topo.Hubs["ops"].Members; !slices.Equal(got, []string{"chief"}) {
		t.Errorf("ops members: %v", got)
	}
	if r := topo.Repos["storefront"]; r.ID != "shop" || r.Path != repo {
		t.Errorf("storefront: %+v", r)
	}
	if got := topo.Agents["intake"].System.Repo; got != "storefront" {
		t.Errorf("system.repo resolves to the repo's name, got %q", got)
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
	if got := topo.Participants("platform"); !slices.Equal(got, []string{"chief", "intake", "inventory", "user"}) {
		t.Errorf("Participants(platform) = %v", got)
	}
	if !topo.CanDirect("intake", "chief") || topo.CanDirect("intake", "inventory-lead") {
		t.Error("contacts follow the hubs by name")
	}

	st := topo.StoreOf()
	if got := st.SystemDir("intake"); got != filepath.Join(topo.Store, "systems", "orders") {
		t.Errorf("SystemDir(intake) = %s", got)
	}
	if got := st.HubWiki("platform"); got != filepath.Join(topo.Store, "hubs", "eng", "wiki") {
		t.Errorf("HubWiki(platform) = %s", got)
	}
	if got := st.VocabularyFile("platform"); got != filepath.Join(topo.Store, "shared", "vocabulary", "eng.yaml") {
		t.Errorf("VocabularyFile(platform) = %s", got)
	}
	if st.SystemName("orders") != "intake" || st.HubName("eng") != "platform" {
		t.Error("the store maps ids back to names")
	}
}

func TestNamesDefaultToIDs(t *testing.T) {
	topo, err := Parse([]byte(tree), t.TempDir())
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
	for name, h := range topo.Hubs {
		if h.ID != name || h.Name != name {
			t.Errorf("hub %q: %+v", name, h)
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
  hubs: {main: {orchestrator: lead, members: [dev]}}
services:
  lead: {x-hikari: {}}
  dev: {x-hikari: {system: {repo: app, scope: src}}}
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
	hub := "  hubs: {h: {orchestrator: a, members: [b]}}"
	two := "  a: {x-hikari: {}}\n  b: {x-hikari: {}}"
	cases := map[string]struct{ hikari, services, want string }{
		"ID2 agent name":  {hub, "  a: {x-hikari: {name: Alpha}}\n  b: {x-hikari: {}}", `agent "a": invalid name "Alpha"`},
		"ID2 hub name":    {"  hubs: {h: {name: h_1, orchestrator: a, members: [b]}}", two, `hub "h": invalid name "h_1"`},
		"ID2 long name":   {hub, "  a: {x-hikari: {name: " + strings.Repeat("a", 33) + "}}\n  b: {x-hikari: {}}", "invalid name"},
		"ID2 project":     {"  name: \"" + strings.Repeat("x", 65) + "\"\n" + hub, two, "x-hikari.name"},
		"ID2 empty":       {"  name: \" \"\n" + hub, two, "x-hikari.name"},
		"ID3 sidecar":     {hub, two + "\n  db: {x-hikari: {serves: a, name: db}}", "its x-hikari holds only serves"},
		"ID4 user":        {hub, "  a: {x-hikari: {name: user}}\n  b: {x-hikari: {}}", `the name "user" is reserved`},
		"ID4 same name":   {hub, "  a: {x-hikari: {name: c}}\n  b: {x-hikari: {name: c}}", `agent "a" and agent "b" have the same name "c"`},
		"ID4 hub vs node": {"  hubs: {h: {name: a, orchestrator: a, members: [b]}}", "  a: {x-hikari: {name: x}}\n  b: {x-hikari: {}}", `hub "h" is named "a", which is the id of agent "a"`},
		"ID4 other's id":  {hub, "  a: {x-hikari: {name: b}}\n  b: {x-hikari: {name: z}}", `agent "a" is named "b", which is the id of agent "b"`},
		"ID5 name as ref": {"  hubs: {h: {orchestrator: a, members: [bee]}}", "  a: {x-hikari: {}}\n  b: {x-hikari: {name: bee}}", `unknown agent "bee"`},
		"ID4 repo name":   {"  repos: {r: {name: project, path: /nonexistent, branch: b}}\n" + hub, two, `the name "project" is reserved`},
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
