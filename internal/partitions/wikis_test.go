package partitions

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/wiki"
)

// publicRules writes the rules order-items, which links orders'
// create-order, and stock-never-negative in the public wiki.
func (f *fixture) publicRules(t *testing.T) {
	t.Helper()
	_, err := f.m.ChangeWiki("lead", true, []wiki.Write{
		{Op: "create", ID: "order-items", Kind: wiki.Rule, Name: "Order items", Description: "An order holds the items a customer orders.", Body: body("The customer orders through [[create-order]].")},
		{Op: "create", ID: "stock-never-negative", Kind: wiki.Rule, Name: "Stock never negative", Description: "Stock never goes below zero.", Body: body("Order items keeps it: [[order-items]].")},
	})
	must(t, err)
}

func TestPublicWikiAccess(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.publicRules(t)
	if _, err := os.Stat(filepath.Join(f.dir, "vault", "public", "wiki", "order-items.md")); err != nil {
		t.Errorf("L1: the rule is in the public wiki: %v", err)
	}
	write := []wiki.Write{{Op: "create", ID: "x", Kind: wiki.Rule, Name: "X", Description: "X.", Body: body("x")}}
	if _, err := f.m.ChangeWiki("orders", true, write); !errors.Is(err, ErrForbidden) {
		t.Errorf("T2: a partition agent changes the public wiki: %v", err)
	}
	// K10: no wiki holds a story.
	story := []wiki.Write{{Op: "create", ID: "x", Kind: "story", Name: "X", Description: "X.", Body: body("x")}}
	for _, public := range []bool{true, false} {
		if _, err := f.m.ChangeWiki("lead", public, story); err == nil || !strings.Contains(err.Error(), "K10") || !strings.Contains(err.Error(), "orchestration feature map as a feature") {
			t.Errorf("K10: a story: %v", err)
		}
	}
	// N2: the public wiki links every partition, and a feature map links the
	// public wiki, not another partition.
	if _, err := f.m.ChangeWiki("orders", false, []wiki.Write{{Op: "create", ID: "orders-never-oversell", Kind: wiki.Rule, Name: "Never oversell", Description: "Orders never sell stock that is not there.", Body: body("It keeps [[stock-never-negative]].")}}); err != nil {
		t.Errorf("N2: a partition rule links a public rule: %v", err)
	}
	if _, err := f.m.ChangeWiki("inventory", false, []wiki.Write{{Op: "create", ID: "sells", Kind: wiki.Principle, Name: "Sells", Description: "x.", Body: body("See [[create-order]].")}}); err == nil {
		t.Error("N2: inventory links a feature of orders")
	}

	// A1 to A4: what each agent reads.
	if _, err := f.m.GetDocument("orders", "order-items"); err != nil {
		t.Errorf("A1: a partition agent reads the public wiki: %v", err)
	}
	if _, err := f.m.GetDocument("lead", "create-order"); err != nil {
		t.Errorf("A2: the orchestration agent reads every feature map: %v", err)
	}
	if _, err := f.m.GetDocument("inventory", "create-order"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "wiki of orders, which you cannot read") {
		t.Errorf("T4: a peer's feature: %v", err)
	}
	hit, err := f.m.GetDocument("orders", "create-order")
	if err != nil || len(hit.Backlinks) != 1 || hit.Backlinks[0] != (api.DocRef{Wiki: "public", ID: "order-items", Kind: wiki.Rule, Name: "Order items"}) {
		t.Errorf("T4, N6: backlinks = %+v, %v", hit.Backlinks, err)
	}
	if hits, _ := f.m.SearchWiki("orders", "customer"); len(hits) != 2 {
		t.Errorf("T5: orders finds its feature and the rule: %+v", hits)
	}
}

func TestRenameAcrossWikisAndSupport(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.publicRules(t)
	publicWiki := filepath.Join(f.dir, "vault", "public", "wiki")
	os.WriteFile(filepath.Join(publicWiki, "Roadmap.md"), []byte("---\nkind: plan\nstatus: idea\n---\nGrow [[create-order]].\n"), 0o644)
	f.m.wikis[api.PublicWikiRef].files.Reload()

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
		t.Errorf("N5: the rule and the plan follow the rename:\n%s\n%s", story, plan)
	}

	// H2: the support of a rule or a principle is the health of the
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
	sys, _ := f.m.WikiView(api.FeatureMapRef("orders"))
	if len(sys.Documents) != 1 || sys.Documents[0].Health != "failing" || sys.Documents[0].File != "partitions/orders/read-only/features/place-order.md" || sys.Documents[0].Hash == "" {
		t.Errorf("U: the feature map view = %+v", sys.Documents)
	}
}

func TestWikiNotices(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.TurnContext(t)
	f.publicRules(t)
	// E1: the orchestration agent's own change is no notice for it; the partition agents
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
	// A plan that the user writes in a feature map is news to its partition agent.
	must(t, f.m.WriteDocument(api.FeatureMapRef("orders"), "Next steps", "Add refunds.\n"))
	tc = f.m.TurnContext("orders")
	if len(tc.WikiChanged) != 1 || tc.WikiChanged[0].Wiki != "orders" || tc.WikiChanged[0].Docs[0] != (api.DocChange{ID: "Next steps", Kind: wiki.Note}) {
		t.Errorf("E1: the user's note = %+v", tc.WikiChanged)
	}
	// The orchestration agent does not hear of its visible partitions' wikis.
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
	f.publicRules(t)
	ref := api.PublicWikiRef
	if err := f.m.RemoveDocument(ref, "order-items"); err == nil {
		t.Error("U8: an agent document is not removed in the UI")
	}
	bad := "---\nid: refunds\nkind: rule\nname: Refunds\ndescription: A customer gets a refund.\n---\n\n## Purpose\n\nThrough [[ghost]].\n"
	if err := f.m.WriteDocument(ref, "refunds", bad); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("U8: a rule with a dead link: %v", err)
	}
	if err := f.m.WriteDocument(ref, "refunds", strings.Replace(bad, "[[ghost]]", "[[create-order]]", 1)); err != nil {
		t.Errorf("decision 8: you write a rule: %v", err)
	}
	if err := f.m.WriteDocument(ref, "refunds", strings.Replace(bad, "kind: rule", "kind: story", 1)); err == nil || !strings.Contains(err.Error(), "K10") {
		t.Errorf("K10: you write a story: %v", err)
	}
	if err := f.m.WriteDocument(ref, "a/b", "x"); err == nil {
		t.Error("D2: a slash in a name")
	}
	if err := f.m.WriteDocument("partition:ghost", "x", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such wiki: %v", err)
	}
	must(t, f.m.WriteDocument(api.FeatureMapRef("orders"), "Ideas", "Refunds?\n"))
	must(t, f.m.RemoveDocument(api.FeatureMapRef("orders"), "Ideas"))
}

func TestReleaseCopiesAgentDocuments(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	must(t, f.m.WriteDocument(api.FeatureMapRef("orders"), "Ideas", "Refunds?\n"))
	_, err := f.m.MarkCurrent("orders")
	must(t, err)
	staging := filepath.Join(f.dir, "vault", "partitions", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# Orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	if _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "First."}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.dir, "vault", "partitions", "orders", "read-only", "releases", "python-library", "1.0.0", vault.FeaturesDir)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 || entries[0].Name() != "create-order.md" || entries[1].Name() != "map.yaml" {
		t.Errorf("L2: the release's wiki holds %v", entries)
	}
}

// TestOrchestrationFeatureMap checks the orchestration agent's feature map
// (rules K4 to K8, K10, and K11 of the skill packs spec).
func TestOrchestrationFeatureMap(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	_, err := f.m.ChangeWiki("inventory", false, []wiki.Write{{Op: "create", ID: "reserve-stock", Name: "Reserve stock", Description: "Holds stock for an order.", Body: body("An order holds the stock it needs.")}})
	must(t, err)
	f.publicRules(t)
	f.TurnContext(t)

	// K4, decision 2: change_wiki writes the orchestration agent's own map.
	created, err := f.m.ChangeWiki("lead", false, []wiki.Write{{Op: "create", ID: "customer-orders", Name: "A customer orders", Description: "A customer orders items, and stock is held for each.",
		Related: []string{"stock-never-negative"}, Body: body("The customer orders through [[create-order]], which holds stock with [[reserve-stock]].")}})
	must(t, err)
	if strings.Contains(strings.Join(created.Warnings, "\n"), "no neighbor") {
		t.Errorf("N7: the partition features it links are its neighbors: %v", created.Warnings)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "vault", "orchestration", "read-only", "features", "customer-orders.md")); err != nil {
		t.Errorf("K4: the feature is in the orchestration feature map: %v", err)
	}
	health := func() string {
		v, _ := f.m.WikiView(api.OrchestrationWikiRef)
		if v.Scope != "orchestration" || len(v.Documents) != 1 || v.Documents[0].Kind != wiki.Feature {
			t.Fatalf("K4: the view = %+v", v)
		}
		return v.Documents[0].Health
	}
	// K6: its health follows the partition features it links.
	if h := health(); h != "pending" {
		t.Errorf("K6: create-order's test has not run: %q", h)
	}
	f.runOrders(t, testmap.Passed)
	if h := health(); h != "passing" {
		t.Errorf("K6: a linked feature passes: %q", h)
	}
	f.runOrders(t, testmap.Failed)
	if h := health(); h != "failing" {
		t.Errorf("K6: a linked feature fails: %q", h)
	}
	if h := orchestrationHealth(&wiki.Document{Body: "## Purpose\n\nKeeps [[stock-never-negative]].\n"}, f.m.featureHealth()); h != "untested" {
		t.Errorf("K6: a feature that links no partition feature: %q", h)
	}
	if hit, err := f.m.GetDocument("lead", "customer-orders"); err != nil || hit.Wiki != "orchestration" || hit.Health != "failing" {
		t.Errorf("K8: the orchestration agent reads its feature: %+v, %v", hit, err)
	}

	// K5: no other wiki links it, and only the orchestration agent reads it.
	link := []wiki.Write{{Op: "create", ID: "x", Kind: wiki.Rule, Name: "X", Description: "X.", Body: body("See [[customer-orders]].")}}
	if _, err := f.m.ChangeWiki("orders", false, link); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("K5: a feature map links the orchestration feature map: %v", err)
	}
	if _, err := f.m.ChangeWiki("lead", true, link); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("K5: the public wiki links the orchestration feature map: %v", err)
	}
	if _, err := f.m.GetDocument("orders", "customer-orders"); !errors.Is(err, ErrNotFound) {
		t.Errorf("K5: a partition agent reads the orchestration feature map: %v", err)
	}

	// K11: a partition agent sees no backlink from the orchestration feature
	// map; the orchestration agent sees it.
	backlinks := func(agent string) []string {
		hit, err := f.m.GetDocument(agent, "create-order")
		must(t, err)
		var out []string
		for _, b := range hit.Backlinks {
			out = append(out, b.Wiki+"/"+b.ID)
		}
		return out
	}
	if got := backlinks("orders"); !slices.Equal(got, []string{"public/order-items"}) {
		t.Errorf("K11: orders sees the backlinks %v", got)
	}
	if got := backlinks("lead"); !slices.Equal(got, []string{"orchestration/customer-orders", "public/order-items"}) {
		t.Errorf("K11: lead sees the backlinks %v", got)
	}
	if hits, _ := f.m.SearchWiki("orders", "customer-orders"); len(hits) != 0 {
		t.Errorf("K11: orders finds %+v", hits)
	}

	// K7, K8: a rename of a partition feature rewrites the link, and the
	// orchestration agent hears that someone else changed its document.
	if tc := f.m.TurnContext("lead"); len(tc.WikiChanged) != 0 {
		t.Errorf("E1: its own change is no notice: %+v", tc.WikiChanged)
	}
	res, err := f.m.ChangeWiki("orders", false, []wiki.Write{{Op: "rename", ID: "create-order", To: "place-order"}})
	must(t, err)
	if !slices.Contains(res.Rewritten, "orchestration/customer-orders") {
		t.Errorf("K7: rewritten = %v", res.Rewritten)
	}
	data, _ := os.ReadFile(filepath.Join(f.dir, "vault", "orchestration", "read-only", "features", "customer-orders.md"))
	if !strings.Contains(string(data), "[[place-order]]") {
		t.Errorf("K7: the link follows the rename:\n%s", data)
	}
	tc := f.m.TurnContext("lead")
	if len(tc.WikiChanged) != 2 || tc.WikiChanged[0].Wiki != "orchestration" || tc.WikiChanged[0].Docs[0].ID != "customer-orders" || tc.WikiChanged[1].Wiki != "public" {
		t.Errorf("K8: lead hears %+v", tc.WikiChanged)
	}
	if tc := f.m.TurnContext("orders"); len(tc.WikiChanged) != 0 {
		t.Errorf("K8: orders hears of the orchestration feature map: %+v", tc.WikiChanged)
	}

	// K12: you write a plan there.
	must(t, f.m.WriteDocument(api.OrchestrationWikiRef, "Next", "---\nkind: plan\n---\nRefunds [[customer-orders]].\n"))
	if tc := f.m.TurnContext("lead"); len(tc.WikiChanged) != 1 || tc.WikiChanged[0].Docs[0] != (api.DocChange{ID: "Next", Kind: wiki.Plan}) {
		t.Errorf("K12: lead hears of your plan: %+v", tc.WikiChanged)
	}
}
