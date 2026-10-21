// Package sysmap holds the documents of one system. The map (map.yaml) is
// the public surface of the system: its purpose, its interfaces, and its
// features with their test health. It holds no paths into the code. The
// private documents (private/layout.yaml and private/test.yaml) hold where
// each feature lives in the code and how it is tested; only the owner and
// the user read them.
//
// The owner is the only author of all three. It changes the map through the
// server, which checks each change and writes the file. It edits the private
// documents directly; the server reads and checks them.
package sysmap

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// Dir is the directory of the private documents inside a scope:
// <scope>/.hikari/private. The map lives in the store.
const Dir = ".hikari"

const version = 2

type Map struct {
	Version  int       `yaml:"version" json:"version"`
	System   System    `yaml:"system" json:"system"`
	Features []Feature `yaml:"features" json:"features"`
}

type System struct {
	Name             string         `yaml:"name" json:"name"`
	Summary          string         `yaml:"summary,omitempty" json:"summary,omitempty"`
	Responsibilities []string       `yaml:"responsibilities,omitempty" json:"responsibilities,omitempty"`
	Interfaces       Interfaces     `yaml:"interfaces,omitempty" json:"interfaces"`
	DescribedState   DescribedState `yaml:"described_state,omitempty" json:"described_state"`
}

type Interfaces struct {
	Provides []Interface `yaml:"provides,omitempty" json:"provides"`
	Consumes []Interface `yaml:"consumes,omitempty" json:"consumes"`
}

type Interface struct {
	Name        string `yaml:"name" json:"name"`
	Kind        string `yaml:"kind" json:"kind"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// DescribedState is the commit whose code the map describes.
type DescribedState struct {
	Commit  string    `yaml:"commit,omitempty" json:"commit,omitempty"`
	Updated time.Time `yaml:"updated,omitempty" json:"updated,omitzero"`
}

type Feature struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Interfaces  []string `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	Tests       Tests    `yaml:"tests,omitempty" json:"tests"`
	// Health is computed by the server. The value in the file is the health
	// at the last write.
	Health   string   `yaml:"health,omitempty" json:"health"`
	Findings []string `yaml:"findings,omitempty" json:"findings,omitempty"`
	Plans    []string `yaml:"plans,omitempty" json:"plans,omitempty"`
	Related  []string `yaml:"related,omitempty" json:"related,omitempty"`
}

// Tests holds the last recorded results of a feature's tests. The test plan
// itself is private.
type Tests struct {
	Passed int       `yaml:"passed,omitempty" json:"passed"`
	Failed int       `yaml:"failed,omitempty" json:"failed"`
	Ran    time.Time `yaml:"ran,omitempty" json:"ran,omitzero"`
	// ContentHash is the hash of the feature's code and test files when the
	// results were recorded.
	ContentHash string `yaml:"content_hash,omitempty" json:"content_hash,omitempty"`
}

// New returns the map of a system that its owner has not described yet.
func New(name string) *Map {
	return &Map{Version: version, System: System{Name: name}}
}

// Clone returns a deep copy of m.
func (m *Map) Clone() *Map {
	c := *m
	c.System.Responsibilities = slices.Clone(m.System.Responsibilities)
	c.System.Interfaces.Provides = slices.Clone(m.System.Interfaces.Provides)
	c.System.Interfaces.Consumes = slices.Clone(m.System.Interfaces.Consumes)
	c.Features = make([]Feature, len(m.Features))
	for i, f := range m.Features {
		f.Interfaces = slices.Clone(f.Interfaces)
		f.Findings = slices.Clone(f.Findings)
		f.Plans = slices.Clone(f.Plans)
		f.Related = slices.Clone(f.Related)
		c.Features[i] = f
	}
	return &c
}

// IDs returns the set of feature ids.
func (m *Map) IDs() map[string]bool {
	out := map[string]bool{}
	for _, f := range m.Features {
		out[f.ID] = true
	}
	return out
}

// SystemPatch changes the fields of System that are not nil.
type SystemPatch struct {
	Summary          *string      `json:"summary,omitempty"`
	Responsibilities *[]string    `json:"responsibilities,omitempty"`
	Provides         *[]Interface `json:"provides,omitempty"`
	Consumes         *[]Interface `json:"consumes,omitempty"`
}

func (m *Map) ApplySystem(p SystemPatch) {
	s := &m.System
	if p.Summary != nil {
		s.Summary = *p.Summary
	}
	if p.Responsibilities != nil {
		s.Responsibilities = *p.Responsibilities
	}
	if p.Provides != nil {
		s.Interfaces.Provides = *p.Provides
	}
	if p.Consumes != nil {
		s.Interfaces.Consumes = *p.Consumes
	}
}

// FeatureInput is a feature as its owner describes it. Test results are
// recorded separately.
type FeatureInput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Interfaces  []string `json:"interfaces,omitempty"`
	Findings    []string `json:"findings,omitempty"`
	Plans       []string `json:"plans,omitempty"`
	Related     []string `json:"related,omitempty"`
}

// Upsert adds the feature, or replaces the one with the same ID and keeps its
// test results.
func (m *Map) Upsert(in FeatureInput) {
	f := Feature{
		ID: in.ID, Name: in.Name, Description: in.Description, Interfaces: in.Interfaces,
		Findings: in.Findings, Plans: in.Plans, Related: in.Related,
	}
	if i := m.index(in.ID); i >= 0 {
		f.Tests = m.Features[i].Tests
		m.Features[i] = f
		return
	}
	m.Features = append(m.Features, f)
}

// Remove removes a feature and the references to it.
func (m *Map) Remove(id string) error {
	i := m.index(id)
	if i < 0 {
		return fmt.Errorf("no feature %q", id)
	}
	m.Features = slices.Delete(m.Features, i, i+1)
	for j := range m.Features {
		m.Features[j].Related = slices.DeleteFunc(m.Features[j].Related, func(r string) bool { return r == id })
	}
	return nil
}

type TestResult struct {
	Feature string `json:"feature"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
}

// RecordTests stores results with the time and the content hash of each
// feature's files. hash returns that hash for a feature id.
func (m *Map) RecordTests(results []TestResult, now time.Time, hash func(id string) (string, error)) error {
	for _, r := range results {
		i := m.index(r.Feature)
		if i < 0 {
			return fmt.Errorf("no feature %q", r.Feature)
		}
		if r.Passed < 0 || r.Failed < 0 {
			return fmt.Errorf("feature %q: test counts cannot be negative", r.Feature)
		}
		h, err := hash(r.Feature)
		if err != nil {
			return err
		}
		f := &m.Features[i]
		f.Tests = Tests{Passed: r.Passed, Failed: r.Failed, Ran: now.UTC(), ContentHash: h}
	}
	return nil
}

// MarkCurrent records commit as the code that the map describes.
func (m *Map) MarkCurrent(commit string, now time.Time) {
	m.System.DescribedState = DescribedState{Commit: commit, Updated: now.UTC()}
}

func (m *Map) index(id string) int {
	return slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == id })
}

var (
	validID   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
	validKind = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Validate checks the rules S1 to S3 and S7 of the spec.
func Validate(m *Map) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	if m.Version != version {
		fail("version", "version must be %d", version)
	}

	declared := map[string]bool{}
	for _, group := range [][]Interface{m.System.Interfaces.Provides, m.System.Interfaces.Consumes} {
		for _, in := range group {
			if in.Name == "" {
				fail("S7", "an interface has no name")
			}
			if !validKind.MatchString(in.Kind) {
				fail("S7", "interface %q: kind %q must be one lowercase word", in.Name, in.Kind)
			}
			declared[in.Name] = true
		}
	}

	ids := map[string]bool{}
	for _, f := range m.Features {
		if !validID.MatchString(f.ID) {
			fail("S1", "feature id %q must match %s", f.ID, validID)
		}
		if ids[f.ID] {
			fail("S1", "feature id %q is used more than once", f.ID)
		}
		ids[f.ID] = true
	}
	for _, f := range m.Features {
		if f.Name == "" {
			fail("S1", "feature %q has no name", f.ID)
		}
		for _, name := range f.Interfaces {
			if !declared[name] {
				fail("S2", "feature %q uses interface %q, which the system neither provides nor consumes", f.ID, name)
			}
		}
		for _, r := range f.Related {
			if r == f.ID {
				fail("S3", "feature %q lists itself as related", f.ID)
			} else if !ids[r] {
				fail("S3", "feature %q is related to %q, which is not a feature of this system", f.ID, r)
			}
		}
	}
	return errors.Join(errs...)
}

// Health values.
const (
	Untested = "untested"
	Stale    = "stale"
	Failing  = "failing"
	Passing  = "passing"
)

// Health computes the health of f (rules S8 to S12). tested reports whether
// the test plan lists tests for the feature, and currentHash is the current
// hash of its code and test files.
func Health(f Feature, tested bool, currentHash string) string {
	switch {
	case !tested || f.Tests.Ran.IsZero():
		return Untested
	case f.Tests.ContentHash != currentHash:
		return Stale
	case f.Tests.Failed > 0:
		return Failing
	default:
		return Passing
	}
}

// Map states.
const (
	Unbuilt = "unbuilt"
	Current = "current"
)

// State computes the state of m from the paths that changed since its
// described commit (rules S13 to S15).
func State(m *Map, changed []string) string {
	switch {
	case m.System.DescribedState.Commit == "":
		return Unbuilt
	case len(changed) > 0:
		return Stale
	default:
		return Current
	}
}
