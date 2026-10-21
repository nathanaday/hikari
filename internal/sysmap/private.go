package sysmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// The private documents live in <scope>/.hikari/private. The owner edits
// them directly.
const (
	PrivateDir = "private"
	LayoutFile = "layout.yaml"
	TestFile   = "test.yaml"
)

const privateVersion = 1

// Layout says where things are in the code: the main parts of the system and
// the code of each feature.
type Layout struct {
	Version  int                      `yaml:"version" json:"version"`
	Parts    []Part                   `yaml:"parts" json:"parts"`
	Features map[string]FeatureLayout `yaml:"features" json:"features"`
}

type Part struct {
	Path        string `yaml:"path" json:"path"`
	Description string `yaml:"description" json:"description"`
}

type FeatureLayout struct {
	Code  []string `yaml:"code" json:"code"`
	Notes string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// TestPlan says how each feature is tested: the suites that run tests, and
// for each feature what its tests cover, which tests those are, and what
// they leave out.
type TestPlan struct {
	Version  int                     `yaml:"version" json:"version"`
	Suites   map[string]Suite        `yaml:"suites" json:"suites"`
	Features map[string]FeatureTests `yaml:"features" json:"features"`
}

type Suite struct {
	Command     string `yaml:"command" json:"command"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type FeatureTests struct {
	Covers string      `yaml:"covers,omitempty" json:"covers,omitempty"`
	Tests  []TestEntry `yaml:"tests" json:"tests"`
	Gaps   []string    `yaml:"gaps,omitempty" json:"gaps,omitempty"`
}

// TestEntry is one group of tests of a feature: the suite that runs them,
// their files, and optionally a command that runs only them.
type TestEntry struct {
	Suite   string   `yaml:"suite" json:"suite"`
	Files   []string `yaml:"files,omitempty" json:"files,omitempty"`
	Command string   `yaml:"command,omitempty" json:"command,omitempty"`
}

type Private struct {
	Layout Layout   `json:"layout"`
	Tests  TestPlan `json:"tests"`
}

func privatePath(root, file string) string {
	return filepath.Join(root, Dir, PrivateDir, file)
}

// writePrivateSkeletons creates the private documents that do not exist yet.
// testCommand, when set, becomes the first suite of the test plan.
func writePrivateSkeletons(root, testCommand string) error {
	if err := os.MkdirAll(filepath.Join(root, Dir, PrivateDir), 0o755); err != nil {
		return err
	}
	tests := TestPlan{Version: privateVersion, Suites: map[string]Suite{}, Features: map[string]FeatureTests{}}
	if testCommand != "" {
		tests.Suites["all"] = Suite{Command: testCommand, Description: "Runs every test of the system."}
	}
	docs := map[string]any{
		LayoutFile: Layout{Version: privateVersion, Parts: []Part{}, Features: map[string]FeatureLayout{}},
		TestFile:   tests,
	}
	for file, doc := range docs {
		path := privatePath(root, file)
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		data, err := marshalYAML(doc)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// LoadPrivate reads the private documents of the scope root. A missing
// document reads as empty.
func LoadPrivate(root string) (Private, error) {
	p := Private{
		Layout: Layout{Version: privateVersion, Features: map[string]FeatureLayout{}},
		Tests:  TestPlan{Version: privateVersion, Suites: map[string]Suite{}, Features: map[string]FeatureTests{}},
	}
	var errs []error
	for file, doc := range map[string]any{LayoutFile: &p.Layout, TestFile: &p.Tests} {
		data, err := os.ReadFile(privatePath(root, file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			errs = append(errs, err)
			continue
		}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(doc); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, fmt.Errorf("P1: %s/%s: %w", PrivateDir, file, err))
		}
	}
	if p.Layout.Features == nil {
		p.Layout.Features = map[string]FeatureLayout{}
	}
	if p.Tests.Suites == nil {
		p.Tests.Suites = map[string]Suite{}
	}
	if p.Tests.Features == nil {
		p.Tests.Features = map[string]FeatureTests{}
	}
	return p, errors.Join(errs...)
}

// Validate checks the rules P1 to P4 against the features of the map. exists
// reports whether a path relative to the scope exists.
func (p Private) Validate(ids map[string]bool, exists func(rel string) bool) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	if p.Layout.Version != privateVersion {
		fail("P1", "%s: version must be %d", LayoutFile, privateVersion)
	}
	if p.Tests.Version != privateVersion {
		fail("P1", "%s: version must be %d", TestFile, privateVersion)
	}
	checkPath := func(where, path string) {
		if !filepath.IsLocal(filepath.Clean(path)) {
			fail("P3", "%s: path %q must be relative to the scope and stay inside it", where, path)
		} else if !exists(filepath.Clean(path)) {
			fail("P3", "%s: path %q does not exist in the scope", where, path)
		}
	}
	for _, part := range p.Layout.Parts {
		checkPath(LayoutFile+" parts", part.Path)
	}
	for _, id := range sortedKeys(p.Layout.Features) {
		if !ids[id] {
			fail("P2", "%s: %q is not a feature of the map", LayoutFile, id)
		}
		for _, path := range p.Layout.Features[id].Code {
			checkPath(fmt.Sprintf("%s feature %q", LayoutFile, id), path)
		}
	}
	for _, name := range sortedKeys(p.Tests.Suites) {
		if p.Tests.Suites[name].Command == "" {
			fail("P4", "%s: suite %q has no command", TestFile, name)
		}
	}
	for _, id := range sortedKeys(p.Tests.Features) {
		if !ids[id] {
			fail("P2", "%s: %q is not a feature of the map", TestFile, id)
		}
		for _, t := range p.Tests.Features[id].Tests {
			if _, ok := p.Tests.Suites[t.Suite]; !ok {
				fail("P4", "%s feature %q: suite %q is not defined under suites", TestFile, id, t.Suite)
			}
			for _, path := range t.Files {
				checkPath(fmt.Sprintf("%s feature %q", TestFile, id), path)
			}
		}
	}
	return errors.Join(errs...)
}

// Tested reports whether the test plan lists tests for the feature.
func (p Private) Tested(id string) bool {
	return len(p.Tests.Features[id].Tests) > 0
}

// Paths returns the code and test files of a feature, which its content
// hash covers.
func (p Private) Paths(id string) []string {
	paths := slices.Clone(p.Layout.Features[id].Code)
	for _, t := range p.Tests.Features[id].Tests {
		paths = append(paths, t.Files...)
	}
	return paths
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
