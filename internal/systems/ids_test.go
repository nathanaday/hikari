package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// renamedTopology is a topology with a renamed repo, hub, orchestrator, and
// owner.
const renamedTopology = `
name: test
x-hikari:
  repos:
    shop: {name: store, path: shop, branch: hikari/work}
  hubs:
    eng: {name: platform, orchestrator: eng-lead, members: [orders, inventory]}
services:
  eng-lead: {x-hikari: {name: chief}}
  orders: {x-hikari: {name: intake, system: {repo: shop, scope: services/orders}, grants: {deliverables: [python-library]}}}
  inventory: {x-hikari: {system: {repo: shop, scope: services/inventory}}}
`

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	return string(data)
}

// TestRenamedEntities checks that a renamed system and hub work by name,
// and that the store keys them by id (rules ID6 and ID7 of the ids spec).
func TestRenamedEntities(t *testing.T) {
	f := newFixture(t)
	git(t, f.repo, "add", "--all")
	git(t, f.repo, "commit", "--quiet", "-m", "private documents")
	topo, err := topology.Parse([]byte(renamedTopology), f.dir)
	must(t, err)
	f.m, err = New(topo)
	must(t, err)
	systems := filepath.Join(f.dir, "store", "systems")

	must(t, f.m.SetSystemInfo("intake", sysmap.SystemPatch{Summary: ptr("Takes orders.")}))
	_, err = f.m.ChangeWiki("intake", "", []wiki.Write{{
		Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.", Body: body("A customer places an order."),
	}})
	must(t, err)
	if got := read(t, filepath.Join(systems, "orders", "map.yaml")); !strings.Contains(got, "name: intake") {
		t.Errorf("ID6: the map of intake is in the directory of its id, and names the system:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(systems, "orders", "wiki", "create-order.md")); err != nil {
		t.Errorf("ID6: the wiki of intake: %v", err)
	}
	if _, err := os.Stat(filepath.Join(systems, "intake")); !os.IsNotExist(err) {
		t.Errorf("ID6: no directory is keyed by the name: %v", err)
	}
	if v := f.m.SystemView("intake"); v.Owner != "intake" || v.Repo != "store" {
		t.Errorf("ID13: the view names the system and its repo: owner %q, repo %q", v.Owner, v.Repo)
	}

	_, err = f.m.ChangeWiki("chief", "platform", []wiki.Write{
		{Op: "create", ID: "order-items", Kind: wiki.Story, Name: "Order items", Description: "A customer orders items.", Body: body("Through [[create-order]].")},
	})
	must(t, err)
	if _, err := os.Stat(filepath.Join(f.dir, "store", "hubs", "eng", "wiki", "order-items.md")); err != nil {
		t.Errorf("ID6: the wiki of platform is in the directory of its id: %v", err)
	}
	if _, ok := f.m.WikiView(api.HubWikiRef("platform")); !ok {
		t.Error("the hub wiki is named by the hub's name")
	}

	must(t, f.m.DefineTerm("chief", "platform", vocab.Term{Term: "order", Definition: "A request to buy items."}))
	if got := read(t, filepath.Join(f.dir, "store", "shared", "vocabulary", "eng.yaml")); !strings.Contains(got, "hub: eng") {
		t.Errorf("ID6: the vocabulary file holds the hub's id:\n%s", got)
	}
	if v, err := f.m.GetVocabulary("chief", "platform"); err != nil || v.Hub != "platform" || len(v.Terms) != 1 {
		t.Errorf("the vocabulary is named by the hub's name: %+v, %v", v, err)
	}

	staging := filepath.Join(systems, "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err = f.m.MarkCurrent("intake")
	must(t, err)
	rel, hubs, err := f.m.PublishRelease("intake", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "A release."})
	must(t, err)
	if rel.System != "intake" || len(hubs) != 1 || hubs[0] != "platform" {
		t.Errorf("ID7: the release names the system and hub by name: %+v, %v", rel, hubs)
	}
	manifest := read(t, filepath.Join(systems, "orders", "releases", "python-library", "1.0.0", "release.yaml"))
	if !strings.Contains(manifest, "system: orders") || !strings.Contains(manifest, "- eng") {
		t.Errorf("ID7: the manifest holds ids:\n%s", manifest)
	}
	if v := f.m.SystemView("intake"); len(v.Releases) != 1 || v.Releases[0].System != "intake" || v.Releases[0].Hubs[0] != "platform" {
		t.Errorf("ID7: a manifest reads back with names: %+v", v.Releases)
	}
}

// TestMapAdoptsNewName checks that a map written under one name takes the
// system's new name when the agent is renamed.
func TestMapAdoptsNewName(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.m.CommitAll()
	src := strings.Replace(testTopology, "orders: {x-hikari: {system:", "orders: {x-hikari: {name: intake, system:", 1)
	topo, err := topology.Parse([]byte(src), f.dir)
	must(t, err)
	m, err := New(topo)
	must(t, err)
	if v, err := m.OwnView("intake"); err != nil || v.System.Name != "intake" || len(v.Features) != 1 {
		t.Errorf("the renamed owner keeps its map: %+v, %v", v, err)
	}
	if got := read(t, filepath.Join(f.dir, "store", "systems", "orders", "map.yaml")); !strings.Contains(got, "name: intake") {
		t.Errorf("the map takes the new name:\n%s", got)
	}
}
