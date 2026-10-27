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

	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/topology"
)

func TestSystemMounts(t *testing.T) {
	got := SystemMounts("/code/shop", "services/orders", []string{"proto"}, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop/services/orders", Target: "/workspace/services/orders"},
		{Source: "/code/shop/proto", Target: "/workspace/proto", ReadOnly: true},
		{Source: "/tmp/empty", Target: "/workspace/services/orders/.hikari", ReadOnly: true},
		{Source: "/code/shop/services/orders/.hikari/private", Target: "/hikari/private"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("M1 to M3:\n got %+v\nwant %+v", got, want)
	}
	for _, m := range got {
		if strings.HasSuffix(m.Target, "/.git") {
			t.Error("M5: a scoped system must not mount anything at .git")
		}
	}
}

func TestWholeRepoScopeHidesGit(t *testing.T) {
	got := SystemMounts("/code/shop", "", nil, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop", Target: "/workspace"},
		{Source: "/tmp/empty", Target: "/workspace/.hikari", ReadOnly: true},
		{Source: "/code/shop/.hikari/private", Target: "/hikari/private"},
		{Source: "/tmp/empty", Target: "/workspace/.git", ReadOnly: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("M5:\n got %+v\nwant %+v", got, want)
	}
}

func TestStoreMounts(t *testing.T) {
	st := store.Store{Root: "/s"}
	owner := StoreMounts(st, "orders", true, nil, []string{"inventory"}, "/tmp/empty")
	wantOwner := []Mount{
		{Source: "/s/shared", Target: "/hikari/public/readonly/shared", ReadOnly: true},
		{Source: "/s/systems/orders", Target: "/hikari/public/export", ReadOnly: true},
		{Source: "/s/systems/orders/staging", Target: "/hikari/public/export/staging"},
		{Source: "/s/systems/inventory/releases", Target: "/hikari/public/readonly/systems/inventory/releases", ReadOnly: true},
	}
	if !slices.Equal(owner, wantOwner) {
		t.Errorf("owner mounts:\n got %+v\nwant %+v", owner, wantOwner)
	}

	lead := StoreMounts(st, "lead", false, []string{"orders"}, []string{"orders", "billing"}, "/tmp/empty")
	wantLead := []Mount{
		{Source: "/s/shared", Target: "/hikari/public/readonly/shared", ReadOnly: true},
		{Source: "/s/systems/orders", Target: "/hikari/public/readonly/systems/orders", ReadOnly: true},
		{Source: "/tmp/empty", Target: "/hikari/public/readonly/systems/orders/staging", ReadOnly: true},
		{Source: "/s/systems/billing/releases", Target: "/hikari/public/readonly/systems/billing/releases", ReadOnly: true},
	}
	if !slices.Equal(lead, wantLead) {
		t.Errorf("orchestrator mounts:\n got %+v\nwant %+v", lead, wantLead)
	}

	owner = StoreMounts(st, "inventory", true, nil, nil, "/tmp/empty")
	if len(owner) != 3 || owner[0].Target != "/hikari/public/readonly/shared" {
		t.Errorf("an owner without grants sees shared and its own directory: %+v", owner)
	}
}

// userCompose is a compose file before compose resolves it. The test fills
// in the repo path.
const userCompose = `name: shop
x-hikari:
  store: store
  repos: {shop: {path: REPO, branch: hikari/work}}
  orchestration: lead
services:
  lead:
    image: hikari-agent
    x-hikari: {}
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    volumes: [venv:/workspace/services/orders/.venv]
    depends_on: [orders-db]
    x-hikari: {system: {repo: shop, scope: services/orders}}
  orders-db:
    image: postgres:17
    x-hikari: {serves: orders}
volumes: {venv: {}}
`

// resolved is userCompose as compose resolves it.
const resolved = `name: shop
x-hikari:
  store: store
  repos: {shop: {path: REPO, branch: hikari/work}}
  orchestration: lead
services:
  lead: {image: hikari-agent, networks: {default: null}, x-hikari: {}}
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    networks: {default: null}
    volumes: [{type: volume, source: venv, target: /workspace/services/orders/.venv}]
    x-hikari: {system: {repo: shop, scope: services/orders}}
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
	topo.File = filepath.Join(dir, "compose.yaml")
	os.WriteFile(topo.File, []byte(strings.ReplaceAll(userCompose, "REPO", repo)), 0o644)
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
	if !slices.Equal(orders.Entrypoint, []string{"/hikari/bin/hikari", "node"}) || !orders.Init || orders.WorkingDir != "/workspace/services/orders" {
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
	if m := targets["/workspace/services/orders"]; m.Source != filepath.Join(repo, "services/orders") || m.ReadOnly {
		t.Errorf("O1 scope: %+v", m)
	}
	if _, ok := targets["/hikari/public/export/staging"]; !ok {
		t.Errorf("O1 store mounts missing: %v", targets)
	}
	prepare := f.Services["orders-prepare"]
	if prepare.Image != "shop-orders" || prepare.User != "root" || prepare.NetworkMode != "none" ||
		!slices.Equal(prepare.Entrypoint, []string{"chown", "agent", "/workspace/services/orders/.venv"}) {
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
	if !slices.ContainsFunc(orders.Volumes, func(m composeMount) bool { return m.Target == "/workspace/services/orders/.venv" }) {
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

// TestOverrideOfRenamedEntities checks that compose and the store key by
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
  orders: {image: hikari-agent, x-hikari: {name: intake, system: {repo: shop, scope: services/orders}}}
  billing: {image: hikari-agent, x-hikari: {system: {repo: shop, scope: services/billing}, grants: {releases: [orders]}}}
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
	store := filepath.Join(dir, "store")
	if targets["/hikari/bin/hikari"] != "/bin/a" {
		t.Errorf("the binary of intake: %v", targets)
	}
	if targets["/hikari/public/export"] != filepath.Join(store, "systems", "orders") {
		t.Errorf("ID6: the owner's store directory by id: %v", targets)
	}
	if targets["/hikari/public/readonly/shared"] != filepath.Join(store, "shared") {
		t.Errorf("the public wiki is in the shared area: %v", targets)
	}
	billing := map[string]string{}
	for _, m := range f.Services["billing"].Volumes {
		billing[m.Target] = m.Source
	}
	if billing["/hikari/public/readonly/systems/intake/releases"] != filepath.Join(store, "systems", "orders", "releases") {
		t.Errorf("ID12: a granted release mounts at the system's name: %v", billing)
	}
}
