package tasks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var clock = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func now() time.Time { return clock }

func features(feats map[string][]string) func(string, string) bool {
	return func(p, id string) bool { return slices.Contains(feats[p], id) }
}

var shopCheck = Check{
	Partitions: []string{"inventory", "orders"},
	Feature: features(map[string][]string{
		"orders":    {"create-order", "cancel-order"},
		"inventory": {"reserve-stock"},
		"":          {"customer-orders-items"},
	}),
}

func openStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, shopCheck, now)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func put(t *testing.T, dir, f, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(f))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRoundTrip checks TK1 and B4: your own properties stay, in place.
func TestRoundTrip(t *testing.T) {
	text := "---\nversion: 1\nid: coupons\nkind: task\nstage: scope\nname: Coupons at checkout\npriority: high\ncreated: \"2026-10-27T09:00:00Z\"\npartitions: [orders]\nfeatures: [\"[[create-order]]\"]\nstatus: testing\nparts: {orders: testing}\narea: [checkout, money]\n---\n\n## Request\n\nLet customers use a coupon.\n"
	task, err := ParseTask(text)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "coupons" || task.Stage != StageScope || !slices.Equal(task.Features, []string{"create-order"}) || task.PartStatus("orders") != Testing {
		t.Fatalf("parsed %+v", task)
	}
	out := task.Format()
	for _, want := range []string{"priority: high", "area: [checkout, money]", "features: [\"[[create-order]]\"]", "parts: {orders: testing}", "## Request\n\nLet customers use a coupon."} {
		if !strings.Contains(out, want) {
			t.Errorf("B4 the write keeps %q:\n%s", want, out)
		}
	}
	again, err := ParseTask(out)
	if err != nil || again.Format() != out {
		t.Errorf("a second round trip changes nothing: %v\n%s", err, again.Format())
	}
}

// TestStubs checks TK2 and TK8.
func TestStubs(t *testing.T) {
	s, dir := openStore(t)
	stub, files, err := s.PlantStub("Coupons at checkout", "Let customers use a coupon.", "user")
	if err != nil || stub.ID != "coupons-at-checkout" || !slices.Equal(files, []string{"coupons-at-checkout.md"}) {
		t.Fatalf("PlantStub = %+v %v %v", stub, files, err)
	}
	if again, _, _ := s.PlantStub("Coupons at checkout", "", "orders"); again.ID != "coupons-at-checkout-2" || again.Author != "orders" {
		t.Errorf("TK8 a second stub of the name gets its own id: %+v", again)
	}
	if _, _, err := s.PlantStub("  ", "x", "user"); err == nil {
		t.Error("a stub needs a name")
	}

	// A stub that you write by hand, with the stage and nothing more.
	put(t, dir, "gift-wrap.md", "---\nstage: stub\n---\nWrap gifts.\n")
	changed, err := s.Reload()
	if err != nil || !slices.Equal(changed, []string{"gift-wrap.md"}) {
		t.Fatalf("TK7 your edit is found: %v %v", changed, err)
	}
	filled, err := s.Fill()
	if err != nil || !slices.Equal(filled, []string{"gift-wrap.md"}) {
		t.Fatalf("TK2 Fill = %v %v", filled, err)
	}
	g := s.Tasks["gift-wrap"]
	if g.Name != "gift-wrap" || g.Kind != KindTask || g.Version != Version || !g.Created.Equal(clock) {
		t.Errorf("TK2 the stub is filled in: %+v", g)
	}
	if changed, _ := s.Reload(); len(changed) != 0 {
		t.Errorf("hikari's own writes are not your edits: %v", changed)
	}
}

// TestProblems checks TK4 and TK6.
func TestProblems(t *testing.T) {
	s, dir := openStore(t)
	put(t, dir, "bad-yaml.md", "---\nname: [unclosed\n---\n")
	put(t, dir, "Two Words.md", "---\nstage: stub\n---\n")
	put(t, dir, "other-id.md", "---\nid: else\nstage: stub\n---\n")
	put(t, dir, "ghost.md", "---\nstage: scope\nname: Ghost\npartitions: [billing]\n---\n")
	put(t, dir, "wrong-feature.md", "---\nstage: scope\nname: W\npartitions: [orders]\nfeatures: [\"[[reserve-stock]]\"]\n---\n")
	put(t, dir, "fine.md", "---\nstage: scope\nname: Fine\npartitions: [orders, inventory]\nfeatures: [\"[[reserve-stock]]\", \"[[customer-orders-items]]\"]\n---\n")
	put(t, dir, "a-stub.md", "---\nstage: stub\nname: S\n---\n")
	put(t, dir, "queues/api.md", "---\nname: API\nstate: paused\nscopes: [\"[[a-stub]]\"]\n---\n")
	put(t, dir, "queues/web.md", "---\nname: Web\nstate: paused\nscopes: [\"[[fine]]\"]\n---\n")
	put(t, dir, "queues/also.md", "---\nname: Also\nstate: paused\nscopes: [\"[[fine]]\"]\n---\n")
	s.Reload()
	want := map[string]string{
		"bad-yaml.md":      "not valid YAML",
		"Two Words.md":     "rename it to two-words.md",
		"other-id.md":      `its id "else" differs`,
		"ghost.md":         `"billing" in partitions is not a partition agent`,
		"wrong-feature.md": "[[reserve-stock]] in features is no feature",
		"queues/api.md":    "[[a-stub]] is a stub",
		"queues/web.md":    "[[fine]] is in the queue also",
	}
	for f, w := range want {
		if !strings.Contains(s.Problems[f], w) {
			t.Errorf("TK6 %s: problem %q, want %q", f, s.Problems[f], w)
		}
	}
	if len(s.Problems) != len(want) {
		t.Errorf("problems = %v", s.Problems)
	}
	if _, ok := s.Tasks["fine"]; !ok || s.QueueOf("fine") != "also" {
		t.Errorf("a scope of features in its partition and the orchestration feature map is fine, in the first queue by id: %q", s.QueueOf("fine"))
	}
	if _, ok := s.Tasks["ghost"]; ok {
		t.Error("TK6 the scheduler never sees a task with a problem")
	}
}

func scoped(t *testing.T, s *Store, id string, partitions ...string) *Task {
	t.Helper()
	if _, ok := s.Tasks[id]; !ok {
		if _, _, err := s.PlantStub(id, "Do "+id+".", "user"); err != nil {
			t.Fatal(err)
		}
	}
	parts := map[string]string{}
	for _, p := range partitions {
		parts[p] = p + " changes."
	}
	task, _, err := s.Scope(ScopeInput{ID: id, Partitions: partitions, Goal: "Goal.", Parts: parts, DoneWhen: "The suites pass."})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// TestScope checks TK3 and TK10.
func TestScope(t *testing.T) {
	s, _ := openStore(t)
	s.PlantStub("Coupons", "Let customers use a coupon.\n\nThe code is on the receipt.", "user")
	task, files, err := s.Scope(ScopeInput{ID: "[[coupons]]", Partitions: []string{"orders"}, Features: []string{"[[create-order]]"},
		Goal: "A coupon lowers the total.", Parts: map[string]string{"orders": "[[create-order]] takes a coupon code."}, DoneWhen: "The suites pass.", Answers: "1. One coupon per order."})
	if err != nil || !slices.Equal(files, []string{"coupons.md"}) {
		t.Fatal(err)
	}
	var order []string
	for _, l := range strings.Split(task.Body, "\n") {
		if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "### ") {
			order = append(order, l)
		}
	}
	if !slices.Equal(order, []string{"## Request", "## Goal", "## Parts", "### orders", "## Done when", "## Answers"}) {
		t.Errorf("TK3 the sections in order: %v", order)
	}
	if req, _ := Section(task.Body, SectionRequest); req != "Let customers use a coupon.\n\nThe code is on the receipt." {
		t.Errorf("TK3 the request keeps the stub's words: %q", req)
	}
	// A second scope of the draft keeps the request.
	task, _, _ = s.Scope(ScopeInput{ID: "coupons", Partitions: []string{"orders"}, Goal: "Goal 2.", Parts: map[string]string{"orders": "x"}, DoneWhen: "y"})
	if req, _ := Section(task.Body, SectionRequest); !strings.HasPrefix(req, "Let customers") {
		t.Errorf("TK3 a revision keeps the request: %q", req)
	}
	for name, in := range map[string]ScopeInput{
		"unknown partition": {ID: "coupons", Partitions: []string{"billing"}, Goal: "g", Parts: map[string]string{"billing": "x"}, DoneWhen: "d"},
		"empty part":        {ID: "coupons", Partitions: []string{"orders", "inventory"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"},
		"foreign feature":   {ID: "coupons", Partitions: []string{"orders"}, Features: []string{"reserve-stock"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"},
		"no goal":           {ID: "coupons", Partitions: []string{"orders"}, Parts: map[string]string{"orders": "x"}, DoneWhen: "d"},
		"no task":           {ID: "nothing", Partitions: []string{"orders"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"},
	} {
		if _, _, err := s.Scope(in); err == nil {
			t.Errorf("%s is refused", name)
		}
	}
	s.ChangeQueue(QueueOp{Op: "create", Name: "API"})
	s.ChangeQueue(QueueOp{Op: "add", Queue: "api", Scope: "coupons"})
	if _, _, err := s.Scope(ScopeInput{ID: "coupons", Partitions: []string{"orders"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"}); err == nil || !strings.Contains(err.Error(), "only the user changes it") {
		t.Errorf("TK10 C4 a scope in a queue is approved: %v", err)
	}
}

// TestQueues checks TK5 and TK11.
func TestQueues(t *testing.T) {
	s, dir := openStore(t)
	for _, id := range []string{"a", "b", "c"} {
		scoped(t, s, id, "orders")
	}
	s.PlantStub("stub", "", "user")
	mustQ := func(op QueueOp) {
		t.Helper()
		if _, err := s.ChangeQueue(op); err != nil {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	mustQ(QueueOp{Op: "create", Name: "API work"})
	mustQ(QueueOp{Op: "create", Name: "UI"})
	if q := s.Queues["api-work"]; q == nil || q.State != Paused {
		t.Fatalf("decision 7 a new queue is paused: %+v", q)
	}
	mustQ(QueueOp{Op: "add", Queue: "api-work", Scope: "a"})
	mustQ(QueueOp{Op: "add", Queue: "api-work", Scope: "b"})
	mustQ(QueueOp{Op: "add", Queue: "api-work", Scope: "c", Position: 1})
	if got := s.Queues["api-work"].Scopes; !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("add at a position: %v", got)
	}
	if s.Tasks["c"].Queue != "api-work" || s.Tasks["c"].Status != Queued {
		t.Errorf("TK5 the scope's queue and status: %+v", s.Tasks["c"])
	}
	mustQ(QueueOp{Op: "move", Queue: "ui", Scope: "a"})
	if !slices.Equal(s.Queues["api-work"].Scopes, []string{"c", "b"}) || !slices.Equal(s.Queues["ui"].Scopes, []string{"a"}) || s.QueueOf("a") != "ui" {
		t.Errorf("D2 a move takes it out of the first queue: %v %v", s.Queues["api-work"].Scopes, s.Queues["ui"].Scopes)
	}
	if _, err := s.ChangeQueue(QueueOp{Op: "add", Queue: "ui", Scope: "b"}); err == nil {
		t.Error("D2 a scope is in at most one queue: add refuses one in another queue")
	}
	if _, err := s.ChangeQueue(QueueOp{Op: "add", Queue: "ui", Scope: "stub"}); err == nil {
		t.Error("TK11 a stub cannot enter a queue")
	}
	if _, err := s.ChangeQueue(QueueOp{Op: "remove", Queue: "ui"}); err == nil {
		t.Error("TK11 a queue with scopes stays")
	}
	s.Tasks["c"].Status = Started
	for _, op := range []QueueOp{{Op: "move", Queue: "ui", Scope: "c"}, {Op: "out", Scope: "c"}, {Op: "move", Queue: "api-work", Scope: "c", Position: 2}} {
		if _, err := s.ChangeQueue(op); err == nil {
			t.Errorf("TK11 D3 a started scope stays where it is: %+v", op)
		}
	}
	mustQ(QueueOp{Op: "move", Queue: "api-work", Scope: "b", Position: 1})
	if got := s.Queues["api-work"].Scopes; !slices.Equal(got, []string{"c", "b"}) {
		t.Errorf("TK11 nothing moves before a started scope: %v", got)
	}
	mustQ(QueueOp{Op: "out", Scope: "a"})
	if s.Tasks["a"].Queue != "" || s.Tasks["a"].Status != "" {
		t.Errorf("a scope out of its queue is a draft again: %+v", s.Tasks["a"])
	}
	mustQ(QueueOp{Op: "rename", Queue: "ui", Name: "Web UI"})
	mustQ(QueueOp{Op: "remove", Queue: "ui"})
	if _, err := os.Stat(filepath.Join(dir, "queues", "ui.md")); err == nil {
		t.Error("remove deletes the queue's file")
	}

	// TK5: your edit of a queue file reaches the scopes' properties.
	put(t, dir, "queues/api-work.md", strings.Replace(s.texts["queues/api-work.md"], `["[[c]]", "[[b]]"]`, `["[[c]]", "[[b]]", "[[a]]"]`, 1))
	s.Reload()
	paths, err := s.SyncQueues()
	if err != nil || !slices.Contains(paths, "a.md") || s.Tasks["a"].Queue != "api-work" || s.Tasks["a"].Status != Queued {
		t.Errorf("TK5 SyncQueues = %v %v: %+v", paths, err, s.Tasks["a"])
	}
}
