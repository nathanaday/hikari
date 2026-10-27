package launcher

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/workspace"
)

func TestPartitionMounts(t *testing.T) {
	ws := workspace.Workspace{
		{Place: "shop/services/orders", Repo: "shop", Path: "services/orders", Host: "/code/shop/services/orders"},
		{Place: "shop/proto", Repo: "shop", Path: "proto", Host: "/code/shop/proto", ReadOnly: true},
	}
	got := PartitionMounts(ws, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop/services/orders", Target: "/workspace/shop/services/orders"},
		{Source: "/tmp/empty", Target: "/workspace/shop/services/orders/.hikari", ReadOnly: true},
		{Source: "/code/shop/proto", Target: "/workspace/shop/proto", ReadOnly: true},
		{Source: "/code/shop/services/orders/.hikari/private", Target: "/hikari/private"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("F4:\n got %+v\nwant %+v", got, want)
	}
	for _, m := range got {
		if strings.HasSuffix(m.Target, "/.git") {
			t.Error("M5: a folder inside a repo must not mount anything at .git")
		}
	}
}

func TestWholeRepoFolderHidesGit(t *testing.T) {
	got := PartitionMounts(workspace.Workspace{{Place: "shop", Repo: "shop", Host: "/code/shop"}}, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop", Target: "/workspace/shop"},
		{Source: "/tmp/empty", Target: "/workspace/shop/.git", ReadOnly: true},
		{Source: "/tmp/empty", Target: "/workspace/shop/.hikari", ReadOnly: true},
		{Source: "/code/shop/.hikari/private", Target: "/hikari/private"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("M5:\n got %+v\nwant %+v", got, want)
	}
}

// TestTwoRepoMounts checks the override file of a partition with a folder
// in each of two repos (rule F19): each folder at its place, the second at
// its at:, the private documents from the first writable folder, and the
// working directory /workspace.
func TestTwoRepoMounts(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"shop/.git", "shop/services/orders", "shop/libs/money", "protos/.git", "protos/orders"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	src := `name: two
x-hikari:
  repos: {shop: {path: shop, branch: hikari}, protos: {path: protos, branch: work}}
services:
  orders:
    image: hikari-agent
    x-hikari:
      partition:
        folders:
          - {repo: shop, path: services/orders}
          - {repo: shop, path: libs/money, read_only: true}
          - {repo: protos, path: orders, at: protos}
`
	topo, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Override(topo, OverrideInput{RunID: "r1", EmptyDir: "/tmp/empty"})
	if err != nil {
		t.Fatal(err)
	}
	var f overrideFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	svc := f.Services["orders"]
	if svc.WorkingDir != "/workspace" {
		t.Errorf("working_dir = %q, want /workspace", svc.WorkingDir)
	}
	want := []composeMount{
		{Type: "bind", Source: filepath.Join(dir, "shop/services/orders"), Target: "/workspace/shop/services/orders"},
		{Type: "bind", Source: "/tmp/empty", Target: "/workspace/shop/services/orders/.hikari", ReadOnly: true},
		{Type: "bind", Source: filepath.Join(dir, "shop/libs/money"), Target: "/workspace/shop/libs/money", ReadOnly: true},
		{Type: "bind", Source: filepath.Join(dir, "protos/orders"), Target: "/workspace/protos"},
		{Type: "bind", Source: "/tmp/empty", Target: "/workspace/protos/.hikari", ReadOnly: true},
		{Type: "bind", Source: filepath.Join(dir, "shop/services/orders/.hikari/private"), Target: "/hikari/private"},
	}
	for _, m := range want {
		if !slices.Contains(svc.Volumes, m) {
			t.Errorf("no mount %+v in %+v", m, svc.Volumes)
		}
	}
}

// TestOverrideVaultMounts checks the vault mounts of the override file
// against the table of rule V3: a partition agent with a release grant and
// a skill grant, a partition agent with neither, and the orchestration
// agent.
func TestOverrideVaultMounts(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	for _, d := range []string{".git", "services/orders", "services/inventory"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	src := `name: shop
x-hikari:
  repos: {shop: {path: shop, branch: hikari}}
  orchestration: lead
services:
  lead: {image: hikari-agent, x-hikari: {grants: {skills: [style]}}}
  orders: {image: hikari-agent, x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}, grants: {releases: [inventory], skills: [style]}}}
  inventory: {image: hikari-agent, x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`
	topo, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Override(topo, OverrideInput{RunID: "r1", EmptyDir: "/tmp/empty"})
	if err != nil {
		t.Fatal(err)
	}
	var f overrideFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	vault := filepath.Join(dir, "vault")
	hikari := func(service string) []composeMount {
		var out []composeMount
		for _, m := range f.Services[service].Volumes {
			if strings.HasPrefix(m.Target, "/hikari/") && m.Target != "/hikari/bin/hikari" {
				out = append(out, m)
			}
		}
		return out
	}
	bind := func(source, target string, readOnly bool) composeMount {
		return composeMount{Type: "bind", Source: source, Target: target, ReadOnly: readOnly}
	}
	want := map[string][]composeMount{
		"orders": {
			bind(filepath.Join(repo, "services/orders/.hikari/private"), "/hikari/private", false),
			bind(filepath.Join(vault, "public"), "/hikari/public", true),
			bind(filepath.Join(vault, "partitions/orders/read-only"), "/hikari/self", true),
			bind(filepath.Join(vault, "partitions/orders/can-edit"), "/hikari/can-edit", false),
			bind(filepath.Join(vault, "partitions/orders/staging"), "/hikari/staging", false),
			bind(filepath.Join(vault, "partitions/inventory/read-only/releases"), "/hikari/grants/inventory/releases", true),
			bind(filepath.Join(vault, "skills/style"), "/hikari/skills/style", true),
		},
		"inventory": {
			bind(filepath.Join(repo, "services/inventory/.hikari/private"), "/hikari/private", false),
			bind(filepath.Join(vault, "public"), "/hikari/public", true),
			bind(filepath.Join(vault, "partitions/inventory/read-only"), "/hikari/self", true),
			bind(filepath.Join(vault, "partitions/inventory/can-edit"), "/hikari/can-edit", false),
			bind(filepath.Join(vault, "partitions/inventory/staging"), "/hikari/staging", false),
		},
		"lead": {
			bind(filepath.Join(vault, "public"), "/hikari/public", true),
			bind(filepath.Join(vault, "orchestration/read-only"), "/hikari/self", true),
			bind(filepath.Join(vault, "orchestration/can-edit"), "/hikari/can-edit", false),
			bind(filepath.Join(vault, "partitions/inventory/read-only"), "/hikari/partitions/inventory", true),
			bind(filepath.Join(vault, "partitions/orders/read-only"), "/hikari/partitions/orders", true),
			bind(filepath.Join(vault, "skills/style"), "/hikari/skills/style", true),
		},
	}
	for service, w := range want {
		if got := hikari(service); !slices.Equal(got, w) {
			t.Errorf("V3: the mounts of %s:\n got %+v\nwant %+v", service, got, w)
		}
	}
	for _, m := range f.Services["orders"].Volumes {
		if strings.Contains(m.Source, "partitions/inventory/can-edit") || strings.Contains(m.Source, "partitions/inventory/staging") || m.Source == "/tmp/empty" && strings.HasPrefix(m.Target, "/hikari/") {
			t.Errorf("V4: orders mounts %+v", m)
		}
	}
}

// userTopology is a topology file before compose resolves it. The test fills
// in the repo path.
const userTopology = `name: shop
x-hikari:
  vault: vault
  repos: {shop: {path: REPO, branch: hikari/work}}
  orchestration: lead
services:
  lead:
    image: hikari-agent
    x-hikari: {}
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    volumes: [venv:/workspace/shop/services/orders/.venv]
    depends_on: [orders-db]
    x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}}
  orders-db:
    image: postgres:17
    x-hikari: {serves: orders}
volumes: {venv: {}}
`

// resolved is userCompose as compose resolves it.
const resolved = `name: shop
x-hikari:
  vault: vault
  repos: {shop: {path: REPO, branch: hikari/work}}
  orchestration: lead
services:
  lead: {image: hikari-agent, networks: {default: null}, x-hikari: {}}
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    networks: {default: null}
    volumes: [{type: volume, source: venv, target: /workspace/shop/services/orders/.venv}]
    x-hikari: {partition: {folders: [{repo: shop, path: services/orders}]}}
  orders-db: {image: postgres:17, networks: {default: null}, x-hikari: {serves: orders}}
`

func shopTopology(t *testing.T) (*topology.Topology, string) {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	for _, d := range []string{".git", "services/orders"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	topo, err := topology.Parse([]byte(strings.ReplaceAll(resolved, "REPO", repo)), dir)
	if err != nil {
		t.Fatal(err)
	}
	topo.File = filepath.Join(dir, "topology.yaml")
	os.WriteFile(topo.File, []byte(strings.ReplaceAll(userTopology, "REPO", repo)), 0o644)
	return topo, repo
}

var input = OverrideInput{
	RunID:     "r1",
	ServerURL: "http://host.docker.internal:7420",
	EmptyDir:  "/tmp/empty",
	Binaries:  map[string]string{"lead": "/bin/hikari-linux-arm64", "orders": "/bin/hikari-linux-amd64"},
	Users:     map[string]string{"orders": "agent"},
}

func TestOverride(t *testing.T) {
	topo, repo := shopTopology(t)
	data, err := Override(topo, input)
	if err != nil {
		t.Fatal(err)
	}
	var f overrideFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	orders := f.Services["orders"]
	if !slices.Equal(orders.Entrypoint, []string{"/hikari/bin/hikari", "driver"}) || !orders.Init || orders.WorkingDir != "/workspace" {
		t.Errorf("O3: %+v", orders)
	}
	if !slices.Equal(orders.Networks, replaced{"agent-orders"}) || !strings.Contains(string(data), "networks: !override") {
		t.Errorf("O4: the agent's networks must replace the user's:\n%s", data)
	}
	if f.Networks["agent-orders"].Name != "shop-agent-orders" || f.Networks["agent-orders"].Labels[RunLabel] != "r1" {
		t.Errorf("O4, O7: %+v", f.Networks)
	}
	if orders.Environment["HIKARI_TOKEN"] != "${HIKARI_TOKEN_ORDERS}" || orders.Environment["HIKARI_SERVER_URL"] != input.ServerURL {
		t.Errorf("O5: %v", orders.Environment)
	}
	targets := map[string]composeMount{}
	for _, m := range orders.Volumes {
		targets[m.Target] = m
	}
	if m := targets["/hikari/bin/hikari"]; m.Source != "/bin/hikari-linux-amd64" || !m.ReadOnly {
		t.Errorf("O2: %+v", m)
	}
	if m := targets["/workspace/shop/services/orders"]; m.Source != filepath.Join(repo, "services/orders") || m.ReadOnly {
		t.Errorf("O1 scope: %+v", m)
	}
	if _, ok := targets["/hikari/staging"]; !ok {
		t.Errorf("O1 vault mounts missing: %v", targets)
	}
	prepare := f.Services["orders-prepare"]
	if prepare.Image != "shop-orders" || prepare.User != "root" || prepare.NetworkMode != "none" ||
		!slices.Equal(prepare.Entrypoint, []string{"chown", "agent", "/workspace/shop/services/orders/.venv"}) {
		t.Errorf("O6 prepare: %+v", prepare)
	}
	if orders.DependsOn["orders-prepare"].Condition != "service_completed_successfully" {
		t.Errorf("O6 depends_on: %+v", orders.DependsOn)
	}
	if _, ok := f.Services["lead-prepare"]; ok {
		t.Error("O6: lead has no named volumes")
	}
	if db := f.Services["orders-db"]; !slices.Equal(db.Networks, replaced{"agent-orders"}) || db.Entrypoint != nil {
		t.Errorf("C14: the sidecar joins only its agent's network: %+v", db)
	}
	if strings.Contains(string(data), "secret") {
		t.Error("the override file holds no secret")
	}
}

// TestOverrideWithCompose checks that compose accepts the override file and
// merges it as intended.
func TestOverrideWithCompose(t *testing.T) {
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not available")
	}
	topo, _ := shopTopology(t)
	p := &Project{Topo: topo}
	if err := p.WriteOverride(filepath.Join(t.TempDir(), "override.yaml"), input); err != nil {
		t.Fatal(err)
	}
	cmd := p.compose(t.Context(), "config")
	cmd.Env = append(os.Environ(), "HIKARI_TOKEN_LEAD=t1", "HIKARI_TOKEN_ORDERS=t2")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("compose rejects the override: %v", err)
	}
	var merged struct {
		Services map[string]struct {
			Networks    map[string]any    `yaml:"networks"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []composeMount    `yaml:"volumes"`
			DependsOn   map[string]any    `yaml:"depends_on"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(out, &merged); err != nil {
		t.Fatal(err)
	}
	orders := merged.Services["orders"]
	if _, ok := orders.Networks["agent-orders"]; !ok || len(orders.Networks) != 1 {
		t.Errorf("orders must have only its own network: %v", orders.Networks)
	}
	if orders.Environment["HIKARI_TOKEN"] != "t2" {
		t.Errorf("token = %q", orders.Environment["HIKARI_TOKEN"])
	}
	if _, ok := orders.DependsOn["orders-db"]; !ok {
		t.Errorf("the user's depends_on must stay: %v", orders.DependsOn)
	}
	if !slices.ContainsFunc(orders.Volumes, func(m composeMount) bool { return m.Target == "/workspace/shop/services/orders/.venv" }) {
		t.Errorf("the user's volume must stay: %+v", orders.Volumes)
	}
	if db := merged.Services["orders-db"]; len(db.Networks) != 1 {
		t.Errorf("orders-db networks = %v", db.Networks)
	}
}

func TestTokenVar(t *testing.T) {
	if got := TokenVar("yami-library"); got != "HIKARI_TOKEN_YAMI_LIBRARY" {
		t.Errorf("TokenVar = %q", got)
	}
}

func TestIsRoot(t *testing.T) {
	for user, want := range map[string]bool{"": true, "root": true, "0": true, "0:0": true, "root:staff": true, "agent": false, "1000:1000": false} {
		if isRoot(user) != want {
			t.Errorf("isRoot(%q) = %v", user, !want)
		}
	}
}

func TestLinuxBinaryMissing(t *testing.T) {
	if _, err := LinuxBinary("riscv64"); err == nil || !strings.Contains(err.Error(), "make build") {
		t.Errorf("error = %v", err)
	}
}

func TestPrefixStripsControlSequences(t *testing.T) {
	var b strings.Builder
	log.SetOutput(&b)
	log.SetFlags(0)
	defer log.SetOutput(os.Stderr)
	prefix("lead", strings.NewReader("\x1b[Klead-1 exited with code 0\n\n  \n"))
	if got := b.String(); got != "[lead] lead-1 exited with code 0\n" {
		t.Errorf("log = %q", got)
	}
}

// TestOverrideOfRenamedEntities checks that compose and the vault key by
// id, and the paths inside the container by name (rules ID9 and ID12 of the
// ids spec).
func TestOverrideOfRenamedEntities(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	for _, d := range []string{".git", "services/orders", "services/billing"} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
	}
	src := `name: shop
x-hikari:
  repos: {shop: {name: store, path: shop, branch: hikari}}
  orchestration: lead
services:
  lead: {image: hikari-agent, x-hikari: {}}
  orders: {image: hikari-agent, x-hikari: {name: intake, partition: {folders: [{repo: shop, path: services/orders}]}}}
  billing: {image: hikari-agent, x-hikari: {partition: {folders: [{repo: shop, path: services/billing}]}, grants: {releases: [orders]}}}
`
	topo, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Override(topo, OverrideInput{RunID: "r1", EmptyDir: "/tmp/empty", Binaries: map[string]string{"intake": "/bin/a"}})
	if err != nil {
		t.Fatal(err)
	}
	var f overrideFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Services["intake"]; ok {
		t.Errorf("ID9: no service is keyed by a name:\n%s", data)
	}
	orders := f.Services["orders"]
	if orders.Environment["HIKARI_TOKEN"] != "${HIKARI_TOKEN_ORDERS}" || !slices.Equal(orders.Networks, replaced{"agent-orders"}) {
		t.Errorf("ID9: token and network by id: %+v", orders)
	}
	if f.Networks["agent-orders"].Name != "shop-agent-orders" {
		t.Errorf("ID9: %+v", f.Networks)
	}
	targets := map[string]string{}
	for _, m := range orders.Volumes {
		targets[m.Target] = m.Source
	}
	vault := filepath.Join(dir, "vault")
	if targets["/hikari/bin/hikari"] != "/bin/a" {
		t.Errorf("the binary of intake: %v", targets)
	}
	if targets["/hikari/self"] != filepath.Join(vault, "partitions", "orders", "read-only") {
		t.Errorf("ID6: the partition agent's vault directory by id: %v", targets)
	}
	if targets["/hikari/public"] != filepath.Join(vault, "public") {
		t.Errorf("the public wiki is in the public area: %v", targets)
	}
	billing := map[string]string{}
	for _, m := range f.Services["billing"].Volumes {
		billing[m.Target] = m.Source
	}
	if billing["/hikari/grants/intake/releases"] != filepath.Join(vault, "partitions", "orders", "read-only", "releases") {
		t.Errorf("ID12: a granted release mounts at the partition's name: %v", billing)
	}
}
