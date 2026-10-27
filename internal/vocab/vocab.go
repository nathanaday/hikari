// Package vocab holds the project's vocabulary: the shared names of domain
// terms and of the interfaces between systems. The orchestrator is its only
// author.
package vocab

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const version = 1

type Vocabulary struct {
	Version    int         `yaml:"version" json:"version"`
	Terms      []Term      `yaml:"terms" json:"terms"`
	Interfaces []Interface `yaml:"interfaces" json:"interfaces"`
}

type Term struct {
	Term       string   `yaml:"term" json:"term"`
	Definition string   `yaml:"definition" json:"definition"`
	Avoid      []string `yaml:"avoid,omitempty" json:"avoid,omitempty"`
}

type Interface struct {
	Name        string `yaml:"name" json:"name"`
	Kind        string `yaml:"kind" json:"kind"`
	Description string `yaml:"description" json:"description"`
}

// Names returns the interface names of v.
func (v Vocabulary) Names() []string {
	out := make([]string, len(v.Interfaces))
	for i, in := range v.Interfaces {
		out[i] = in.Name
	}
	return out
}

const header = `# The vocabulary of the project. hikari writes this file for the
# orchestrator, who changes it through the vocabulary tools.
`

var validKind = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Store holds the vocabulary. The file is created at the first
// change.
type Store struct {
	path string

	mu      sync.Mutex
	v       Vocabulary
	version int
}

// Open loads the vocabulary from path, or starts an empty one if the file
// does not exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, v: Vocabulary{Version: version}, version: 1}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	} else if err != nil {
		return nil, err
	}
	var v Vocabulary
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("vocabulary %s: %w", path, err)
	}
	s.v = v
	return s, nil
}

// Get returns a copy of the vocabulary and its version. The version grows
// with each change.
func (s *Store) Get() (Vocabulary, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.v
	v.Terms = slices.Clone(v.Terms)
	v.Interfaces = slices.Clone(v.Interfaces)
	if v.Terms == nil {
		v.Terms = []Term{}
	}
	if v.Interfaces == nil {
		v.Interfaces = []Interface{}
	}
	return v, s.version
}

// DefineTerm adds a term, or replaces the term with the same name.
func (s *Store) DefineTerm(t Term) error {
	t.Term = strings.TrimSpace(t.Term)
	if t.Term == "" || strings.TrimSpace(t.Definition) == "" {
		return errors.New("a term needs a name and a definition")
	}
	return s.change(func(v *Vocabulary) error {
		if i := slices.IndexFunc(v.Terms, func(x Term) bool { return x.Term == t.Term }); i >= 0 {
			v.Terms[i] = t
		} else {
			v.Terms = append(v.Terms, t)
		}
		return nil
	})
}

// DefineInterface adds an interface, or replaces the interface with the same
// name.
func (s *Store) DefineInterface(in Interface) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || strings.TrimSpace(in.Description) == "" {
		return errors.New("an interface needs a name and a description")
	}
	if !validKind.MatchString(in.Kind) {
		return fmt.Errorf("interface %q: kind %q must be one lowercase word", in.Name, in.Kind)
	}
	return s.change(func(v *Vocabulary) error {
		if i := slices.IndexFunc(v.Interfaces, func(x Interface) bool { return x.Name == in.Name }); i >= 0 {
			v.Interfaces[i] = in
		} else {
			v.Interfaces = append(v.Interfaces, in)
		}
		return nil
	})
}

// Remove removes the term or the interface called name.
func (s *Store) Remove(name string) error {
	return s.change(func(v *Vocabulary) error {
		n := len(v.Terms) + len(v.Interfaces)
		v.Terms = slices.DeleteFunc(v.Terms, func(t Term) bool { return t.Term == name })
		v.Interfaces = slices.DeleteFunc(v.Interfaces, func(in Interface) bool { return in.Name == name })
		if len(v.Terms)+len(v.Interfaces) == n {
			return fmt.Errorf("the vocabulary has no term or interface %q", name)
		}
		return nil
	})
}

func (s *Store) change(fn func(*Vocabulary) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.v
	next.Terms = slices.Clone(s.v.Terms)
	next.Interfaces = slices.Clone(s.v.Interfaces)
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.v = next
	s.version++
	return nil
}

func (s *Store) save(v Vocabulary) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	data := buf.Bytes()
	data = append([]byte(header), data...)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".vocabulary-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Flag is a problem with an interface of a system, as the orchestrator sees
// it.
type Flag struct {
	Interface string `yaml:"interface" json:"interface"`
	Problem   string `yaml:"problem" json:"problem"`
}

const (
	NotInVocabulary = "not in the vocabulary"
	NoProvider      = "consumed, but no visible system provides it"
)

// Check returns the flags V6 and V7 for a system that provides and consumes
// the given interfaces. known holds the names in the orchestrator's
// vocabularies, and provided the names that visible systems provide.
func Check(provides, consumes []string, known, provided map[string]bool) []Flag {
	var flags []Flag
	seen := map[string]bool{}
	for _, name := range append(slices.Clone(provides), consumes...) {
		if !known[name] && !seen[name] {
			flags = append(flags, Flag{Interface: name, Problem: NotInVocabulary})
			seen[name] = true
		}
	}
	for _, name := range consumes {
		if !provided[name] {
			flags = append(flags, Flag{Interface: name, Problem: NoProvider})
		}
	}
	return flags
}
