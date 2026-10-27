package testmap

import (
	"strings"
	"testing"
)

type fakeDisk struct{ files, dirs []string }

func (d fakeDisk) IsFile(rel string) bool { return contains(d.files, rel) }
func (d fakeDisk) IsDir(rel string) bool  { return contains(d.dirs, rel) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestParse(t *testing.T) {
	m, err := Parse([]byte(`version: 1
suites:
  unit: {command: npm test}
tests:
  "test/wiki.test.ts": ["[[wiki]]"]
  "test/live/": []
gaps:
  wiki: [Large vaults.]
`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Suites["unit"].Command != "npm test" || len(m.Tests) != 2 || m.Tests["test/live/"] == nil || m.Gaps["wiki"][0] != "Large vaults." {
		t.Fatalf("parsed %+v", m)
	}
	if empty, err := Parse(nil); err != nil || empty.Version != version || empty.Tests == nil {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
	if _, err := Parse([]byte("version: 1\nfeatures: {}\n")); err == nil || !strings.Contains(err.Error(), "X1") {
		t.Fatalf("unknown field: %v", err)
	}
}

func TestValidate(t *testing.T) {
	features := map[string]bool{"wiki": true, "lint": true}
	cases := []struct {
		name, yaml, want string
	}{
		{"valid", `version: 1
suites: {unit: {command: go test}}
tests: {"a.go": ["[[wiki]]", "[[lint]]"], "b": []}
gaps: {wiki: [one]}`, ""},
		{"version", "version: 2", "X1"},
		{"suite name", "version: 1\nsuites: {Unit: {command: x}}", `X2: tests.yaml: suite name "Unit"`},
		{"suite command", "version: 1\nsuites: {unit: {command: ' '}}", `X2: tests.yaml: suite "unit" has no command`},
		{"empty key", "version: 1\ntests: {'': []}", "X3: tests.yaml: a key under tests is empty"},
		{"bare id", "version: 1\ntests: {a: [wiki]}", `is not a link`},
		{"text", "version: 1\ntests: {a: ['[[wiki|Wiki]]']}", "no |text or #heading"},
		{"heading", "version: 1\ntests: {a: ['[[wiki#x]]']}", "no |text or #heading"},
		{"unknown feature", "version: 1\ntests: {a: ['[[gone]]']}", `links [[gone]], which is not a feature of the wiki`},
		{"gap feature", "version: 1\ngaps: {gone: [x]}", `X4: tests.yaml: gaps: "gone"`},
		{"empty gap", "version: 1\ngaps: {wiki: ['']}", "X4: tests.yaml: gaps of \"wiki\": a gap is empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse([]byte(c.yaml))
			if err != nil {
				t.Fatal(err)
			}
			err = m.Validate(features)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("unexpected: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestLinks(t *testing.T) {
	m := Map{Tests: map[string][]string{"a": {"[[wiki]]", "[[wiki]]", "bad", "[[lint]]"}}}
	if got := strings.Join(m.Links("a"), ","); got != "wiki,lint" {
		t.Fatalf("links %q", got)
	}
}

func TestMatcher(t *testing.T) {
	disk := fakeDisk{
		files: []string{"test/unit/wiki.test.ts", "tests/test_stock.py"},
		dirs:  []string{"test/unit", "test/unit/obsidian", "test", "tests"},
	}
	m := Map{Tests: map[string][]string{
		"test/unit/wiki.test.ts":        {"[[wiki]]"},
		"test/unit/wiki.test.ts > lint": {"[[lint]]"},
		"test/unit/":                    {"[[core]]"},
		"test/unit/obsidian":            {"[[obsidian]]"},
		"tests.test_stock":              {"[[stock]]"},
		"tests.test_stock.StockTest > test_restock_adds_stock": {"[[restock]]"},
		"tests/test_stock.py": {"[[file-key]]"},
		"tests.unit":          {"[[dotted]]"},
		"gone/dir/":           {"[[gone]]"},
	}}
	mt := NewMatcher(m, disk)
	if mt.Kind("tests.unit") != IDKey || mt.Kind("tests/test_stock.py") != FileKey || mt.Kind("test/unit/") != DirKey || mt.Kind("gone/dir/") != IDKey {
		t.Fatal("kinds are wrong")
	}
	cases := []struct{ id, file, want string }{
		{"test/unit/wiki.test.ts > search > finds a tag", "test/unit/wiki.test.ts", "test/unit/wiki.test.ts"},
		{"test/unit/wiki.test.ts > lint > flags a link", "test/unit/wiki.test.ts", "test/unit/wiki.test.ts > lint"},
		{"test/unit/wiki.test.ts > linter", "test/unit/wiki.test.ts", "test/unit/wiki.test.ts"},
		{"test/unit/run.test.ts > runs", "test/unit/run.test.ts", "test/unit/"},
		{"test/unit/obsidian/a.test.ts > x", "test/unit/obsidian/a.test.ts", "test/unit/obsidian"},
		{"tests.test_stock.StockTest > test_reserve", "", "tests.test_stock"},
		{"tests.test_stock.StockTest > test_restock_adds_stock", "tests/test_stock.py", "tests.test_stock.StockTest > test_restock_adds_stock"},
		{"tests.test_stock.StockTest > test_reserve", "tests/test_stock.py", "tests.test_stock"},
		{"tests.test_stockpile.T > x", "", ""},
		{"tests.unit.T > x", "", "tests.unit"},
		{"other > x", "other.py", ""},
	}
	for _, c := range cases {
		got, ok := mt.Key(c.id, c.file)
		if c.want == "" {
			if ok {
				t.Errorf("%s: matched %q, want none", c.id, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s (%s): key %q, want %q", c.id, c.file, got, c.want)
		}
	}
}
