package vault

import (
	"crypto/sha256"
	"encoding/hex"
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

func newVault(t *testing.T) Vault {
	t.Helper()
	s := Vault{Root: filepath.Join(t.TempDir(), "vault")}
	if err := s.Prepare([]string{"inventory"}, false); err != nil {
		t.Fatal(err)
	}
	return s
}

// writeFiles writes files under dir, creating directories as needed.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const pythonLibrary = `version: 1
description: A Python library that other systems install from the release.
layout:
  require: [README.md, "src/**/*.py"]
  allow: [README.md, "src/**/*.py", "docs/**/*.md"]
  forbid: ["**/__pycache__/**", "**/*.pyc"]
`

func defineDeliverable(t *testing.T, s Vault, name, yaml string) {
	t.Helper()
	writeFiles(t, s.DeliverableDir(name), map[string]string{DeliverableFile: yaml, SkillFile: "# Make the library\n"})
}

var goodProduct = map[string]string{
	"README.md":                 "# inventory\n",
	"src/inventory/stock.py":    "class Stock: pass\n",
	"src/inventory/__init__.py": "",
	"docs/limits.md":            "none\n",
}

func stage(t *testing.T, s Vault, deliverable string, files map[string]string) {
	t.Helper()
	writeFiles(t, s.StagingOf("inventory", deliverable), files)
}

func input(version string) PublishInput {
	return PublishInput{
		Partition: "inventory", Deliverable: "python-library", Version: version, Notes: "First release.",
		SourceCommits: []string{"shop@abc123"}, Map: []byte("version: 3\n"), Features: map[string][]byte{"reserve-stock.md": []byte("---\nid: reserve-stock\n---\n")}, Hikari: "0.0.1",
		Now: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestLoadDeliverable(t *testing.T) {
	s := newVault(t)
	defineDeliverable(t, s, "python-library", pythonLibrary+"targets:\n  - {type: github-release, repo: acme/inventory}\n")
	d, err := s.LoadDeliverable("python-library")
	if err != nil {
		t.Fatal(err)
	}
	if d.Description == "" || len(d.Layout.Require) != 2 || len(d.Targets) != 1 || d.Targets[0].Repo != "acme/inventory" {
		t.Errorf("D1: %+v", d)
	}
	names, _ := s.DeliverableNames()
	if !slices.Equal(names, []string{"python-library"}) {
		t.Errorf("names = %v", names)
	}

	for name, c := range map[string]struct{ yaml, skill, want string }{
		"D1 unknown field": {pythonLibrary + "extra: 1\n", "x", "field extra not found"},
		"D1 version":       {strings.Replace(pythonLibrary, "version: 1", "version: 2", 1), "x", "must have version: 1"},
		"D1 empty":         {"", "x", "must have version: 1"},
		"D2 description":   {strings.Replace(pythonLibrary, "description: A Python", "description: ''\n#", 1), "x", "description is required"},
		"D3 bad glob":      {strings.Replace(pythonLibrary, `"src/**/*.py"`, `"src/[py"`, 1), "x", `layout.require pattern "src/[py" is not a valid glob`},
		"D4 bad type":      {pythonLibrary + "targets: [{type: ftp}]\n", "x", "type must be one of"},
		"D4 twice":         {pythonLibrary + "targets: [{type: dockerhub}, {type: dockerhub}]\n", "x", "two entries of type dockerhub"},
		"D4 wrong pin":     {pythonLibrary + "targets: [{type: dockerhub, repo: a/b}]\n", "x", "cannot pin repo"},
		"D4 bad kind":      {pythonLibrary + "targets: [{type: huggingface, kind: weights}]\n", "x", "kind must be one of"},
		"D5 no skill":      {pythonLibrary, "", "SKILL.md is empty"},
		"D1 image":         {pythonLibrary + "image: {dockerfile: Dockerfile}\n", "x", "field image not found"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newVault(t)
			writeFiles(t, s.DeliverableDir("lib"), map[string]string{DeliverableFile: c.yaml, SkillFile: c.skill})
			_, err := s.LoadDeliverable("lib")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
	if _, err := s.LoadDeliverable("ghost"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing deliverable: %v", err)
	}
	if _, err := s.LoadDeliverable("Bad Name"); err == nil || !strings.Contains(err.Error(), "not a valid name") {
		t.Errorf("bad name: %v", err)
	}
	os.Remove(filepath.Join(s.DeliverableDir("python-library"), DeliverableFile))
	if _, err := s.LoadDeliverable("python-library"); err == nil || !strings.Contains(err.Error(), "has no deliverable.yaml") {
		t.Errorf("missing yaml: %v", err)
	}
}

func TestCodeMapCheck(t *testing.T) {
	paths := []string{"README.md", "src/a.py", "src/pkg/b.py", "src/pkg/__pycache__/b.pyc", "notes/README.md"}
	cases := map[string]struct {
		layout Layout
		want   []string
	}{
		"G1 require met":     {Layout{Require: []string{"README.md", "**/*.py"}}, nil},
		"G1 require missed":  {Layout{Require: []string{"LICENSE"}}, []string{`no file matches "LICENSE"`}},
		"G2 allow":           {Layout{Allow: []string{"README.md", "src/**"}}, []string{"match no allowed pattern: notes/README.md"}},
		"G3 forbid":          {Layout{Forbid: []string{"**/__pycache__/**"}}, []string{`"**/__pycache__/**" matches src/pkg/__pycache__/b.pyc`}},
		"G4 top level only":  {Layout{Require: []string{"a.py"}}, []string{`no file matches "a.py"`}},
		"G4 star in segment": {Layout{Forbid: []string{"src/*.pyc"}}, nil},
		"exact map":          {Layout{Require: []string{"README.md"}, Allow: []string{"README.md"}}, []string{"match no allowed pattern: src/a.py, src/pkg/b.py, src/pkg/__pycache__/b.pyc, notes/README.md"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			errs := c.layout.Check(paths)
			if len(errs) != len(c.want) {
				t.Fatalf("errors = %v, want %v", errs, c.want)
			}
			for i, w := range c.want {
				if !strings.Contains(errs[i].Error(), w) {
					t.Errorf("error %d = %v, want %q", i, errs[i], w)
				}
			}
		})
	}
}

const (
	hex40s = "0123456789abcdef0123456789abcdef01234567"
	hex64s = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestCheckCertificates(t *testing.T) {
	targets := []Target{{Type: TargetGitHubRelease, Repo: "acme/inventory"}, {Type: TargetDockerHub}, {Type: TargetHuggingFace, Kind: "model"}}
	good := []Certificate{
		{Type: TargetGitHubRelease, Repo: "acme/inventory", Tag: "v1.0.0", URL: "https://github.com/acme/inventory/releases/tag/v1.0.0", Assets: []Asset{{Name: "x.tar.gz", SHA256: hex64s}}},
		{Type: TargetDockerHub, Repository: "acme/inventory", Tag: "1.0.0", Digest: "sha256:" + hex64s},
		{Type: TargetHuggingFace, Repo: "acme/inventory-model", Kind: "model", Revision: hex40s, URL: "https://huggingface.co/acme/inventory-model"},
	}
	if errs := CheckCertificates(targets, good); len(errs) != 0 {
		t.Errorf("good certificates: %v", errs)
	}
	replace := func(i int, f func(c *Certificate)) []Certificate {
		out := slices.Clone(good)
		f(&out[i])
		return out
	}
	cases := map[string]struct {
		certs []Certificate
		want  string
	}{
		"C2 missing":      {good[:2], "promises a huggingface target, and no certificate"},
		"C2 no target":    {append(slices.Clone(good), Certificate{Type: TargetDockerHub}), "two entries of type dockerhub"},
		"C1 bad type":     {append(slices.Clone(good), Certificate{Type: "ftp"}), "type must be one of"},
		"C3 pin":          {replace(0, func(c *Certificate) { c.Repo = "acme/other" }), `repo is "acme/other", and the deliverable pins it to "acme/inventory"`},
		"C3 kind pin":     {replace(2, func(c *Certificate) { c.Kind = "dataset" }), `kind is "dataset", and the deliverable pins it to "model"`},
		"C4 required":     {replace(1, func(c *Certificate) { c.Digest = "" }), "digest is required"},
		"C4 wrong field":  {replace(1, func(c *Certificate) { c.URL = "https://x" }), "url is not a field of this type"},
		"C4 assets":       {replace(1, func(c *Certificate) { c.Assets = []Asset{{Name: "a", SHA256: hex64s}} }), "assets is not a field of this type"},
		"C4 url":          {replace(0, func(c *Certificate) { c.URL = "http://github.com/x" }), "not an absolute https URL"},
		"C4 digest":       {replace(1, func(c *Certificate) { c.Digest = "sha256:abc" }), "is not sha256:<64 hex>"},
		"C4 revision":     {replace(2, func(c *Certificate) { c.Revision = "abc" }), "is not 40 hex characters"},
		"C4 owner/name":   {replace(1, func(c *Certificate) { c.Repository = "inventory" }), `repository "inventory" is not owner/name`},
		"C4 asset digest": {replace(0, func(c *Certificate) { c.Assets = []Asset{{Name: "a", SHA256: "xyz"}} }), "assets[0] needs a name and a 64-hex sha256"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			errs := CheckCertificates(targets, c.certs)
			if !slices.ContainsFunc(errs, func(e error) bool { return strings.Contains(e.Error(), c.want) }) {
				t.Errorf("errors = %v, want %q", errs, c.want)
			}
		})
	}
	if errs := CheckCertificates(nil, []Certificate{good[1]}); len(errs) != 1 || !strings.Contains(errs[0].Error(), "has no dockerhub target") {
		t.Errorf("C2 certificate without target: %v", errs)
	}
}

func TestPublish(t *testing.T) {
	s := newVault(t)
	defineDeliverable(t, s, "python-library", pythonLibrary)
	stage(t, s, "python-library", goodProduct)
	os.MkdirAll(filepath.Join(s.StagingOf("inventory", "python-library"), "src", "empty"), 0o755)

	check, err := s.Check("inventory", "python-library")
	if err != nil || check.Files != 4 {
		t.Fatalf("R15 check = %+v, %v", check, err)
	}

	m, err := s.Publish(input("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Deliverable != "python-library" || m.Version != "1.0.0" || m.Partition != "inventory" || !slices.Equal(m.SourceCommits, []string{"shop@abc123"}) ||
		m.Notes != "First release." || m.Map != "features/map.yaml" || m.Hikari != "0.0.1" ||
		m.Product.Files != 4 || m.Product.Path != "product/" || !strings.HasPrefix(m.Product.Digest, "sha256:") || m.Certificates == nil {
		t.Errorf("R14 manifest: %+v", m)
	}
	dir := s.ReleaseDir("inventory", "python-library", "1.0.0")
	for _, p := range []string{"product/README.md", "product/src/inventory/stock.py", "product/src/empty", "release.yaml", "SHA256SUMS", "features/map.yaml", "features/reserve-stock.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("R17: release lacks %s", p)
		}
	}
	if entries, _ := os.ReadDir(s.StagingOf("inventory", "python-library")); len(entries) != 0 {
		t.Errorf("R17: staging must be empty, has %v", entries)
	}

	sums, _ := os.ReadFile(filepath.Join(dir, SumsFile))
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 4 || !strings.HasSuffix(lines[0], "  product/README.md") {
		t.Errorf("R12 SHA256SUMS:\n%s", sums)
	}
	for _, line := range lines {
		sum, path, _ := strings.Cut(line, "  ")
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != sum {
			t.Errorf("R12: digest of %s is wrong", path)
		}
	}
	if d := sha256.Sum256(sums); m.Product.Digest != "sha256:"+hex.EncodeToString(d[:]) {
		t.Errorf("R13 product digest %s", m.Product.Digest)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if !strings.Contains(string(data), "released: 2026-10-22T10:00:00Z") || !strings.Contains(string(data), "certificates: []") {
		t.Errorf("written manifest:\n%s", data)
	}
	if old, err := ParseManifest(append(data, "image: {tag: x, id: y, platform: linux/amd64}\n"...)); err != nil || old.Version != "1.0.0" {
		t.Errorf("the image key of an older release is ignored: %+v, %v", old, err)
	}
	if old, err := ParseManifest(append(data, "hubs: [shop]\n"...)); err != nil || old.Version != "1.0.0" {
		t.Errorf("the hubs key of an older release is ignored: %+v, %v", old, err)
	}
	if strings.Contains(string(data), "hubs:") {
		t.Errorf("a new manifest has no hubs:\n%s", data)
	}
	if !strings.Contains(string(data), "partition: inventory") {
		t.Errorf("a new manifest names its partition:\n%s", data)
	}
	if old, err := ParseManifest([]byte(strings.Replace(string(data), "partition: inventory", "system: inventory", 1))); err != nil || old.Partition != "inventory" {
		t.Errorf("the system key of an older release names the partition: %+v, %v", old, err)
	}
	if _, err := ParseManifest(append(data, "extra: 1\n"...)); err == nil {
		t.Error("an unknown key of a manifest is an error")
	}
	if link, _ := os.Readlink(filepath.Join(s.DeliverableReleasesDir("inventory", "python-library"), "latest")); link != "1.0.0" {
		t.Errorf("R17 latest -> %q", link)
	}

	stage(t, s, "python-library", goodProduct)
	if _, err := s.Publish(input("1.0.0")); err == nil || !strings.Contains(err.Error(), "not greater") {
		t.Errorf("R19: republishing 1.0.0: %v", err)
	}
	if _, err := s.Publish(input("1.1.0-rc.1")); err != nil {
		t.Fatal(err)
	}
	if link, _ := os.Readlink(filepath.Join(s.DeliverableReleasesDir("inventory", "python-library"), "latest")); link != "1.1.0-rc.1" {
		t.Errorf("latest -> %q", link)
	}

	defineDeliverable(t, s, "docs", "version: 1\ndescription: The docs.\n")
	stage(t, s, "docs", map[string]string{"index.md": "hi\n"})
	in := input("0.1.0")
	in.Deliverable = "docs"
	if _, err := s.Publish(in); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Releases("inventory")
	if len(list) != 3 || list[0].Deliverable != "docs" || list[1].Version != "1.1.0-rc.1" || list[2].Version != "1.0.0" {
		t.Errorf("Releases by deliverable, newest first: %+v", list)
	}
}

func TestPublishChecks(t *testing.T) {
	type tc struct {
		version string
		notes   string
		files   map[string]string
		certs   []Certificate
		yaml    string
		want    string
	}
	base := tc{version: "1.0.0", notes: "ok", files: goodProduct, yaml: pythonLibrary}
	mod := func(f func(c *tc)) tc { c := base; f(&c); return c }
	cases := map[string]tc{
		"R16 version": mod(func(c *tc) { c.version = "1.0"; c.want = "is not semver" }),
		"R16 notes":   mod(func(c *tc) { c.notes = " "; c.want = "notes are required" }),
		"no product":  mod(func(c *tc) { c.files = nil; c.want = "there is no staged product" }),
		"G1 require": mod(func(c *tc) {
			c.files = with(goodProduct, "README.md", "")
			delete(c.files, "README.md")
			c.want = `no file matches "README.md"`
		}),
		"G2 allow": mod(func(c *tc) {
			c.files = with(goodProduct, "tests/test_stock.py", "x")
			c.want = "match no allowed pattern: tests/test_stock.py"
		}),
		"G3 forbid": mod(func(c *tc) {
			c.files = with(goodProduct, "src/inventory/__pycache__/stock.pyc", "x")
			c.want = `"**/__pycache__/**" matches`
		}),
		"G6 empty": mod(func(c *tc) {
			c.yaml = "version: 1\ndescription: Anything.\n"
			c.files = map[string]string{}
			c.want = "the product has no files"
		}),
		"C2 certificate": mod(func(c *tc) {
			c.yaml = pythonLibrary + "targets: [{type: dockerhub}]\n"
			c.want = "promises a dockerhub target"
		}),
		"invalid def": mod(func(c *tc) { c.yaml = "version: 3\n"; c.want = "must have version: 1" }),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newVault(t)
			defineDeliverable(t, s, "python-library", c.yaml)
			if c.files != nil {
				os.MkdirAll(s.StagingOf("inventory", "python-library"), 0o755)
				stage(t, s, "python-library", c.files)
			}
			in := input(c.version)
			in.Notes, in.Certificates = c.notes, c.certs
			_, err := s.Publish(in)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
			if versions, _ := s.Versions("inventory", "python-library"); len(versions) != 0 {
				t.Errorf("a rejected release was published: %v", versions)
			}
		})
	}

	t.Run("G5 link", func(t *testing.T) {
		s := newVault(t)
		defineDeliverable(t, s, "python-library", pythonLibrary)
		stage(t, s, "python-library", goodProduct)
		os.Symlink("README.md", filepath.Join(s.StagingOf("inventory", "python-library"), "LINK.md"))
		if _, err := s.Publish(input("1.0.0")); err == nil || !strings.Contains(err.Error(), "must not contain links: LINK.md") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("G6 targets only", func(t *testing.T) {
		s := newVault(t)
		defineDeliverable(t, s, "python-library", "version: 1\ndescription: An image.\ntargets: [{type: dockerhub, repository: acme/inventory}]\n")
		os.MkdirAll(s.StagingOf("inventory", "python-library"), 0o755)
		in := input("1.0.0")
		in.Certificates = []Certificate{{Type: TargetDockerHub, Repository: "acme/inventory", Tag: "1.0.0", Digest: "sha256:" + hex64s}}
		m, err := s.Publish(in)
		if err != nil || m.Product.Files != 0 || len(m.Certificates) != 1 || m.Certificates[0].Digest != in.Certificates[0].Digest {
			t.Errorf("C5: %+v, %v", m, err)
		}
		list, _ := s.Releases("inventory")
		if len(list) != 1 || list[0].Certificates[0].Repository != "acme/inventory" {
			t.Errorf("read back: %+v", list)
		}
	})
}

func TestPrepare(t *testing.T) {
	s := newVault(t)
	for _, p := range []string{"README.md", "public/vocabulary", "public/policies", "public/deliverables", "public/wiki", "skills",
		"partitions/inventory/read-only/features", "partitions/inventory/read-only/releases", "partitions/inventory/read-only/granted",
		"partitions/inventory/can-edit", "partitions/inventory/staging"} {
		if _, err := os.Stat(filepath.Join(s.Root, p)); err != nil {
			t.Errorf("V1: missing %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Root, "orchestration")); err == nil {
		t.Error("V2: orchestration/ exists only when the topology names an orchestration agent")
	}
	if err := s.Prepare([]string{"inventory"}, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"orchestration/read-only/features", "orchestration/read-only/granted", "orchestration/can-edit"} {
		if _, err := os.Stat(filepath.Join(s.Root, p)); err != nil {
			t.Errorf("V2: missing %s", p)
		}
	}
	if versions, err := s.Versions("ghost", "x"); err != nil || versions != nil {
		t.Errorf("Versions of an unknown partition: %v, %v", versions, err)
	}
}

func TestOldLayout(t *testing.T) {
	s := newVault(t)
	if got := s.OldLayout(); len(got) != 0 {
		t.Errorf("a new vault: %v", got)
	}
	os.MkdirAll(filepath.Join(s.Root, "shared", "vocabulary"), 0o755)
	os.MkdirAll(filepath.Join(s.Root, "systems", "inventory"), 0o755)
	got := s.OldLayout()
	if len(got) != 2 || !strings.Contains(got[0], "move shared/vocabulary") || !strings.Contains(got[1], "partitions/<id>/read-only/features/") || !strings.Contains(got[1], Migrating("the-vault-layout")) {
		t.Errorf("V11: %q", got)
	}
	os.RemoveAll(filepath.Join(s.Root, "shared"))
	os.RemoveAll(filepath.Join(s.Root, "systems"))
	os.MkdirAll(filepath.Join(s.Root, "hubs", "main", "wiki"), 0o755)
	os.WriteFile(filepath.Join(s.Public(Vocabulary), "main.yaml"), []byte("version: 1\nhub: main\n"), 0o644)
	got = s.OldLayout()
	if len(got) != 2 || !strings.Contains(got[0], "holds hubs/") || !strings.Contains(got[1], "holds main.yaml and no terms.yaml") {
		t.Errorf("M5 hubs/ and a hub's vocabulary: %q", got)
	}
	os.WriteFile(s.VocabularyFile(), []byte("version: 1\n"), 0o644)
	if got = s.OldLayout(); len(got) != 1 {
		t.Errorf("terms.yaml beside another file: %q", got)
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
