package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	ordered := []string{"0.9.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.10", "1.10.0", "2.0.0"}
	for i := range ordered {
		for j := range ordered {
			got := CompareVersions(ordered[i], ordered[j])
			if (i < j && got >= 0) || (i > j && got <= 0) || (i == j && got != 0) {
				t.Errorf("CompareVersions(%s, %s) = %d", ordered[i], ordered[j], got)
			}
		}
	}
	for _, bad := range []string{"1.0", "v1.0.0", "01.0.0", "1.0.0+build", "1.0.0-"} {
		if ValidVersion(bad) {
			t.Errorf("%q must not be valid", bad)
		}
	}
}

func newStore(t *testing.T) Store {
	t.Helper()
	s := Store{Root: filepath.Join(t.TempDir(), "store")}
	if err := s.Prepare([]string{"inventory"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func stage(t *testing.T, s Store, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(s.Staging("inventory"), name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const manifest = `product: {kind: files, path: product/}
runtime: {requires: [python >= 3.11], install: add product/ to PYTHONPATH}
docs: {index: docs/README.md}
interfaces: [Stock.reserve]
`

var goodRelease = map[string]string{
	"release.yaml":                  manifest,
	"product/inventory/stock.py":    "class Stock: pass\n",
	"product/inventory/__init__.py": "",
	"docs/README.md":                "# inventory\n",
}

func input(version string) PublishInput {
	return PublishInput{
		System: "inventory", Version: version, Notes: "First release.", SourceCommit: "abc123",
		Provides: map[string]bool{"Stock.reserve": true}, Map: []byte("version: 2\n"),
		Now: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestPublish(t *testing.T) {
	s := newStore(t)
	stage(t, s, goodRelease)
	m, err := s.Publish(input("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.0.0" || m.System != "inventory" || m.SourceCommit != "abc123" || m.Notes != "First release." || m.Map != "map.yaml" {
		t.Errorf("R4 manifest: %+v", m)
	}
	dir := s.ReleaseDir("inventory", "1.0.0")
	for _, p := range []string{"product/inventory/stock.py", "docs/README.md", "release.yaml", "map.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("R3: release lacks %s", p)
		}
	}
	if entries, _ := os.ReadDir(s.Staging("inventory")); len(entries) != 0 {
		t.Errorf("R3: staging must be empty, has %v", entries)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if !strings.Contains(string(data), "source_commit: abc123") || !strings.Contains(string(data), "released: 2026-10-22T10:00:00Z") {
		t.Errorf("R4 written manifest:\n%s", data)
	}
	if link, _ := os.Readlink(filepath.Join(s.ReleasesDir("inventory"), "latest")); link != "1.0.0" {
		t.Errorf("R5 latest -> %q", link)
	}

	stage(t, s, goodRelease)
	if _, err := s.Publish(input("1.0.0")); err == nil || !strings.Contains(err.Error(), "not greater") {
		t.Errorf("R7: republishing 1.0.0: %v", err)
	}
	if _, err := s.Publish(input("1.1.0-rc.1")); err != nil {
		t.Fatal(err)
	}
	if link, _ := os.Readlink(filepath.Join(s.ReleasesDir("inventory"), "latest")); link != "1.1.0-rc.1" {
		t.Errorf("R5 latest -> %q", link)
	}
	list, _ := s.Releases("inventory")
	if len(list) != 2 || list[0].Version != "1.1.0-rc.1" || list[1].Version != "1.0.0" {
		t.Errorf("Releases newest first: %+v", list)
	}
}

func TestPublishChecks(t *testing.T) {
	cases := map[string]struct {
		version string
		files   map[string]string
		want    string
	}{
		"bad version":        {"1.0", goodRelease, "is not semver"},
		"no manifest":        {"1.0.0", map[string]string{"docs/README.md": "x"}, "has no release.yaml"},
		"unknown field":      {"1.0.0", with(goodRelease, "release.yaml", manifest+"extra: 1\n"), "field extra not found"},
		"missing product":    {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "path: product/", "path: build/", 1)), `product.path "build/" does not exist`},
		"bad kind":           {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "kind: files", "kind: zip", 1)), "product.kind must be"},
		"remote without ref": {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "{kind: files, path: product/}", "{kind: remote, location: {type: docker}}", 1)), "location.ref is empty"},
		"remote bad type":    {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "{kind: files, path: product/}", "{kind: remote, location: {type: ftp, ref: x}}", 1)), "location.type must be one of"},
		"empty docs":         {"1.0.0", with(goodRelease, "docs/README.md", ""), "is empty"},
		"missing docs":       {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "docs/README.md", "docs/INDEX.md", 1)), "is not a file"},
		"unknown interface":  {"1.0.0", with(goodRelease, "release.yaml", strings.Replace(manifest, "Stock.reserve", "Stock.steal", 1)), `"Stock.steal" is not one that the map provides`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			stage(t, s, c.files)
			_, err := s.Publish(input(c.version))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
			if versions, _ := s.Versions("inventory"); len(versions) != 0 {
				t.Errorf("a rejected release was published: %v", versions)
			}
		})
	}
}

func TestPublishRemote(t *testing.T) {
	s := newStore(t)
	stage(t, s, map[string]string{
		"release.yaml":   "product: {kind: remote, location: {type: docker, ref: docker.io/acme/inventory:1.0.0}}\ndocs: {index: docs/README.md}\n",
		"docs/README.md": "# inventory\n",
	})
	m, err := s.Publish(input("1.0.0"))
	if err != nil || m.Product.Location.Ref != "docker.io/acme/inventory:1.0.0" {
		t.Errorf("remote release: %+v, %v", m, err)
	}
}

func TestPrepare(t *testing.T) {
	s := newStore(t)
	for _, p := range []string{"README.md", "shared/vocabulary", "shared/policies", "shared/skills", "shared/agents", "systems/inventory/staging", "systems/inventory/releases"} {
		if _, err := os.Stat(filepath.Join(s.Root, p)); err != nil {
			t.Errorf("L1: missing %s", p)
		}
	}
	if versions, err := s.Versions("ghost"); err != nil || versions != nil {
		t.Errorf("Versions of an unknown system: %v, %v", versions, err)
	}
	if !slices.Equal([]string{s.VocabularyFile("eng")}, []string{filepath.Join(s.Root, "shared", "vocabulary", "eng.yaml")}) {
		t.Error("vocabulary path")
	}
}

func with(files map[string]string, name, content string) map[string]string {
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	out[name] = content
	return out
}
