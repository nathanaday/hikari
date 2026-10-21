package sysmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

func ptr[T any](v T) *T { return &v }

func describe(m *Map) {
	m.ApplySystem(SystemPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]Interface{{Name: "POST /orders", Kind: "http"}}),
		Consumes: ptr([]Interface{{Name: "inventory.reserve", Kind: "function"}}),
		Layout:   ptr([]LayoutEntry{{Path: "orders/", Description: "Order code."}}),
	})
	m.Upsert(FeatureInput{
		ID: "create-order", Name: "Create an order",
		Interfaces: []string{"POST /orders", "inventory.reserve"},
		Code:       []string{"orders/create.py"}, TestRefs: []string{"tests/test_create.py"},
		Related: []string{"cancel-order"},
	})
	m.Upsert(FeatureInput{ID: "cancel-order", Name: "Cancel an order", Code: []string{"orders/cancel.py"}})
}

func TestValidate(t *testing.T) {
	root := newScope(t, orderFiles)
	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(root, rel)); return err == nil }
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
		"S4 missing code":      {func(m *Map) { m.Features[0].Code = []string{"orders/missing.py"} }, "S4"},
		"S4 escaping path":     {func(m *Map) { m.Features[0].Code = []string{"../secret"} }, "S4"},
		"S4 missing test":      {func(m *Map) { m.Features[0].Tests.Refs = []string{"tests/nope.py"} }, "S4"},
		"S4 missing layout":    {func(m *Map) { m.System.Layout[0].Path = "nowhere/" }, "S4"},
		"S7 bad kind":          {func(m *Map) { m.System.Interfaces.Provides[0].Kind = "HTTP API" }, "S7"},
	}
	base := New("orders", "")
	describe(base)
	if err := Validate(base, exists); err != nil {
		t.Fatalf("valid map: %v", err)
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := base.Clone()
			c.change(m)
			err := Validate(m, exists)
			if err == nil || !strings.Contains(err.Error(), c.rule+":") {
				t.Errorf("error = %v, want rule %s", err, c.rule)
			}
		})
	}
}

func TestUpsertKeepsResultsAndRemoveDropsReferences(t *testing.T) {
	m := New("orders", "")
	describe(m)
	m.Features[0].Tests.Passed = 4
	m.Upsert(FeatureInput{ID: "create-order", Name: "Renamed", TestRefs: []string{"tests/test_create.py"}})
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
	health := func() map[string]string {
		v := NewView(s.Map(), nil, "", func(p []string) string { return s.hashOrEmpty(p) })
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
			return m.RecordTests([]TestResult{{Feature: "create-order", Passed: passed, Failed: failed}}, now, s.Hash)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	record(3, 0)
	if h := health(); h["create-order"] != Passing || h["cancel-order"] != Untested {
		t.Errorf("S11 and S8 after a passing run: %v", h)
	}
	if got := s.Map().Features[0].Health; got != Passing {
		t.Errorf("the file stores health %q", got)
	}
	record(2, 1)
	if h := health(); h["create-order"] != Failing {
		t.Errorf("S10 after a failing run: %v", h)
	}
	writeFile(t, root, "tests/test_create.py", "def test(): assert False\n")
	if h := health(); h["create-order"] != Stale {
		t.Errorf("S9 after a test file changed: %v (stale must win over failing)", h)
	}
}

func TestState(t *testing.T) {
	m := New("orders", "")
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

func TestStoreRejectsInvalidChanges(t *testing.T) {
	root := newScope(t, orderFiles)
	s, created, err := Open(root, "orders", "python3 -m unittest")
	if err != nil || !created {
		t.Fatalf("Open = %v, created %v", err, created)
	}
	before, _ := os.ReadFile(s.Path())
	if !strings.HasPrefix(string(before), "# The system map of orders.") || !strings.Contains(string(before), "test_command: python3 -m unittest") {
		t.Errorf("new map file:\n%s", before)
	}
	err = s.Update(func(m *Map) error {
		m.Upsert(FeatureInput{ID: "x", Name: "X", Code: []string{"missing.py"}})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "S4") {
		t.Fatalf("invalid update = %v", err)
	}
	after, _ := os.ReadFile(s.Path())
	if string(after) != string(before) || len(s.Map().Features) != 0 {
		t.Error("an invalid update changed the map")
	}
	entries, _ := os.ReadDir(filepath.Join(root, Dir))
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestStoreReloadsHandEdits(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, _ := Open(root, "orders", "")
	if s.Reload() {
		t.Error("Reload without a change reported one")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 1\nsystem:\n  name: orders\n  summary: Edited by hand.\n")
	if !s.Reload() || s.Map().System.Summary != "Edited by hand." || s.Error() != "" {
		t.Fatalf("valid hand edit: summary %q, error %q", s.Map().System.Summary, s.Error())
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 1\nsystem:\n  name: orders\nfeatures:\n  - id: BAD\n    name: x\n")
	if !s.Reload() || s.Error() == "" || s.Map().System.Summary != "Edited by hand." {
		t.Fatalf("invalid hand edit: error %q, summary %q", s.Error(), s.Map().System.Summary)
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 1\nsystem:\n  name: someone-else\n")
	if !s.Reload() || !strings.Contains(s.Error(), "belongs to \"orders\"") {
		t.Fatalf("renamed system: error %q", s.Error())
	}
}

func TestOpenParsesTheSpecExample(t *testing.T) {
	root := newScope(t, orderFiles)
	writeFile(t, root, ".hikari/map.yaml", `version: 1
system:
  name: orders
  summary: Takes orders from checkout and stores them.
  responsibilities: [Validate and store orders.]
  interfaces:
    provides:
      - {name: POST /orders, kind: http, description: Creates an order.}
    consumes:
      - {name: inventory.reserve, kind: function, description: Reserves stock.}
  layout:
    - {path: orders/, description: Order validation and storage.}
  test_command: python3 -m unittest
  described_state: {commit: 4e8ada5, updated: 2026-10-21T09:30:00Z}
features:
  - id: create-order
    name: Create an order
    description: Validates an order, reserves stock, and stores it.
    interfaces: [POST /orders, inventory.reserve]
    code: [orders/create.py]
    tests:
      refs: [tests/test_create.py]
      passed: 6
      failed: 0
      ran: 2026-10-21T09:28:00Z
      content_hash: 9f2c
    health: passing
    findings: [A client retry can create a duplicate order.]
    plans: [Add idempotency keys.]
    related: [cancel-order]
  - {id: cancel-order, name: Cancel an order}
`)
	s, created, err := Open(root, "orders", "")
	if err != nil || created {
		t.Fatalf("Open = %v, created %v", err, created)
	}
	if m := s.Map(); m.System.DescribedState.Commit != "4e8ada5" || m.Features[0].Tests.Passed != 6 {
		t.Errorf("parsed map: %+v", m)
	}
}

func TestRedactLeavesOutPaths(t *testing.T) {
	m := New("orders", "")
	describe(m)
	m.Features[0].Tests.ContentHash = "abc"
	m.Features[0].Tests.Passed = 2
	r := Redact(NewView(m, nil, "", func([]string) string { return "abc" }))
	data, _ := json.Marshal(r)
	for _, leak := range []string{"layout", "orders/create.py", "tests/test_create.py", "content_hash", "refs", `"code"`} {
		if strings.Contains(string(data), leak) {
			t.Errorf("redacted view contains %q:\n%s", leak, data)
		}
	}
	if r.Features[0].Tests.Passed != 2 || !slices.Equal(r.Features[0].Interfaces, []string{"POST /orders", "inventory.reserve"}) {
		t.Errorf("redacted view lost data: %+v", r.Features[0])
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
