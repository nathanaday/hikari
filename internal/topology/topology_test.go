package topology

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// compose returns a resolved topology file with the given top-level x-hikari
// body and services, both indented by two spaces.
func compose(hikari, services string) string {
	return "name: test\nx-hikari:\n" + hikari + "\nservices:\n" + services
}

func TestParse(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	topo, err := Parse([]byte(partitionsTopology(repo, twoPartitionAgents)), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if topo.Name != "test" || topo.Orchestration != "lead" {
		t.Errorf("name %q, orchestration agent %q", topo.Name, topo.Orchestration)
	}
	if got := topo.Participants(); !slices.Equal(got, []string{"lead", "inventory", "orders", "user"}) {
		t.Errorf("Participants() = %v", got)
	}
	if topo.Role("lead") != "orchestration" || topo.Role("orders") != "partition" || topo.Role(User) != "operator" {
		t.Error("roles")
	}

	// Without an orchestration agent, every agent has a partition.
	src := strings.Replace(partitionsTopology(repo, twoPartitionAgents), "  orchestration: lead\n", "", 1)
	src = strings.Replace(src, "  lead: {x-hikari: {}}\n", "", 1)
	topo, err = Parse([]byte(src), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Participants(); topo.Orchestration != "" || !slices.Equal(got, []string{"inventory", "orders", "user"}) {
		t.Errorf("Participants() = %v", got)
	}
}

func TestParseErrors(t *testing.T) {
	orch := "  orchestration: a"
	cases := map[string]struct{ src, want string }{
		"hubs":             {compose("  hubs: {h: {orchestrator: a}}", "  a: {x-hikari: {}}"), "x-hikari.hubs is gone"},
		"sidecar":          {compose("  orchestration: db", "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: a}}"), `"db" is not an agent's service id`},
		"user":             {compose("  orchestration: user", "  a: {x-hikari: {}}"), `"user" is not an agent's service id`},
		"empty":            {``, "is empty"},
		"C1 no name":       {strings.Replace(compose(orch, "  a: {x-hikari: {}}"), "name: test\n", "", 1), "no top-level name"},
		"no x-hikari":      {"name: t\nservices: {a: {x-hikari: {}}}", "no top-level x-hikari"},
		"unknown key":      {compose(orch+"\n  agents: {}", "  a: {x-hikari: {}}"), "field agents not found"},
		"C2 plain service": {compose(orch, "  a: {x-hikari: {}}\n  db: {image: postgres}"), `service "db" has no x-hikari key`},
		"C2 both":          {compose(orch, "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: a, description: x}}"), "its x-hikari holds only serves"},
		"unknown field":    {compose(orch, "  a: {x-hikari: {packages: [python3]}}"), "field packages not found"},
		"C6 serves":        {compose(orch, "  a: {x-hikari: {}}\n  db: {x-hikari: {serves: ghost}}"), `serves "ghost", which is not an agent`},
		"no partition":     {compose("  vault: s", "  a: {x-hikari: {}}"), `agent "a" has no partition and is not the orchestration agent`},
		"reserved name":    {compose("  orchestration: user", "  user: {x-hikari: {}}"), "reserved"},
		"C3 bad name":      {compose("  orchestration: a_b", "  a_b: {x-hikari: {}}"), `invalid agent name "a_b"`},
		"unreachable":      {compose(orch, "  a: {x-hikari: {}}\n  b: {x-hikari: {}}"), `agent "b" has no partition and is not the orchestration agent`},
		"MO3 unknown":      {compose(orch+"\n  modules: [tasks, chores]", "  a: {x-hikari: {}}"), `"chores" is not a module; the modules are tasks`},
		"MO3 twice":        {compose(orch+"\n  modules: [tasks, tasks]", "  a: {x-hikari: {}}"), `"tasks" is listed twice`},
		"hikari is taken":  {compose("  orchestration: hikari", "  hikari: {x-hikari: {}}"), `agent name "hikari" is reserved for hikari itself`},
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

func partitionsTopology(repo, services string) string {
	return compose(`
  repos:
    shop: {path: `+repo+`, branch: hikari/work}
  orchestration: lead`, `
  lead: {x-hikari: {}}
`+services)
}

const twoPartitionAgents = `
  orders:
    x-hikari:
      partition: {folders: [{repo: shop, path: services/orders}, {repo: shop, path: proto, read_only: true}], test: make test}
  inventory:
    x-hikari:
      partition: {folders: [{repo: shop, path: ./services/inventory/}]}
`

func TestPartitions(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	topo, err := Parse([]byte(partitionsTopology(repo, twoPartitionAgents)), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.PartitionAgents(); !slices.Equal(got, []string{"inventory", "orders"}) {
		t.Errorf("Partition agents() = %v", got)
	}
	if got := topo.Agents["inventory"].Partition.Folders[0]; got.Path != "services/inventory" || got.Place != "shop/services/inventory" {
		t.Errorf("folder is not cleaned: %+v", got)
	}
	if got := topo.Workspace("orders"); len(got) != 2 || got[1].Place != "shop/proto" || !got[1].ReadOnly || got.Home().Host != filepath.Join(repo, "services/orders") {
		t.Errorf("Workspace(orders) = %+v", got)
	}
	if got := topo.VisiblePartitions("lead"); !slices.Equal(got, []string{"inventory", "orders"}) {
		t.Errorf("VisibleSystems(lead) = %v", got)
	}
	if got := topo.VisiblePartitions("orders"); len(got) != 0 {
		t.Errorf("a partition agent sees no partitions, got %v", got)
	}
}

func TestRelativeRepoPath(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "code", "shop")
	for _, d := range []string{".git", "services/orders", "services/inventory"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	topo, err := Parse([]byte(partitionsTopology("code/shop", `
  orders: {x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}}}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`)), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Repos["shop"].Path; got != repo {
		t.Errorf("C5 repo path = %q, want %q", got, repo)
	}
}

const validPartitionAgents = `
  orders: {x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}}}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`

func TestPartitionErrors(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory", "proto")
	valid := validPartitionAgents
	cases := map[string]struct{ src, want string }{
		"T1 not a repo": {
			strings.Replace(partitionsTopology(repo, valid), repo, t.TempDir(), 1),
			"is not the top directory of a git repo",
		},
		"T2 bad branch": {
			strings.Replace(partitionsTopology(repo, valid), "hikari/work", "bad..branch", 1),
			"is not a valid branch name",
		},
		"T3 unknown repo": {
			partitionsTopology(repo, strings.Replace(valid, "repo: shop, path: services/orders", "repo: nope, path: services/orders", 1)),
			`unknown repo "nope"`,
		},
		"T4 escaping path": {
			partitionsTopology(repo, strings.Replace(valid, "path: services/orders", "path: ../elsewhere", 1)),
			"must be a relative path inside the repo",
		},
		"T4 missing path": {
			partitionsTopology(repo, strings.Replace(valid, "path: services/orders", "path: services/missing", 1)),
			`path "services/missing" is not a directory`,
		},
		"F3 nested writable folders": {
			partitionsTopology(repo, strings.Replace(valid, "path: services/inventory", "path: services", 1)),
			"writable folders",
		},
		"F3 whole-repo folder": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/inventory}", "{repo: shop}", 1)),
			"writable folders",
		},
		"F3 read-only folder in another partition's folder": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: services/orders}, {repo: shop, path: services/inventory, read_only: true}", 1)),
			`the read-only folder shop/services/inventory overlaps the writable folder shop/services/inventory of "inventory"`,
		},
		"F2 no writable folder": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: proto, read_only: true}", 1)),
			"at least one folder without read_only",
		},
		"F2 places nest": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: services/orders}, {repo: shop, path: proto, at: shop/services/orders/proto, read_only: true}", 1)),
			"which nest",
		},
		"F2 folders of one repo nest": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: services/orders}, {repo: shop, path: services/orders/x, at: x, read_only: true}", 1)),
			"one folder of a repo cannot hold another",
		},
		"F2 place in .hikari": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: services/orders}, {repo: shop, path: proto, at: .hikari/proto, read_only: true}", 1)),
			"which hikari keeps for itself",
		},
		"F1 escaping at": {
			partitionsTopology(repo, strings.Replace(valid, "{repo: shop, path: services/orders}", "{repo: shop, path: services/orders, at: ../x}", 1)),
			"must be a relative path under /workspace",
		},
		"M5 old store key": {
			strings.Replace(partitionsTopology(repo, valid), "  orchestration: lead", "  orchestration: lead\n  store: store", 1),
			"x-hikari.store is now x-hikari.vault",
		},
		"M5 old hubs key": {
			strings.Replace(partitionsTopology(repo, valid), "  orchestration: lead", "  hubs: {main: {orchestrator: lead, members: [orders]}}", 1),
			"docs/guides/migrating.md#the-orchestration-agent",
		},
		"M5 old system key": {
			partitionsTopology(repo, strings.Replace(valid, "partition: {folders: [{repo: shop, path: services/orders}]}", "system: {repo: shop, scope: services/orders}", 1)),
			"x-hikari.system is now x-hikari.partition",
		},
		"decision 5 old keys": {
			partitionsTopology(repo, strings.Replace(valid, "{folders: [{repo: shop, path: services/orders}]}", "{repo: shop, scope: services/orders}", 1)),
			"the partition uses the old keys repo, scope, and read",
		},
		"T7 orchestration agent has a partition": {
			strings.Replace(partitionsTopology(repo, valid), "lead: {x-hikari: {}}", "lead: {x-hikari: {partition: {folders: [{repo: shop, path: proto}]}}}", 1),
			"the orchestration agent cannot have a partition",
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
	src := partitionsTopology(repo, validPartitionAgents)
	src = strings.Replace(src, "  repos:\n", "  repos:\n    spare: {path: "+repo+", branch: main}\n", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err == nil || !strings.Contains(err.Error(), `repo "spare" is not used by any partition`) {
		t.Errorf("error = %v", err)
	}
}

func TestEveryViolationIsReported(t *testing.T) {
	src := compose("  orchestration: ok", "  Bad: {x-hikari: {}}\n  ok: {x-hikari: {}, privileged: true}")
	_, err := Parse([]byte(src), t.TempDir())
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`invalid agent name "Bad"`, `agent "Bad" has no partition and is not the orchestration agent`, `service "ok": sets privileged`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestVaultAndGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	dir := t.TempDir()
	src := partitionsTopology(repo, `
  orders: {x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}, grants: {releases: [inventory]}}}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`)
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if topo.Vault != filepath.Join(dir, "vault") {
		t.Errorf("T10 default vault = %q", topo.Vault)
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

	custom, err := Parse([]byte(strings.Replace(src, "x-hikari:\n", "x-hikari:\n  vault: ../shared-store\n", 1)), dir)
	if err != nil || custom.Vault != filepath.Join(filepath.Dir(dir), "shared-store") {
		t.Errorf("T10 relative vault = %q, %v", custom.Vault, err)
	}

	for name, c := range map[string]struct{ services, want string }{
		"T11 own releases":    {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {releases: [orders]}", 1), "cannot be granted its own releases"},
		"T11 not a partition": {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {releases: [lead]}", 1), `release grant "lead" is not a partition`},
		"T11 unknown agent":   {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {releases: [ghost]}", 1), `release grant "ghost" is not a partition`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(partitionsTopology(repo, c.services)), dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestDeliverableGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	dir := t.TempDir()
	src := partitionsTopology(repo, strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {deliverables: [python-library, docs]}", 1))
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := topo.Agents["orders"].Grants.Deliverables; !slices.Equal(got, []string{"python-library", "docs"}) {
		t.Errorf("T13 deliverables = %v", got)
	}
	for name, c := range map[string]struct{ services, want string }{
		"T13 bad name":            {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {deliverables: [Python_Lib]}", 1), `deliverable "Python_Lib" is not a valid name`},
		"T13 twice":               {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {deliverables: [lib, lib]}", 1), `deliverable "lib" is granted twice`},
		"T14 not partition agent": {validPartitionAgents + "  writer: {x-hikari: {grants: {deliverables: [lib]}}}\n", `only an agent with a partition can hold deliverables`},
		"V7 skill name":           {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {skills: [Style]}", 1), `skill "Style" is not a valid name`},
		"V7 skill twice":          {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {skills: [style, style]}", 1), `skill "style" is granted twice`},
		"V7 hikari skill":         {strings.Replace(validPartitionAgents, "services/orders}]}", "services/orders}]}, grants: {skills: [hikari-mine]}", 1), `the prefix hikari- is reserved`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(partitionsTopology(repo, c.services)), dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestPartitionAgentAndOrchestrationGrants(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	src := strings.Replace(partitionsTopology(repo, validPartitionAgents), "lead: {x-hikari: {}}", "lead: {x-hikari: {grants: {releases: [orders]}}}", 1)
	if _, err := Parse([]byte(src), t.TempDir()); err != nil {
		t.Errorf("T12: an orchestration agent can have grants: %v", err)
	}
}

func TestServices(t *testing.T) {
	repo := fakeRepo(t, "services/orders", "services/inventory")
	src := partitionsTopology(repo, `
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    user: agent
    environment: {CLAUDE_CODE_OAUTH_TOKEN: secret, UNSET: null}
    networks: {default: null}
    volumes:
      - {type: volume, source: venv, target: /workspace/shop/services/orders/.venv}
      - {type: tmpfs, target: /workspace/shop/services/orders/tmp}
      - {type: bind, source: /data/sets, target: /data, read_only: true}
    x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}}
  orders-db: {image: postgres:17, x-hikari: {serves: orders}}
  inventory: {image: hikari-agent, x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
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
	vault := t.TempDir()
	withOrders := func(keys string) string {
		src := partitionsTopology(repo, strings.Replace(validPartitionAgents, "  orders: {x-hikari:", "  orders: {"+keys+", x-hikari:", 1))
		return strings.Replace(src, "x-hikari:\n", "x-hikari:\n  vault: "+vault+"\n", 1)
	}
	cases := map[string]struct{ src, want string }{
		"C7 entrypoint":      {withOrders("entrypoint: [sh]"), "sets entrypoint, which hikari controls"},
		"C7 command":         {withOrders("command: [run]"), "sets command"},
		"C7 working_dir":     {withOrders("working_dir: /src"), "sets working_dir"},
		"C7 networks":        {withOrders("networks: {default: null, shared: null}"), "sets networks"},
		"C7 network_mode":    {withOrders("network_mode: host"), "sets network_mode"},
		"C7 privileged":      {withOrders("privileged: true"), "sets privileged"},
		"C7 cap_add":         {withOrders("cap_add: [SYS_ADMIN]"), "sets cap_add"},
		"C7 devices":         {withOrders("devices: [/dev/sda]"), "sets devices"},
		"C7 pid":             {withOrders("pid: host"), "sets pid"},
		"C7 volumes_from":    {withOrders("volumes_from: [inventory]"), "sets volumes_from"},
		"C8 repo":            {withOrders("volumes: [{type: bind, source: " + repo + "/services/inventory, target: /inv}]"), "overlaps " + realPath(repo)},
		"C8 parent of repo":  {withOrders("volumes: [{type: bind, source: " + filepath.Dir(repo) + ", target: /all}]"), "hikari mounts the repos and the vault itself"},
		"C8 vault":           {withOrders("volumes: [{type: bind, source: " + vault + "/shared, target: /s}]"), "overlaps " + realPath(vault)},
		"C8 docker socket":   {withOrders("volumes: [{type: bind, source: /var/run/docker.sock, target: /var/run/docker.sock}]"), "mounts the Docker socket"},
		"C9 hikari":          {withOrders("volumes: [{type: volume, source: v, target: /hikari/bin}]"), "hikari owns /hikari"},
		"C9 bind in folder":  {withOrders("volumes: [{type: bind, source: /data, target: /workspace/shop/services/orders/data}]"), "only named volumes and tmpfs"},
		"C9 other folder":    {withOrders("volumes: [{type: volume, source: v, target: /workspace/shop/services/inventory/x}]"), "only named volumes and tmpfs"},
		"C9 over the folder": {withOrders("volumes: [{type: volume, source: v, target: /workspace/shop/services/orders}]"), "only named volumes and tmpfs"},
		"C9 private":         {withOrders("volumes: [{type: volume, source: v, target: /workspace/shop/services/orders/.hikari/x}]"), "only named volumes and tmpfs"},
		"C10 HIKARI_":        {withOrders("environment: {HIKARI_TOKEN: x}"), "sets HIKARI_TOKEN"},
		"C14 sidecar net":    {withOrders("image: x") + "  db: {network_mode: host, x-hikari: {serves: orders}}\n", `service "db": sets network_mode`},
		"C15 sidecar bind":   {withOrders("image: x") + "  db: {volumes: [{type: bind, source: " + repo + ", target: /r}], x-hikari: {serves: orders}}\n", `service "db": mounts`},
		"prepare clash":      {withOrders("volumes: [{type: volume, source: v, target: /workspace/shop/services/orders/v}]") + "  orders-prepare: {x-hikari: {serves: orders}}\n", "taken by the prepare service"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}

	// A member without a partition has no scope, so nothing of /workspace.
	src := strings.Replace(partitionsTopology(repo, validPartitionAgents), "lead: {x-hikari: {}}", "lead: {volumes: [{type: volume, source: v, target: /workspace/x}], x-hikari: {}}", 1)
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
  orchestration: lead`, `
  lead: {x-hikari: {}}
`+services)
}

const projectSite = "  site: {x-hikari: {partition: {folders: [{repo: project, path: site}]}}}\n"

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
	if got := topo.VaultInProject(); got != "vault" {
		t.Errorf("VaultInProject() = %q", got)
	}

	// PR6: the project repository need not hold a partition.
	if _, err := Parse([]byte(projectTopology("", "")), dir); err != nil {
		t.Errorf("PR6: %v", err)
	}
	// Without the key there is no project, and the vault is anywhere.
	plain, err := Parse([]byte(compose("  orchestration: a", "  a: {x-hikari: {}}")), dir)
	if err != nil || plain.Project != nil || plain.VaultInProject() != "" {
		t.Errorf("no project key: %+v, %v", plain.Project, err)
	}
}

func TestProjectErrors(t *testing.T) {
	dir := fakeRepo(t, "site", "vault/x")
	outside := t.TempDir()
	cases := map[string]struct{ src, dir, want string }{
		"PR1 bad branch":       {strings.Replace(projectTopology("", projectSite), "branch: hikari", "branch: a..b", 1), dir, `"a..b" is not a valid branch name`},
		"PR1 unknown field":    {strings.Replace(projectTopology("", projectSite), "{branch: hikari}", "{branch: hikari, path: x}", 1), dir, "field path not found"},
		"PR2 not a repo":       {projectTopology("", projectSite), outside, "is not the top directory of a git repository"},
		"PR3 vault outside":    {projectTopology("\n  vault: "+outside, projectSite), dir, "must be a directory inside the project repository"},
		"PR3 vault is the top": {projectTopology("\n  vault: .", projectSite), dir, "must be a directory inside the project repository"},
		"PR4 reserved name":    {projectTopology("\n  repos: {project: {path: "+dir+", branch: x}}", projectSite), dir, `"project" is reserved`},
		"PR4 no project key":   {strings.Replace(projectTopology("", projectSite), "  project: {branch: hikari}", "  vault: s", 1), dir, "which needs x-hikari.project"},
		"PR5 whole repo":       {projectTopology("", "  site: {x-hikari: {partition: {folders: [{repo: project, path: .}]}}}\n"), dir, "needs a path that does not overlap the vault"},
		"PR5 in the vault":     {projectTopology("", "  site: {x-hikari: {partition: {folders: [{repo: project, path: vault/x}]}}}\n"), dir, "needs a path that does not overlap the vault"},
		"PR7 scope":            {projectTopology("", projectSite+"  db: {volumes: [{type: bind, source: "+dir+"/site, target: /e}], x-hikari: {serves: site}}\n"), dir, "hikari mounts the repos and the vault itself"},
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

// linkedTopology is a project with the linked repo app, owned by dev.
func linkedTopology(repo string) string {
	return compose(`
  project: {branch: hikari-alex}
  repos:
    app: `+repo+`
  orchestration: lead`, `
  lead: {x-hikari: {}}
  dev: {x-hikari: {partition: {folders: [{repo: app, path: src}]}}}
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
	if _, err := Parse([]byte(linkedTopology("{url: https://example.com/app.git}")), dir); err == nil || !strings.Contains(err.Error(), `path "src" is not a directory`) {
		t.Errorf("a cloned repo's folders are checked: %v", err)
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
			"  lead: {x-hikari: {}}\n  site: {x-hikari: {partition: {folders: [{repo: project, path: repos}]}}}\n", 1), "does not overlap the clone folder"},
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
  orchestration: lead`, `
  lead: {x-hikari: {}}
`)
	topo, err := Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(topo.Warnings, "the linked repo app is not used by any partition yet") {
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
  orchestration: lead`, `
  lead: {x-hikari: {}}
  api: {x-hikari: {partition: {folders: [{repo: app, path: api}]}}}
  util: {x-hikari: {partition: {folders: [{repo: lib, path: src}, {repo: lib, path: proto, read_only: true}]}}}
`)
	if _, err := Parse([]byte(src), dir); err == nil || !strings.Contains(err.Error(), "is not the top directory of a git repo") {
		t.Errorf("a path repo that does not exist is an error: %v", err)
	}
	if _, err := parse([]byte(src), dir, true); err != nil {
		t.Errorf("DR1 a draft names repos and folders that do not exist yet: %v", err)
	}
	bad := strings.Replace(src, "path: src", "path: ../src", 1)
	if _, err := parse([]byte(bad), dir, true); err == nil || !strings.Contains(err.Error(), "must be a relative path") {
		t.Errorf("DR1 a draft keeps the other rules: %v", err)
	}
}

// TestModules checks MO3 of the task pipeline spec.
func TestModules(t *testing.T) {
	topo, err := Parse([]byte(compose("  orchestration: a\n  modules: [tasks]", "  a: {x-hikari: {}}")), t.TempDir())
	if err != nil || !slices.Equal(topo.Modules, []string{"tasks"}) || !topo.HasModule("tasks") {
		t.Fatalf("modules = %v, %v", topo, err)
	}
	if topo.Role(Hikari) != "hikari" || slices.Contains(topo.Participants(), Hikari) {
		t.Error("hikari has the role hikari, and is no participant of the room")
	}
	topo, _ = Parse([]byte(compose("  orchestration: a", "  a: {x-hikari: {}}")), t.TempDir())
	if len(topo.Modules) != 0 || topo.HasModule("tasks") {
		t.Errorf("MO5 a project without the key has no module: %v", topo.Modules)
	}
}
