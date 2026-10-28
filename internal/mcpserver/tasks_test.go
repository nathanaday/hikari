package mcpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/topology"
)

func sessionTools(t *testing.T, s *mcp.ClientSession) []string {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tool := range res.Tools {
		out = append(out, tool.Name)
	}
	return out
}

// TestTaskPipeline runs a stub to a done scope through the tools, the UI's
// route, and the real server (rules TK8 to TK21 of the task pipeline spec).
func TestTaskPipeline(t *testing.T) {
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
  modules: [tasks]
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
	must := func(s *mcp.ClientSession, tool string, args map[string]any, want string) string {
		t.Helper()
		out, isErr := call(t, s, tool, args)
		if isErr || !strings.Contains(out, want) {
			t.Fatalf("%s: %q, want %q", tool, out, want)
		}
		return out
	}
	refused := func(s *mcp.ClientSession, tool string, args map[string]any, want string) {
		t.Helper()
		if out, isErr := call(t, s, tool, args); !isErr || !strings.Contains(out, want) {
			t.Errorf("%s is refused with %q: %q", tool, want, out)
		}
	}

	// MO4: each kind of agent gets its tools.
	ot, lt := sessionTools(t, orders), sessionTools(t, lead)
	for _, name := range []string{"plant_stub", "get_task", "list_tasks", "set_task_status"} {
		if !slices.Contains(ot, name) || !slices.Contains(lt, name) {
			t.Errorf("both agents have %s", name)
		}
	}
	if slices.Contains(ot, "scope_task") || slices.Contains(ot, "change_queue") || !slices.Contains(lt, "scope_task") || !slices.Contains(lt, "change_queue") {
		t.Errorf("scope_task and change_queue are the orchestration agent's: orders %v", ot)
	}

	// orders maps create-order, whose health decides done.
	must(orders, "change_wiki", map[string]any{"writes": []map[string]any{
		{"op": "create", "id": "create-order", "name": "Create an order", "description": "Stores an order.", "body": "## Purpose\n\nA customer places an order.\n"},
	}}, "Applied 1 write(s)")

	// TK8: an agent plants a stub; G2: a partition agent cannot read it.
	must(orders, "plant_stub", map[string]any{"name": "Cancel orders", "body": "A customer cancels an order before it ships."}, "Planted the stub [[cancel-orders]]")
	refused(orders, "get_task", map[string]any{"id": "cancel-orders"}, "no scope [[cancel-orders]] names you")
	must(lead, "get_task", map[string]any{"id": "cancel-orders"}, "author: orders")

	// TK10, TK11: the orchestration agent scopes it and queues it.
	must(lead, "scope_task", map[string]any{"id": "cancel-orders", "partitions": []string{"orders"}, "features": []string{"create-order"},
		"goal": "A customer can cancel.", "parts": map[string]string{"orders": "[[create-order]] gains a cancel."}, "done_when": "create-order passes."}, "Scoped [[cancel-orders]] for orders")
	must(lead, "change_queue", map[string]any{"op": "create", "name": "API"}, "Done: create")
	must(lead, "change_queue", map[string]any{"op": "add", "queue": "api", "scope": "cancel-orders"}, "Done: add")
	refused(lead, "scope_task", map[string]any{"id": "cancel-orders", "partitions": []string{"orders"}, "goal": "g", "parts": map[string]string{"orders": "x"}, "done_when": "d"}, "only the user changes it")
	refused(orders, "set_task_status", map[string]any{"task": "cancel-orders", "status": "started"}, "has not started")

	// TK12: you start the queue through the UI's route, and hikari tells lead.
	req, _ := http.NewRequest(http.MethodPost, hs.URL+"/api/ui/tasks/state", bytes.NewReader([]byte(`{"queue":"api","state":"running"}`)))
	req.AddCookie(&http.Cookie{Name: server.UICookie, Value: "ui"})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("start the queue: %v %v", resp, err)
	}
	msgs := srv.Take(context.Background(), "lead", 0)
	if !slices.ContainsFunc(msgs, func(m api.Message) bool {
		return m.From == topology.Hikari && strings.Contains(m.Body, "The queue api started [[cancel-orders]]")
	}) {
		t.Errorf("TK13 hikari tells lead the scope started: %+v", msgs)
	}

	// TK21, TK24: orders reads its scope, and its turn names it.
	must(orders, "get_task", map[string]any{"id": "[[cancel-orders]]"}, "status: started")
	must(orders, "list_tasks", nil, "[[cancel-orders]] Cancel orders: started (orders queued)")
	tc, err := (&api.Client{URL: hs.URL, Token: srv.IssueToken("orders"), HTTP: http.DefaultClient}).TurnContext(context.Background())
	if err != nil || len(tc.Notices) != 1 || !strings.Contains(tc.Notices[0], "your part of [[cancel-orders]] is queued") {
		t.Errorf("TK24 the notice names the part: %+v %v", tc.Notices, err)
	}

	// TK14: orders works its part; done waits for create-order to pass.
	must(orders, "set_task_status", map[string]any{"task": "cancel-orders", "status": "started"}, "The status is started.")
	must(orders, "set_task_status", map[string]any{"task": "cancel-orders", "status": "testing"}, "The status is testing.")
	must(orders, "set_task_status", map[string]any{"task": "cancel-orders", "status": "done"}, "Done is not granted yet; the status stays testing, because [[create-order]] is")
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "code-map.yaml"), []byte("version: 1\nparts: [{path: shop/orders/create.py, description: Order creation.}]\nfeatures:\n  create-order: {code: [shop/orders/create.py]}\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "tests.yaml"), []byte("version: 1\nsuites:\n  unit: {command: python3 -m pytest}\ntests:\n  shop/orders/tests/test_create.py: [\"[[create-order]]\"]\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "orders", ".hikari", "private", "reports", "unit.xml"), []byte(`<testsuite><testcase classname="tests.test_create.T" name="test_a" file="shop/orders/tests/test_create.py"/></testsuite>`), 0o644)
	must(orders, "record_test_run", map[string]any{"suite": "unit"}, "now passing")
	must(orders, "set_task_status", map[string]any{"task": "cancel-orders", "status": "done"}, "The status is done.")
	msgs = srv.Take(context.Background(), "lead", 0)
	if !slices.ContainsFunc(msgs, func(m api.Message) bool { return strings.Contains(m.Body, "Every part of [[cancel-orders]] is done") }) {
		t.Errorf("TK15 hikari tells lead to validate: %+v", msgs)
	}

	// TK16: lead validates, and the scope is done.
	must(lead, "set_task_status", map[string]any{"task": "cancel-orders", "status": "done", "note": "create-order passes"}, "The status is done.")
	out := must(lead, "list_tasks", nil, "Queue api (API), running:")
	if !strings.Contains(out, "[[cancel-orders]] Cancel orders: done") {
		t.Errorf("the scope is done in its queue:\n%s", out)
	}
	text, _ := os.ReadFile(filepath.Join(dir, "vault", "tasks", "cancel-orders.md"))
	for _, want := range []string{"## Request\n\nA customer cancels an order before it ships.", "orders set the part of orders to done (was testing)", "lead set the scope to done (was validating): create-order passes"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the document holds %q:\n%s", want, text)
		}
	}
}
