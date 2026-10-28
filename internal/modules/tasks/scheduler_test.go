package tasks

import (
	"slices"
	"strings"
	"testing"
)

// pipeline is a store of the shop with an orchestration agent, and a health
// that the test sets.
func pipeline(t *testing.T) (*Store, map[string]string) {
	t.Helper()
	health := map[string]string{"orders/create-order": "failing", "inventory/reserve-stock": "passing", "/customer-orders-items": "failing"}
	check := shopCheck
	check.Orchestration = "lead"
	check.Health = func(p, id string) (string, bool) {
		h, ok := health[p+"/"+id]
		return h, ok
	}
	s, err := Open(t.TempDir(), check, now)
	if err != nil {
		t.Fatal(err)
	}
	return s, health
}

func notesTo(c Change, to string) []string {
	var out []string
	for _, n := range c.Notes {
		if n.To == to {
			out = append(out, n.Body)
		}
	}
	return out
}

func status(t *testing.T, s *Store, caller, id, part, st, note string) (Grant, Change) {
	t.Helper()
	var c Change
	g, err := s.SetStatus(caller, id, part, st, note, &c)
	if err != nil {
		t.Fatalf("%s sets %s of %s to %s: %v", caller, part, id, st, err)
	}
	return g, c
}

// TestSchedule checks TK13, TK17, D4, and D5: two running queues that
// share a partition, and a scope that needs another.
func TestSchedule(t *testing.T) {
	s, _ := pipeline(t)
	coupons := scoped(t, s, "coupons", "orders")
	scoped(t, s, "restock", "inventory")
	scoped(t, s, "cancel", "orders", "inventory")
	scoped(t, s, "report", "inventory")
	s.Tasks["report"].Needs = []string{"cancel"}
	for _, op := range []QueueOp{{Op: "create", Name: "API"}, {Op: "create", Name: "UI"},
		{Op: "add", Queue: "api", Scope: "coupons"}, {Op: "add", Queue: "api", Scope: "restock"},
		{Op: "add", Queue: "ui", Scope: "cancel"}, {Op: "add", Queue: "ui", Scope: "report"}} {
		if _, err := s.ChangeQueue(op); err != nil {
			t.Fatal(err)
		}
	}
	var c Change
	if err := s.SetQueueState("api", Running, &c); err != nil {
		t.Fatal(err)
	}
	if coupons.Status != Started || coupons.PartStatus("orders") != Queued {
		t.Fatalf("TK13 the first scope starts, its parts queued: %+v", coupons)
	}
	if got := notesTo(c, "lead"); len(got) != 1 || !strings.Contains(got[0], "The queue api started [[coupons]]") {
		t.Errorf("TK13 hikari tells the orchestration agent: %v", c.Notes)
	}
	if c.Writes[0].Author != User || !slices.Contains(c.Writes[1].Paths, "coupons.md") || c.Writes[1].Author != Hikari {
		t.Errorf("G3 the queue's state is yours, the start is hikari's: %+v", c.Writes)
	}

	c = Change{}
	s.SetQueueState("ui", Running, &c)
	cancel := s.Tasks["cancel"]
	if cancel.Status != Waiting || !strings.Contains(cancel.Body, "orders is busy with [[coupons]]") {
		t.Errorf("D4 cancel waits for orders: %s\n%s", cancel.Status, cancel.Body)
	}

	// coupons finishes; cancel starts in its turn, and report still needs it.
	status(t, s, "orders", "coupons", "", Started, "")
	_, c = status(t, s, User, "coupons", "orders", Done, "")
	if coupons.Status != Validating || len(notesTo(c, "lead")) != 2 {
		t.Errorf("TK15 every part done: validating, and lead is told: %s %v", coupons.Status, c.Notes)
	}
	// TK13: the queues go in the order of their ids, so api's restock
	// takes inventory first, and cancel, which needs it too, waits.
	_, c = status(t, s, User, "coupons", "", Done, "")
	restock := s.Tasks["restock"]
	if restock.Status != Started || cancel.Status != Waiting || cancel.Wait != "inventory is busy with [[restock]]" {
		t.Errorf("TK17 the freed orders and inventory go to api first: restock %s, cancel %s\n%s", restock.Status, cancel.Status, cancel.Body)
	}
	status(t, s, User, "restock", "", Done, "")
	if cancel.Status != Started {
		t.Errorf("TK17 cancel starts once inventory is free: %s", cancel.Status)
	}
	if report := s.Tasks["report"]; report.Status != Queued {
		t.Errorf("report waits behind cancel in its queue: %s", report.Status)
	}
	status(t, s, User, "cancel", "", Done, "")
	if report := s.Tasks["report"]; report.Status != Started {
		t.Errorf("D5 report starts once cancel, which it needs, is done: %s", report.Status)
	}

	// D5: needs keeps a scope waiting while the scope it names is not done.
	s2, _ := pipeline(t)
	scoped(t, s2, "first", "orders")
	scoped(t, s2, "second", "inventory")
	s2.Tasks["second"].Needs = []string{"first"}
	s2.ChangeQueue(QueueOp{Op: "create", Name: "Q"})
	s2.ChangeQueue(QueueOp{Op: "add", Queue: "q", Scope: "second"})
	c = Change{}
	s2.SetQueueState("q", Running, &c)
	if second := s2.Tasks["second"]; second.Status != Waiting || !strings.Contains(second.Body, "it needs [[first]] done") {
		t.Errorf("D5 second waits for first: %s", second.Status)
	}

	// Decision 5: a paused queue starts no next scope.
	s3, _ := pipeline(t)
	scoped(t, s3, "one", "orders")
	scoped(t, s3, "two", "orders")
	s3.ChangeQueue(QueueOp{Op: "create", Name: "Q"})
	s3.ChangeQueue(QueueOp{Op: "add", Queue: "q", Scope: "one"})
	s3.ChangeQueue(QueueOp{Op: "add", Queue: "q", Scope: "two"})
	s3.SetQueueState("q", Running, &c)
	s3.SetQueueState("q", Paused, &c)
	status(t, s3, User, "one", "", Done, "")
	if s3.Tasks["two"].Status != Queued {
		t.Errorf("decision 5 a paused queue starts nothing: %s", s3.Tasks["two"].Status)
	}
}

// TestStatuses checks TK14 to TK16, TK18, and TK19.
func TestStatuses(t *testing.T) {
	s, health := pipeline(t)
	scoped(t, s, "cancel", "orders", "inventory")
	cancel := s.Tasks["cancel"]
	cancel.Features = []string{"create-order", "reserve-stock", "customer-orders-items"}
	if _, err := s.SetStatus("orders", "cancel", "", Started, "", &Change{}); err == nil || !strings.Contains(err.Error(), "has not started") {
		t.Errorf("a part of a draft does not start: %v", err)
	}
	s.ChangeQueue(QueueOp{Op: "create", Name: "Q"})
	s.ChangeQueue(QueueOp{Op: "add", Queue: "q", Scope: "cancel"})
	s.SetQueueState("q", Running, &Change{})

	status(t, s, "orders", "cancel", "", Started, "")
	status(t, s, "inventory", "cancel", "", Testing, "the suites run")
	if cancel.Status != Started {
		t.Errorf("TK15 one part started, one testing: started, not %s", cancel.Status)
	}
	if _, err := s.SetStatus("inventory", "cancel", "", Blocked, "", &Change{}); err == nil {
		t.Error("a block needs a note")
	}
	_, c := status(t, s, "inventory", "cancel", "", Blocked, "the stock API is down")
	if cancel.Status != Blocked || !slices.ContainsFunc(notesTo(c, "lead"), func(b string) bool {
		return strings.Contains(b, "inventory: the part of [[cancel]] is blocked: the stock API is down")
	}) {
		t.Errorf("TK15 TK19 a blocked part blocks the scope, and lead is told: %s %v", cancel.Status, c.Notes)
	}
	_, c = status(t, s, "inventory", "cancel", "", Testing, "")
	if cancel.Status != Started || !strings.Contains(strings.Join(notesTo(c, "lead"), " "), "no longer blocked") {
		t.Errorf("TK19 the block clears: %s %v", cancel.Status, c.Notes)
	}

	// TK14: done waits for the health of the partition's features.
	g, _ := status(t, s, "orders", "cancel", "", Done, "")
	if g.Status != Testing || !slices.Equal(g.Refused, []string{"[[create-order]] is failing"}) || cancel.PartStatus("orders") != Testing {
		t.Errorf("TK14 done is refused while create-order fails: %+v %s", g, cancel.PartStatus("orders"))
	}
	if cancel.Status != Testing {
		t.Errorf("TK15 every part testing: testing, not %s", cancel.Status)
	}
	health["orders/create-order"] = "passing"
	if g, _ = status(t, s, "orders", "cancel", "", Done, ""); g.Status != Done {
		t.Errorf("TK14 done is granted once it passes: %+v", g)
	}
	_, c = status(t, s, "inventory", "cancel", "", Done, "")
	if cancel.Status != Validating || !slices.ContainsFunc(notesTo(c, "lead"), func(b string) bool { return strings.Contains(b, "Every part of [[cancel]] is done") }) {
		t.Errorf("TK15 validating, and lead validates: %s %v", cancel.Status, c.Notes)
	}

	// TK16: the orchestration agent's done waits for its own features.
	if g, _ = status(t, s, "lead", "cancel", "", Done, ""); g.Status != Validating || !slices.Equal(g.Refused, []string{"[[customer-orders-items]] is failing"}) {
		t.Errorf("TK16 done waits for the orchestration feature: %+v", g)
	}
	status(t, s, "lead", "cancel", "", Blocked, "the flow fails at payment")
	if _, err := s.SetStatus("lead", "cancel", "", Done, "", &Change{}); err == nil {
		t.Error("TK16 a blocked scope is not done")
	}
	status(t, s, "lead", "cancel", "", Started, "fixed")
	if cancel.Status != Validating {
		t.Errorf("TK16 a cleared block returns to what the parts say: %s", cancel.Status)
	}
	health["/customer-orders-items"] = "passing"
	if g, _ = status(t, s, "lead", "cancel", "", Done, "it works"); g.Status != Done || cancel.Status != Done {
		t.Errorf("TK16 done is granted: %+v", g)
	}

	// Who may set what.
	if _, err := s.SetStatus("billing", "cancel", "", Started, "", &Change{}); err == nil {
		t.Error("an unknown caller sets nothing")
	}
	scoped(t, s, "other", "orders")
	if _, err := s.SetStatus("inventory", "other", "", Started, "", &Change{}); err == nil || !strings.Contains(err.Error(), "no part for inventory") {
		t.Errorf("G2 a partition agent sets only its own part: %v", err)
	}

	// TK18: each change is a line of the log, with who and why.
	log, _ := Section(cancel.Body, SectionLog)
	for _, want := range []string{
		"hikari set the scope to started (was queued): the queue q started it",
		"inventory set the part of inventory to blocked (was testing): the stock API is down",
		"orders asked for done of the part of orders: refused, since [[create-order]] is failing",
		"lead asked for done of the scope: refused, since [[customer-orders-items]] is failing",
		"lead set the scope to blocked (was validating): the flow fails at payment",
		"lead set the scope to done (was validating): it works",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("TK18 the log says %q:\n%s", want, log)
		}
	}
}
