package sysmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rewriteTests = `version: 1
# The suites of the system.
suites:
  unit: {command: python3 -m pytest}
tests:
  # The stock tests.
  tests/test_stock.py: ["[[reserve-stock]]", "[[check-availability]]"]
  tests/test_release.py: ["[[release-stock]]"]
  tests/test_both.py: ["[[release-stock]]", "[[check-availability]]"]
  tests/test_live.py: []
gaps:
  reserve-stock: [Non-positive quantity.]
  release-stock: [Nothing.]
`

const rewriteLayout = `version: 1
parts: []
features:
  reserve-stock: {code: [stock.py]} # the reservation
  release-stock: {code: [stock.py]}
`

func TestRewriteIDsOnRename(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".hikari/private/tests.yaml", rewriteTests)
	writeFile(t, root, ".hikari/private/layout.yaml", rewriteLayout)
	notes := RewriteIDs(root, map[string]string{"reserve-stock": "hold-stock"}, nil)
	if len(notes) != 0 {
		t.Fatal(notes)
	}
	p, err := LoadPrivate(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Tests.Tests["tests/test_stock.py"], ","); got != "[[hold-stock]],[[check-availability]]" {
		t.Errorf("E7: links %s", got)
	}
	if p.Tests.Gaps["hold-stock"] == nil || p.Tests.Gaps["reserve-stock"] != nil {
		t.Errorf("E7: gaps %v", p.Tests.Gaps)
	}
	if p.Layout.Features["hold-stock"].Code == nil || p.Layout.Features["reserve-stock"].Code != nil {
		t.Errorf("E7: layout %v", p.Layout.Features)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".hikari/private/tests.yaml"))
	layout, _ := os.ReadFile(filepath.Join(root, ".hikari/private/layout.yaml"))
	for _, want := range []string{"# The suites of the system.", "# The stock tests."} {
		if !strings.Contains(string(data), want) {
			t.Errorf("E9: comment %q lost:\n%s", want, data)
		}
	}
	if !strings.Contains(string(layout), "# the reservation") {
		t.Errorf("E9: comment lost:\n%s", layout)
	}
	if strings.Index(string(data), "suites:") > strings.Index(string(data), "tests:") {
		t.Errorf("E9: order changed:\n%s", data)
	}
}

func TestRewriteIDsOnRemove(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".hikari/private/tests.yaml", rewriteTests)
	writeFile(t, root, ".hikari/private/layout.yaml", rewriteLayout)
	RewriteIDs(root, nil, map[string]string{"release-stock": ""})
	p, _ := LoadPrivate(root)
	if _, ok := p.Tests.Tests["tests/test_release.py"]; ok {
		t.Errorf("E8: a key whose list a remove empties is dropped: %v", p.Tests.Tests)
	}
	if got := strings.Join(p.Tests.Tests["tests/test_both.py"], ","); got != "[[check-availability]]" {
		t.Errorf("E8: link dropped: %s", got)
	}
	if l, ok := p.Tests.Tests["tests/test_live.py"]; !ok || len(l) != 0 {
		t.Errorf("E8: an empty list the owner wrote stays: %v", p.Tests.Tests)
	}
	if p.Tests.Gaps["release-stock"] != nil {
		t.Errorf("E8: gaps of a removed feature go: %v", p.Tests.Gaps)
	}
	if p.Layout.Features["release-stock"].Code == nil {
		t.Error("E8: a remove leaves layout.yaml to the owner")
	}

	writeFile(t, root, ".hikari/private/tests.yaml", rewriteTests)
	RewriteIDs(root, nil, map[string]string{"release-stock": "check-availability"})
	p, _ = LoadPrivate(root)
	if got := strings.Join(p.Tests.Tests["tests/test_both.py"], ","); got != "[[check-availability]]" {
		t.Errorf("E8: a redirect that repeats a link is dropped once: %s", got)
	}
	if got := strings.Join(p.Tests.Tests["tests/test_release.py"], ","); got != "[[check-availability]]" {
		t.Errorf("E8: redirect: %s", got)
	}
}

func TestRewriteIDsInvalidFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".hikari/private/tests.yaml", "tests: [unclosed\n")
	notes := RewriteIDs(root, map[string]string{"a": "b"}, nil)
	if len(notes) != 1 || !strings.Contains(notes[0], "tests.yaml could not be rewritten") {
		t.Errorf("E9: notes %v", notes)
	}
	if got := RewriteIDs(root, nil, nil); got != nil {
		t.Errorf("nothing to rewrite: %v", got)
	}
}
