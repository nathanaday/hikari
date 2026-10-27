package featuremap

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

	"github.com/nathanaday/hikari/internal/testmap"
	"gopkg.in/yaml.v3"
)

// The private documents live in <scope>/.hikari/private. The partition agent edits
// layout.yaml and tests.yaml directly. The server writes results.yaml, and
// runs write their reports into reports/.
const (
	PrivateDir  = "private"
	CodeMapFile = "layout.yaml"
	// OldTestFile is the test plan of earlier versions, which the server
	// deletes (rule O8).
	OldTestFile = "test.yaml"
)

// gitignore keeps the results and the reports out of commits (rule L13).
const gitignore = testmap.ReportsDir + "/\n" + testmap.ResultsFile + "\n"

const privateVersion = 1

// CodeMap says where things are in the code: the main parts of the partition and
// the code of each feature.
type CodeMap struct {
	Version  int                    `yaml:"version" json:"version"`
	Parts    []Part                 `yaml:"parts" json:"parts"`
	Features map[string]FeatureCode `yaml:"features" json:"features"`
}

type Part struct {
	Path        string `yaml:"path" json:"path"`
	Description string `yaml:"description" json:"description"`
}

type FeatureCode struct {
	Code  []string `yaml:"code" json:"code"`
	Notes string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Private is the partition agent's code map and test map.
type Private struct {
	CodeMap CodeMap     `json:"code_map"`
	Tests   testmap.Map `json:"tests"`
}

func privatePath(root, file string) string {
	return filepath.Join(root, Dir, PrivateDir, file)
}

// preparePrivate creates the private documents that do not exist yet, the
// reports directory, and the ignore file, and deletes an old test plan.
// testCommand, when set, becomes the suite all of a new test map. It
// returns a note when it deleted the old test plan.
func preparePrivate(root, testCommand string) (string, error) {
	if err := os.MkdirAll(privatePath(root, testmap.ReportsDir), 0o755); err != nil {
		return "", err
	}
	ignore := privatePath(root, ".gitignore")
	if data, err := os.ReadFile(ignore); err != nil || string(data) != gitignore {
		if err := os.WriteFile(ignore, []byte(gitignore), 0o644); err != nil {
			return "", err
		}
	}
	docs := map[string]any{
		CodeMapFile:     CodeMap{Version: privateVersion, Parts: []Part{}, Features: map[string]FeatureCode{}},
		testmap.MapFile: testmap.Empty(testCommand),
	}
	for file, doc := range docs {
		path := privatePath(root, file)
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		data, err := marshalYAML(doc)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
	}
	old := privatePath(root, OldTestFile)
	if _, err := os.Stat(old); err != nil {
		return "", nil
	}
	if err := os.Remove(old); err != nil {
		return "", err
	}
	return fmt.Sprintf("deleted %s: the test plan of an earlier hikari; the partition agent writes %s again", old, testmap.MapFile), nil
}

// LoadPrivate reads the private documents of the scope root. A missing
// document reads as empty.
func LoadPrivate(root string) (Private, error) {
	p := Private{
		CodeMap: CodeMap{Version: privateVersion, Features: map[string]FeatureCode{}},
		Tests:   testmap.Empty(""),
	}
	var errs []error
	if data, err := readPrivate(root, CodeMapFile); err != nil {
		errs = append(errs, err)
	} else if data != nil {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&p.CodeMap); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, fmt.Errorf("P1: %s/%s: %w", PrivateDir, CodeMapFile, err))
		}
	}
	if data, err := readPrivate(root, testmap.MapFile); err != nil {
		errs = append(errs, err)
	} else if data != nil {
		tm, err := testmap.Parse(data)
		if err != nil {
			errs = append(errs, err)
		}
		p.Tests = tm
	}
	if p.CodeMap.Features == nil {
		p.CodeMap.Features = map[string]FeatureCode{}
	}
	return p, errors.Join(errs...)
}

func readPrivate(root, file string) ([]byte, error) {
	data, err := os.ReadFile(privatePath(root, file))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Validate checks the rules P1 to P3 of the code map and X1 to X4 of the
// test map against the features of the map. exists reports whether a path
// relative to the scope exists.
func (p Private) Validate(ids map[string]bool, exists func(rel string) bool) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	if p.CodeMap.Version != privateVersion {
		fail("P1", "%s: version must be %d", CodeMapFile, privateVersion)
	}
	checkPath := func(where, path string) {
		if !filepath.IsLocal(filepath.Clean(path)) {
			fail("P3", "%s: path %q must be relative to the scope and stay inside it", where, path)
		} else if !exists(filepath.Clean(path)) {
			fail("P3", "%s: path %q does not exist in the scope", where, path)
		}
	}
	for _, part := range p.CodeMap.Parts {
		checkPath(CodeMapFile+" parts", part.Path)
	}
	for _, id := range sortedKeys(p.CodeMap.Features) {
		if !ids[id] {
			fail("P2", "%s: %q is not a feature of the map", CodeMapFile, id)
		}
		for _, path := range p.CodeMap.Features[id].Code {
			checkPath(fmt.Sprintf("%s feature %q", CodeMapFile, id), path)
		}
	}
	return errors.Join(append(errs, p.Tests.Validate(ids))...)
}

// Code returns the code paths of a feature.
func (p Private) Code(id string) []string {
	return slices.Clone(p.CodeMap.Features[id].Code)
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
