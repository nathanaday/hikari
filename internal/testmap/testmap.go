// Package testmap holds a partition agent's test map (tests.yaml), which links the
// tests that a runner reports to the features of the wiki; the results of
// the last run of each suite (results.yaml); the JUnit XML reports that runs
// write; and the health of each feature, computed from all three.
package testmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// File names inside the private directory of a scope.
const (
	MapFile     = "tests.yaml"
	ResultsFile = "results.yaml"
	ReportsDir  = "reports"
)

const version = 1

// Map is the test map: the suites, the links of each key, and the gaps of
// each feature.
type Map struct {
	Version int                 `yaml:"version" json:"version"`
	Suites  map[string]Suite    `yaml:"suites" json:"suites"`
	Tests   map[string][]string `yaml:"tests" json:"tests"`
	Gaps    map[string][]string `yaml:"gaps,omitempty" json:"gaps,omitempty"`
}

type Suite struct {
	Command     string `yaml:"command" json:"command"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Empty returns a map with no suites and no keys. command, when set, is
// the command of one suite, all.
func Empty(command string) Map {
	m := Map{Version: version, Suites: map[string]Suite{}, Tests: map[string][]string{}}
	if command != "" {
		m.Suites["all"] = Suite{Command: command, Description: "Runs every test of the partition."}
	}
	return m
}

// Parse reads a test map. Empty data is an empty map (rule X1).
func Parse(data []byte) (Map, error) {
	m := Empty("")
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		return Empty(""), fmt.Errorf("X1: %s: %w", MapFile, err)
	}
	if m.Suites == nil {
		m.Suites = map[string]Suite{}
	}
	if m.Tests == nil {
		m.Tests = map[string][]string{}
	}
	return m, nil
}

var (
	validSuite = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	linkForm   = regexp.MustCompile(`^\[\[([^\[\]]*)\]\]$`)
	validID    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
)

// LinkID returns the feature id of a link "[[id]]", or an error that says
// what is wrong with it.
func LinkID(link string) (string, error) {
	m := linkForm.FindStringSubmatch(link)
	switch {
	case m == nil:
		return "", fmt.Errorf("%q is not a link; write \"[[feature-id]]\"", link)
	case strings.ContainsAny(m[1], "|#"):
		return "", fmt.Errorf("%q: a link in %s is only [[feature-id]], with no |text or #heading", link, MapFile)
	case !validID.MatchString(m[1]):
		return "", fmt.Errorf("%q: %q is not a feature id", link, m[1])
	}
	return m[1], nil
}

// Link returns the link to a feature id.
func Link(id string) string { return "[[" + id + "]]" }

// Validate checks the rules X1 to X4. features is the set of feature ids
// of the wiki.
func (m Map) Validate(features map[string]bool) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+MapFile+": "+format, args...))
	}
	if m.Version != version {
		fail("X1", "version must be %d", version)
	}
	for _, name := range sorted(m.Suites) {
		if !validSuite.MatchString(name) {
			fail("X2", "suite name %q must match %s", name, validSuite)
		}
		if strings.TrimSpace(m.Suites[name].Command) == "" {
			fail("X2", "suite %q has no command", name)
		}
	}
	for _, key := range sorted(m.Tests) {
		if strings.TrimSpace(key) == "" {
			fail("X3", "a key under tests is empty")
			continue
		}
		for _, link := range m.Tests[key] {
			id, err := LinkID(link)
			if err != nil {
				fail("X3", "key %q: %v", key, err)
			} else if !features[id] {
				fail("X3", "key %q links %s, which is not a feature of the wiki", key, link)
			}
		}
	}
	for _, id := range sorted(m.Gaps) {
		if !features[id] {
			fail("X4", "gaps: %q is not a feature of the wiki", id)
		}
		for _, g := range m.Gaps[id] {
			if strings.TrimSpace(g) == "" {
				fail("X4", "gaps of %q: a gap is empty", id)
			}
		}
	}
	return errors.Join(errs...)
}

// Links returns the feature ids that a key links. It skips links that are
// not valid, which Validate reports.
func (m Map) Links(key string) []string {
	var out []string
	for _, link := range m.Tests[key] {
		if id, err := LinkID(link); err == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// Kind is the kind of a key (rules X6 to X8).
type Kind int

const (
	IDKey Kind = iota
	FileKey
	DirKey
)

// Disk says what a path relative to the scope names on disk.
type Disk interface {
	IsFile(rel string) bool
	IsDir(rel string) bool
}

// Matcher decides the key of each test. It takes the kind of each key from
// the disk once.
type Matcher struct {
	m     Map
	kinds map[string]Kind
	paths map[string]string
}

// NewMatcher returns a matcher of the keys of m.
func NewMatcher(m Map, disk Disk) *Matcher {
	mt := &Matcher{m: m, kinds: map[string]Kind{}, paths: map[string]string{}}
	for key := range m.Tests {
		p := path.Clean(strings.TrimSpace(key))
		switch {
		case !local(p):
			mt.kinds[key] = IDKey
		case disk.IsDir(p):
			mt.kinds[key], mt.paths[key] = DirKey, p
		case disk.IsFile(p):
			mt.kinds[key], mt.paths[key] = FileKey, p
		default:
			mt.kinds[key] = IDKey
		}
	}
	return mt
}

// Kind returns the kind of a key.
func (mt *Matcher) Kind(key string) Kind { return mt.kinds[key] }

// Key returns the key that decides the links of a test with this id and
// file, and false when no key matches (rule X9). An id key beats a file
// key, and a file key beats a directory key. Among id keys or directory
// keys, the longest wins.
func (mt *Matcher) Key(id, file string) (string, bool) {
	best, bestKind, found := "", IDKey, false
	better := func(key string, kind Kind) bool {
		if !found {
			return true
		}
		if kind != bestKind {
			return rank(kind) > rank(bestKind)
		}
		if len(key) != len(best) {
			return len(key) > len(best)
		}
		return key < best
	}
	for key, kind := range mt.kinds {
		if !mt.matches(key, kind, id, file) {
			continue
		}
		if better(key, kind) {
			best, bestKind, found = key, kind, true
		}
	}
	return best, found
}

func rank(k Kind) int {
	switch k {
	case IDKey:
		return 2
	case FileKey:
		return 1
	}
	return 0
}

// Matches reports whether a key matches a test, whether or not it decides
// the test's links.
func (mt *Matcher) Matches(key, id, file string) bool {
	kind, ok := mt.kinds[key]
	return ok && mt.matches(key, kind, id, file)
}

func (mt *Matcher) matches(key string, kind Kind, id, file string) bool {
	switch kind {
	case FileKey:
		return file != "" && path.Clean(file) == mt.paths[key]
	case DirKey:
		return file != "" && under(path.Clean(file), mt.paths[key])
	}
	return id == key || strings.HasPrefix(id, key+" > ") || strings.HasPrefix(id, key+".")
}

// under reports whether p is inside the directory dir. The directory "."
// holds every relative path.
func under(p, dir string) bool {
	return dir == "." || strings.HasPrefix(p, dir+"/")
}

// local reports whether p is a relative path that stays inside the scope.
func local(p string) bool {
	return p != "" && !path.IsAbs(p) && p != ".." && !strings.HasPrefix(p, "../")
}

func sorted[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
