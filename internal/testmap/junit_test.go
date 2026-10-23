package testmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readReport(t *testing.T, name, workDir string, disk Disk) []Case {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := ParseReport(data, workDir, disk)
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestVitestReport(t *testing.T) {
	disk := fakeDisk{files: []string{"test/unit/text-ops.test.ts"}}
	cases := readReport(t, "vitest.xml", "/workspace/yami", disk)
	if len(cases) != 7 {
		t.Fatalf("%d cases", len(cases))
	}
	c := cases[0]
	if !strings.HasPrefix(c.ID, "test/unit/text-ops.test.ts > the text operations of yami/document > diffOps turns") {
		t.Fatalf("id %q", c.ID)
	}
	if c.File != "test/unit/text-ops.test.ts" || c.Outcome != Passed {
		t.Fatalf("case %+v", c)
	}
	if none := readReport(t, "vitest.xml", "/workspace/yami", fakeDisk{}); none[0].File != "" {
		t.Fatalf("a class name that is no file of the scope gave file %q", none[0].File)
	}
}

func TestPytestReports(t *testing.T) {
	x1 := readReport(t, "pytest-xunit1.xml", "/workspace/inventory", fakeDisk{})
	if len(x1) != 3 || x1[1].ID != "tests.test_stock.StockTest > test_reserve_holds_stock" || x1[1].File != "tests/test_stock.py" {
		t.Fatalf("xunit1 %+v", x1)
	}
	x2 := readReport(t, "pytest-xunit2.xml", "/workspace/inventory", fakeDisk{})
	if x2[1].File != "" {
		t.Fatalf("xunit2 has no file attribute, got %q", x2[1].File)
	}
	fails := readReport(t, "pytest-failures.xml", "/workspace/inventory", fakeDisk{})
	got := map[string]Case{}
	for _, c := range fails {
		got[c.ID] = c
	}
	if c := got["tests.test_fail.FailTest > test_fails"]; c.Outcome != Failed || c.Message != "AssertionError: 1 != 2 : one is not two" {
		t.Fatalf("failure %+v", c)
	}
	if c := got["tests.test_fail.FailTest > test_errors"]; c.Outcome != Failed || c.Message != "RuntimeError: boom" {
		t.Fatalf("error %+v", c)
	}
	if c := got["tests.test_fail.FailTest > test_skipped"]; c.Outcome != Skipped {
		t.Fatalf("skip %+v", c)
	}
}

func TestEdgeReport(t *testing.T) {
	cases := readReport(t, "edge.xml", "/workspace/lib", fakeDisk{})
	got := map[string]Case{}
	for _, c := range cases {
		got[c.ID] = c
	}
	if len(cases) != 7 {
		t.Fatalf("%d cases: %+v", len(cases), cases)
	}
	if c := got["outer > uses the suite file"]; c.File != "test/outer.test.ts" {
		t.Fatalf("suite file: %+v", c)
	}
	if c := got["outer > twice"]; c.Outcome != Failed || c.Message != "first line of the text" {
		t.Fatalf("duplicate: %+v", c)
	}
	if got["outer > skipped twice"].Outcome != Skipped || got["outer > skipped once"].Outcome != Passed {
		t.Fatal("a duplicate is skipped only when every case is")
	}
	if c := got["pkg/thing > TestThing"]; c.Outcome != Failed || c.Message != "panic: nil map" || c.File != "" {
		t.Fatalf("error: %+v", c)
	}
	if _, ok := got["no class"]; !ok {
		t.Fatal("a case with no class name keeps its name as id")
	}
	if c := got["x > outside"]; c.File != "" {
		t.Fatalf("a file outside the scope gave %q", c.File)
	}
}

func TestBadReports(t *testing.T) {
	for name, data := range map[string]string{
		"not xml":  "hello <",
		"no cases": `<testsuites><testsuite name="s"/></testsuites>`,
	} {
		if _, err := ParseReport([]byte(data), "/w", fakeDisk{}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	big := make([]byte, MaxReportBytes+1)
	if _, err := ParseReport(big, "/w", fakeDisk{}); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("size: %v", err)
	}
	if got := message(&junitProblem{Message: strings.Repeat("é", 300)}); len([]rune(got)) != maxMessage {
		t.Errorf("message has %d runes", len([]rune(got)))
	}
}

func TestResultsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ResultsFile)
	r, err := LoadResults(path)
	if err != nil || len(r.Runs) != 0 {
		t.Fatalf("missing file: %+v, %v", r, err)
	}
	cases := []Case{{ID: "a > x", File: "a.py", Outcome: Passed}, {ID: "a > y", File: "a.py", Outcome: Failed, Message: "no"}, {ID: "b", Outcome: Skipped}}
	now := time.Date(2026, 10, 22, 5, 0, 0, 0, time.UTC)
	hashed := 0
	run := NewRun(cases, now, map[string]string{"wiki": "c1"}, func(rel string) string { hashed++; return "h-" + rel })
	if hashed != 1 || run.Files["a.py"] != "h-a.py" || len(run.Tests) != 3 {
		t.Fatalf("run %+v, hashed %d", run, hashed)
	}
	r.Runs["unit"] = run
	if err := SaveResults(path, r); err != nil {
		t.Fatal(err)
	}
	back, err := LoadResults(path)
	if err != nil || !back.Runs["unit"].Ran.Equal(now) || back.Runs["unit"].Tests[1].Message != "no" || back.Runs["unit"].Code["wiki"] != "c1" {
		t.Fatalf("back %+v, %v", back, err)
	}
	os.WriteFile(path, []byte("version: 9\n"), 0o644)
	if bad, err := LoadResults(path); err == nil || len(bad.Runs) != 0 {
		t.Fatalf("invalid file: %+v, %v", bad, err)
	}
}
