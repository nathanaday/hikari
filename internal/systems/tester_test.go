package systems

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/e2e"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/wiki"
)

// The tester topology: e2e tests #eng, whose members own orders and
// inventory.
const testerTopology = `
name: test
x-hikari:
  repos:
    shop: {path: shop, branch: hikari/work}
  hubs:
    eng: {orchestrator: eng-lead, members: [orders, inventory, e2e]}
services:
  eng-lead: {x-hikari: {}}
  orders: {x-hikari: {system: {repo: shop, scope: services/orders}, grants: {deliverables: [python-library]}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
  e2e: {x-hikari: {system: {repo: shop, scope: e2e}, tests: eng}}
`

func newTesterFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	write(t, f.repo, "e2e/tests/test_orders.py", "def test(): pass\n")
	git(t, f.repo, "add", "--all")
	git(t, f.repo, "commit", "--quiet", "-m", "e2e")
	topo, err := topology.Parse([]byte(testerTopology), f.dir)
	must(t, err)
	f.m, err = New(topo)
	must(t, err)
	return f
}

// publishOrders publishes orders' library at version.
func (f *fixture) publishOrders(t *testing.T, version string) {
	t.Helper()
	staging := filepath.Join(f.dir, "store", "systems", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders "+version+"\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err := f.m.MarkCurrent("orders")
	must(t, err)
	_, _, err = f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: version, Notes: "A release."})
	must(t, err)
}

// runE2E records one run of the tester's suite with one test of each
// outcome, in tests/test_orders.py.
func (f *fixture) runE2E(t *testing.T, outcome string) {
	t.Helper()
	body := `<testsuites><testsuite name="pytest"><testcase classname="tests.test_orders.T" name="test_order" file="tests/test_orders.py">`
	if outcome == "failed" {
		body += `<failure message="no stock held"/>`
	}
	body += `</testcase></testsuite></testsuites>`
	write(t, f.repo, "e2e/.hikari/private/reports/e2e.xml", body)
	_, err := f.m.RecordRun("e2e", "e2e")
	must(t, err)
}

func storyHealth(t *testing.T, f *fixture, id string) api.StoryHealth {
	t.Helper()
	p, err := f.m.GetTestPlan("eng-lead", "eng")
	must(t, err)
	for _, s := range p.Stories {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no story %q in %+v", id, p.Stories)
	return api.StoryHealth{}
}

func TestTester(t *testing.T) {
	f := newTesterFixture(t)
	f.describeOrders(t)
	f.story(t)
	f.publishOrders(t, "1.0.0")

	// LK1: pins name readable releases; hikari writes the digest.
	if _, err := f.m.PinReleases("orders", nil); !errors.Is(err, ErrForbidden) {
		t.Errorf("LK1: only a tester pins: %v", err)
	}
	if _, err := f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "9.9.9"}}); err == nil || !strings.Contains(err.Error(), "no release 9.9.9") {
		t.Errorf("LK1: a missing release: %v", err)
	}
	lock, err := f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.0.0"}})
	if err != nil || len(lock.Releases) != 1 || !strings.HasPrefix(lock.Releases[0].Digest, "sha256:") {
		t.Fatalf("LK1 lock = %+v, %v", lock, err)
	}

	// TT1: the tester's test map links the stories of its hub.
	write(t, f.repo, "e2e/.hikari/private/layout.yaml", "version: 1\nparts: [{path: tests, description: The E2E tests.}]\nfeatures: {}\n")
	write(t, f.repo, "e2e/.hikari/private/tests.yaml", "version: 1\nsuites:\n  e2e: {command: python3 -m pytest}\ntests:\n  tests/test_orders.py: [\"[[order-items]]\"]\n")
	if v, _ := f.m.OwnView("e2e"); len(v.PrivateErrors) != 0 {
		t.Fatalf("TT1: the test map may link a story: %v", v.PrivateErrors)
	}
	if got := storyHealth(t, f, "order-items").Health; got != "pending" {
		t.Errorf("EH1: before a run, the story is %s", got)
	}
	f.runE2E(t, "passed")
	if got := storyHealth(t, f, "order-items"); got.Health != "passing" || got.Passed != 1 || got.Support == "" {
		t.Errorf("EH1: after a run = %+v", got)
	}
	if got := storyHealth(t, f, "stock-never-negative").Health; got != "untested" {
		t.Errorf("EH1: a rule no test links = %s", got)
	}
	v, _ := f.m.WikiView(api.HubWikiRef("eng"))
	for _, d := range v.Documents {
		if d.ID == "order-items" && d.Health != "passing" {
			t.Errorf("EH2: the wiki view = %+v", d)
		}
	}
	if hit, _ := f.m.GetDocument("orders", "order-items"); hit.Health != "passing" {
		t.Errorf("EH2: get_document = %+v", hit)
	}

	// EH3: a failing story names its suspects.
	f.runE2E(t, "failed")
	if got := storyHealth(t, f, "order-items"); got.Health != "failing" || !slices.Equal(got.Suspects, []string{"orders"}) {
		t.Errorf("EH3 = %+v", got)
	}
	f.runE2E(t, "passed")

	// LK3 and TT3: a newer release is news, and a new pin makes the
	// results pending.
	f.publishOrders(t, "1.1.0")
	if tc := f.m.TurnContext("e2e"); !slices.Equal(tc.NewerReleases, []string{"orders/python-library 1.1.0 is newer than your pinned 1.0.0"}) {
		t.Errorf("LK3 = %v", tc.NewerReleases)
	}
	_, err = f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.1.0"}})
	must(t, err)
	if got := storyHealth(t, f, "order-items").Health; got != "pending" {
		t.Errorf("TT3: after a new pin, the story is %s", got)
	}
	if tc := f.m.TurnContext("e2e"); len(tc.NewerReleases) != 0 {
		t.Errorf("LK3 after the pin = %v", tc.NewerReleases)
	}

	// LK2: a lock that does not match the releases is a problem.
	os.WriteFile(filepath.Join(f.repo, "e2e", e2e.LockFile), []byte("version: 1\nreleases: [{system: orders, deliverable: python-library, version: 1.1.0, digest: 'sha256:00'}]\n"), 0o644)
	if v, _ := f.m.OwnView("e2e"); !strings.Contains(strings.Join(v.PrivateErrors, "\n"), "LK2") {
		t.Errorf("LK2 = %v", v.PrivateErrors)
	}
	if _, err := f.m.RecordRun("e2e", "e2e"); err == nil {
		t.Error("LK2: a broken lock blocks a run")
	}
}

func TestPlanAndGaps(t *testing.T) {
	f := newTesterFixture(t)
	f.describeOrders(t)
	f.story(t)
	write(t, f.repo, "e2e/plan.md", `---
components:
  orders: {release: orders/python-library}
gaps:
  real-stock:
    what: Stock in a real warehouse
    why: no warehouse is reachable
    touches: ["[[order-items]]"]
---

## The test system

Orders, installed from its release.
`)
	if _, err := f.m.GetTestPlan("orders", "eng"); !errors.Is(err, ErrForbidden) {
		t.Errorf("PL7: an owner reads the plan: %v", err)
	}
	p, err := f.m.GetTestPlan("e2e", "eng")
	if err != nil || p.Tester != "e2e" || !strings.Contains(p.Body, "installed from its release") {
		t.Fatalf("PL7 = %+v, %v", p, err)
	}
	if !strings.Contains(strings.Join(p.Warnings, "\n"), `component "orders" uses orders/python-library, which releases.lock does not pin`) {
		t.Errorf("PL6 warnings = %v", p.Warnings)
	}
	if len(p.Gaps) != 1 || p.Gaps[0].Approved || !slices.Equal(storyHealth(t, f, "order-items").Gaps, []string{"real-stock"}) {
		t.Errorf("EH4: gaps = %+v", p.Gaps)
	}

	// GA1, GA2: you approve a gap; a changed gap needs a new approval.
	must(t, f.m.SetApproval("eng", "real-stock", true))
	if p, _ := f.m.GetTestPlan("e2e", "eng"); !p.Gaps[0].Approved {
		t.Error("GA1: approved")
	}
	plan, _ := os.ReadFile(filepath.Join(f.repo, "e2e", "plan.md"))
	os.WriteFile(filepath.Join(f.repo, "e2e", "plan.md"), []byte(strings.Replace(string(plan), "no warehouse is reachable", "the warehouse is closed", 1)), 0o644)
	if p, _ := f.m.GetTestPlan("e2e", "eng"); p.Gaps[0].Approved {
		t.Error("GA2: a changed gap is proposed again")
	}
	if err := f.m.SetApproval("eng", "ghost", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such gap: %v", err)
	}

	// PL6: a plan that does not parse blocks runs.
	write(t, f.repo, "e2e/plan.md", "---\nowners: []\n---\n")
	if v, _ := f.m.OwnView("e2e"); !strings.Contains(strings.Join(v.PrivateErrors, "\n"), "PL1") {
		t.Errorf("PL6 private errors = %v", v.PrivateErrors)
	}

	// TT2: a rename of a story follows into the tester's test map.
	write(t, f.repo, "e2e/plan.md", "Prose only.\n")
	write(t, f.repo, "e2e/.hikari/private/tests.yaml", "version: 1\nsuites:\n  e2e: {command: python3 -m pytest}\ntests:\n  tests/test_orders.py: [\"[[order-items]]\"]\n")
	_, err = f.m.ChangeWiki("eng-lead", "eng", []wiki.Write{{Op: "rename", ID: "order-items", To: "customer-orders"}})
	must(t, err)
	data, _ := os.ReadFile(filepath.Join(f.repo, "e2e", ".hikari", "private", "tests.yaml"))
	if !strings.Contains(string(data), "[[customer-orders]]") {
		t.Errorf("TT2: tests.yaml =\n%s", data)
	}
}

func TestBenchResults(t *testing.T) {
	f := newTesterFixture(t)
	f.describeOrders(t)
	f.story(t)
	f.publishOrders(t, "1.0.0")
	_, err := f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.0.0"}})
	must(t, err)
	write(t, f.repo, "e2e/plan.md", "---\nbench:\n  real-stock:\n    title: Stock in a real warehouse\n    touches: [\"[[order-items]]\"]\n    procedure: Order two items from the warehouse and check the shelf.\n---\n")
	if err := f.m.RecordBench("eng", "ghost", true, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("BR1: no such procedure: %v", err)
	}
	must(t, f.m.RecordBench("eng", "real-stock", true, "Two items held."))
	if got := storyHealth(t, f, "order-items"); got.Health != "passing" || !slices.Equal(got.Bench, []string{"real-stock"}) {
		t.Errorf("BH1: a passed bench check = %+v", got)
	}
	p, _ := f.m.GetTestPlan("e2e", "eng")
	if len(p.Bench) != 1 || p.Bench[0].Result == nil || !p.Bench[0].Result.Current || p.Bench[0].Result.Note != "Two items held." {
		t.Errorf("BR4: bench = %+v", p.Bench)
	}
	must(t, f.m.RecordBench("eng", "real-stock", false, "One item short."))
	if got := storyHealth(t, f, "order-items").Health; got != "failing" {
		t.Errorf("BH1: a failed bench check = %s", got)
	}
	// BR2: a new pin makes the result pending.
	f.publishOrders(t, "1.1.0")
	_, err = f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.1.0"}})
	must(t, err)
	if got := storyHealth(t, f, "order-items").Health; got != "pending" {
		t.Errorf("BR2: after a new pin = %s", got)
	}
}

// A fresh clone has no releases: the owner is told to publish what the
// tester pins, and the tester waits for them (rule SY11).
func TestMissingReleasesOnAClone(t *testing.T) {
	f := newTesterFixture(t)
	f.publishOrders(t, "1.0.0")
	if _, err := f.m.PinReleases("e2e", []e2e.LockedRelease{{System: "orders", Deliverable: "python-library", Version: "1.0.0"}}); err != nil {
		t.Fatal(err)
	}
	if tc := f.m.TurnContext("orders"); len(tc.MissingReleases) != 0 || len(f.m.TurnContext("e2e").UnrestoredPins) != 0 {
		t.Fatalf("this machine has the release: %+v", tc)
	}
	if err := os.RemoveAll(filepath.Join(f.dir, "store/systems/orders/releases")); err != nil {
		t.Fatal(err)
	}
	if got := f.m.TurnContext("orders").MissingReleases; !slices.Equal(got, []string{"python-library 1.0.0 (pinned by e2e)"}) {
		t.Errorf("SY11 owner: %v", got)
	}
	if got := f.m.TurnContext("e2e").UnrestoredPins; !slices.Equal(got, []string{"orders/python-library 1.0.0"}) {
		t.Errorf("SY11 tester: %v", got)
	}
	if got := f.m.TurnContext("inventory").MissingReleases; len(got) != 0 {
		t.Errorf("SY11 another owner: %v", got)
	}
}
