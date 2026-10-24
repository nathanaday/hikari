package linked

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
)

// SystemStatus is what the files say of one system (rules SU1 to SU4 of
// the containerize spec).
type SystemStatus struct {
	Name, Repo string
	Cloned     bool
	// Commit is the commit that the map describes, or "".
	Commit   string
	Features int
	// Health counts the features by recorded health.
	Health map[string]int
	// Results says whether this machine has a results file.
	Results bool
	Suites  []SuiteStatus
	Problem string
}

// SuiteStatus is the last recorded run of one suite.
type SuiteStatus struct {
	Name   string
	Ran    time.Time
	Counts map[string]int
	Tests  []testmap.Result
}

// Status reads each owner's map in the store and its results file. It
// writes nothing and needs no server.
func Status(topo *topology.Topology) []SystemStatus {
	st := store.Store{Root: topo.Store}
	var out []SystemStatus
	for _, owner := range topo.Owners() {
		sys := topo.Agents[owner].System
		s := SystemStatus{Name: owner, Repo: sys.Repo, Cloned: !slices.Contains(topo.Missing, sys.Repo), Health: map[string]int{}}
		if data, err := os.ReadFile(st.MapFile(owner)); err == nil {
			var m sysmap.Map
			if err := yaml.Unmarshal(data, &m); err != nil {
				s.Problem = "map.yaml: " + err.Error()
			}
			s.Commit, s.Features = m.System.DescribedState.Commit, len(m.Features)
			for _, f := range m.Features {
				s.Health[f.Health]++
			}
		}
		if s.Cloned {
			path := filepath.Join(topo.Repos[sys.Repo].Path, sys.Scope, sysmap.Dir, sysmap.PrivateDir, testmap.ResultsFile)
			if _, err := os.Stat(path); err == nil {
				s.Results = true
				results, err := testmap.LoadResults(path)
				if err != nil {
					s.Problem = err.Error()
				}
				for _, name := range slices.Sorted(maps.Keys(results.Runs)) {
					run := results.Runs[name]
					su := SuiteStatus{Name: name, Ran: run.Ran, Counts: map[string]int{}, Tests: slices.Clone(run.Tests)}
					for _, t := range run.Tests {
						su.Counts[t.Outcome]++
					}
					slices.SortFunc(su.Tests, func(a, b testmap.Result) int { return strings.Compare(a.ID, b.ID) })
					s.Suites = append(s.Suites, su)
				}
			}
		}
		out = append(out, s)
	}
	return out
}
