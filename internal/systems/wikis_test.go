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

// story writes the story order-items in the public wiki, which links
// orders' create-order.
func (f *fixture) story(t *testing.T) {
	t.Helper()
	_, err := f.m.ChangeWiki("lead", true, []wiki.Write{
		{Op: "create", ID: "order-items", Kind: wiki.Story, Name: "Order items", Description: "A customer orders items.", Body: body("The customer orders through [[create-order]].")},
		{Op: "create", ID: "stock-never-negative", Kind: wiki.Rule, Name: "Stock never negative", Description: "Stock never goes below zero.", Body: body("Order items keeps it: [[order-items]].")},
	})
	must(t, err)
}

func TestPublicWikiAccess(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.story(t)
	if _, err := os.Stat(filepath.Join(f.dir, "store", "shared", "wiki", "order-items.md")); err != nil {
		t.Errorf("L1: the story is in the public wiki: %v", err)
	}
	write := []wiki.Write{{Op: "create", ID: "x", Kind: wiki.Story, Name: "X", Description: "X.", Body: body("x")}}
	if _, err := f.m.ChangeWiki("orders", true, write); !errors.Is(err, ErrForbidden) {
		t.Errorf("T2: an owner changes the public wiki: %v", err)
	}
	if _, err := f.m.ChangeWiki("lead", false, write); !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), "change_public_wiki") {
		t.Errorf("T1: the orchestrator has no system wiki: %v", err)
	}
	// N2: the public wiki links every system, and a system wiki links the
	// public wiki, not another system.
	if _, err := f.m.ChangeWiki("orders", false, []wiki.Write{{Op: "create", ID: "orders-never-oversell", Kind: wiki.Rule, Name: "Never oversell", Description: "Orders never sell stock that is not there.", Body: body("It keeps [[stock-never-negative]].")}}); err != nil {
		t.Errorf("N2: a system rule links a public rule: %v", err)
	}
	if _, err := f.m.ChangeWiki("inventory", false, []wiki.Write{{Op: "create", ID: "sells", Kind: wiki.Principle, Name: "Sells", Description: "x.", Body: body("See [[create-order]].")}}); err == nil {
		t.Error("N2: inventory links a feature of orders")
	}

	// A1 to A4: what each agent reads.
	if _, err := f.m.GetDocument("orders", "order-items"); err != nil {
		t.Errorf("A1: an owner reads the public wiki: %v", err)
	}
	if _, err := f.m.GetDocument("lead", "create-order"); err != nil {
		t.Errorf("A2: the orchestrator reads every system wiki: %v", err)
	}
	if _, err := f.m.GetDocument("inventory", "create-order"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "wiki of orders, which you cannot read") {
		t.Errorf("T4: a peer's feature: %v", err)
	}
	hit, err := f.m.GetDocument("orders", "create-order")
	if err != nil || len(hit.Backlinks) != 1 || hit.Backlinks[0] != (api.DocRef{Wiki: "public", ID: "order-items", Kind: wiki.Story, Name: "Order items"}) {
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
	publicWiki := filepath.Join(f.dir, "store", "shared", "wiki")
	os.WriteFile(filepath.Join(publicWiki, "Roadmap.md"), []byte("---\nkind: plan\nstatus: idea\n---\nGrow [[create-order]].\n"), 0o644)
	f.m.wikis[api.PublicWikiRef].store.Reload()

	res, err := f.m.ChangeWiki("orders", false, []wiki.Write{{Op: "rename", ID: "create-order", To: "place-order"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Rewritten, []string{"public/Roadmap", "public/order-items"}) {
		t.Errorf("N5: rewritten = %v", res.Rewritten)
	}
	story, _ := os.ReadFile(filepath.Join(publicWiki, "order-items.md"))
	plan, _ := os.ReadFile(filepath.Join(publicWiki, "Roadmap.md"))
	if !strings.Contains(string(story), "[[place-order]]") || string(plan) != "---\nkind: plan\nstatus: idea\n---\nGrow [[place-order]].\n" {
		t.Errorf("N5: the story and the plan follow the rename:\n%s\n%s", story, plan)
	}

	// H2: the support of a story or a principle is the health of the
	// features it links.
	f.runOrders(t, testmap.Passed, testmap.Failed)
	_, err = f.m.ChangeWiki("lead", true, []wiki.Write{{Op: "create", ID: "orders-are-kept", Kind: wiki.Principle, Name: "Orders are kept", Description: "x.", Body: body("See [[place-order]].")}})
	must(t, err)
	v, _ := f.m.WikiView(api.PublicWikiRef)
	support := map[string]string{}
	for _, d := range v.Documents {
		support[d.ID] = d.Support
	}
	if support["order-items"] != "failing" || support["orders-are-kept"] != "failing" || support["stock-never-negative"] != "" || support["Roadmap"] != "" {
		t.Errorf("H2: support = %v", support)
	}
	if hit, _ := f.m.GetDocument("lead", "order-items"); hit.Support != "failing" {
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
	// E1: the orchestrator's own change is no notice for it; the owners
	// hear of it.
	if tc := f.m.TurnContext("lead"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: own change = %+v", tc.WikiChanged)
	}
	tc := f.m.TurnContext("orders")
	if len(tc.WikiChanged) != 1 || tc.WikiChanged[0].Wiki != "public" || len(tc.WikiChanged[0].Docs) != 2 {
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
	// The orchestrator does not hear of its visible systems' wikis.
	if tc := f.m.TurnContext("lead"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: lead hears of orders' wiki: %+v", tc.WikiChanged)
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
	ref := api.PublicWikiRef
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
	if err := f.m.WriteDocument("system:ghost", "x", "x"); !errors.Is(err, ErrNotFound) {
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
	if _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "First."}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.dir, "store", "systems", "orders", "releases", "python-library", "1.0.0", store.Wiki)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "create-order.md" {
		t.Errorf("L2: the release's wiki holds %v", entries)
	}
}
