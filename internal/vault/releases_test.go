package vault

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A remote whose home is a directory on this machine: the shop's example,
// in short (rule EX1).
func dirRemote(t *testing.T, s Vault, home string) {
	t.Helper()
	writeFiles(t, s.RemoteDir("local-dir"), map[string]string{RemoteFile: `version: 1
description: A directory on this machine.
push: |
  dest="` + home + `/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION"
  mkdir -p "$dest" && cp -R "$HIKARI_PRODUCT"/. "$dest"/ && echo "$HIKARI_DIGEST" > "$dest.digest"
pull: cp -R "` + home + `/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION"/. "$HIKARI_PRODUCT"/
timeout: 1m
`})
}

func TestTags(t *testing.T) {
	s := newVault(t)
	defineDeliverable(t, s, "python-library", pythonLibrary+"tags: [dev, staging, production, \"rc-[0-9]+\"]\n")
	d, err := s.LoadDeliverable("python-library")
	if err != nil {
		t.Fatal(err)
	}
	for tag, want := range map[string]bool{"dev": true, "production": true, "rc-3": true, "rc-x": false, "prod": false, "latest": false} {
		if d.AllowsTag(tag) != want {
			t.Errorf("TG2 AllowsTag(%q) = %v", tag, !want)
		}
	}
	if _, errs := CompileTagPatterns([]string{"(", ""}); len(errs) != 2 {
		t.Errorf("TG2 invalid patterns: %v", errs)
	}
	defineDeliverable(t, s, "bad", "version: 1\ndescription: x\ntags: ['(']\n")
	if _, err := s.LoadDeliverable("bad"); err == nil || !strings.Contains(err.Error(), "tag pattern") {
		t.Errorf("TG2 a deliverable with an invalid pattern is invalid: %v", err)
	}

	stage(t, s, "python-library", goodProduct)
	in := input("1.0.0")
	in.Tags = []string{"dev"}
	if _, err := s.Publish(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.LoadTags("inventory", "python-library")
	if tags.Tags["latest"] != "1.0.0" || tags.Tags["dev"] != "1.0.0" {
		t.Errorf("TG5 tags after publish: %v", tags.Tags)
	}
	stage(t, s, "python-library", goodProduct)
	in = input("1.1.0")
	in.Tags = []string{"latest"}
	if _, err := s.Publish(t.Context(), in); err == nil || !strings.Contains(err.Error(), `tag "latest"`) {
		t.Errorf("TG1 latest cannot be set: %v", err)
	}
	in.Tags = nil
	if _, err := s.Publish(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTag("inventory", "python-library", "production", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTag("inventory", "python-library", "production", "9.9.9"); err == nil || !strings.Contains(err.Error(), "no release 9.9.9") {
		t.Errorf("TG3 a tag needs a released version: %v", err)
	}
	if err := s.SetTag("inventory", "python-library", "latest", "1.0.0"); err == nil {
		t.Error("TG1 latest is hikari's")
	}
	if err := s.SetTag("inventory", "python-library", "Prod", "1.0.0"); err == nil {
		t.Error("TG1 a tag name is lower case")
	}
	tags, _ = s.LoadTags("inventory", "python-library")
	if tags.Tags["latest"] != "1.1.0" || tags.Tags["dev"] != "1.0.0" || tags.Tags["production"] != "1.0.0" {
		t.Errorf("tags: %v", tags.Tags)
	}
	for tag, want := range map[string]string{"latest": "../1.1.0", "dev": "../1.0.0", "production": "../1.0.0"} {
		if link, _ := os.Readlink(filepath.Join(s.TagsDir("inventory", "python-library"), tag)); link != want {
			t.Errorf("RL5 tags/%s -> %q, want %q", tag, link, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(s.DeliverableReleasesDir("inventory", "python-library"), "latest")); err == nil {
		t.Error("RL5 the old latest link is gone")
	}
	if err := s.RemoveTag("inventory", "python-library", "dev"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.TagsDir("inventory", "python-library"), "dev")); err == nil {
		t.Error("RL5 a removed tag has no link")
	}
	if err := s.RemoveTag("inventory", "python-library", "latest"); err == nil {
		t.Error("TG6 latest cannot be removed")
	}
	for ref, want := range map[string]string{"1.0.0": "1.0.0", "production": "1.0.0", "latest": "1.1.0"} {
		if v, err := s.Resolve("inventory", "python-library", ref); v != want || err != nil {
			t.Errorf("Resolve(%q) = %q, %v", ref, v, err)
		}
	}
	if _, err := s.Resolve("inventory", "python-library", "dev"); err == nil {
		t.Error("Resolve of a removed tag fails")
	}

	// RL4: a vault from before tags has only the version directories and the
	// old link; RefreshTags writes tags.yaml and the links.
	os.Remove(s.TagsFile("inventory", "python-library"))
	os.RemoveAll(s.TagsDir("inventory", "python-library"))
	os.Symlink("1.1.0", filepath.Join(s.DeliverableReleasesDir("inventory", "python-library"), "latest"))
	wrote, err := s.RefreshTags("inventory")
	if err != nil || !slices.Equal(wrote, []string{"partitions/inventory/read-only/releases/python-library/tags.yaml"}) {
		t.Errorf("RL4 RefreshTags = %v, %v", wrote, err)
	}
	tags, _ = s.LoadTags("inventory", "python-library")
	if len(tags.Tags) != 1 || tags.Tags["latest"] != "1.1.0" {
		t.Errorf("RL4 refreshed tags: %v", tags.Tags)
	}
	if _, err := os.Lstat(filepath.Join(s.DeliverableReleasesDir("inventory", "python-library"), "latest")); err == nil {
		t.Error("RL5 the old latest link is removed at refresh")
	}
	if wrote, _ := s.RefreshTags("inventory"); len(wrote) != 0 {
		t.Errorf("RL4 a second refresh writes nothing: %v", wrote)
	}

	cat, err := s.Catalog("inventory")
	if err != nil || len(cat) != 2 || cat[0].Version != "1.1.0" || !slices.Equal(cat[0].Tags, []string{"latest"}) || !cat[0].Product || len(cat[1].Tags) != 0 {
		t.Errorf("LS1 catalog: %+v, %v", cat, err)
	}
	os.RemoveAll(filepath.Join(s.ReleaseDir("inventory", "python-library", "1.0.0"), ProductDir))
	cat, _ = s.Catalog("inventory")
	if cat[1].Product {
		t.Error("RL7 a release without product/ is listed as absent")
	}
	if paths := s.RecordPaths("inventory", "python-library", "1.1.0"); !slices.Equal(paths, []string{
		"partitions/inventory/read-only/releases/python-library/1.1.0", "partitions/inventory/read-only/releases/python-library/tags.yaml"}) {
		t.Errorf("RL3 RecordPaths = %v", paths)
	}
}

func TestRemote(t *testing.T) {
	s := newVault(t)
	home := filepath.Join(t.TempDir(), "home")
	dirRemote(t, s, home)
	if r, err := s.LoadRemote("local-dir"); err != nil || r.Wait().Minutes() != 1 {
		t.Fatalf("RM1 LoadRemote = %+v, %v", r, err)
	}
	for name, text := range map[string]string{
		"no-commands": "version: 1\ndescription: x\n",
		"bad-timeout": "version: 1\ndescription: x\npull: true\ntimeout: soon\n",
		"unknown":     "version: 1\ndescription: x\npull: true\nextra: 1\n",
		"no-version":  "description: x\npull: true\n",
	} {
		writeFiles(t, s.RemoteDir(name), map[string]string{RemoteFile: text})
		if _, err := s.LoadRemote(name); err == nil {
			t.Errorf("RM1 %s is invalid", name)
		}
	}
	if _, err := s.LoadRemote("missing"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("RM1 a missing remote: %v", err)
	}
	if names, _ := s.RemoteNames(); len(names) != 5 {
		t.Errorf("RemoteNames = %v", names)
	}

	defineDeliverable(t, s, "python-library", pythonLibrary+"remote: local-dir\n")
	defineDeliverable(t, s, "orphan", "version: 1\ndescription: x\nremote: missing\n")
	if _, err := s.LoadDeliverable("orphan"); err == nil || !strings.Contains(err.Error(), "remote") {
		t.Errorf("RM2 a deliverable with a missing remote is invalid: %v", err)
	}

	// RM5: the push runs with the record written and the product staged.
	stage(t, s, "python-library", goodProduct)
	m, err := s.Publish(t.Context(), input("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Remote != "local-dir" {
		t.Errorf("RL6 manifest.remote = %q", m.Remote)
	}
	pushed := filepath.Join(home, "inventory", "python-library", "1.0.0")
	if _, err := os.Stat(filepath.Join(pushed, "src", "inventory", "stock.py")); err != nil {
		t.Errorf("RM5 the push copied the product: %v", err)
	}
	if digest, _ := os.ReadFile(pushed + ".digest"); strings.TrimSpace(string(digest)) != m.Product.Digest {
		t.Errorf("RM3 HIKARI_DIGEST = %q, want %s", digest, m.Product.Digest)
	}
	data, _ := os.ReadFile(filepath.Join(s.ReleaseDir("inventory", "python-library", "1.0.0"), ManifestFile))
	if !strings.Contains(string(data), "remote: local-dir") {
		t.Errorf("RL6 written manifest:\n%s", data)
	}

	// RM5: a push that fails leaves no release and the staging area intact.
	writeFiles(t, s.RemoteDir("local-dir"), map[string]string{RemoteFile: "version: 1\ndescription: x\npush: 'echo nope; exit 3'\n"})
	stage(t, s, "python-library", goodProduct)
	if _, err := s.Publish(t.Context(), input("1.1.0")); err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("RM4 a failed push: %v", err)
	}
	if _, err := os.Stat(s.ReleaseDir("inventory", "python-library", "1.1.0")); err == nil {
		t.Error("RM5 a failed push leaves no release")
	}
	if entries, _ := os.ReadDir(s.StagingOf("inventory", "python-library")); len(entries) == 0 {
		t.Error("RM5 a failed push leaves the staging area")
	}
	if versions, _ := s.Versions("inventory", "python-library"); !slices.Equal(versions, []string{"1.0.0"}) {
		t.Errorf("versions after a failed push: %v", versions)
	}
	writeFiles(t, s.RemoteDir("local-dir"), map[string]string{RemoteFile: "version: 1\ndescription: x\npush: sleep 5\ntimeout: 1s\n"})
	if _, err := s.Publish(t.Context(), input("1.1.0")); err == nil || !strings.Contains(err.Error(), "ran past 1s") {
		t.Errorf("RM4 a push past its timeout: %v", err)
	}
	dirRemote(t, s, home)

	// RM6: a pull gets the product back and checks it.
	release := s.ReleaseDir("inventory", "python-library", "1.0.0")
	if err := os.RemoveAll(filepath.Join(release, ProductDir)); err != nil {
		t.Fatal(err)
	}
	if err := s.Pull(t.Context(), "inventory", "python-library", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(release, ProductDir, "README.md")); err != nil || string(data) != "# inventory\n" {
		t.Errorf("RM6 pulled product: %q, %v", data, err)
	}
	if err := s.Pull(t.Context(), "inventory", "python-library", "1.0.0"); err != nil {
		t.Errorf("RM6 a present product is left alone: %v", err)
	}
	os.RemoveAll(filepath.Join(release, ProductDir))
	os.WriteFile(filepath.Join(pushed, "README.md"), []byte("tampered\n"), 0o644)
	os.WriteFile(filepath.Join(pushed, "extra.txt"), []byte("x\n"), 0o644)
	os.Remove(filepath.Join(pushed, "src", "inventory", "__init__.py"))
	err = s.Pull(t.Context(), "inventory", "python-library", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "README.md has another digest") || !strings.Contains(err.Error(), "extra.txt is not in the record") || !strings.Contains(err.Error(), "src/inventory/__init__.py is missing") {
		t.Errorf("RM6 a mismatch names the files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(release, ProductDir)); err == nil {
		t.Error("RM6 a mismatched pull leaves no product")
	}
	if entries, _ := os.ReadDir(release); slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == pulling }) {
		t.Error("RM6 a mismatched pull leaves no temporary directory")
	}
	if err := s.Pull(t.Context(), "inventory", "python-library", "2.0.0"); err == nil || !strings.Contains(err.Error(), "no release 2.0.0") {
		t.Errorf("RM6 pull of an unknown version: %v", err)
	}
	defineDeliverable(t, s, "docs", "version: 1\ndescription: The docs.\n")
	stage(t, s, "docs", map[string]string{"index.md": "hi\n"})
	in := input("0.1.0")
	in.Deliverable = "docs"
	if _, err := s.Publish(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(s.ReleaseDir("inventory", "docs", "0.1.0"), ProductDir))
	if err := s.Pull(t.Context(), "inventory", "docs", "0.1.0"); err == nil || !strings.Contains(err.Error(), "publish it again on this machine") {
		t.Errorf("RM6 a deliverable without a remote: %v", err)
	}
	if s.CanPull("docs") || !s.CanPull("python-library") {
		t.Error("CanPull")
	}

	// RM8: a pull into the staging area, for record_release.
	os.WriteFile(filepath.Join(pushed, "README.md"), []byte("# inventory\n"), 0o644)
	os.Remove(filepath.Join(pushed, "extra.txt"))
	os.WriteFile(filepath.Join(pushed, "src", "inventory", "__init__.py"), nil, 0o644)
	staging := s.StagingOf("inventory", "python-library")
	os.RemoveAll(staging)
	if name, err := s.PullInto(t.Context(), "inventory", "python-library", "1.0.0", staging); err != nil || name != "local-dir" {
		t.Fatalf("RM8 PullInto = %q, %v", name, err)
	}
	if _, err := os.Stat(filepath.Join(staging, "README.md")); err != nil {
		t.Error("RM8 the staging area holds the pulled product")
	}
	if _, err := s.PullInto(t.Context(), "inventory", "python-library", "1.0.0", staging); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Errorf("RM8 a staging area that is not empty: %v", err)
	}
}

func TestPins(t *testing.T) {
	s := newVault(t)
	s.Partitions = map[string]string{"orders": "orders-id"}
	s.Prepare([]string{"inventory", "orders"}, false)
	defineDeliverable(t, s, "python-library", pythonLibrary)
	stage(t, s, "python-library", goodProduct)
	in := input("1.0.0")
	in.Tags = []string{"staging"}
	if _, err := s.Publish(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if pins, err := s.LoadPins("orders"); err != nil || pins != nil {
		t.Errorf("PN1 no pins yet: %v, %v", pins, err)
	}
	if _, err := s.SetPin("orders", "inventory", "python-library", "2.0.0"); err == nil {
		t.Error("PN2 a pin needs a released version")
	}
	if _, err := s.SetPin("orders", "inventory", "python-library", "production"); err == nil {
		t.Error("PN2 a pin needs a tag of tags.yaml")
	}
	pin, err := s.SetPin("orders", "inventory", "python-library", "1.0.0")
	if err != nil || pin.Version != "1.0.0" || pin.Tag != "" || pin.Ref() != "1.0.0" {
		t.Fatalf("PN2 SetPin = %+v, %v", pin, err)
	}
	pin, err = s.SetPin("orders", "inventory", "python-library", "staging")
	if err != nil || pin.Tag != "staging" || pin.Version != "" {
		t.Fatalf("PN2 SetPin by tag = %+v, %v", pin, err)
	}
	pins, _ := s.LoadPins("orders")
	if len(pins) != 1 || pins[0].Partition != "inventory" || pins[0].Tag != "staging" {
		t.Errorf("PN1 one pin for each partition and deliverable: %+v", pins)
	}
	data, _ := os.ReadFile(s.PinsFile("orders"))
	if !strings.Contains(string(data), "partition: inventory\n") || !strings.Contains(string(data), "tag: staging\n") || strings.Contains(string(data), "version: 1.0.0") {
		t.Errorf("PN1 pins.yaml:\n%s", data)
	}
	if by := s.PinnedBy([]string{"orders", "inventory"}, "inventory", "python-library", "1.0.0"); !slices.Equal(by, []string{"orders"}) {
		t.Errorf("PinnedBy = %v", by)
	}
	if err := s.RemovePin("orders", "inventory", "docs"); err == nil {
		t.Error("PN3 removing a pin that does not exist fails")
	}
	if err := s.RemovePin("orders", "inventory", "python-library"); err != nil {
		t.Fatal(err)
	}
	if pins, _ := s.LoadPins("orders"); len(pins) != 0 {
		t.Errorf("PN3 pins after unpin: %+v", pins)
	}
	if by := s.PinnedBy([]string{"orders"}, "inventory", "python-library", "1.0.0"); len(by) != 0 {
		t.Errorf("PinnedBy after unpin = %v", by)
	}
}

func TestIgnoreText(t *testing.T) {
	s := newVault(t)
	if wrote, err := s.WriteIgnore(); err != nil || !wrote {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(s.Root, IgnoreFile))
	want := "partitions/*/staging/\npartitions/*/read-only/releases/*/*/product/\npartitions/*/read-only/releases/*/tags/\n"
	if string(data) != want {
		t.Errorf("RL2 ignore file:\n%s", data)
	}
	if wrote, _ := s.WriteIgnore(); wrote {
		t.Error("RL2 the ignore file is written once")
	}
}
