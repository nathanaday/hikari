package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/topology"
)

// connect starts the MCP server of agent against srv and returns a client
// session.
func connect(t *testing.T, srv *server.Server, base, agent string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	c := &api.Client{URL: base, Token: srv.IssueToken(agent), HTTP: http.DefaultClient}
	self, err := c.Self(ctx)
	if err != nil {
		t.Fatal(err)
	}
	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := New(c, self).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestToolsAgainstTheServer(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	os.MkdirAll(filepath.Join(repo, "orders", "tests"), 0o755)
	os.WriteFile(filepath.Join(repo, "orders", "create.py"), []byte("def create(): pass\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "orders", "tests", "test_create.py"), []byte("def test(): pass\n"), 0o644)
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}, {"commit", "--quiet", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	topo, err := topology.Parse([]byte(`
name: test
x-hikari:
  repos: {shop: {path: shop, branch: hikari/work}}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {partition: {folders: [{repo: shop, path: orders}]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	sys, err := partitions.New(topo)
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(topo, nil, sys)
	hs := httptest.NewServer(srv.Handler("ui"))
	defer hs.Close()

	orders := connect(t, srv, hs.URL, "orders")
	lead := connect(t, srv, hs.URL, "lead")

	if out, isErr := call(t, orders, "set_partition_info", map[string]any{
		"summary":  "Takes orders.",
		"provides": []map[string]any{{"name": "POST /orders", "kind": "http", "description": "Creates an order."}},
	}); isErr {
		t.Fatalf("set_partition_info: %s", out)
	}
	if out, isErr := call(t, orders, "change_wiki", map[string]any{"writes": []map[string]any{
		{"op": "create", "id": "create-order", "name": "Create an order", "description": "Stores an order.", "interfaces": []string{"POST /orders"}, "related": []string{"validate-order"}, "body": "## Purpose\n\nA customer places an order.\n"},
		{"op": "create", "id": "validate-order", "name": "Validate an order", "description": "Refuses bad orders.", "body": "## Purpose\n\nAn order must be sound before [[create-order]] stores it.\n"},
	}}); isErr || !strings.Contains(out, "Applied 2 write(s)") {
		t.Fatalf("change_wiki with a forward reference: %s", out)
	}
	if out, isErr := call(t, orders, "change_wiki", map[string]any{"writes": []map[string]any{{"op": "create", "id": "x", "name": "X", "description": "X.", "code": []string{"create.py"}}}}); !isErr || !strings.Contains(out, `"code"`) {
		t.Errorf("the wiki must not take code paths: %q", out)
	}
	if out, isErr := call(t, orders, "record_test_run", map[string]any{"suite": "unit"}); !isErr || !strings.Contains(out, `no suite "unit"`) {
		t.Errorf("a run before the test map has the suite: %q", out)
	}
	// The partition agent writes its private documents directly, as the skill says.
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "code-map.yaml"), []byte("version: 1\nparts: [{path: shop/orders/create.py, description: Order creation.}]\nfeatures:\n  create-order: {code: [shop/orders/create.py]}\n  validate-order: {code: [shop/orders/create.py]}\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "tests.yaml"), []byte("version: 1\nsuites:\n  unit: {command: python3 -m pytest}\ntests:\n  shop/orders/tests/test_create.py: [\"[[create-order]]\"]\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "reports", "unit.xml"), []byte(`<testsuite><testcase classname="tests.test_create.T" name="test_a" file="shop/orders/tests/test_create.py"/><testcase classname="tests.test_create.T" name="test_b" file="shop/orders/tests/test_create.py"/><testcase classname="tests.other.T" name="test_c"/></testsuite>`), 0o644)
	if out, isErr := call(t, orders, "record_test_run", map[string]any{"suite": "unit"}); isErr || !strings.Contains(out, "Recorded suite unit: 3 passed, 0 failed, 0 skipped. 1 test(s) touch no feature yet") || !strings.Contains(out, "create-order: was pending, now passing") {
		t.Fatalf("record_test_run: %s", out)
	}
	if out, isErr := call(t, orders, "mark_map_current", nil); isErr || !strings.Contains(out, "The map now describes shop@") {
		t.Fatalf("mark_map_current: %s", out)
	}
	out, _ := call(t, orders, "get_my_partition", nil)
	for _, want := range []string{"state: current", "health: passing", "health: untested"} {
		if !strings.Contains(out, want) {
			t.Errorf("get_my_partition lacks %q:\n%s", want, out)
		}
	}
	os.WriteFile(filepath.Join(repo, "orders", "cancel.py"), []byte("def cancel(): pass\n"), 0o644)
	if out, _ := call(t, orders, "apply", map[string]any{"message": "Add cancel"}); !strings.Contains(out, "Committed") {
		t.Errorf("apply: %s", out)
	}

	if out, isErr := call(t, lead, "define_interface", map[string]any{"name": "POST /orders", "kind": "http", "description": "Creates an order."}); isErr {
		t.Fatalf("define_interface: %s", out)
	}
	out, _ = call(t, lead, "get_partition", map[string]any{"name": "orders"})
	if !strings.Contains(out, "health: passing") || strings.Contains(out, "create.py") || strings.Contains(out, "python3") {
		t.Errorf("get_partition must show health and nothing private:\n%s", out)
	}
	if out, _ := call(t, lead, "search_wiki", map[string]any{"query": "create"}); !strings.Contains(out, "wiki: orders") {
		t.Errorf("search_wiki:\n%s", out)
	}
	if out, isErr := call(t, lead, "change_public_wiki", map[string]any{"writes": []map[string]any{
		{"op": "create", "id": "orders-are-sound", "kind": "rule", "name": "Orders are sound", "description": "Every order is checked before it is stored.", "body": "## Purpose\n\nThrough [[validate-order]].\n"},
	}}); isErr || !strings.Contains(out, "Applied 1 write(s)") {
		t.Fatalf("change_public_wiki: %s", out)
	}
	// Decision 2: the orchestration agent's change_wiki writes its own
	// feature map.
	if out, isErr := call(t, lead, "change_wiki", map[string]any{"writes": []map[string]any{
		{"op": "create", "id": "customer-orders", "name": "A customer orders", "description": "A customer places a sound order.", "body": "## Purpose\n\nThrough [[validate-order]] and [[create-order]].\n"},
	}}); isErr || !strings.Contains(out, "Applied 1 write(s)") {
		t.Fatalf("change_wiki of the orchestration agent: %s", out)
	}
	if out, _ := call(t, orders, "get_document", map[string]any{"id": "validate-order"}); !strings.Contains(out, "## Purpose") || !strings.Contains(out, "backlinks:") || !strings.Contains(out, "id: orders-are-sound") || strings.Contains(out, "customer-orders") {
		t.Errorf("K11: get_document returns the document with the backlinks that orders can read:\n%s", out)
	}
	if out, _ := call(t, lead, "get_document", map[string]any{"id": "customer-orders"}); !strings.Contains(out, "wiki: orchestration") || !strings.Contains(out, "health: ") {
		t.Errorf("K8: the orchestration agent reads its feature with its health:\n%s", out)
	}
	if _, err := orders.CallTool(context.Background(), &mcp.CallToolParams{Name: "define_term", Arguments: map[string]any{"term": "x", "definition": "y"}}); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("a partition agent has no define_term tool, got %v", err)
	}
	if out, _ := call(t, orders, "get_vocabulary", nil); !strings.Contains(out, "POST /orders") {
		t.Errorf("get_vocabulary:\n%s", out)
	}
	if out, isErr := call(t, orders, "post", map[string]any{"body": "Done."}); isErr || !strings.Contains(out, "to the room") || !strings.Contains(out, "lead") {
		t.Errorf("post: %s", out)
	}
	if out, _ := call(t, orders, "list_agents", nil); !strings.Contains(out, "- lead, orchestration") || !strings.Contains(out, "- orders (you), partition") || !strings.Contains(out, "- user, operator") {
		t.Errorf("list_agents:\n%s", out)
	}
}
