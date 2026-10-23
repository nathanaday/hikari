package testmap

import (
	"strings"
	"testing"
	"time"
)

// world is a scope in memory: the hash of each file and feature's code.
type world struct {
	files, code map[string]string
	calls       int
}

func (w *world) hashes() Hashes {
	return Hashes{
		File: func(rel string) string { w.calls++; return w.files[rel] },
		Code: func(id string) string { w.calls++; return w.code[id] },
	}
}

func healthFixture() (Map, *world, Results) {
	m := Map{Version: 1, Tests: map[string][]string{
		"tests/stock.py":                 {"[[reserve]]", "[[restock]]"},
		"tests/stock.py > test_restock":  {"[[restock]]"},
		"tests/live.py":                  {},
		"tests/gone.py > test_old_thing": {"[[cancel]]"},
	}}
	w := &world{
		files: map[string]string{"tests/stock.py": "s1", "tests/live.py": "l1"},
		code:  map[string]string{"reserve": "r1", "restock": "k1", "cancel": "c1", "audit": "a1"},
	}
	ran := time.Date(2026, 10, 22, 5, 0, 0, 0, time.UTC)
	r := Results{Runs: map[string]Run{"unit": {
		Ran:   ran,
		Code:  map[string]string{"reserve": "r1", "restock": "k1", "cancel": "c1", "audit": "a1"},
		Files: map[string]string{"tests/stock.py": "s1", "tests/live.py": "l1"},
		Tests: []Result{
			{ID: "tests/stock.py > test_reserve", File: "tests/stock.py", Outcome: Passed},
			{ID: "tests/stock.py > test_reserve_fails", File: "tests/stock.py", Outcome: Passed},
			{ID: "tests/stock.py > test_restock", File: "tests/stock.py", Outcome: Passed},
			{ID: "tests/live.py > test_live", File: "tests/live.py", Outcome: Skipped},
			{ID: "tests/new.py > test_a", File: "tests/new.py", Outcome: Passed},
			{ID: "no_file_test", Outcome: Passed},
		},
	}}}
	return m, w, r
}

func disk() Disk {
	return fakeDisk{files: []string{"tests/stock.py", "tests/live.py"}, dirs: []string{"tests"}}
}

func compute(m Map, w *world, r Results) Report {
	return Compute(m, NewMatcher(m, disk()), r, []string{"audit", "cancel", "reserve", "restock"}, w.hashes())
}

func TestHealthRules(t *testing.T) {
	m, w, r := healthFixture()
	rep := compute(m, w, r)
	want := map[string]string{"audit": Untested, "cancel": Pending, "reserve": Passing, "restock": Passing}
	for id, h := range want {
		if got := rep.Features[id].Health; got != h {
			t.Errorf("%s: %s, want %s", id, got, h)
		}
	}
	if f := rep.Features["restock"]; f.Passed != 3 || f.Failed != 0 || f.Pending != 0 || f.Ran.IsZero() || strings.Join(f.Files, ",") != "tests/stock.py" {
		t.Errorf("restock counts %+v", f)
	}
	if f := rep.Features["reserve"]; f.Passed != 2 {
		t.Errorf("reserve counts %+v: the id key gives test_restock only to restock", f)
	}
	if strings.Join(rep.StaleKeys, ",") != "tests/gone.py > test_old_thing" {
		t.Errorf("stale keys %v", rep.StaleKeys)
	}
	if len(rep.Unmapped) != 2 || rep.Unmapped[0].File != "(no file)" || rep.Unmapped[1].File != "tests/new.py" {
		t.Errorf("unmapped %+v", rep.Unmapped)
	}
	if !rep.TestFiles["tests/new.py"] || !rep.TestFiles["tests/live.py"] {
		t.Errorf("test files %v", rep.TestFiles)
	}
	if strings.Join(rep.Pending(), ",") != "cancel" {
		t.Errorf("pending %v", rep.Pending())
	}
	if w.calls > 5 {
		t.Errorf("hashed %d times; each path and feature at most once", w.calls)
	}
}

func TestFailingBeatsPending(t *testing.T) {
	m, w, r := healthFixture()
	run := r.Runs["unit"]
	run.Tests[0].Outcome, run.Tests[0].Message = Failed, "no stock held"
	r.Runs["unit"] = run
	rep := compute(m, w, r)
	if rep.Features["reserve"].Health != Failing || rep.Features["restock"].Health != Failing {
		t.Fatalf("one failing test fails every feature it touches: %+v", rep.Features)
	}
	if len(rep.Failing) != 1 || rep.Failing[0].Message != "no stock held" || strings.Join(rep.Failing[0].Features, ",") != "reserve,restock" || rep.Failing[0].Suite != "unit" {
		t.Fatalf("failing %+v", rep.Failing)
	}
	w.code["restock"] = "k2"
	rep = compute(m, w, r)
	if rep.Features["restock"].Health != Pending || rep.Features["reserve"].Health != Pending || len(rep.Failing) != 0 {
		t.Fatalf("a result whose touched code changed is not current: %+v %+v", rep.Features, rep.Failing)
	}
}

func TestChangesMakeResultsPending(t *testing.T) {
	m, w, r := healthFixture()
	w.files["tests/stock.py"] = "s2"
	if rep := compute(m, w, r); rep.Features["reserve"].Health != Pending || rep.Features["reserve"].Pending != 2 {
		t.Fatalf("a changed test file: %+v", rep.Features["reserve"])
	}
	m, w, r = healthFixture()
	w.code["reserve"] = "r2"
	rep := compute(m, w, r)
	if rep.Features["reserve"].Health != Pending || rep.Features["restock"].Health != Pending {
		t.Fatalf("a test that touches reserve depends on its code: %+v", rep.Features)
	}
	m, w, r = healthFixture()
	run := r.Runs["unit"]
	delete(run.Code, "restock")
	r.Runs["unit"] = run
	if rep := compute(m, w, r); rep.Features["restock"].Health != Pending {
		t.Fatalf("a feature the run did not know: %+v", rep.Features["restock"])
	}
}

func TestShadowedKey(t *testing.T) {
	m, w, r := healthFixture()
	delete(m.Tests, "tests/gone.py > test_old_thing")
	// A class key that every test overrides is harmless: not stale.
	m.Tests["tests/stock.py > test_restock"] = []string{"[[restock]]"}
	m.Tests["tests/stock.py > test_reserve"] = []string{"[[reserve]]"}
	m.Tests["tests/stock.py > test_reserve_fails"] = []string{"[[reserve]]"}
	m.Tests["tests/stock.py"] = []string{"[[cancel]]"}
	rep := compute(m, w, r)
	if len(rep.StaleKeys) != 0 || len(rep.Shadowed) != 1 || rep.Shadowed[0] != "tests/stock.py" {
		t.Fatalf("stale %v, shadowed %v", rep.StaleKeys, rep.Shadowed)
	}
	if rep.Features["cancel"].Health != Untested || rep.Features["reserve"].Health != Passing {
		t.Fatalf("a feature that only a shadowed key links has no touching test: %+v", rep.Features)
	}
}

func TestSkippedIsPending(t *testing.T) {
	m, w, r := healthFixture()
	m.Tests["tests/live.py"] = []string{"[[audit]]"}
	if rep := compute(m, w, r); rep.Features["audit"].Health != Pending || rep.Features["audit"].Pending != 1 {
		t.Fatalf("skipped: %+v", rep.Features["audit"])
	}
}

func TestMappingAfterTheRun(t *testing.T) {
	m, w, r := healthFixture()
	m.Tests["tests/new.py > test_a"] = []string{"[[audit]]"}
	rep := compute(m, w, r)
	if rep.Features["audit"].Health != Pending {
		t.Fatalf("tests/new.py was not hashed at the run, so its result is not current: %+v", rep.Features["audit"])
	}
	m.Tests["no_file_test"] = []string{"[[audit]]"}
	delete(m.Tests, "tests/new.py > test_a")
	if rep := compute(m, w, r); rep.Features["audit"].Health != Passing {
		t.Fatalf("a key written after the run takes effect at once: %+v", rep.Features["audit"])
	}
}

func TestUnmappedCap(t *testing.T) {
	var tests []Result
	for i := range 25 {
		tests = append(tests, Result{ID: "t > " + string(rune('a'+i)), File: "big.py", Outcome: Passed})
	}
	r := Results{Runs: map[string]Run{"unit": {Tests: tests}}}
	rep := Compute(Map{}, NewMatcher(Map{}, fakeDisk{}), r, nil, Hashes{File: func(string) string { return "" }, Code: func(string) string { return "" }})
	if len(rep.Unmapped) != 1 || len(rep.Unmapped[0].Tests) != MaxUnmappedIDs || rep.Unmapped[0].More != 5 {
		t.Fatalf("unmapped %+v", rep.Unmapped)
	}
}
