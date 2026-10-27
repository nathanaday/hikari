package featuremap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/testmap"
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

const codeMapDoc = `version: 1
parts:
  - {path: orders/, description: Order code.}
features:
  create-order: {code: [orders/create.py], notes: Entry point create().}
  cancel-order: {code: [orders/cancel.py]}
`

const testDoc = `version: 1
suites:
  unit: {command: python3 -m pytest, description: Unit tests.}
tests:
  tests/test_create.py: ["[[create-order]]"]
gaps:
  create-order: [Not tested against the real inventory.]
`

func ptr[T any](v T) *T { return &v }

func describe(m *Map) {
	m.ApplyPartition(PartitionPatch{
		Summary:  ptr("Takes orders."),
		Provides: ptr([]Interface{{Name: "POST /orders", Kind: "http"}}),
		Consumes: ptr([]Interface{{Name: "inventory.reserve", Kind: "function"}}),
	})
	m.Project([]FeatureEntry{
		{ID: "create-order", Name: "Create an order", Interfaces: []string{"POST /orders", "inventory.reserve"}, Related: []string{"cancel-order"}},
		{ID: "cancel-order", Name: "Cancel an order", Related: []string{"create-order"}},
	})
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		change func(*Map)
		rule   string
	}{
		"S1 bad id":            {func(m *Map) { m.Features[0].ID = "Create Order" }, "S1"},
		"S1 duplicate id":      {func(m *Map) { m.Features[0].ID = "create-order" }, "S1"},
		"S1 no name":           {func(m *Map) { m.Features[0].Name = "" }, "S1"},
		"S2 unknown interface": {func(m *Map) { m.Features[0].Interfaces = []string{"GET /nope"} }, "S2"},
		"S3 unknown related":   {func(m *Map) { m.Features[0].Related = []string{"ghost"} }, "S3"},
		"S3 self related":      {func(m *Map) { m.Features[0].Related = []string{"cancel-order"} }, "S3"},
		"S7 bad kind":          {func(m *Map) { m.Partition.Interfaces.Provides[0].Kind = "HTTP API" }, "S7"},
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

	writeFile(t, root, ".hikari/private/code-map.yaml", codeMapDoc)
	writeFile(t, root, ".hikari/private/tests.yaml", testDoc)
	p, err := LoadPrivate(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(m.IDs(), exists); err != nil {
		t.Fatalf("valid documents: %v", err)
	}
	if got := strings.Join(p.Code("create-order"), ","); got != "orders/create.py" || len(p.Code("ghost")) != 0 {
		t.Errorf("Code = %s", got)
	}
	if p.Tests.Gaps["create-order"][0] != "Not tested against the real inventory." {
		t.Errorf("tests: %+v", p.Tests)
	}

	cases := map[string]struct {
		codeMap, tests, rule string
	}{
		"P1 unknown field":     {codeMapDoc + "extra: 1\n", testDoc, "P1"},
		"P1 wrong version":     {strings.Replace(codeMapDoc, "version: 1", "version: 3", 1), testDoc, "P1"},
		"P2 unknown feature":   {codeMapDoc + "  ghost: {code: [orders/create.py]}\n", testDoc, "P2"},
		"P3 missing code":      {strings.Replace(codeMapDoc, "orders/cancel.py", "orders/gone.py", 1), testDoc, "P3"},
		"P3 escaping part":     {strings.Replace(codeMapDoc, "path: orders/", "path: ../secret", 1), testDoc, "P3"},
		"X1 unknown field":     {codeMapDoc, testDoc + "features: {}\n", "X1"},
		"X2 suite without cmd": {codeMapDoc, strings.Replace(testDoc, "command: python3 -m pytest, ", "", 1), "X2"},
		"X3 unknown feature":   {codeMapDoc, strings.Replace(testDoc, "[[create-order]]", "[[ghost]]", 1), "X3"},
		"X4 unknown gap":       {codeMapDoc, strings.Replace(testDoc, "  create-order: [Not", "  ghost: [Not", 1), "X4"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			writeFile(t, root, ".hikari/private/code-map.yaml", c.codeMap)
			writeFile(t, root, ".hikari/private/tests.yaml", c.tests)
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
	writeFile(t, root, ".hikari/private/code-map.yaml", "")
	if _, err := LoadPrivate(root); err != nil {
		t.Errorf("an empty file: %v", err)
	}
}

func TestProject(t *testing.T) {
	m := New("orders")
	describe(m)
	m.Project([]FeatureEntry{
		{ID: "place-order", Name: "Place an order", Tests: Tests{Passed: 4}, Health: testmap.Passing},
		{ID: "cancel-order", Name: "Cancel"},
	})
	if len(m.Features) != 2 || m.Features[0].ID != "cancel-order" || m.Features[1].ID != "place-order" {
		t.Fatalf("Q1: features in id order: %+v", m.Features)
	}
	if m.Features[1].Tests.Passed != 4 || m.Features[1].Health != testmap.Passing {
		t.Errorf("Q6: the projection carries tests and health: %+v", m.Features[1])
	}
	m.Project([]FeatureEntry{{ID: "cancel-order", Name: "Cancel"}})
	if len(m.Features) != 1 {
		t.Errorf("a removed feature leaves the map: %+v", m.Features)
	}
}

func TestViewHealth(t *testing.T) {
	m := New("orders")
	describe(m)
	ran := now
	v := NewView(m, nil, map[string]testmap.FeatureHealth{
		"create-order": {Health: testmap.Failing, Passed: 2, Failed: 1, Pending: 3, Ran: ran},
	}, nil)
	byID := map[string]Feature{}
	for _, f := range v.Features {
		byID[f.ID] = f
	}
	if f := byID["create-order"]; f.Health != testmap.Failing || f.Tests != (Tests{Passed: 2, Failed: 1, Pending: 3, Ran: ran}) {
		t.Errorf("Q5: %+v", f)
	}
	if f := byID["cancel-order"]; f.Health != testmap.Untested {
		t.Errorf("S24: a feature with no health is untested: %+v", f)
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
	s, created, err := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "python3 -m unittest")
	if err != nil || !created {
		t.Fatalf("Open = %v, created %v", err, created)
	}
	data, _ := os.ReadFile(s.Path())
	if strings.HasPrefix(string(data), "#") || !strings.HasPrefix(string(data), "version: 4\npartition:\n  name: orders\n") {
		t.Errorf("the map file must start with its version, have no comment, and indent by two spaces:\n%s", data)
	}
	p, err := s.Private()
	if err != nil {
		t.Fatal(err)
	}
	if p.Tests.Suites["all"].Command != "python3 -m unittest" || len(p.CodeMap.Parts) != 0 {
		t.Errorf("skeletons: %+v", p)
	}
	if errs := s.PrivateErrors(s.Map()); len(errs) != 0 {
		t.Errorf("new skeletons must be valid: %v", errs)
	}
	if ignore, _ := os.ReadFile(filepath.Join(root, ".hikari/private/.gitignore")); string(ignore) != "reports/\nresults.yaml\n" {
		t.Errorf("L13: .gitignore = %q", ignore)
	}
	if info, err := os.Stat(filepath.Join(root, ".hikari/private/reports")); err != nil || !info.IsDir() {
		t.Errorf("L13: no reports directory: %v", err)
	}
	if len(s.Notes()) != 0 {
		t.Errorf("notes for a new scope: %v", s.Notes())
	}

	// Open keeps documents that exist.
	writeFile(t, root, ".hikari/private/code-map.yaml", "version: 1\nparts: [{path: orders/, description: kept}]\nfeatures: {}\n")
	if _, _, err := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "other"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Private(); p.CodeMap.Parts[0].Description != "kept" || p.Tests.Suites["all"].Command != "python3 -m unittest" {
		t.Errorf("Open overwrote existing documents: %+v", p)
	}
}

func TestOpenDeletesTheOldTestPlan(t *testing.T) {
	root := newScope(t, orderFiles)
	writeFile(t, root, ".hikari/private/test.yaml", "version: 1\nsuites: {}\nfeatures: {}\n")
	writeFile(t, root, ".hikari/private/.gitignore", "reports/\nresults.yaml\nenvironment/\n")
	s, _, err := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hikari/private/test.yaml")); err == nil {
		t.Error("O8: test.yaml is still there")
	}
	if len(s.Notes()) != 1 || !strings.Contains(s.Notes()[0], "test.yaml") {
		t.Errorf("O8: notes %v", s.Notes())
	}
	if ignore, _ := os.ReadFile(filepath.Join(root, ".hikari/private/.gitignore")); string(ignore) != "reports/\nresults.yaml\n" {
		t.Errorf("L13: a different .gitignore is rewritten: %q", ignore)
	}
}

func TestFilesRejectInvalidChanges(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, _ := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "")
	before, _ := os.ReadFile(s.Path())
	err := s.Update(func(m *Map) error {
		m.Project([]FeatureEntry{{ID: "Bad ID", Name: "X"}})
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

func TestOldKeyIsRewritten(t *testing.T) {
	root := newScope(t, orderFiles)
	writeFile(t, root, ".hikari/map.yaml", "version: 4\nsystem:\n  name: orders\n  summary: Kept.\nfeatures: []\n")
	s, created, err := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "")
	if err != nil || created || s.Map().Partition.Summary != "Kept." {
		t.Fatalf("an old map loads: %v, %v", created, err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".hikari", "map.yaml")); !strings.Contains(string(data), "partition:\n  name: orders\n  summary: Kept.") || strings.Contains(string(data), "system:") {
		t.Errorf("the old key is rewritten:\n%s", data)
	}
	if len(s.Notes()) != 1 {
		t.Errorf("notes = %v", s.Notes())
	}
	if m, err := Decode([]byte("version: 4\nsystem: {name: orders}\n")); err != nil || m.Partition.Name != "orders" {
		t.Errorf("Decode reads the old key: %+v, %v", m, err)
	}
}

func TestFilesReloadHandEdits(t *testing.T) {
	root := newScope(t, orderFiles)
	s, _, _ := Open(root, filepath.Join(root, ".hikari", "map.yaml"), "orders", "")
	if s.Reload() {
		t.Error("Reload without a change reported one")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 4\npartition:\n  name: orders\n  summary: Edited by hand.\n")
	if !s.Reload() || s.Map().Partition.Summary != "Edited by hand." || s.Error() != "" {
		t.Fatalf("valid hand edit: summary %q, error %q", s.Map().Partition.Summary, s.Error())
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/map.yaml", "version: 4\npartition:\n  name: orders\nfeatures:\n  - id: BAD\n    name: x\n")
	if !s.Reload() || s.Error() == "" || s.Map().Partition.Summary != "Edited by hand." {
		t.Fatalf("invalid hand edit: error %q, summary %q", s.Error(), s.Map().Partition.Summary)
	}
	if s.Reload() {
		t.Error("the same invalid file must not count as a change again")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, ".hikari/private/tests.yaml", testDoc)
	if !s.Reload() {
		t.Error("a change to a private document must count as a change")
	}
	if s.Reload() {
		t.Error("the same private document must not count twice")
	}
}

func TestPublicViewHoldsNoPaths(t *testing.T) {
	m := New("orders")
	describe(m)
	v := NewView(m, []string{"orders/create.py"}, map[string]testmap.FeatureHealth{"create-order": {Health: testmap.Passing, Files: []string{"tests/test_create.py"}}}, map[string]string{"create-order": Stale})
	data, _ := json.Marshal(v)
	for _, leak := range []string{"orders/", "tests/", "python3", "changed"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("the public view contains %q:\n%s", leak, data)
		}
	}
	if v.State != Unbuilt || v.Features[1].State != Stale {
		t.Errorf("state = %s, features %+v", v.State, v.Features)
	}
}

func TestOpenDeletesOldVersions(t *testing.T) {
	root := newScope(t, orderFiles)
	mapPath := filepath.Join(root, ".hikari", "map.yaml")
	writeFile(t, root, ".hikari/map.yaml", "version: 2\npartition:\n  name: orders\n  summary: Old.\nfeatures:\n  - {id: x, name: X}\n")
	s, created, err := Open(root, mapPath, "orders", "")
	if err != nil || !created || len(s.Notes()) != 1 || !strings.Contains(s.Notes()[0], "version 2") {
		t.Fatalf("Q4: Open = %v, created %v, notes %q", err, created, s.Notes())
	}
	if m := s.Map(); m.Version != 4 || len(m.Features) != 0 || m.Partition.Summary != "" {
		t.Errorf("Q4: the old map must be replaced, got %+v", m)
	}
	if s, _, err := Open(root, mapPath, "orders", ""); err != nil || len(s.Notes()) != 0 {
		t.Errorf("a current map is kept: %v %q", err, s.Notes())
	}
}

func TestFeatureStates(t *testing.T) {
	root := newScope(t, orderFiles)
	writeFile(t, root, ".hikari/private/code-map.yaml", codeMapDoc)
	priv, _ := LoadPrivate(root)
	paths := func(id string) []string {
		if id == "create-order" {
			return append(priv.Code(id), "tests/test_create.py")
		}
		return priv.Code(id)
	}
	testFiles := map[string]bool{"tests/test_create.py": true, "tests/test_other.py": true}
	described := func(id string) bool { return id != "cancel-order" }
	changed := []string{"orders/create.py", "tests/test_create.py", "orders/cancel.py", "README.md", "tests/test_other.py"}
	st := FeatureStates([]string{"cancel-order", "create-order"}, described, paths, testFiles, changed)
	if st.ByID["create-order"] != Stale || st.ByID["cancel-order"] != Undescribed {
		t.Errorf("S20: %v", st.ByID)
	}
	if len(st.Stale) != 1 || st.Stale[0].ID != "create-order" || !slices.Equal(st.Stale[0].Paths, []string{"orders/create.py", "tests/test_create.py"}) {
		t.Errorf("S20: stale = %+v", st.Stale)
	}
	if !slices.Equal(st.Undescribed, []string{"cancel-order"}) || !slices.Equal(st.Unmapped, []string{"README.md"}) {
		t.Errorf("S21: undescribed %v, unmapped %v", st.Undescribed, st.Unmapped)
	}
	if st := FeatureStates([]string{"create-order"}, described, paths, testFiles, nil); st.ByID["create-order"] != Current {
		t.Errorf("S20: nothing changed: %v", st.ByID)
	}
	// A directory in the code map covers the files below it.
	writeFile(t, root, ".hikari/private/code-map.yaml", "version: 1\nparts: []\nfeatures:\n  create-order: {code: [orders/]}\n")
	priv, _ = LoadPrivate(root)
	if st := FeatureStates([]string{"create-order"}, described, priv.Code, nil, []string{"orders/create.py"}); st.ByID["create-order"] != Stale {
		t.Errorf("S19: a directory covers its files: %v", st.ByID)
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
