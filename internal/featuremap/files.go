package featuremap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/workspace"
	"gopkg.in/yaml.v3"
)

// Files holds the map of one partition and keeps its file up to date.
type Files struct {
	ws   workspace.Workspace
	root string // the first writable folder on the host
	path string // the map file on the host
	name string

	mu          sync.Mutex
	m           *Map
	modTime     time.Time
	size        int64
	loadErr     string
	privateSeen string
	notes       []string
}

// Open loads the map of the partition name from mapPath. ws is the
// partition's workspace; its first writable folder holds the private
// documents. Open writes an unbuilt map and empty private documents for
// those that do not exist, and reports whether it created the map.
// testCommand, when set, becomes the suite all of a new test map.
func Open(ws workspace.Workspace, mapPath, name, testCommand string) (*Files, bool, error) {
	root := ws.Home().Host
	s := &Files{ws: ws, root: root, path: mapPath, name: name}
	note, err := preparePrivate(root, testCommand)
	if err != nil {
		return nil, false, err
	}
	if note != "" {
		s.notes = append(s.notes, note)
	}
	s.privateSeen = s.privateStamp()
	if _, err := os.Stat(s.path); errors.Is(err, fs.ErrNotExist) {
		s.m = New(name)
		return s, true, s.save(s.m)
	}
	if v, err := fileVersion(s.path); err == nil && v == version-1 {
		if err := s.convert(); err != nil {
			return nil, false, fmt.Errorf("feature map %s: %w", s.path, err)
		}
	} else if err == nil && v != version {
		// A map from an earlier hikari is not converted (rule Q4).
		if err := os.Remove(s.path); err != nil {
			return nil, false, err
		}
		s.notes = append(s.notes, fmt.Sprintf("deleted %s: it is a version %d map, and this hikari writes version %d; the partition agent describes the partition again", s.path, v, version))
		s.m = New(name)
		return s, true, s.save(s.m)
	}
	if err := s.adoptName(); err != nil {
		return nil, false, fmt.Errorf("feature map %s: %w", s.path, err)
	}
	m, err := s.read()
	if err != nil {
		return nil, false, fmt.Errorf("feature map %s: %w", s.path, err)
	}
	s.m = m
	return s, false, nil
}

// adoptName writes the partition's name into a map that holds another one.
// The map's place in the vault, which is keyed by the partition's id, says
// which partition it describes, so a different name is a rename (rule ID12
// of the ids spec). A map that an earlier hikari wrote under the key system
// is rewritten under partition.
func (s *Files) adoptName() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	m, old, err := decodeLoose(data, s.ws.Home().Repo)
	if err != nil || (!old && m.Partition.Name == s.name) {
		return nil
	}
	if old {
		s.notes = append(s.notes, fmt.Sprintf("renamed the key system to partition in %s", s.path))
	}
	if m.Partition.Name != s.name {
		s.notes = append(s.notes, fmt.Sprintf("renamed the partition %q to %q in %s", m.Partition.Name, s.name, s.path))
		m.Partition.Name = s.name
	}
	return s.save(&m)
}

// convert rewrites a map of the version before, whose one described
// commit is in the repo of the first writable folder (rule F8).
func (s *Files) convert() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	m, _, err := decodeLoose(data, s.ws.Home().Repo)
	if err != nil {
		return err
	}
	m.Version = version
	s.notes = append(s.notes, fmt.Sprintf("converted %s to version %d: it records one described commit for each repo", s.path, version))
	return s.save(&m)
}

// Decode reads a map without checking it, as a teammate's branch or an
// earlier hikari may have written it. A described commit of the version
// before belongs to repo.
func Decode(data []byte, repo string) (Map, error) {
	m, _, err := decodeLoose(data, repo)
	return m, err
}

// decodeLoose reads a map and reports whether it holds the old key system.
// The one described commit of the version before becomes an entry of repo.
func decodeLoose(data []byte, repo string) (Map, bool, error) {
	var v struct {
		Map    `yaml:",inline"`
		System *Partition `yaml:"system"`
	}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return Map{}, false, err
	}
	if v.System != nil {
		v.Map.Partition = *v.System
	}
	var old struct {
		Partition struct {
			DescribedState struct {
				Commit string `yaml:"commit"`
			} `yaml:"described_state"`
		} `yaml:"partition"`
		System struct {
			DescribedState struct {
				Commit string `yaml:"commit"`
			} `yaml:"described_state"`
		} `yaml:"system"`
	}
	yaml.Unmarshal(data, &old)
	commit := old.Partition.DescribedState.Commit
	if v.System != nil {
		commit = old.System.DescribedState.Commit
	}
	if commit != "" && len(v.Map.Partition.DescribedState.Commits) == 0 {
		v.Map.Partition.DescribedState.Commits = []string{workspace.Commit(repo, commit)}
	}
	return v.Map, v.System != nil, nil
}

// Notes returns what Open did to files from an earlier version.
func (s *Files) Notes() []string { return s.notes }

// fileVersion reads only the version of a map file.
func fileVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var v struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return 0, err
	}
	return v.Version, nil
}

// Root returns the first writable folder on the host, which holds the
// private documents.
func (s *Files) Root() string { return s.root }

// Workspace returns the partition's workspace.
func (s *Files) Workspace() workspace.Workspace { return s.ws }

// Path returns the map file on the host.
func (s *Files) Path() string { return s.path }

// Map returns a copy of the current map.
func (s *Files) Map() *Map {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.Clone()
}

// Error returns the problem with the file on disk, when a hand edit made it
// invalid. Files then keeps the last valid map.
func (s *Files) Error() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

// Update applies fn to a copy of the map, checks the result, and writes it.
// The file never holds an invalid map. The health in the file is what the
// caller sets.
func (s *Files) Update(fn func(*Map) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.m.Clone()
	if err := fn(next); err != nil {
		return err
	}
	if err := s.check(next); err != nil {
		return err
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.m, s.loadErr = next, ""
	return nil
}

// Reload reads the map again if it changed on disk since the last read or
// write, and reports whether the map or a private document changed. If the
// map file is invalid, Files keeps the last valid map and records the
// error.
func (s *Files) Reload() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	privateChanged := false
	if stamp := s.privateStamp(); stamp != s.privateSeen {
		s.privateSeen, privateChanged = stamp, true
	}
	info, err := os.Stat(s.path)
	if err != nil {
		msg := fmt.Sprintf("the map file is missing: %v", err)
		changed := msg != s.loadErr
		s.loadErr = msg
		return changed
	}
	if info.ModTime().Equal(s.modTime) && info.Size() == s.size {
		return privateChanged
	}
	m, err := s.read()
	if err != nil {
		s.loadErr = err.Error()
		return true
	}
	s.m, s.loadErr = m, ""
	return true
}

// Watch reloads the file every interval until ctx ends, and calls changed
// after each reload that found a change.
func (s *Files) Watch(ctx context.Context, interval time.Duration, changed func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.Reload() {
				changed()
			}
		}
	}
}

// Private reads the private documents.
func (s *Files) Private() (Private, error) {
	return LoadPrivate(s.root)
}

// PrivateErrors returns the problems of the private documents, checked
// against the features of m.
func (s *Files) PrivateErrors(m *Map) []string {
	p, err := s.Private()
	if err == nil {
		err = p.Validate(m.IDs(), s.exists)
	}
	if err == nil {
		return nil
	}
	return strings.Split(err.Error(), "\n")
}

// stat returns the file info of a workspace path.
func (s *Files) stat(p string) (fs.FileInfo, error) {
	host, ok := s.ws.Host(filepath.ToSlash(p))
	if !ok {
		return nil, fs.ErrNotExist
	}
	return os.Stat(host)
}

func (s *Files) exists(p string) bool {
	_, err := s.stat(p)
	return err == nil
}

// Exists reports whether a workspace path exists.
func (s *Files) Exists(p string) bool { return s.exists(p) }

// IsFile reports whether a workspace path is a file.
func (s *Files) IsFile(p string) bool {
	info, err := s.stat(p)
	return err == nil && info.Mode().IsRegular()
}

// IsDir reports whether a workspace path is a directory. The workspace
// itself, ".", is one.
func (s *Files) IsDir(p string) bool {
	if p == "." {
		return true
	}
	info, err := s.stat(p)
	return err == nil && info.IsDir()
}

// PrivatePath returns the host path of a file in the private directory.
func (s *Files) PrivatePath(file string) string { return privatePath(s.root, file) }

// privateStamp identifies the current version of the private documents.
func (s *Files) privateStamp() string {
	var b strings.Builder
	for _, file := range []string{CodeMapFile, testmap.MapFile} {
		if info, err := os.Stat(privatePath(s.root, file)); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", file, info.ModTime().UnixNano(), info.Size())
		}
	}
	return b.String()
}

func (s *Files) check(m *Map) error {
	if m.Partition.Name != s.name {
		return fmt.Errorf("partition.name is %q, but this partition belongs to %q", m.Partition.Name, s.name)
	}
	return Validate(m)
}

// read parses and checks the file, and records its size and time.
func (s *Files) read() (*Map, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// Record the file as seen even when it is invalid, so that the same
	// invalid file is not reported again on every reload.
	s.modTime, s.size = info.ModTime(), info.Size()
	var m Map
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := s.check(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// save writes m with an atomic rename, so that readers never see a partial
// file.
func (s *Files) save(m *Map) error {
	data, err := marshalYAML(m)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".map-*.yaml")
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
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return err
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	s.modTime, s.size = info.ModTime(), info.Size()
	return nil
}

// ContentHash hashes the contents of the workspace paths of ws. A
// directory counts as all the files below it. A missing path changes the
// hash too.
func ContentHash(ws workspace.Workspace, paths []string) (string, error) {
	h := sha256.New()
	sorted := slices.Sorted(slices.Values(paths))
	for _, p := range slices.Compact(sorted) {
		abs, ok := ws.Host(filepath.ToSlash(p))
		if !ok {
			fmt.Fprintf(h, "missing\x00%s\x00", p)
			continue
		}
		info, err := os.Stat(abs)
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(h, "missing\x00%s\x00", p)
			continue
		} else if err != nil {
			return "", err
		}
		if !info.IsDir() {
			if err := hashFile(h, p, abs); err != nil {
				return "", err
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(abs, path)
			return hashFile(h, filepath.Join(p, rel), path)
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

func hashFile(h io.Writer, rel, abs string) error {
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(h, "file\x00%s\x00", rel)
	_, err = io.Copy(h, f)
	h.Write([]byte{0})
	return err
}
