package testmap

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Results holds the last run of each suite. Only the server writes it.
type Results struct {
	Version int            `yaml:"version" json:"version"`
	Runs    map[string]Run `yaml:"runs" json:"runs"`
}

// Run is one recorded run of a suite, with the hashes that its results
// depend on (rule J7).
type Run struct {
	Ran time.Time `yaml:"ran" json:"ran"`
	// Code is the content hash of each feature's code at the run.
	Code map[string]string `yaml:"code" json:"code"`
	// Files is the content hash of each test file at the run.
	Files map[string]string `yaml:"files" json:"files"`
	Tests []Result          `yaml:"tests" json:"tests"`
}

// Result is the outcome of one test in the last run of its suite.
type Result struct {
	ID      string `yaml:"id" json:"id"`
	File    string `yaml:"file,omitempty" json:"file,omitempty"`
	Outcome string `yaml:"outcome" json:"outcome"`
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// NewRun records cases as a run at now. code is the hash of each feature's
// code, and hashFile returns the hash of a test file.
func NewRun(cases []Case, now time.Time, code map[string]string, hashFile func(rel string) string) Run {
	r := Run{Ran: now.UTC(), Code: code, Files: map[string]string{}, Tests: make([]Result, len(cases))}
	for i, c := range cases {
		r.Tests[i] = Result{ID: c.ID, File: c.File, Outcome: c.Outcome, Message: c.Message}
		if c.File != "" {
			if _, ok := r.Files[c.File]; !ok {
				r.Files[c.File] = hashFile(c.File)
			}
		}
	}
	return r
}

// LoadResults reads the results file. A missing file is no results. An
// invalid file is no results and an error, which the owner view shows
// (rule J8).
func LoadResults(path string) (Results, error) {
	empty := Results{Version: version, Runs: map[string]Run{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	} else if err != nil {
		return empty, err
	}
	var r Results
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil || r.Version != version {
		if err == nil {
			err = fmt.Errorf("version must be %d", version)
		}
		return empty, fmt.Errorf("%s is not valid, so hikari starts with no results; run the suites again: %w", ResultsFile, err)
	}
	if r.Runs == nil {
		r.Runs = map[string]Run{}
	}
	return r, nil
}

// SaveResults writes r with an atomic rename.
func SaveResults(path string, r Results) error {
	r.Version = version
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(r); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".results-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
