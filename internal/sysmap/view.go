package sysmap

import (
	"path"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/testmap"
)

// View is the public view of a map: the map with current health and state.
// It is what other agents see, and it holds no path into the code.
type View struct {
	Map   `yaml:",inline"`
	State string `yaml:"state" json:"state"`
}

// StaleFeature is a feature whose code changed since its document was
// marked current, with the paths that changed.
type StaleFeature struct {
	ID    string   `yaml:"id" json:"id"`
	Paths []string `yaml:"paths" json:"paths"`
}

// OwnerView is what the owner sees of its own system: the public view, the
// paths that changed since the described commit and the features they
// belong to, the failing, pending, and unmapped tests, the warnings of the
// wiki and the test map, and the problems of the map file, the wiki, and the
// private documents.
type OwnerView struct {
	View                `yaml:",inline"`
	Changed             []string               `yaml:"changed,omitempty" json:"changed,omitempty"`
	StaleFeatures       []StaleFeature         `yaml:"stale_features,omitempty" json:"stale_features,omitempty"`
	UndescribedFeatures []string               `yaml:"undescribed_features,omitempty" json:"undescribed_features,omitempty"`
	UnmappedPaths       []string               `yaml:"unmapped_paths,omitempty" json:"unmapped_paths,omitempty"`
	FailingTests        []testmap.FailingTest  `yaml:"failing_tests,omitempty" json:"failing_tests,omitempty"`
	PendingFeatures     []string               `yaml:"pending_features,omitempty" json:"pending_features,omitempty"`
	UnmappedTests       []testmap.UnmappedFile `yaml:"unmapped_tests,omitempty" json:"unmapped_tests,omitempty"`
	StaleKeys           []string               `yaml:"stale_keys,omitempty" json:"stale_keys,omitempty"`
	Warnings            []string               `yaml:"warnings,omitempty" json:"warnings,omitempty"`
	Error               string                 `yaml:"error,omitempty" json:"error,omitempty"`
	PrivateErrors       []string               `yaml:"private_errors,omitempty" json:"private_errors,omitempty"`
}

// NewView computes the public view of m. health holds the health of each
// feature, and states its state.
func NewView(m *Map, changed []string, health map[string]testmap.FeatureHealth, states map[string]string) View {
	m = m.Clone()
	for i := range m.Features {
		f := &m.Features[i]
		f.Tests, f.Health = TestsOf(health[f.ID])
		f.State = states[f.ID]
	}
	if m.Features == nil {
		m.Features = []Feature{}
	}
	if m.System.Interfaces.Provides == nil {
		m.System.Interfaces.Provides = []Interface{}
	}
	if m.System.Interfaces.Consumes == nil {
		m.System.Interfaces.Consumes = []Interface{}
	}
	return View{Map: *m, State: State(m, changed)}
}

// Feature states (rule S20).
const Undescribed = "undescribed"

// States is the state of each feature, with the lists that the owner and
// the driver report (rules S19 to S21).
type States struct {
	ByID        map[string]string
	Stale       []StaleFeature
	Undescribed []string
	Unmapped    []string
}

// TestsOf returns the map's counts and health from a feature's health.
func TestsOf(h testmap.FeatureHealth) (Tests, string) {
	health := h.Health
	if health == "" {
		health = testmap.Untested
	}
	return Tests{Passed: h.Passed, Failed: h.Failed, Pending: h.Pending, Ran: h.Ran}, health
}

// FeatureStates computes the state of each feature in ids from the paths
// that changed since the described commit (rules S19 to S21). described
// reports whether the feature's document has a described commit, paths
// gives each feature's code paths and test files, and testFiles holds the
// test file of every recorded test.
func FeatureStates(ids []string, described func(id string) bool, paths func(id string) []string, testFiles map[string]bool, changed []string) States {
	st := States{ByID: map[string]string{}}
	mapped := map[string]bool{}
	for _, p := range changed {
		if testFiles[path.Clean(p)] {
			mapped[p] = true
		}
	}
	for _, id := range ids {
		var hits []string
		own := paths(id)
		for _, p := range changed {
			if slices.ContainsFunc(own, func(q string) bool { return under(p, q) }) {
				hits = append(hits, p)
				mapped[p] = true
			}
		}
		switch {
		case !described(id):
			st.ByID[id] = Undescribed
			st.Undescribed = append(st.Undescribed, id)
		case len(hits) > 0:
			st.ByID[id] = Stale
			st.Stale = append(st.Stale, StaleFeature{ID: id, Paths: hits})
		default:
			st.ByID[id] = Current
		}
	}
	for _, p := range changed {
		if !mapped[p] {
			st.Unmapped = append(st.Unmapped, p)
		}
	}
	return st
}

// under reports whether p is q or inside the directory q.
func under(p, q string) bool {
	p, q = path.Clean(p), path.Clean(q)
	return p == q || strings.HasPrefix(p, q+"/")
}
