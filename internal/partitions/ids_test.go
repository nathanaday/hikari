package partitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

// renamedTopology is a topology with a renamed repo, orchestration agent, and
// partition agent.
const renamedTopology = `
name: test
x-hikari:
  repos:
    shop: {name: store, path: shop, branch: hikari/work}
  orchestration: eng-lead
services:
  eng-lead: {x-hikari: {name: chief}}
  orders: {x-hikari: {name: intake, partition: {folders: [{repo: shop, path: services/orders}]}, grants: {deliverables: [python-library]}}}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: services/inventory}]}}}
`

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	return string(data)
}

// TestRenamedEntities checks that a renamed partition and orchestration agent work by name,
// and that the vault keys them by id (rules ID6 and ID7 of the ids spec).
func TestRenamedEntities(t *testing.T) {
	f := newFixture(t)
	git(t, f.repo, "add", "--all")
	git(t, f.repo, "commit", "--quiet", "-m", "private documents")
	topo, err := topology.Parse([]byte(renamedTopology), f.dir)
	must(t, err)
	f.m, err = New(topo)
	must(t, err)
	partitions := filepath.Join(f.dir, "vault", "partitions")

	must(t, f.m.SetPartitionInfo("intake", featuremap.PartitionPatch{Summary: ptr("Takes orders.")}))
	_, err = f.m.ChangeWiki("intake", false, []wiki.Write{{
		Op: "create", ID: "create-order", Name: "Create an order", Description: "Stores a new order.", Body: body("A customer places an order."),
	}})
	must(t, err)
	if got := read(t, filepath.Join(partitions, "orders", "read-only", "features", "map.yaml")); !strings.Contains(got, "name: intake") {
		t.Errorf("ID6: the map of intake is in the directory of its id, and names the partition:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(partitions, "orders", "read-only", "features", "create-order.md")); err != nil {
		t.Errorf("ID6: the wiki of intake: %v", err)
	}
	if _, err := os.Stat(filepath.Join(partitions, "intake")); !os.IsNotExist(err) {
		t.Errorf("ID6: no directory is keyed by the name: %v", err)
	}
	if v := f.m.PartitionView("intake"); v.PartitionAgent != "intake" || v.Folders[0].Repo != "store" || v.Repos[0].Name != "store" {
		t.Errorf("ID13: the view names the partition and its repo: partition agent %q, folders %+v", v.PartitionAgent, v.Folders)
	}

	_, err = f.m.ChangeWiki("chief", true, []wiki.Write{
		{Op: "create", ID: "order-items", Kind: wiki.Story, Name: "Order items", Description: "A customer orders items.", Body: body("Through [[create-order]].")},
	})
	must(t, err)
	if _, err := os.Stat(filepath.Join(f.dir, "vault", "public", "wiki", "order-items.md")); err != nil {
		t.Errorf("the renamed orchestration agent writes the public wiki: %v", err)
	}
	must(t, f.m.DefineTerm("chief", vocab.Term{Term: "order", Definition: "A request to buy items."}))
	if v := f.m.Vocabulary(); len(v.Terms) != 1 {
		t.Errorf("the renamed orchestration agent writes the vocabulary: %+v", v)
	}

	staging := filepath.Join(partitions, "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err = f.m.MarkCurrent("intake")
	must(t, err)
	rel, err := f.m.PublishRelease("intake", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "A release."})
	must(t, err)
	if rel.Partition != "intake" {
		t.Errorf("ID7: the release names the partition by name: %+v", rel)
	}
	manifest := read(t, filepath.Join(partitions, "orders", "read-only", "releases", "python-library", "1.0.0", "release.yaml"))
	if !strings.Contains(manifest, "partition: orders") || strings.Contains(manifest, "hubs:") {
		t.Errorf("ID7: the manifest holds ids:\n%s", manifest)
	}
	if v := f.m.PartitionView("intake"); len(v.Releases) != 1 || v.Releases[0].Partition != "intake" {
		t.Errorf("ID7: a manifest reads back with names: %+v", v.Releases)
	}
}

// TestMapAdoptsNewName checks that a map written under one name takes the
// partition's new name when the agent is renamed.
func TestMapAdoptsNewName(t *testing.T) {
	f := newFixture(t)
	f.describeOrders(t)
	f.m.CommitAll()
	src := strings.Replace(testTopology, "orders: {x-hikari: {partition:", "orders: {x-hikari: {name: intake, partition:", 1)
	topo, err := topology.Parse([]byte(src), f.dir)
	must(t, err)
	m, err := New(topo)
	must(t, err)
	if v, err := m.OwnView("intake"); err != nil || v.Partition.Name != "intake" || len(v.Features) != 1 {
		t.Errorf("the renamed partition agent keeps its map: %+v, %v", v, err)
	}
	if got := read(t, filepath.Join(f.dir, "vault", "partitions", "orders", "read-only", "features", "map.yaml")); !strings.Contains(got, "name: intake") {
		t.Errorf("the map takes the new name:\n%s", got)
	}
}
