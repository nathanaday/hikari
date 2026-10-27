package systems

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/wiki"
)

// story writes the story order-items in #eng, which links orders'
// create-order.
func (f *fixture) story(t *testing.T) {
	t.Helper()
	_, err := f.m.ChangeWiki("eng-lead", "eng", []wiki.Write{
		{Op: "create", ID: "order-items", Kind: wiki.Story, Name: "Order items", Description: "A customer orders items.", Body: body("The customer orders through [[create-order]].")},
		{Op: "create", ID: "stock-never-negative", Kind: wiki.Rule, Name: "Stock never negative", Description: "Stock never goes below zero.", Body: body("Order items keeps it: [[order-items]].")},
	})
	must(t, err)
}

func TestHubWikiAccess(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.story(t)
	if _, err := os.Stat(filepath.Join(f.dir, "store", "hubs", "eng", "wiki", "order-items.md")); err != nil {
		t.Errorf("L1: the story is in the hub's wiki: %v", err)
	}
	write := []wiki.Write{{Op: "create", ID: "x", Kind: wiki.Story, Name: "X", Description: "X.", Body: body("x")}}
	if _, err := f.m.ChangeWiki("orders", "eng", write); !errors.Is(err, ErrForbidden) {
		t.Errorf("T2: an owner changes a hub wiki: %v", err)
	}
	if _, err := f.m.ChangeWiki("lead", "eng", write); !errors.Is(err, ErrForbidden) {
		t.Errorf("T2: another orchestrator changes #eng: %v", err)
	}
	if _, err := f.m.ChangeWiki("eng-lead", "ghost", write); !errors.Is(err, ErrNotFound) {
		t.Errorf("T2: no such hub: %v", err)
	}
	// N2: #product links the systems of its members only: eng-lead owns
	// none, but #eng is its child.
	if _, err := f.m.ChangeWiki("lead", "product", []wiki.Write{{Op: "create", ID: "shop-sells", Kind: wiki.Story, Name: "Shop sells", Description: "The shop sells.", Body: body("Through [[create-order]].")}}); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("N2: #product links a feature of a child hub's system: %v", err)
	}
	if _, err := f.m.ChangeWiki("lead", "product", []wiki.Write{{Op: "create", ID: "shop-sells", Kind: wiki.Story, Name: "Shop sells", Description: "The shop sells.", Body: body("Through [[order-items]].")}}); err != nil {
		t.Errorf("N2: #product links a story of its child hub: %v", err)
	}
	// N2: a system wiki links the wiki of its hubs.
	if _, err := f.m.ChangeWiki("orders", "", []wiki.Write{{Op: "create", ID: "orders-never-oversell", Kind: wiki.Rule, Name: "Never oversell", Description: "Orders never sell stock that is not there.", Body: body("It keeps [[stock-never-negative]].")}}); err != nil {
		t.Errorf("N2: a system rule links a hub rule: %v", err)
	}
	if _, err := f.m.ChangeWiki("orders", "", []wiki.Write{{Op: "create", ID: "sells", Kind: wiki.Principle, Name: "Sells", Description: "x.", Body: body("See [[shop-sells]].")}}); err == nil {
		t.Error("N2: orders is not in #product")
	}

	// A1 to A4: what each agent reads.
	if _, err := f.m.GetDocument("orders", "order-items"); err != nil {
		t.Errorf("A1: an owner reads its hub's wiki: %v", err)
	}
	if _, err := f.m.GetDocument("lead", "order-items"); err != nil {
		t.Errorf("A2: an orchestrator reads its child hub's wiki: %v", err)
	}
	if _, err := f.m.GetDocument("inventory", "create-order"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "wiki of orders, which you cannot read") {
		t.Errorf("T4: a peer's feature: %v", err)
	}
	hit, err := f.m.GetDocument("orders", "create-order")
	if err != nil || len(hit.Backlinks) != 1 || hit.Backlinks[0] != (api.DocRef{Wiki: "#eng", ID: "order-items", Kind: wiki.Story, Name: "Order items"}) {
		t.Errorf("T4, N6: backlinks = %+v, %v", hit.Backlinks, err)
	}
	if hits, _ := f.m.SearchWiki("orders", "customer"); len(hits) != 2 {
		t.Errorf("T5: orders finds its feature and the story: %+v", hits)
	}
}

func TestRenameAcrossWikisAndSupport(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.story(t)
	hubWiki := filepath.Join(f.dir, "store", "hubs", "eng", "wiki")
	os.WriteFile(filepath.Join(hubWiki, "Roadmap.md"), []byte("---\nkind: plan\nstatus: idea\n---\nGrow [[create-order]].\n"), 0o644)
	f.m.wikis[api.HubWikiRef("eng")].store.Reload()

	res, err := f.m.ChangeWiki("orders", "", []wiki.Write{{Op: "rename", ID: "create-order", To: "place-order"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Rewritten, []string{"#eng/Roadmap", "#eng/order-items"}) {
		t.Errorf("N5: rewritten = %v", res.Rewritten)
	}
	story, _ := os.ReadFile(filepath.Join(hubWiki, "order-items.md"))
	plan, _ := os.ReadFile(filepath.Join(hubWiki, "Roadmap.md"))
	if !strings.Contains(string(story), "[[place-order]]") || string(plan) != "---\nkind: plan\nstatus: idea\n---\nGrow [[place-order]].\n" {
		t.Errorf("N5: the story and the plan follow the rename:\n%s\n%s", story, plan)
	}

	// H2: the support of a story or a principle is the health of the
	// features it links.
	f.runOrders(t, testmap.Passed, testmap.Failed)
	_, err = f.m.ChangeWiki("eng-lead", "eng", []wiki.Write{{Op: "create", ID: "orders-are-kept", Kind: wiki.Principle, Name: "Orders are kept", Description: "x.", Body: body("See [[place-order]].")}})
	must(t, err)
	v, _ := f.m.WikiView(api.HubWikiRef("eng"))
	support := map[string]string{}
	for _, d := range v.Documents {
		support[d.ID] = d.Support
	}
	if support["order-items"] != "failing" || support["orders-are-kept"] != "failing" || support["stock-never-negative"] != "" || support["Roadmap"] != "" {
		t.Errorf("H2: support = %v", support)
	}
	if hit, _ := f.m.GetDocument("eng-lead", "order-items"); hit.Support != "failing" {
		t.Errorf("T4: support = %q", hit.Support)
	}
	sys, _ := f.m.WikiView(api.SystemWikiRef("orders"))
	if len(sys.Documents) != 1 || sys.Documents[0].Health != "failing" || sys.Documents[0].File != "systems/orders/wiki/place-order.md" || sys.Documents[0].Hash == "" {
		t.Errorf("U: the system wiki view = %+v", sys.Documents)
	}
}

func TestWikiNotices(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.TurnContext(t)
	f.story(t)
	// E1: the orchestrator's own change is no notice for it; the owners of
	// its hub hear of it.
	if tc := f.m.TurnContext("eng-lead"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: own change = %+v", tc.WikiChanged)
	}
	tc := f.m.TurnContext("orders")
	if len(tc.WikiChanged) != 1 || tc.WikiChanged[0].Wiki != "#eng" || len(tc.WikiChanged[0].Docs) != 2 {
		t.Errorf("E1: orders hears = %+v", tc.WikiChanged)
	}
	if tc := f.m.TurnContext("orders"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: once only: %+v", tc.WikiChanged)
	}
	// A plan that the user writes in a system wiki is news to its owner.
	must(t, f.m.WriteDocument(api.SystemWikiRef("orders"), "Next steps", "Add refunds.\n"))
	tc = f.m.TurnContext("orders")
	if len(tc.WikiChanged) != 1 || tc.WikiChanged[0].Wiki != "orders" || tc.WikiChanged[0].Docs[0] != (api.DocChange{ID: "Next steps", Kind: wiki.Note}) {
		t.Errorf("E1: the user's note = %+v", tc.WikiChanged)
	}
	// An orchestrator does not hear of its visible systems' wikis.
	if tc := f.m.TurnContext("eng-lead"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: eng-lead hears of orders' wiki: %+v", tc.WikiChanged)
	}
}

// TurnContext reads every agent's context once, so that the next read
// reports only what follows.
func (f *fixture) TurnContext(t *testing.T) {
	for _, a := range f.m.topo.AgentNames() {
		f.m.TurnContext(a)
	}
}

func TestUserChangesInTheUI(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.story(t)
	ref := api.HubWikiRef("eng")
	if err := f.m.SetKey(ref, "order-items", true); err != nil || !f.m.wikis[ref].store.Wiki()["order-items"].Key {
		t.Errorf("K3: mark key: %v", err)
	}
	if err := f.m.SetKey(ref, "stock-never-negative", true); err == nil {
		t.Error("K3: a rule is never key")
	}
	if err := f.m.RemoveDocument(ref, "order-items"); err == nil {
		t.Error("U8: an agent document is not removed in the UI")
	}
	bad := "---\nid: refunds\nkind: story\nname: Refunds\ndescription: A customer gets a refund.\n---\n\n## Purpose\n\nThrough [[ghost]].\n"
	if err := f.m.WriteDocument(ref, "refunds", bad); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("U8: a story with a dead link: %v", err)
	}
	if err := f.m.WriteDocument(ref, "refunds", strings.Replace(bad, "[[ghost]]", "[[create-order]]", 1)); err != nil {
		t.Errorf("decision 8: you write a story: %v", err)
	}
	if err := f.m.WriteDocument(ref, "a/b", "x"); err == nil {
		t.Error("D2: a slash in a name")
	}
	if err := f.m.WriteDocument("hub:ghost", "x", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such wiki: %v", err)
	}
	must(t, f.m.WriteDocument(api.SystemWikiRef("orders"), "Ideas", "Refunds?\n"))
	must(t, f.m.RemoveDocument(api.SystemWikiRef("orders"), "Ideas"))
}

func TestReleaseCopiesAgentDocuments(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	must(t, f.m.WriteDocument(api.SystemWikiRef("orders"), "Ideas", "Refunds?\n"))
	_, err := f.m.MarkCurrent("orders")
	must(t, err)
	staging := filepath.Join(f.dir, "store", "systems", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# Orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	if _, _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "First."}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.dir, "store", "systems", "orders", "releases", "python-library", "1.0.0", store.Wiki)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "create-order.md" {
		t.Errorf("L2: the release's wiki holds %v", entries)
	}
}
