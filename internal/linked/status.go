package linked

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
)

// PartitionStatus is what the files say of one partition (rules SU1 to SU4 of
// the containerize spec).
type PartitionStatus struct {
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

// Status reads each partition agent's map in the vault and its results file. It
// writes nothing and needs no server.
func Status(topo *topology.Topology) []PartitionStatus {
	st := topo.VaultOf()
	var out []PartitionStatus
	for _, partitionAgent := range topo.PartitionAgents() {
		sys := topo.Agents[partitionAgent].Partition
		s := PartitionStatus{Name: partitionAgent, Repo: sys.Repo, Cloned: !slices.Contains(topo.Missing, sys.Repo), Health: map[string]int{}}
		if data, err := os.ReadFile(st.MapFile(partitionAgent)); err == nil {
			m, err := featuremap.Decode(data)
			if err != nil {
				s.Problem = "map.yaml: " + err.Error()
			}
			s.Commit, s.Features = m.Partition.DescribedState.Commit, len(m.Features)
			for _, f := range m.Features {
				s.Health[f.Health]++
			}
		}
		if s.Cloned {
			path := filepath.Join(topo.Repos[sys.Repo].Path, sys.Scope, featuremap.Dir, featuremap.PrivateDir, testmap.ResultsFile)
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
