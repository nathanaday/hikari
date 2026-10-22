package sysmap

import (
	"path"
	"slices"
	"strings"
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
// belong to, the warnings of the wiki, and the problems of the map file, the
// wiki, and the private documents.
type OwnerView struct {
	View                `yaml:",inline"`
	Changed             []string       `yaml:"changed,omitempty" json:"changed,omitempty"`
	StaleFeatures       []StaleFeature `yaml:"stale_features,omitempty" json:"stale_features,omitempty"`
	UndescribedFeatures []string       `yaml:"undescribed_features,omitempty" json:"undescribed_features,omitempty"`
	UnmappedPaths       []string       `yaml:"unmapped_paths,omitempty" json:"unmapped_paths,omitempty"`
	Warnings            []string       `yaml:"warnings,omitempty" json:"warnings,omitempty"`
	Error               string         `yaml:"error,omitempty" json:"error,omitempty"`
	PrivateErrors       []string       `yaml:"private_errors,omitempty" json:"private_errors,omitempty"`
}

// NewView computes the public view of m. hash returns the current content
// hash of a feature's files, and states holds the state of each feature.
func NewView(m *Map, priv Private, changed []string, hash func(id string) string, states map[string]string) View {
	m = m.Clone()
	for i := range m.Features {
		f := &m.Features[i]
		f.Health = Health(*f, priv.Tested(f.ID), hash(f.ID))
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

// FeatureStates computes the state of each feature in ids from the paths
// that changed since the described commit. described reports whether the
// feature's document has a described commit, and priv gives each feature's
// code paths and test files.
func FeatureStates(ids []string, described func(id string) bool, priv Private, changed []string) States {
	st := States{ByID: map[string]string{}}
	mapped := map[string]bool{}
	for _, id := range ids {
		var hits []string
		for _, p := range changed {
			if slices.ContainsFunc(priv.Paths(id), func(q string) bool { return under(p, q) }) {
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
