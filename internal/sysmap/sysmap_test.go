package sysmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)

// newScope writes files into a temporary scope and returns its path.
func newScope(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeFile(t, root, name, content)
	}
	return root
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

var orderFiles = map[string]string{
	"orders/create.py":     "def create(): pass\n",
	"orders/cancel.py":     "def cancel(): pass\n",
	"tests/test_create.py": "def test(): pass\n",
}

const layoutDoc = `version: 1
parts:
  - {path: orders/, description: Order code.}
features:
  create-order: {code: [orders/create.py], notes: Entry point create().}
  cancel-order: {code: [orders/cancel.py]}
`

const testDoc = `version: 1
suites:
  unit: {command: python3 -m unittest, description: Unit tests.}
features:
  create-order:
    covers: Orders reserve each item.
    tests:
      - {suite: unit, files: [tests/test_create.py], command: python3 -m unittest tests.test_create}
    gaps: [Not tested against the real inventory.]
`

func ptr[T any](v T) *T { return &v }

func describe(m *Map) {
	m.ApplySystem(SystemPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]Interface{{Name: "POST /orders", Kind: "http"}}),
		Consumes: ptr([]Interface{{Name: "inventory.reserve", Kind: "function"}}),
	})
	m.Upsert(FeatureInput{
		ID: "create-order", Name: "Create an order",
		Interfaces: []string{"POST /orders", "inventory.reserve"},
		Related:    []string{"cancel-order"},
	})
	m.Upsert(FeatureInput{ID: "cancel-order", Name: "Cancel an order"})
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		change func(*Map)
		rule   string
	}{
		"S1 bad id":            {func(m *Map) { m.Features[0].ID = "Create Order" }, "S1"},
		"S1 duplicate id":      {func(m *Map) { m.Features[1].ID = "create-order" }, "S1"},
		"S1 no name":           {func(m *Map) { m.Features[0].Name = "" }, "S1"},
		"S2 unknown interface": {func(m *Map) { m.Features[0].Interfaces = []string{"GET /nope"} }, "S2"},
		"S3 unknown related":   {func(m *Map) { m.Features[0].Related = []string{"ghost"} }, "S3"},
		"S3 self related":      {func(m *Map) { m.Features[0].Related = []string{"create-order"} }, "S3"},
		"S7 bad kind":          {func(m *Map) { m.System.Interfaces.Provides[0].Kind = "HTTP API" }, "S7"},
		"old version":          {func(m *Map) { m.Version = 1 }, "version"},
	}
	base := New("orders")
	describe(base)
	if err := Validate(base); err != nil {
		t.Fatalf("valid map: %v", err)
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := base.Clone()
			c.change(m)
			if err := Validate(m); err == nil || !strings.Contains(err.Error(), c.rule+":") {
				t.Errorf("error = %v, want rule %s", err, c.rule)
			}
		})
	}
}

func TestPrivateRules(t *testing.T) {
	root := newScope(t, orderFiles)
	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(root, rel)); return err == nil }
	m := New("orders")
	describe(m)

	writeFile(t, root, ".hikari/private/layout.yaml", layoutDoc)
	writeFile(t, root, ".hikari/private/test.yaml", testDoc)
	p, err := LoadPrivate(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(m.IDs(), exists); err != nil {
		t.Fatalf("valid documents: %v", err)
	}
	if !p.Tested("create-order") || p.Tested("cancel-order") {
		t.Error("only create-order has tests")
	}
	if got := strings.Join(p.Paths("create-order"), ","); got != "orders/create.py,tests/test_create.py" {
		t.Errorf("Paths = %s", got)
	}

	cases := map[string]struct {
		layout, tests, rule string
	}{
		"P1 unknown field":     {layoutDoc + "extra: 1\n", testDoc, "P1"},
		"P1 wrong version":     {strings.Replace(layoutDoc, "version: 1", "version: 3", 1), testDoc, "P1"},
		"P2 unknown feature":   {layoutDoc + "  ghost: {code: [orders/create.py]}\n", testDoc, "P2"},
		"P3 missing code":      {strings.Replace(layoutDoc, "orders/cancel.py", "orders/gone.py", 1), testDoc, "P3"},
		"P3 escaping part":     {strings.Replace(layoutDoc, "path: orders/", "path: ../secret", 1), testDoc, "P3"},
		"P3 missing test file": {layoutDoc, strings.Replace(testDoc, "tests/test_create.py", "tests/gone.py", 1), "P3"},
		"P4 undefined suite":   {layoutDoc, strings.Replace(testDoc, "suite: unit", "suite: e2e", 1), "P4"},
		"P4 suite without cmd": {layoutDoc, strings.Replace(testDoc, "command: python3 -m unittest, ", "", 1), "P4"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			writeFile(t, root, ".hikari/private/layout.yaml", c.layout)
			writeFile(t, root, ".hikari/private/test.yaml", c.tests)
			p, err := LoadPrivate(root)
			if err == nil {
				err = p.Validate(m.IDs(), exists)
			}
			if err == nil || !strings.Contains(err.Error(), c.rule+":") {
				t.Errorf("error = %v, want rule %s", err, c.rule)
			}
		})
	}
}

func TestEmptyPrivateDocumentsAreValid(t *testing.T) {
	root := newScope(t, nil)
	writeFile(t, root, ".hikari/private/layout.yaml", "")
	if _, err := LoadPrivate(root); err != nil {
		t.Errorf("an empty file: %v", err)
	}
}

func TestUpsertKeepsResultsAndRemoveDropsReferences(t *testing.T) {
	m := New("orders")
	describe(m)
	m.Features[0].Tests.Passed = 4
	m.Upsert(FeatureInput{ID: "create-order", Name: "Renamed"})
	if f := m.Features[0]; f.Name != "Renamed" || f.Tests.Passed != 4 {
		t.Errorf("after upsert: %+v", f)
	}
	m.Features[0].Related = []string{"cancel-order"}
	if err := m.Remove("cancel-order"); err != nil {
		t.Fatal(err)
	}
	if len(m.Features) != 1 || len(m.Features[0].Related) != 0 {
		t.Errorf("after remove: %+v", m.Features)
	}
	if err := m.Remove("ghost"); err == nil {
		t.Error("removing an unknown feature must fail")
	}
}

func TestHealth(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, err := Open(root, "orders", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(m *Map) error { describe(m); return nil }); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".hikari/private/layout.yaml", layoutDoc)
	writeFile(t, root, ".hikari/private/test.yaml", testDoc)
	health := func() map[string]string {
		priv, _ := s.Private()
		v := NewView(s.Map(), priv, nil, func(id string) string { h, _ := s.FeatureHash(id); return h })
		out := map[string]string{}
		for _, f := range v.Features {
			out[f.ID] = f.Health
		}
		return out
	}
	if h := health(); h["create-order"] != Untested || h["cancel-order"] != Untested {
		t.Errorf("S8 before any run: %v", h)
	}
	record := func(passed, failed int) {
		t.Helper()
		err := s.Update(func(m *Map) error {
			return m.RecordTests([]TestResult{{Feature: "create-order", Passed: passed, Failed: failed}}, now, s.FeatureHash)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	record(3, 0)
	if h := health(); h["create-order"] != Passing || h["cancel-order"] != Untested {
		t.Errorf("S11 after a passing run, S8 for a feature without tests: %v", h)
	}
	if got := s.Map().Features[0].Health; got != Passing {
		t.Errorf("the file stores health %q", got)
	}
	record(2, 1)
	if h := health(); h["create-order"] != Failing {
		t.Errorf("S10 after a failing run: %v", h)
	}
	writeFile(t, root, "orders/create.py", "def create(): return 1\n")
	if h := health(); h["create-order"] != Stale {
		t.Errorf("S9 after the code changed: %v (stale must win over failing)", h)
	}
	writeFile(t, root, ".hikari/private/test.yaml", "version: 1\nsuites: {}\nfeatures: {}\n")
	if h := health(); h["create-order"] != Untested {
		t.Errorf("S8 after the tests left the plan: %v", h)
	}
}

func TestState(t *testing.T) {
	m := New("orders")
	if got := State(m, nil); got != Unbuilt {
		t.Errorf("S13: %q", got)
	}
	m.MarkCurrent("abc123", now)
	if got := State(m, []string{"orders/create.py"}); got != Stale {
		t.Errorf("S14: %q", got)
	}
	if got := State(m, nil); got != Current {
		t.Errorf("S15: %q", got)
	}
}

func TestOpenWritesSkeletons(t *testing.T) {
	root := newScope(t, orderFiles)
	s, created, err := Open(root, "orders", "python3 -m unittest")
	if err != nil || !created {
		t.Fatalf("Open = %v, created %v", err, created)
	}
	data, _ := os.ReadFile(s.Path())
	if strings.HasPrefix(string(data), "#") || !strings.HasPrefix(string(data), "version: 2\nsystem:\n  name: orders\n") {
		t.Errorf("the map file must start with its version, have no comment, and indent by two spaces:\n%s", data)
	}
	p, err := s.Private()
	if err != nil {
		t.Fatal(err)
	}
	if p.Tests.Suites["all"].Command != "python3 -m unittest" || len(p.Layout.Parts) != 0 {
		t.Errorf("skeletons: %+v", p)
	}
	if errs := s.PrivateErrors(s.Map()); len(errs) != 0 {
		t.Errorf("new skeletons must be valid: %v", errs)
	}

	// Open keeps documents that exist.
	writeFile(t, root, ".hikari/private/layout.yaml", "version: 1\nparts: [{path: orders/, description: kept}]\nfeatures: {}\n")
	if _, _, err := Open(root, "orders", "other"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Private(); p.Layout.Parts[0].Description != "kept" || p.Tests.Suites["all"].Command != "python3 -m unittest" {
		t.Errorf("Open overwrote existing documents: %+v", p)
	}
}

func TestStoreRejectsInvalidChanges(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, _ := Open(root, "orders", "")
	before, _ := os.ReadFile(s.Path())
	err := s.Update(func(m *Map) error {
		m.Upsert(FeatureInput{ID: "Bad ID", Name: "X"})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "S1") {
		t.Fatalf("invalid update = %v", err)
	}
	after, _ := os.ReadFile(s.Path())
	if string(after) != string(before) || len(s.Map().Features) != 0 {
		t.Error("an invalid update changed the map")
	}
	entries, _ := os.ReadDir(filepath.Join(root, Dir))
	if len(entries) != 2 {
		t.Errorf("want only map.yaml and private/, got %v", entries)
	}
}

func TestStoreReloadsHandEdits(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, _ := Open(root, "orders", "")
	if s.Reload() {
		t.Error("Reload without a change reported one")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 2\nsystem:\n  name: orders\n  summary: Edited by hand.\n")
	if !s.Reload() || s.Map().System.Summary != "Edited by hand." || s.Error() != "" {
		t.Fatalf("valid hand edit: summary %q, error %q", s.Map().System.Summary, s.Error())
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 2\nsystem:\n  name: orders\nfeatures:\n  - id: BAD\n    name: x\n")
	if !s.Reload() || s.Error() == "" || s.Map().System.Summary != "Edited by hand." {
		t.Fatalf("invalid hand edit: error %q, summary %q", s.Error(), s.Map().System.Summary)
	}
	if s.Reload() {
		t.Error("the same invalid file must not count as a change again")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/private/test.yaml", testDoc)
	if !s.Reload() {
		t.Error("a change to a private document must count as a change")
	}
	if s.Reload() {
		t.Error("the same private document must not count twice")
	}
}

func TestPublicViewHoldsNoPaths(t *testing.T) {
	root := newScope(t, orderFiles)
	writeFile(t, root, ".hikari/private/layout.yaml", layoutDoc)
	writeFile(t, root, ".hikari/private/test.yaml", testDoc)
	m := New("orders")
	describe(m)
	priv, _ := LoadPrivate(root)
	v := NewView(m, priv, []string{"orders/create.py"}, func(string) string { return "h" })
	data, _ := json.Marshal(v)
	for _, leak := range []string{"orders/", "tests/", "python3", "changed"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("the public view contains %q:\n%s", leak, data)
		}
	}
	if v.State != Unbuilt {
		t.Errorf("state = %s", v.State)
	}
}

func TestContentHash(t *testing.T) {
	root := newScope(t, orderFiles)
	a, _ := ContentHash(root, []string{"orders", "tests/test_create.py"})
	b, _ := ContentHash(root, []string{"tests/test_create.py", "orders"})
	if a != b {
		t.Error("the hash depends on path order")
	}
	writeFile(t, root, "orders/new.py", "x")
	if c, _ := ContentHash(root, []string{"orders", "tests/test_create.py"}); c == a {
		t.Error("a new file in a directory did not change the hash")
	}
	os.Remove(filepath.Join(root, "tests/test_create.py"))
	if d, err := ContentHash(root, []string{"tests/test_create.py"}); err != nil || d == "" {
		t.Errorf("a missing path: %q, %v", d, err)
	}
}
