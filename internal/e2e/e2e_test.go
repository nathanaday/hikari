package e2e

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const plan = `---
components:
  tally:   {release: tally/python-library}
  ballots: {release: ballots/python-library}
  sample:  {fixture: true, note: hand-written ballots}
  machine: {simulates: a voting machine}
  db:      {image: postgres:16, substitutes: the managed database}
actors:
  operator: {runs: tests, count: 1}
  feeder:   {runs: machine, count: 3}
scenarios:
  smoke: {stories: [winner], actors: {operator: 1}}
gaps:
  real-machine:
    what: Ballots from a real voting machine
    why: no machine is reachable
    touches: ["[[accept-ballot]]", "[[winner|the story]]"]
---

## The test system

Prose.
`

func TestParsePlan(t *testing.T) {
	p, body, err := ParsePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Components) != 5 || p.Actors["feeder"].Count != 3 || p.Scenarios["smoke"].Stories[0] != "winner" || !strings.HasPrefix(body, "## The test system") {
		t.Errorf("plan = %+v, body %q", p, body)
	}
	if got := p.Gaps["real-machine"].Links(); !slices.Equal(got, []string{"accept-ballot", "winner"}) {
		t.Errorf("links = %v", got)
	}
	if got := p.GapsTouching("winner"); !slices.Equal(got, []string{"real-machine"}) {
		t.Errorf("GapsTouching = %v", got)
	}
	if p, _, err := ParsePlan("Just prose.\n"); err != nil || len(p.Components) != 0 {
		t.Errorf("a plan without a header: %v", err)
	}
	for name, c := range map[string]struct{ text, want string }{
		"PL1 field":   {"---\nowners: {}\n---\n", "field owners not found"},
		"PL1 close":   {"---\ncomponents: {}\n", "no closing"},
		"PL2 two":     {"---\ncomponents: {x: {release: a/b, image: i}}\n---\n", "exactly one"},
		"PL2 none":    {"---\ncomponents: {x: {note: n}}\n---\n", "exactly one"},
		"PL2 release": {"---\ncomponents: {x: {release: ab}}\n---\n", "<system>/<deliverable>"},
		"PL2 subst":   {"---\ncomponents: {x: {fixture: true, substitutes: s}}\n---\n", "only a release or an image"},
		"PL3 runs":    {"---\nactors: {a: {runs: ghost, count: 1}}\n---\n", "no component"},
		"PL3 count":   {"---\nactors: {a: {runs: tests, count: 0}}\n---\n", "count of at least 1"},
		"PL4 stories": {"---\nscenarios: {s: {stories: []}}\n---\n", "names no story"},
		"PL4 actor":   {"---\nscenarios: {s: {stories: [x], actors: {ghost: 1}}}\n---\n", "does not name"},
		"PL5 id":      {"---\ngaps: {Bad: {what: w, why: y, touches: [x]}}\n---\n", "gap id"},
		"PL5 why":     {"---\ngaps: {g: {what: w, touches: [x]}}\n---\n", "needs what and why"},
		"PL5 touches": {"---\ngaps: {g: {what: w, why: y}}\n---\n", "touches nothing"},
	} {
		if _, _, err := ParsePlan(c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestPlanWarnings(t *testing.T) {
	p, _, _ := ParsePlan(plan)
	c := Context{
		Lock:      Lock{Releases: []LockedRelease{{System: "tally", Deliverable: "python-library"}}},
		Stories:   map[string]bool{"winner": true, "validity": true},
		Touchable: map[string]bool{"winner": true},
		Key:       []string{"winner", "validity"},
		Covered:   map[string]bool{},
	}
	got := strings.Join(p.Warnings(c), "\n")
	for _, want := range []string{
		`component "ballots" uses ballots/python-library, which releases.lock does not pin`,
		`gap "real-machine" touches "accept-ballot", which is no story`,
		`the key story "validity" has no test and no gap`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PL6 warnings lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"winner" has no test`) || strings.Contains(got, "tally/python-library, which") {
		t.Errorf("PL6: a gapped key story and a pinned release are fine:\n%s", got)
	}
}

func TestLockAndApprovals(t *testing.T) {
	l := Lock{Releases: []LockedRelease{{System: "tally", Deliverable: "lib", Version: "0.1.0", Digest: "sha256:b"}, {System: "ballots", Deliverable: "lib", Version: "0.2.0", Digest: "sha256:a"}}}
	data := l.Render()
	back, err := ParseLock(data)
	if err != nil || back.Releases[0].System != "ballots" || back.Version != 1 {
		t.Errorf("LK1 round trip: %+v, %v\n%s", back, err, data)
	}
	if _, err := ParseLock([]byte("version: 2\n")); err == nil {
		t.Error("LK1: a wrong version")
	}
	dir := t.TempDir()
	if l, err := LoadLock(filepath.Join(dir, "none")); err != nil || len(l.Releases) != 0 {
		t.Errorf("a missing lock: %v", err)
	}

	g := Gap{What: "w", Why: "y", Touches: []string{"[[x]]"}}
	a := Approvals{"g": g.Digest()}
	path := filepath.Join(dir, "approvals.yaml")
	if err := a.Save(path); err != nil {
		t.Fatal(err)
	}
	back2, err := LoadApprovals(path)
	if err != nil || !back2.Approved("g", g) {
		t.Errorf("GA2: approvals round trip: %v %v", back2, err)
	}
	g.Why = "changed"
	if back2.Approved("g", g) {
		t.Error("GA2: a changed gap needs a new approval")
	}
}
