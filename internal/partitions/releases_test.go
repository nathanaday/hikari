package partitions

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/wiki"
)

const releaseTopology = `
name: test
x-hikari:
  project: {branch: hikari}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  inventory: {x-hikari: {partition: {folders: [{repo: project, path: inv}]}, grants: {deliverables: [lib], tags: {lib: [dev, staging]}}}}
  orders: {x-hikari: {partition: {folders: [{repo: project, path: app}]}, grants: {releases: [inventory], pins: {inventory: [staging, production]}}}}
`

// newReleaseProject makes a project repository where inventory holds the
// deliverable lib, with the remote local-dir whose home is a directory
// beside the project, and orders is granted inventory's releases.
func newReleaseProject(t *testing.T) (dir, home string, m *Manager) {
	t.Helper()
	f := newFixture(t)
	dir = filepath.Join(f.dir, "project")
	home = filepath.Join(f.dir, "home")
	write(t, dir, ".gitignore", ".env\n/.hikari/\n")
	write(t, dir, "topology.yaml", releaseTopology)
	write(t, dir, "inv/stock.py", "def stock(): pass\n")
	write(t, dir, "app/create.py", "def create(): pass\n")
	write(t, dir, "vault/public/deliverables/lib/deliverable.yaml", "version: 1\ndescription: The library.\nlayout: {require: [README.md]}\nremote: local-dir\ntags: [dev, staging, production]\n")
	write(t, dir, "vault/public/deliverables/lib/SKILL.md", "# lib\n")
	write(t, dir, "vault/public/remotes/local-dir/remote.yaml", `version: 1
description: A directory beside the project.
push: mkdir -p "`+home+`/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION" && cp -R "$HIKARI_PRODUCT"/. "`+home+`/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION"/
pull: cp -R "`+home+`/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION"/. "$HIKARI_PRODUCT"/
`)
	git(t, dir, "init", "--quiet", "--initial-branch=main")
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "--quiet", "-m", "initial")
	return dir, home, openRelease(t, dir)
}

func openRelease(t *testing.T, dir string) *Manager {
	t.Helper()
	topo, err := topology.Parse([]byte(releaseTopology), dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(topo)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func describe(t *testing.T, dir string, m *Manager, agent, folder, file string) {
	t.Helper()
	must(t, m.SetPartitionInfo(agent, featuremap.PartitionPatch{Summary: ptr("Does things.")}))
	_, err := m.ChangeWiki(agent, false, []wiki.Write{{Op: "create", ID: agent + "-main", Name: "The main feature", Description: "Does the thing.", Body: body("It does the thing.")}})
	must(t, err)
	write(t, dir, folder+"/.hikari/private/code-map.yaml", "version: 1\nparts: [{path: project/"+folder+"/"+file+", description: The code.}]\nfeatures:\n  "+agent+"-main: {code: [project/"+folder+"/"+file+"]}\n")
	write(t, dir, folder+"/.hikari/private/tests.yaml", "version: 1\nsuites:\n  unit: {command: pytest}\n")
	if _, err := m.MarkCurrent(agent); err != nil {
		t.Fatal(err)
	}
}

func TestReleasesTravel(t *testing.T) {
	dir, home, m := newReleaseProject(t)
	ctx := context.Background()
	describe(t, dir, m, "inventory", "inv", "stock.py")
	describe(t, dir, m, "orders", "app", "create.py")
	st := m.Vault()

	// RL3, TG5, RM5: a publish with a tag pushes, writes the record, and
	// commits it; the product stays out of git.
	write(t, dir, "vault/partitions/inventory/staging/lib/README.md", "# lib\n")
	if _, err := m.PublishRelease("inventory", api.PublishRequest{Deliverable: "lib", Version: "1.0.0", Notes: "First.", Tags: []string{"production"}}); err == nil || !strings.Contains(err.Error(), "may set only the tags of lib that match dev, staging") {
		t.Errorf("TG4 a tag outside grants.tags: %v", err)
	}
	rel, err := m.PublishRelease("inventory", api.PublishRequest{Deliverable: "lib", Version: "1.0.0", Notes: "First.", Tags: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	if rel.Remote != "local-dir" {
		t.Errorf("RL6 remote = %q", rel.Remote)
	}
	a, s, files := lastCommit(t, dir)
	for _, want := range []string{"1.0.0/SHA256SUMS", "1.0.0/features/map.yaml", "1.0.0/features/inventory-main.md", "1.0.0/release.yaml", "tags.yaml"} {
		if !slices.Contains(files, "vault/partitions/inventory/read-only/releases/lib/"+want) {
			t.Errorf("RL3 release commit lacks %s: %v", want, files)
		}
	}
	if a != "inventory (hikari)" || s != "inventory: release lib 1.0.0" || slices.ContainsFunc(files, func(f string) bool { return strings.Contains(f, "/product/") }) {
		t.Errorf("RL3 release commit = %s | %s | %v", a, s, files)
	}
	if _, err := os.Stat(filepath.Join(home, "inventory", "lib", "1.0.0", "README.md")); err != nil {
		t.Errorf("RM5 the push ran: %v", err)
	}

	// TG3: a tag by the holder, within its grant.
	if err := m.TagRelease("inventory", api.TagRequest{Deliverable: "lib", Version: "1.0.0", Tag: "production"}); err == nil {
		t.Error("TG4 inventory may not set production")
	}
	if err := m.TagRelease("inventory", api.TagRequest{Deliverable: "lib", Version: "1.0.0", Tag: "rc"}); err == nil || !strings.Contains(err.Error(), "allows no tag") {
		t.Errorf("TG3 a tag the deliverable does not declare: %v", err)
	}
	must(t, m.TagRelease("inventory", api.TagRequest{Deliverable: "lib", Version: "1.0.0", Tag: "staging"}))
	if a, s, files := lastCommit(t, dir); a != "inventory (hikari)" || s != "inventory: tag lib 1.0.0 as staging" || !slices.Equal(files, []string{"vault/partitions/inventory/read-only/releases/lib/tags.yaml"}) {
		t.Errorf("RL3 tag commit = %s | %s | %v", a, s, files)
	}
	if err := m.TagRelease("orders", api.TagRequest{Deliverable: "lib", Version: "1.0.0", Tag: "staging"}); err == nil {
		t.Error("TG3 only a holder tags")
	}
	// TG6: the user sets any tag, and removes one.
	must(t, m.TagAsUser("inventory", "lib", "production", "1.0.0"))
	if a, s, _ := lastCommit(t, dir); a != "Test User" || s != "user: tag inventory lib 1.0.0 as production" {
		t.Errorf("RL3 user tag commit = %s | %s", a, s)
	}
	must(t, m.TagAsUser("inventory", "lib", "dev", ""))
	if tags, _ := st.LoadTags("inventory", "lib"); len(tags.Tags) != 3 || tags.Tags["production"] != "1.0.0" || tags.Tags["dev"] != "" {
		t.Errorf("tags = %v", tags.Tags)
	}

	// PN2, PN4: orders pins within its grant, and the product is present.
	if _, _, err := m.PinRelease(ctx, "orders", api.PinRequest{Partition: "inventory", Deliverable: "lib", Ref: "1.0.0"}); err == nil || !strings.Contains(err.Error(), "may pin only the tags") {
		t.Errorf("PN4 a version where pins restrict: %v", err)
	}
	if _, _, err := m.PinRelease(ctx, "inventory", api.PinRequest{Partition: "orders", Deliverable: "lib", Ref: "1.0.0"}); err == nil {
		t.Error("PN2 a partition not granted")
	}
	pin, note, err := m.PinRelease(ctx, "orders", api.PinRequest{Partition: "inventory", Deliverable: "lib", Ref: "staging"})
	if err != nil || pin.Tag != "staging" || !strings.Contains(note, "points at 1.0.0") {
		t.Fatalf("PN2 pin = %+v, %q, %v", pin, note, err)
	}
	if a, s, files := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: pin inventory lib staging" || !slices.Equal(files, []string{"vault/partitions/orders/read-only/pins.yaml"}) {
		t.Errorf("RL3 pin commit = %s | %s | %v", a, s, files)
	}
	if pins := m.Pins("orders"); len(pins) != 1 || pins[0].Partition != "inventory" {
		t.Errorf("PN7 pins = %+v", pins)
	}
	if self := m.Self("orders"); len(self.Pins) != 1 {
		t.Errorf("PN7 Self.Pins = %+v", self.Pins)
	}

	// LS1: the catalog, as each agent reads it.
	list, err := m.ListReleases("orders", "")
	if err != nil || len(list) != 1 || list[0].Partition != "inventory" || !slices.Equal(list[0].Tags, []string{"latest", "production", "staging"}) || !list[0].Product || !slices.Equal(list[0].PinnedBy, []string{"orders"}) {
		t.Errorf("LS1 orders' catalog = %+v, %v", list, err)
	}
	if list, err := m.ListReleases("lead", "inventory"); err != nil || len(list) != 1 {
		t.Errorf("LS1 lead's catalog = %+v, %v", list, err)
	}
	if _, err := m.ListReleases("inventory", "orders"); err == nil {
		t.Error("RM7 inventory does not read orders' releases")
	}
	if v, err := m.GetPartition("lead", "orders"); err != nil || len(v.Pins) != 1 || len(v.Releases) != 0 {
		t.Errorf("PN7 the orchestration view carries the pins: %+v, %v", v, err)
	}

	// RM7: a pull of an absent product.
	product := filepath.Join(st.ReleaseDir("inventory", "lib", "1.0.0"), vault.ProductDir)
	os.RemoveAll(product)
	if _, err := m.PullRelease(ctx, "inventory", api.ReleaseRef{Partition: "orders", Deliverable: "lib", Ref: "1.0.0"}); err == nil {
		t.Error("RM7 a pull needs read access")
	}
	if version, err := m.PullRelease(ctx, "orders", api.ReleaseRef{Partition: "inventory", Deliverable: "lib", Ref: "production"}); err != nil || version != "1.0.0" {
		t.Errorf("RM7 pull = %q, %v", version, err)
	}
	if _, err := os.Stat(filepath.Join(product, "README.md")); err != nil {
		t.Error("RM7 the product is back")
	}

	// RM8: a release built elsewhere is recorded from the remote.
	write(t, home, "inventory/lib/1.1.0/README.md", "# lib 1.1.0\n")
	if _, err := m.RecordRelease(ctx, "inventory", api.PublishRequest{Deliverable: "lib", Version: "1.1.0", Notes: "Built by CI.", Tags: []string{"dev"}}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(st.ReleaseDir("inventory", "lib", "1.1.0"), vault.ProductDir, "README.md")); string(data) != "# lib 1.1.0\n" {
		t.Errorf("RM8 recorded product: %q", data)
	}
	if tags, _ := st.LoadTags("inventory", "lib"); tags.Tags["latest"] != "1.1.0" || tags.Tags["dev"] != "1.1.0" || tags.Tags["staging"] != "1.0.0" {
		t.Errorf("RM8 tags after record: %v", tags.Tags)
	}
	if _, err := m.RecordRelease(ctx, "inventory", api.PublishRequest{Deliverable: "lib", Version: "1.2.0", Notes: "Nope."}); err == nil {
		t.Error("RM8 a version the remote lacks fails")
	}
	if entries, _ := os.ReadDir(st.StagingOf("inventory", "lib")); len(entries) != 0 {
		t.Errorf("RM8 a failed record leaves nothing staged: %v", entries)
	}

	// PN8: the user pins for a consumer, with its grant checks.
	if _, _, err := m.PinAsUser(ctx, "orders", api.PinRequest{Partition: "inventory", Deliverable: "lib", Ref: "dev"}); err == nil {
		t.Error("PN8 the consumer's pins grant applies")
	}
	if _, _, err := m.PinAsUser(ctx, "orders", api.PinRequest{Partition: "inventory", Deliverable: "lib", Ref: "production"}); err != nil {
		t.Fatal(err)
	}
	if a, s, _ := lastCommit(t, dir); a != "Test User" || s != "user: pin inventory lib production" {
		t.Errorf("RL3 user pin commit = %s | %s", a, s)
	}
	must(t, m.UnpinRelease("orders", api.PinRequest{Partition: "inventory", Deliverable: "lib"}))
	if a, s, _ := lastCommit(t, dir); a != "orders (hikari)" || s != "orders: unpin inventory lib" {
		t.Errorf("RL3 unpin commit = %s | %s", a, s)
	}

	// RL4, PN6: a clone has the records and no products; the start keeps the
	// catalog and warns about the pin.
	if _, _, err := m.PinAsUser(ctx, "orders", api.PinRequest{Partition: "inventory", Deliverable: "lib", Ref: "staging"}); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(filepath.Dir(dir), "clone")
	git(t, dir, "clone", "--quiet", "--branch", "hikari", dir, clone)
	m2 := openRelease(t, clone)
	if versions, _ := m2.Vault().Versions("inventory", "lib"); !slices.Equal(versions, []string{"1.1.0", "1.0.0"}) {
		t.Errorf("RL1 the clone has the records: %v", versions)
	}
	cat, _ := m2.ListReleases("lead", "inventory")
	if len(cat) != 2 || cat[0].Product || cat[1].Product {
		t.Errorf("RL7 the clone has no products: %+v", cat)
	}
	if link, _ := os.Readlink(filepath.Join(m2.Vault().TagsDir("inventory", "lib"), "staging")); link != "../1.0.0" {
		t.Errorf("RL5 the clone's tag links are written at start: %q", link)
	}
	if w := m2.Warnings(); !slices.ContainsFunc(w, func(s string) bool { return strings.Contains(s, "orders pins inventory lib staging, whose product is absent on this machine; run hikari sync") }) {
		t.Errorf("PN6 warnings = %v", w)
	}
	if version, err := m2.PullAsUser(ctx, api.ReleaseRef{Partition: "inventory", Deliverable: "lib", Ref: "staging"}); err != nil || version != "1.0.0" {
		t.Errorf("RM7 pull in the clone = %q, %v", version, err)
	}
	if w := openRelease(t, clone).Warnings(); slices.ContainsFunc(w, func(s string) bool { return strings.Contains(s, "orders pins") }) {
		t.Errorf("PN6 no warning once the product is present: %v", w)
	}

	// RL4: a vault from before this version ignored releases/ whole, so its
	// records are on disk and not in git; the start writes the ignore file
	// and commits the records.
	write(t, clone, "vault/.gitignore", "partitions/*/staging/\npartitions/*/read-only/releases/\n")
	git(t, clone, "rm", "-r", "--cached", "--quiet", "vault/partitions/inventory/read-only/releases")
	git(t, clone, "commit", "--quiet", "--all", "-m", "the old layout")
	openRelease(t, clone)
	a, s, files = lastCommit(t, clone)
	if a != "hikari (hikari)" || s != "hikari: add the records of releases" {
		t.Errorf("RL4 commit = %s | %s | %v", a, s, files)
	}
	for _, want := range []string{"lib/1.1.0/release.yaml", "lib/1.0.0/release.yaml", "lib/tags.yaml"} {
		if !slices.Contains(files, "vault/partitions/inventory/read-only/releases/"+want) {
			t.Errorf("RL4 commit lacks %s: %v", want, files)
		}
	}
	if slices.ContainsFunc(files, func(f string) bool { return strings.Contains(f, "/product/") || strings.Contains(f, "/tags/") }) {
		t.Errorf("RL2 the products and the tag links stay out of git: %v", files)
	}
}
