package testmap

import (
	"slices"
	"time"
)

// Health values (rules S24 to S27).
const (
	Untested = "untested"
	Failing  = "failing"
	Pending  = "pending"
	Passing  = "passing"
)

// FeatureHealth is the health of one feature, with counts over its
// touching tests (rule Q5) and their test files (rule S19).
type FeatureHealth struct {
	Health  string
	Passed  int
	Failed  int
	Pending int
	Ran     time.Time
	Files   []string
}

// FailingTest is a test whose current result failed.
type FailingTest struct {
	ID       string   `yaml:"id" json:"id"`
	Suite    string   `yaml:"suite" json:"suite"`
	Message  string   `yaml:"message,omitempty" json:"message,omitempty"`
	Features []string `yaml:"features" json:"features"`
}

// UnmappedFile lists the unmapped tests of one test file: at most
// MaxUnmappedIDs ids, and the count of the others.
type UnmappedFile struct {
	File  string   `yaml:"file" json:"file"`
	Tests []string `yaml:"tests" json:"tests"`
	More  int      `yaml:"more,omitempty" json:"more,omitempty"`
}

const MaxUnmappedIDs = 20

// Report is the health of every feature and what the partition agent must look at.
type Report struct {
	Features  map[string]FeatureHealth
	Failing   []FailingTest
	Unmapped  []UnmappedFile
	StaleKeys []string
	// Shadowed lists the keys that match recorded tests but decide none,
	// because a more specific key decides each of them.
	Shadowed []string
	// TestFiles holds the test file of every recorded test (rule S21).
	TestFiles map[string]bool
}

// Hashes give the current content hash of a test file and of a feature's
// code. Compute calls each at most once per path or feature.
type Hashes struct {
	File func(rel string) string
	Code func(id string) string
}

// Compute computes the health of features from the test map, the matcher
// of its keys, and the results (rules S24 to S30).
func Compute(m Map, mt *Matcher, r Results, features []string, h Hashes) Report {
	fileHash, codeHash := memo(h.File), memo(h.Code)
	known := map[string]bool{}
	health := map[string]*FeatureHealth{}
	for _, id := range features {
		known[id] = true
		health[id] = &FeatureHealth{}
	}
	rep := Report{Features: map[string]FeatureHealth{}, TestFiles: map[string]bool{}}
	used, matched := map[string]bool{}, map[string]bool{}
	unmapped := map[string][]string{}
	for _, suite := range sorted(r.Runs) {
		run := r.Runs[suite]
		for _, t := range run.Tests {
			if t.File != "" {
				rep.TestFiles[t.File] = true
			}
			for key := range m.Tests {
				if !matched[key] && mt.Matches(key, t.ID, t.File) {
					matched[key] = true
				}
			}
			key, ok := mt.Key(t.ID, t.File)
			if !ok {
				unmapped[t.File] = append(unmapped[t.File], t.ID)
				continue
			}
			used[key] = true
			links := m.Links(key)
			current := true
			if t.File != "" {
				got, ok := run.Files[t.File]
				current = ok && got == fileHash(t.File)
			}
			for _, id := range links {
				if got, ok := run.Code[id]; !ok || !known[id] || got != codeHash(id) {
					current = false
				}
			}
			for _, id := range links {
				f := health[id]
				if f == nil {
					continue
				}
				if t.File != "" && !slices.Contains(f.Files, t.File) {
					f.Files = append(f.Files, t.File)
				}
				if run.Ran.After(f.Ran) {
					f.Ran = run.Ran
				}
				switch {
				case !current || t.Outcome == Skipped:
					f.Pending++
				case t.Outcome == Failed:
					f.Failed++
				default:
					f.Passed++
				}
			}
			if current && t.Outcome == Failed && len(links) > 0 {
				rep.Failing = append(rep.Failing, FailingTest{ID: t.ID, Suite: suite, Message: t.Message, Features: links})
			}
		}
	}
	staleLinked := map[string]bool{}
	for _, key := range sorted(m.Tests) {
		switch {
		case !matched[key]:
			rep.StaleKeys = append(rep.StaleKeys, key)
			for _, id := range m.Links(key) {
				staleLinked[id] = true
			}
		case !used[key]:
			rep.Shadowed = append(rep.Shadowed, key)
		}
	}
	for _, id := range features {
		f := health[id]
		touched := f.Passed+f.Failed+f.Pending > 0
		switch {
		case f.Failed > 0:
			f.Health = Failing
		case f.Pending > 0 || staleLinked[id]:
			f.Health = Pending
		case !touched:
			f.Health = Untested
		default:
			f.Health = Passing
		}
		slices.Sort(f.Files)
		rep.Features[id] = *f
	}
	for _, file := range sorted(unmapped) {
		ids := unmapped[file]
		u := UnmappedFile{File: file, Tests: ids}
		if len(ids) > MaxUnmappedIDs {
			u.Tests, u.More = ids[:MaxUnmappedIDs], len(ids)-MaxUnmappedIDs
		}
		if file == "" {
			u.File = "(no file)"
		}
		rep.Unmapped = append(rep.Unmapped, u)
	}
	return rep
}

// Pending returns the ids of the pending features, sorted.
func (r Report) Pending() []string {
	var out []string
	for _, id := range sorted(r.Features) {
		if r.Features[id].Health == Pending {
			out = append(out, id)
		}
	}
	return out
}

func memo(fn func(string) string) func(string) string {
	seen := map[string]string{}
	return func(k string) string {
		if v, ok := seen[k]; ok {
			return v
		}
		v := fn(k)
		seen[k] = v
		return v
	}
}
