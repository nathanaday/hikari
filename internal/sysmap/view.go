package sysmap

import "time"

// View is the full view of a map (S16): the map with current health, its
// state, and the paths that changed since its described commit.
type View struct {
	Map     `yaml:",inline"`
	State   string   `yaml:"state" json:"state"`
	Changed []string `yaml:"changed,omitempty" json:"changed,omitempty"`
	// Error describes a hand edit that made the file invalid.
	Error string `yaml:"error,omitempty" json:"error,omitempty"`
}

// NewView computes the full view of m. hash returns the current content hash
// of a feature's paths.
func NewView(m *Map, changed []string, loadErr string, hash func([]string) string) View {
	m = m.Clone()
	for i := range m.Features {
		m.Features[i].Health = Health(m.Features[i], hash(m.Features[i].Paths()))
	}
	normalize(m)
	return View{Map: *m, State: State(m, changed), Changed: changed, Error: loadErr}
}

// normalize replaces nil lists with empty ones, so that JSON clients get
// arrays.
func normalize(m *Map) {
	if m.Features == nil {
		m.Features = []Feature{}
	}
	if m.System.Interfaces.Provides == nil {
		m.System.Interfaces.Provides = []Interface{}
	}
	if m.System.Interfaces.Consumes == nil {
		m.System.Interfaces.Consumes = []Interface{}
	}
}

// Redacted is the orchestrator view of a map (S17). It leaves out the layout
// and every code and test path.
type Redacted struct {
	System   RedactedSystem    `yaml:"system" json:"system"`
	Features []RedactedFeature `yaml:"features" json:"features"`
	State    string            `yaml:"state" json:"state"`
}

type RedactedSystem struct {
	Name             string         `yaml:"name" json:"name"`
	Summary          string         `yaml:"summary,omitempty" json:"summary,omitempty"`
	Responsibilities []string       `yaml:"responsibilities,omitempty" json:"responsibilities,omitempty"`
	Interfaces       Interfaces     `yaml:"interfaces" json:"interfaces"`
	TestCommand      string         `yaml:"test_command,omitempty" json:"test_command,omitempty"`
	DescribedState   DescribedState `yaml:"described_state,omitempty" json:"described_state"`
}

type RedactedFeature struct {
	ID          string        `yaml:"id" json:"id"`
	Name        string        `yaml:"name" json:"name"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	Interfaces  []string      `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	Tests       RedactedTests `yaml:"tests" json:"tests"`
	Health      string        `yaml:"health" json:"health"`
	Findings    []string      `yaml:"findings,omitempty" json:"findings,omitempty"`
	Plans       []string      `yaml:"plans,omitempty" json:"plans,omitempty"`
	Related     []string      `yaml:"related,omitempty" json:"related,omitempty"`
}

type RedactedTests struct {
	Passed int       `yaml:"passed" json:"passed"`
	Failed int       `yaml:"failed" json:"failed"`
	Ran    time.Time `yaml:"ran,omitempty" json:"ran,omitzero"`
}

// Redact returns the orchestrator view of v.
func Redact(v View) Redacted {
	r := Redacted{
		System: RedactedSystem{
			Name:             v.System.Name,
			Summary:          v.System.Summary,
			Responsibilities: v.System.Responsibilities,
			Interfaces:       v.System.Interfaces,
			TestCommand:      v.System.TestCommand,
			DescribedState:   v.System.DescribedState,
		},
		Features: make([]RedactedFeature, len(v.Features)),
		State:    v.State,
	}
	for i, f := range v.Features {
		r.Features[i] = RedactFeature(f)
	}
	return r
}

func RedactFeature(f Feature) RedactedFeature {
	return RedactedFeature{
		ID:          f.ID,
		Name:        f.Name,
		Description: f.Description,
		Interfaces:  f.Interfaces,
		Tests:       RedactedTests{Passed: f.Tests.Passed, Failed: f.Tests.Failed, Ran: f.Tests.Ran},
		Health:      f.Health,
		Findings:    f.Findings,
		Plans:       f.Plans,
		Related:     f.Related,
	}
}
