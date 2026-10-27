package topology

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// compose returns a resolved compose file with the given top-level x-hikari
// body and services, both indented by two spaces.
func compose(hikari, services string) string {
	return "name: test\nx-hikari:\n" + hikari + "\nservices:\n" + services
}

// company: ceo orchestrates eng-lead. eng: eng-lead orchestrates dev.
var tree = compose(`
  hubs:
    company: {orchestrator: ceo, members: [eng-lead]}
    eng: {orchestrator: eng-lead, members: [dev]}`, `
  ceo: {image: hikari-agent, x-hikari: {description: runs the company}}
  eng-lead: {image: hikari-agent, x-hikari: {}}
  dev: {image: hikari-agent, x-hikari: {}}
`)

func TestParse(t *testing.T) {
	topo, err := Parse([]byte(tree), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if topo.Name != "test" || topo.Agents["ceo"].Description != "runs the company" {
		t.Errorf("name %q, ceo %+v", topo.Name, topo.Agents["ceo"])
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
	hub := "  hubs: {h: {orchestrator: a}}"
	cases := map[string]struct{ src, want string }{
		"empty":            {``, "is empty"},
		"C1 no name":       {strings.Replace(compose(hub, "  a: {x-hikari: {}}"), "name: test\n", "", 1), "no top-level name"},
		"no x-hikari":      {"name: t\nservices: {a: {x-hikari: {}}}", "no top-level x-hikari"},
		"unknown key":      {compose(hub+"\n  agents: {}", "  a: {x-hikari: {}}"), "field agents not found"},
		"C2 plain service": {compose(hub, "  a: {x-hikari: {}}\n  db: {image: postgres}"), `service "db" has no x-hikari key`},
		"C2 both":          {compose(hub, "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: a, description: x}}"), "its x-hikari holds only serves"},
		"unknown field":    {compose(hub, "  a: {x-hikari: {packages: [python3]}}"), "field packages not found"},
		"C6 serves":        {compose(hub, "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: ghost}}"), `serves "ghost", which is not an agent`},
		"no hubs":          {compose("  store: s", "  a: {x-hikari: {}}"), "no hubs"},
		"reserved name":    {compose("  hubs: {h: {orchestrator: user}}", "  user: {x-hikari: {}}"), "reserved"},
		"C3 bad name":      {compose("  hubs: {h: {orchestrator: a_b}}", "  a_b: {x-hikari: {}}"), `invalid agent name "a_b"`},
		"no orchestrator":  {compose("  hubs: {h: {members: [a]}}", "  a: {x-hikari: {}}"), "no orchestrator"},
		"unknown member":   {compose("  hubs: {h: {orchestrator: a, members: [b]}}", "  a: {x-hikari: {}}"), `unknown agent "b"`},
		"sidecar member":   {compose("  hubs: {h: {orchestrator: a, members: [db]}}", "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: a}}"), `unknown agent "db"`},
		"duplicate member": {compose("  hubs: {h: {orchestrator: a, members: [a]}}", "  a: {x-hikari: {}}"), "more than once"},
		"name clash":       {compose("  hubs: {a: {orchestrator: a}}", "  a: {x-hikari: {}}"), "same name as an agent"},
		"unreachable":      {compose(hub, "  a: {x-hikari: {}}\n  b: {x-hikari: {}}"), `"b" is in no hub`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
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

func systemsTopology(repo, services string) string {
	return compose(`
  repos:
    shop: {path: `+repo+`, branch: hikari/work}
  hubs:
    h: {orchestrator: lead, members: [orders, inventory]}`, `
  lead: {x-hikari: {}}
`+services)
}

const twoOwners = `
  orders:
    x-hikari:
      system: {repo: shop, scope: services/orders, read: [proto], test: make test}
  inventory:
    x-hikari:
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
  orders: {x-hikari: {system: {repo: shop, scope: services/orders}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
`)), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Repos["shop"].Path; got != repo {
		t.Errorf("C5 repo path = %q, want %q", got, repo)
	}
}

const validOwners = `
  orders: {x-hikari: {system: {repo: shop, scope: services/orders}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
`

func TestSystemErrors(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	valid := validOwners
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
			strings.Replace(systemsTopology(repo, valid), "lead: {x-hikari: {}}", "lead: {x-hikari: {system: {repo: shop, scope: proto}}}", 1),
			"an orchestrator cannot own a system",
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
	src := systemsTopology(repo, validOwners)
	src = strings.Replace(src, "  repos:\n", "  repos:\n    spare: {path: "+repo+", branch: main}\n", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err == nil || !strings.Contains(err.Error(), `repo "spare" is not used by any system`) {
		t.Errorf("error = %v", err)
	}
}

func TestEveryViolationIsReported(t *testing.T) {
	src := compose("  hubs: {h: {orchestrator: ok}}", "  Bad: {x-hikari: {}}\n  ok: {x-hikari: {}, privileged: true}")
	_, err := Parse([]byte(src), t.TempDir())
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`invalid agent name "Bad"`, `agent "Bad" is in no hub`, `service "ok": sets privileged`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestStoreAndGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	dir := t.TempDir()
	src := systemsTopology(repo, `
  orders: {x-hikari: {system: {repo: shop, scope: services/orders}, grants: {releases: [inventory]}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
`)
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if topo.Store != filepath.Join(dir, "store") {
		t.Errorf("T10 default store = %q", topo.Store)
	}
	if got := topo.ReadableReleases("orders"); !slices.Equal(got, []string{"inventory"}) {
		t.Errorf("orders reads %v", got)
	}
	if got := topo.ReadableReleases("lead"); !slices.Equal(got, []string{"inventory", "orders"}) {
		t.Errorf("lead reads %v", got)
	}
	if got := topo.ReadableReleases("inventory"); len(got) != 0 {
		t.Errorf("inventory reads %v", got)
	}

	custom, err := Parse([]byte(strings.Replace(src, "x-hikari:\n", "x-hikari:\n  store: ../shared-store\n", 1)), dir)
	if err != nil || custom.Store != filepath.Join(filepath.Dir(dir), "shared-store") {
		t.Errorf("T10 relative store = %q, %v", custom.Store, err)
	}

	for name, c := range map[string]struct{ services, want string }{
		"T11 own releases":  {strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {releases: [orders]}", 1), "cannot be granted its own releases"},
		"T11 not a system":  {strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {releases: [lead]}", 1), `release grant "lead" is not a system`},
		"T11 unknown agent": {strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {releases: [ghost]}", 1), `release grant "ghost" is not a system`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(systemsTopology(repo, c.services)), dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestDeliverableGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	dir := t.TempDir()
	src := systemsTopology(repo, strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {deliverables: [python-library, docs]}", 1))
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Agents["orders"].Grants.Deliverables; !slices.Equal(got, []string{"python-library", "docs"}) {
		t.Errorf("T13 deliverables = %v", got)
	}
	for name, c := range map[string]struct{ services, want string }{
		"T13 bad name":  {strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {deliverables: [Python_Lib]}", 1), `deliverable "Python_Lib" is not a valid name`},
		"T13 twice":     {strings.Replace(validOwners, "services/orders}", "services/orders}, grants: {deliverables: [lib, lib]}", 1), `deliverable "lib" is granted twice`},
		"T14 not owner": {validOwners + "  writer: {x-hikari: {grants: {deliverables: [lib]}}}\n", `only an agent with a system can hold deliverables`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(systemsTopology(repo, c.services)), dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestMemberAndOrchestratorGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	src := strings.Replace(systemsTopology(repo, validOwners), "lead: {x-hikari: {}}", "lead: {x-hikari: {grants: {releases: [orders]}}}", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err != nil {
		t.Errorf("T12: an orchestrator can have grants: %v", err)
	}
}

func TestServices(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	src := systemsTopology(repo, `
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    user: agent
    environment: {CLAUDE_CODE_OAUTH_TOKEN: secret, UNSET: null}
    networks: {default: null}
    volumes:
      - {type: volume, source: venv, target: /workspace/services/orders/.venv}
      - {type: tmpfs, target: /workspace/services/orders/tmp}
      - {type: bind, source: /data/sets, target: /data, read_only: true}
    x-hikari: {system: {repo: shop, scope: services/orders}}
  orders-db: {image: postgres:17, x-hikari: {serves: orders}}
  inventory: {image: hikari-agent, x-hikari: {system: {repo: shop, scope: services/inventory}}}
`)
	topo, err := Parse([]byte(src), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orders := topo.Services["orders"]
	if orders.Image != "test-orders" || !orders.Build || orders.User != "agent" {
		t.Errorf("orders = %+v; a built image is named <project>-<service>", orders)
	}
	if !orders.Credentials() || topo.Services["inventory"].Credentials() {
		t.Error("R1: orders has a credential and inventory has none")
	}
	if got := topo.Sidecars("orders"); !slices.Equal(got, []string{"orders-db"}) {
		t.Errorf("Sidecars(orders) = %v", got)
	}
	if _, ok := topo.Agents["orders-db"]; ok {
		t.Error("a sidecar is not an agent")
	}
	if got := topo.NamedVolumes("orders"); len(got) != 1 || got[0].Source != "venv" {
		t.Errorf("NamedVolumes(orders) = %v", got)
	}
}

func TestServiceErrors(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	store := t.TempDir()
	withOrders := func(keys string) string {
		src := systemsTopology(repo, strings.Replace(validOwners, "  orders: {x-hikari:", "  orders: {"+keys+", x-hikari:", 1))
		return strings.Replace(src, "x-hikari:\n", "x-hikari:\n  store: "+store+"\n", 1)
	}
	cases := map[string]struct{ src, want string }{
		"C7 entrypoint":     {withOrders("entrypoint: [sh]"), "sets entrypoint, which hikari controls"},
		"C7 command":        {withOrders("command: [run]"), "sets command"},
		"C7 working_dir":    {withOrders("working_dir: /src"), "sets working_dir"},
		"C7 networks":       {withOrders("networks: {default: null, shared: null}"), "sets networks"},
		"C7 network_mode":   {withOrders("network_mode: host"), "sets network_mode"},
		"C7 privileged":     {withOrders("privileged: true"), "sets privileged"},
		"C7 cap_add":        {withOrders("cap_add: [SYS_ADMIN]"), "sets cap_add"},
		"C7 devices":        {withOrders("devices: [/dev/sda]"), "sets devices"},
		"C7 pid":            {withOrders("pid: host"), "sets pid"},
		"C7 volumes_from":   {withOrders("volumes_from: [inventory]"), "sets volumes_from"},
		"C8 repo":           {withOrders("volumes: [{type: bind, source: " + repo + "/services/inventory, target: /inv}]"), "overlaps " + realPath(repo)},
		"C8 parent of repo": {withOrders("volumes: [{type: bind, source: " + filepath.Dir(repo) + ", target: /all}]"), "hikari mounts the repos and the store itself"},
		"C8 store":          {withOrders("volumes: [{type: bind, source: " + store + "/shared, target: /s}]"), "overlaps " + realPath(store)},
		"C8 docker socket":  {withOrders("volumes: [{type: bind, source: /var/run/docker.sock, target: /var/run/docker.sock}]"), "mounts the Docker socket"},
		"C9 hikari":         {withOrders("volumes: [{type: volume, source: v, target: /hikari/bin}]"), "hikari owns /hikari"},
		"C9 bind in scope":  {withOrders("volumes: [{type: bind, source: /data, target: /workspace/services/orders/data}]"), "only named volumes and tmpfs"},
		"C9 other scope":    {withOrders("volumes: [{type: volume, source: v, target: /workspace/services/inventory/x}]"), "only named volumes and tmpfs"},
		"C9 over the scope": {withOrders("volumes: [{type: volume, source: v, target: /workspace/services/orders}]"), "only named volumes and tmpfs"},
		"C9 private":        {withOrders("volumes: [{type: volume, source: v, target: /workspace/services/orders/.hikari/x}]"), "only named volumes and tmpfs"},
		"C10 HIKARI_":       {withOrders("environment: {HIKARI_TOKEN: x}"), "sets HIKARI_TOKEN"},
		"C14 sidecar net":   {withOrders("image: x") + "  db: {network_mode: host, x-hikari: {serves: orders}}\n", `service "db": sets network_mode`},
		"C15 sidecar bind":  {withOrders("image: x") + "  db: {volumes: [{type: bind, source: " + repo + ", target: /r}], x-hikari: {serves: orders}}\n", `service "db": mounts`},
		"prepare clash":     {withOrders("volumes: [{type: volume, source: v, target: /workspace/services/orders/v}]") + "  orders-prepare: {x-hikari: {serves: orders}}\n", "taken by the prepare service"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}

	// A member without a system has no scope, so nothing of /workspace.
	src := strings.Replace(systemsTopology(repo, validOwners), "lead: {x-hikari: {}}", "lead: {volumes: [{type: volume, source: v, target: /workspace/x}], x-hikari: {}}", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err == nil || !strings.Contains(err.Error(), "only named volumes and tmpfs") {
		t.Errorf("C9 member: %v", err)
	}
	// A sidecar may run its own command.
	if _, err := Parse([]byte(withOrders("image: x")+"  db: {command: [postgres], x-hikari: {serves: orders}}\n"), t.TempDir()); err != nil {
		t.Errorf("a sidecar may set command: %v", err)
	}
}

func TestComposeMessages(t *testing.T) {
	stderr := `time="2026-10-21T16:43:29-07:00" level=warning msg="The \"TOKEN\" variable is not set. Defaulting to a blank string."
WARN[0000] plain warning
`
	got := composeMessages(stderr)
	want := []string{`The "TOKEN" variable is not set. Defaulting to a blank string.`, "WARN[0000] plain warning"}
	if !slices.Equal(got, want) {
		t.Errorf("composeMessages = %q", got)
	}
}

// projectTopology is a topology in a project repository at dir, with the
// given extra hikari keys.
func projectTopology(extra, services string) string {
	return compose(`
  project: {branch: hikari}`+extra+`
  hubs:
    h: {orchestrator: lead, members: [site]}`, `
  lead: {x-hikari: {}}
`+services)
}

const projectSite = "  site: {x-hikari: {system: {repo: project, scope: site}}}\n"

func TestProject(t *testing.T) {
	dir := fakeRepo(t, "site", "config")
	topo, err := Parse([]byte(projectTopology("", projectSite+"  db: {volumes: [{type: bind, source: "+dir+"/config, target: /c}], x-hikari: {serves: site}}\n")), dir)
	if err != nil {
		t.Fatal(err)
	}
	if topo.Project == nil || topo.Project.Path != dir || topo.Project.Branch != "hikari" {
		t.Fatalf("PR1 project = %+v", topo.Project)
	}
	if r := topo.Repos[ProjectRepo]; r.Path != dir || r.Branch != "hikari" {
		t.Errorf("the project is not among the repos: %+v", topo.Repos)
	}
	if got := topo.StoreInProject(); got != "store" {
		t.Errorf("StoreInProject() = %q", got)
	}

	// PR6: the project repository need not hold a system.
	if _, err := Parse([]byte(projectTopology("", "  site: {x-hikari: {}}\n")), dir); err != nil {
		t.Errorf("PR6: %v", err)
	}
	// Without the key there is no project, and the store is anywhere.
	plain, err := Parse([]byte(tree), dir)
	if err != nil || plain.Project != nil || plain.StoreInProject() != "" {
		t.Errorf("no project key: %+v, %v", plain.Project, err)
	}
}

func TestProjectErrors(t *testing.T) {
	dir := fakeRepo(t, "site", "store/x")
	outside := t.TempDir()
	cases := map[string]struct{ src, dir, want string }{
		"PR1 bad branch":       {strings.Replace(projectTopology("", projectSite), "branch: hikari", "branch: a..b", 1), dir, `"a..b" is not a valid branch name`},
		"PR1 unknown field":    {strings.Replace(projectTopology("", projectSite), "{branch: hikari}", "{branch: hikari, path: x}", 1), dir, "field path not found"},
		"PR2 not a repo":       {projectTopology("", projectSite), outside, "is not the top directory of a git repository"},
		"PR3 store outside":    {projectTopology("\n  store: "+outside, projectSite), dir, "must be a directory inside the project repository"},
		"PR3 store is the top": {projectTopology("\n  store: .", projectSite), dir, "must be a directory inside the project repository"},
		"PR4 reserved name":    {projectTopology("\n  repos: {project: {path: "+dir+", branch: x}}", projectSite), dir, `"project" is reserved`},
		"PR4 no project key":   {strings.Replace(projectTopology("", projectSite), "  project: {branch: hikari}", "  store: s", 1), dir, "which needs x-hikari.project"},
		"PR5 whole repo":       {projectTopology("", "  site: {x-hikari: {system: {repo: project, scope: .}}}\n"), dir, "needs a scope that does not overlap the store"},
		"PR5 in the store":     {projectTopology("", "  site: {x-hikari: {system: {repo: project, scope: store/x}}}\n"), dir, "needs a scope that does not overlap the store"},
		"PR7 scope":            {projectTopology("", projectSite+"  db: {volumes: [{type: bind, source: "+dir+"/site, target: /e}], x-hikari: {serves: site}}\n"), dir, "hikari mounts the repos and the store itself"},
		"PR7 git":              {projectTopology("", projectSite+"  db: {volumes: [{type: bind, source: "+dir+"/.git, target: /g}], x-hikari: {serves: site}}\n"), dir, "overlaps"},
		"PR7 whole project":    {projectTopology("", projectSite+"  db: {volumes: [{type: bind, source: "+dir+", target: /p}], x-hikari: {serves: site}}\n"), dir, "overlaps"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), c.dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestHubRelations(t *testing.T) {
	topo, err := Parse([]byte(tree), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.ChildHubs("company"); !slices.Equal(got, []string{"eng"}) {
		t.Errorf("ChildHubs(company) = %v", got)
	}
	if got := topo.ParentHubs("eng"); !slices.Equal(got, []string{"company"}) {
		t.Errorf("ParentHubs(eng) = %v", got)
	}
	if got := topo.MemberHubs("eng-lead"); !slices.Equal(got, []string{"company"}) {
		t.Errorf("MemberHubs(eng-lead) = %v", got)
	}
	if got := topo.ReadableHubWikis("ceo"); !slices.Equal(got, []string{"company", "eng"}) {
		t.Errorf("A2 ReadableHubWikis(ceo) = %v", got)
	}
	if got := topo.ReadableHubWikis("dev"); !slices.Equal(got, []string{"eng"}) {
		t.Errorf("A1 ReadableHubWikis(dev) = %v", got)
	}
}

// linkedTopology is a project with the linked repo app, owned by dev.
func linkedTopology(repo string) string {
	return compose(`
  project: {branch: hikari-alex}
  repos:
    app: `+repo+`
  hubs:
    h: {orchestrator: lead, members: [dev]}`, `
  lead: {x-hikari: {}}
  dev: {x-hikari: {system: {repo: app, scope: src}}}
`)
}

func TestLinkedRepo(t *testing.T) {
	dir := fakeRepo(t)
	topo, err := Parse([]byte(linkedTopology("{url: https://example.com/app.git}")), dir)
	if err != nil {
		t.Fatal(err)
	}
	r := topo.Repos["app"]
	if r.Path != filepath.Join(dir, "repos", "app") || r.Branch != "hikari-alex" || r.URL != "https://example.com/app.git" {
		t.Errorf("LR2, LR4 app = %+v", r)
	}
	if !slices.Equal(topo.Missing, []string{"app"}) {
		t.Errorf("LR6 Missing = %v", topo.Missing)
	}
	if !slices.Equal(topo.Linked(), []string{"app"}) {
		t.Errorf("Linked() = %v", topo.Linked())
	}

	// Once cloned, the scope is checked.
	if err := os.MkdirAll(filepath.Join(dir, "repos", "app", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]byte(linkedTopology("{url: https://example.com/app.git}")), dir); err == nil || !strings.Contains(err.Error(), `scope "src" is not a directory`) {
		t.Errorf("a cloned repo's scopes are checked: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "repos", "app", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	topo, err = Parse([]byte(linkedTopology("{url: https://example.com/app.git, branch: own}")), dir)
	if err != nil || len(topo.Missing) != 0 || topo.Repos["app"].Branch != "own" {
		t.Errorf("a cloned repo with its own branch: %+v, %v", topo, err)
	}
}

func TestLinkedRepoErrors(t *testing.T) {
	dir := fakeRepo(t, "site")
	if err := os.MkdirAll(filepath.Join(dir, "repos", "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	noProject := strings.Replace(linkedTopology("{url: https://example.com/app.git}"), "  project: {branch: hikari-alex}\n", "", 1)
	cases := map[string]struct{ src, want string }{
		"LR1 both":        {linkedTopology("{url: https://example.com/app.git, path: /x}"), "has both url and path"},
		"LR1 neither":     {linkedTopology("{branch: hikari}"), "has neither url nor path"},
		"LR2 no project":  {noProject, "needs x-hikari.project"},
		"LR3 dash":        {linkedTopology(`{url: "--upload-pack=x"}`), "is not a valid url"},
		"LR3 space":       {linkedTopology(`{url: "a b"}`), "is not a valid url"},
		"LR4 path branch": {linkedTopology("{path: " + dir + "}"), `"" is not a valid branch name`},
		"LR5 clone folder": {strings.Replace(linkedTopology("{url: https://example.com/app.git}"), "  lead: {x-hikari: {}}\n",
			"  lead: {x-hikari: {}}\n  site: {x-hikari: {system: {repo: project, scope: repos}}}\n", 1), "does not overlap the clone folder"},
		"not a clone": {strings.Replace(linkedTopology("{url: https://example.com/app.git}"), "app: {url", "junk: {url", 1), "exists but is not the top directory of a git repo"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestUnusedLinkedRepoIsAWarning(t *testing.T) {
	dir := fakeRepo(t)
	src := compose(`
  project: {branch: hikari}
  repos:
    app: {url: https://example.com/app.git}
  hubs:
    h: {orchestrator: lead}`, `
  lead: {x-hikari: {}}
`)
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(topo.Warnings, "the linked repo app is not used by any system yet") {
		t.Errorf("warnings = %v", topo.Warnings)
	}
}

func TestDraft(t *testing.T) {
	dir := fakeRepo(t)
	src := compose(`
  project: {branch: hikari}
  repos:
    app: {url: https://example.com/app.git}
    lib: {path: `+filepath.Join(dir, "nowhere")+`, branch: hikari}
  hubs:
    h: {orchestrator: lead, members: [api, util]}`, `
  lead: {x-hikari: {}}
  api: {x-hikari: {system: {repo: app, scope: api}}}
  util: {x-hikari: {system: {repo: lib, scope: src, read: [proto]}}}
`)
	if _, err := Parse([]byte(src), dir); err == nil || !strings.Contains(err.Error(), "is not the top directory of a git repo") {
		t.Errorf("a path repo that does not exist is an error: %v", err)
	}
	if _, err := parse([]byte(src), dir, true); err != nil {
		t.Errorf("DR1 a draft names repos and scopes that do not exist yet: %v", err)
	}
	bad := strings.Replace(src, "scope: src", "scope: ../src", 1)
	if _, err := parse([]byte(bad), dir, true); err == nil || !strings.Contains(err.Error(), "must be a relative path") {
		t.Errorf("DR1 a draft keeps the other rules: %v", err)
	}
}
