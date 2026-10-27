// Package featuremap holds the documents of one partition. The map (map.yaml) is
// the public surface of the partition: its purpose, its interfaces, and its
// features with their test health. It holds no paths into the code. The
// private documents (private/code-map.yaml and private/tests.yaml) hold where
// each feature lives in the code and which tests touch it; only the partition agent
// and the user read them.
//
// The partition agent changes the map through the server, which checks each change
// and writes the file. It edits the private documents directly; the server
// reads and checks them.
package featuremap

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Dir is the directory of the private documents inside a scope:
// <scope>/.hikari/private. The map lives in the vault.
const Dir = ".hikari"

const version = 4

type Map struct {
	Version   int       `yaml:"version" json:"version"`
	Partition Partition `yaml:"partition" json:"partition"`
	Features  []Feature `yaml:"features" json:"features"`
}

type Partition struct {
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
	Health string `yaml:"health,omitempty" json:"health"`
	// State says whether the feature's document describes its code:
	// undescribed, stale, or current. The view computes it.
	State   string   `yaml:"state,omitempty" json:"state"`
	Related []string `yaml:"related,omitempty" json:"related,omitempty"`
	// Created is when the feature's document was created (rule Q7).
	Created time.Time `yaml:"created,omitempty" json:"created,omitzero"`
}

// Tests counts the touching tests of a feature (rule Q5): passed and
// failed count current results; pending counts results that are not
// current, and skipped tests. The tests themselves are private.
type Tests struct {
	Passed  int       `yaml:"passed,omitempty" json:"passed"`
	Failed  int       `yaml:"failed,omitempty" json:"failed"`
	Pending int       `yaml:"pending,omitempty" json:"pending"`
	Ran     time.Time `yaml:"ran,omitempty" json:"ran,omitzero"`
}

// New returns the map of a partition that its partition agent has not described yet.
func New(name string) *Map {
	return &Map{Version: version, Partition: Partition{Name: name}}
}

// Clone returns a deep copy of m.
func (m *Map) Clone() *Map {
	c := *m
	c.Partition.Responsibilities = slices.Clone(m.Partition.Responsibilities)
	c.Partition.Interfaces.Provides = slices.Clone(m.Partition.Interfaces.Provides)
	c.Partition.Interfaces.Consumes = slices.Clone(m.Partition.Interfaces.Consumes)
	c.Features = make([]Feature, len(m.Features))
	for i, f := range m.Features {
		f.Interfaces = slices.Clone(f.Interfaces)
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

// PartitionPatch changes the fields of Partition that are not nil.
type PartitionPatch struct {
	Summary          *string      `json:"summary,omitempty"`
	Responsibilities *[]string    `json:"responsibilities,omitempty"`
	Provides         *[]Interface `json:"provides,omitempty"`
	Consumes         *[]Interface `json:"consumes,omitempty"`
}

func (m *Map) ApplyPartition(p PartitionPatch) {
	s := &m.Partition
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

// FeatureEntry is one feature as the feature wiki projects it into the map.
type FeatureEntry struct {
	ID          string
	Name        string
	Description string
	Interfaces  []string
	Related     []string
	Created     time.Time
	Tests       Tests
	Health      string
}

// Project replaces the features with entries, in id order.
func (m *Map) Project(entries []FeatureEntry) {
	out := make([]Feature, 0, len(entries))
	for _, e := range entries {
		out = append(out, Feature{
			ID: e.ID, Name: e.Name, Description: e.Description,
			Interfaces: slices.Clone(e.Interfaces), Related: slices.Clone(e.Related), Created: e.Created, Tests: e.Tests, Health: e.Health,
		})
	}
	slices.SortFunc(out, func(a, b Feature) int { return strings.Compare(a.ID, b.ID) })
	m.Features = out
}

// MarkCurrent records commit as the code that the map describes.
func (m *Map) MarkCurrent(commit string, now time.Time) {
	m.Partition.DescribedState = DescribedState{Commit: commit, Updated: now.UTC()}
}

func (m *Map) index(id string) int {
	return slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == id })
}

var (
	validID   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
	validKind = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Validate checks the rules S1 to S3 and S7 of the spec. The features come
// from the feature wiki, which checks them first; these checks keep the map
// consistent with itself.
func Validate(m *Map) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	if m.Version != version {
		fail("version", "version must be %d", version)
	}

	declared := map[string]bool{}
	for _, group := range [][]Interface{m.Partition.Interfaces.Provides, m.Partition.Interfaces.Consumes} {
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
				fail("S2", "feature %q uses interface %q, which the partition neither provides nor consumes", f.ID, name)
			}
		}
		for _, r := range f.Related {
			if r == f.ID {
				fail("S3", "feature %q lists itself as related", f.ID)
			} else if !ids[r] {
				fail("S3", "feature %q is related to %q, which is not a feature of this partition", f.ID, r)
			}
		}
	}
	return errors.Join(errs...)
}

// Map states.
const (
	Unbuilt = "unbuilt"
	Stale   = "stale"
	Current = "current"
)

// State computes the state of m from the paths that changed since its
// described commit (rules S13 to S15).
func State(m *Map, changed []string) string {
	switch {
	case m.Partition.DescribedState.Commit == "":
		return Unbuilt
	case len(changed) > 0:
		return Stale
	default:
		return Current
	}
}
