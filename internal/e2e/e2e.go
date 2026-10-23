// Package e2e holds the documents of a tester: the architecture plan of the
// E2E test system, the lock of the release set it tests, and the user's
// approvals of the plan's gaps.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The files of a tester's scope.
const (
	PlanFile = "plan.md"
	LockFile = "releases.lock"
)

// Plan is the header of plan.md (rules PL1 to PL5 of the tester spec).
type Plan struct {
	Components map[string]Component `yaml:"components,omitempty" json:"components"`
	Actors     map[string]Actor     `yaml:"actors,omitempty" json:"actors"`
	Scenarios  map[string]Scenario  `yaml:"scenarios,omitempty" json:"scenarios"`
	Gaps       map[string]Gap       `yaml:"gaps,omitempty" json:"gaps"`
}

// Component is one part of the test system.
type Component struct {
	Release     string `yaml:"release,omitempty" json:"release,omitempty"`
	Image       string `yaml:"image,omitempty" json:"image,omitempty"`
	Fixture     bool   `yaml:"fixture,omitempty" json:"fixture,omitempty"`
	Simulates   string `yaml:"simulates,omitempty" json:"simulates,omitempty"`
	Substitutes string `yaml:"substitutes,omitempty" json:"substitutes,omitempty"`
	Note        string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Actor is a client that drives the test system.
type Actor struct {
	Runs  string `yaml:"runs" json:"runs"`
	Count int    `yaml:"count" json:"count"`
	Note  string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Scenario is a set of stories run with set actor counts.
type Scenario struct {
	Stories []string       `yaml:"stories" json:"stories"`
	Actors  map[string]int `yaml:"actors,omitempty" json:"actors"`
	Minutes int            `yaml:"minutes,omitempty" json:"minutes,omitempty"`
}

// Gap is something the plan cannot test, with the reason.
type Gap struct {
	What    string   `yaml:"what" json:"what"`
	Why     string   `yaml:"why" json:"why"`
	Touches []string `yaml:"touches" json:"touches"`
}

// Digest identifies the text of a gap, for its approval (rule GA2).
func (g Gap) Digest() string {
	sum := sha256.Sum256([]byte(g.What + "\x00" + g.Why + "\x00" + strings.Join(g.Touches, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// TestsActor is the runs value of an actor that is the tester's own suite.
const TestsActor = "tests"

var gapID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// ParsePlan reads plan.md: its header and its body. A file without a
// header is a plan with no sections.
func ParsePlan(text string) (Plan, string, error) {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return Plan{}, text, nil
	}
	header, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		if h, ok2 := strings.CutSuffix(rest, "\n---"); ok2 {
			header, body, ok = h, "", true
		}
	}
	if !ok {
		return Plan{}, "", fmt.Errorf("PL1: %s: the property header has no closing ---", PlanFile)
	}
	var p Plan
	dec := yaml.NewDecoder(strings.NewReader(header))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return Plan{}, "", fmt.Errorf("PL1: %s: %w", PlanFile, err)
	}
	return p, strings.TrimPrefix(body, "\n"), p.shape()
}

// shape checks the rules of the plan that need nothing outside it (PL2 to
// PL5).
func (p Plan) shape() error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s: "+format, append([]any{rule, PlanFile}, args...)...))
	}
	for _, name := range sorted(p.Components) {
		c := p.Components[name]
		n := 0
		for _, set := range []bool{c.Release != "", c.Image != "", c.Fixture, c.Simulates != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			fail("PL2", "component %q needs exactly one of release, image, fixture, or simulates", name)
		}
		if c.Release != "" {
			if sys, del, ok := strings.Cut(c.Release, "/"); !ok || sys == "" || del == "" || strings.Contains(del, "/") {
				fail("PL2", "component %q: release %q must be <system>/<deliverable>", name, c.Release)
			}
		}
		if c.Substitutes != "" && c.Release == "" && c.Image == "" {
			fail("PL2", "component %q: only a release or an image substitutes for a service", name)
		}
	}
	for _, name := range sorted(p.Actors) {
		a := p.Actors[name]
		if _, ok := p.Components[a.Runs]; !ok && a.Runs != TestsActor {
			fail("PL3", "actor %q runs %q, which is no component and not %q", name, a.Runs, TestsActor)
		}
		if a.Count < 1 {
			fail("PL3", "actor %q needs a count of at least 1", name)
		}
	}
	for _, name := range sorted(p.Scenarios) {
		s := p.Scenarios[name]
		if len(s.Stories) == 0 {
			fail("PL4", "scenario %q names no story", name)
		}
		for actor, n := range s.Actors {
			if _, ok := p.Actors[actor]; !ok {
				fail("PL4", "scenario %q uses the actor %q, which the plan does not name", name, actor)
			} else if n < 1 {
				fail("PL4", "scenario %q needs at least 1 of actor %q", name, actor)
			}
		}
		if s.Minutes < 0 {
			fail("PL4", "scenario %q: minutes must be at least 1", name)
		}
	}
	for _, id := range sorted(p.Gaps) {
		g := p.Gaps[id]
		if !gapID.MatchString(id) {
			fail("PL5", "gap id %q must match %s", id, gapID)
		}
		if strings.TrimSpace(g.What) == "" || strings.TrimSpace(g.Why) == "" {
			fail("PL5", "gap %q needs what and why", id)
		}
		if len(g.Touches) == 0 {
			fail("PL5", "gap %q touches nothing; link the stories, rules, or features it leaves untested", id)
		}
	}
	return errors.Join(errs...)
}

// Links returns the ids that a gap touches.
func (g Gap) Links() []string {
	out := make([]string, 0, len(g.Touches))
	for _, l := range g.Touches {
		id := strings.TrimSpace(l)
		if inner, ok := strings.CutPrefix(id, "[["); ok {
			id, _, _ = strings.Cut(strings.TrimSuffix(inner, "]]"), "|")
		}
		out = append(out, strings.TrimSpace(id))
	}
	return out
}

// Context is what the checks of a plan need from outside it.
type Context struct {
	// Lock is the release set the tester pinned.
	Lock Lock
	// Stories are the stories and rules of the tested hub.
	Stories map[string]bool
	// Touchable are the ids a gap may touch: the stories and rules of the
	// hub, and the features of its systems.
	Touchable map[string]bool
	// Key are the key stories of the hub.
	Key []string
	// Covered are the stories and rules that a test of the tester links.
	Covered map[string]bool
}

// Warnings lists what the plan says that does not hold (rule PL6).
func (p Plan) Warnings(c Context) []string {
	var out []string
	pinned := map[string]bool{}
	for _, r := range c.Lock.Releases {
		pinned[r.System+"/"+r.Deliverable] = true
	}
	for _, name := range sorted(p.Components) {
		if r := p.Components[name].Release; r != "" && !pinned[r] {
			out = append(out, fmt.Sprintf("component %q uses %s, which %s does not pin; call pin_releases", name, r, LockFile))
		}
	}
	for _, name := range sorted(p.Scenarios) {
		for _, id := range p.Scenarios[name].Stories {
			if !c.Stories[id] {
				out = append(out, fmt.Sprintf("scenario %q names %q, which is no story or rule of the hub", name, id))
			}
		}
	}
	gapped := map[string]bool{}
	for _, id := range sorted(p.Gaps) {
		for _, t := range p.Gaps[id].Links() {
			gapped[t] = true
			if !c.Touchable[t] {
				out = append(out, fmt.Sprintf("gap %q touches %q, which is no story, rule, or feature of the hub", id, t))
			}
		}
	}
	for _, id := range c.Key {
		if !c.Covered[id] && !gapped[id] {
			out = append(out, fmt.Sprintf("the key story %q has no test and no gap", id))
		}
	}
	return out
}

// GapsTouching returns the ids of the gaps that touch id, sorted.
func (p Plan) GapsTouching(id string) []string {
	var out []string
	for _, g := range sorted(p.Gaps) {
		if slices.Contains(p.Gaps[g].Links(), id) {
			out = append(out, g)
		}
	}
	return out
}

// Lock is releases.lock: the release set that the tester tests (rule LK1).
type Lock struct {
	Version  int             `yaml:"version" json:"version"`
	Releases []LockedRelease `yaml:"releases" json:"releases"`
}

type LockedRelease struct {
	System      string `yaml:"system" json:"system"`
	Deliverable string `yaml:"deliverable" json:"deliverable"`
	Version     string `yaml:"version" json:"version"`
	Digest      string `yaml:"digest" json:"digest"`
}

const lockVersion = 1

// ParseLock reads releases.lock.
func ParseLock(data []byte) (Lock, error) {
	var l Lock
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil && !errors.Is(err, io.EOF) {
		return Lock{}, fmt.Errorf("LK1: %s: %w", LockFile, err)
	}
	if l.Version != lockVersion {
		return Lock{}, fmt.Errorf("LK1: %s: version must be %d; write it with pin_releases", LockFile, lockVersion)
	}
	return l, nil
}

// Render writes the lock with its releases sorted.
func (l Lock) Render() []byte {
	l.Version = lockVersion
	slices.SortFunc(l.Releases, func(a, b LockedRelease) int {
		return strings.Compare(a.System+"/"+a.Deliverable, b.System+"/"+b.Deliverable)
	})
	var b bytes.Buffer
	b.WriteString("# The release set of the E2E tests. Write it with pin_releases.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	enc.Encode(l)
	return b.Bytes()
}

// LoadLock reads the lock at path. A missing lock is an empty lock.
func LoadLock(path string) (Lock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Lock{Version: lockVersion}, nil
	}
	if err != nil {
		return Lock{}, err
	}
	return ParseLock(data)
}

// Approvals are the user's approvals of the gaps of one hub's plan: each
// gap id with the digest of the text that was approved (rule GA2).
type Approvals map[string]string

// LoadApprovals reads the approvals at path. A missing file has none.
func LoadApprovals(path string) (Approvals, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Approvals{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file struct {
		Approved map[string]string `yaml:"approved"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if file.Approved == nil {
		return Approvals{}, nil
	}
	return Approvals(file.Approved), nil
}

// Save writes the approvals to path.
func (a Approvals) Save(path string) error {
	var b bytes.Buffer
	b.WriteString("# The gaps of the E2E plan that the user approved, with a digest of each gap's text.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{"approved": map[string]string(a)}); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// Approved reports whether the gap's current text is approved.
func (a Approvals) Approved(id string, g Gap) bool { return a[id] != "" && a[id] == g.Digest() }

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
