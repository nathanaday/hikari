package sysmap

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
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const header = `# The system map of %s. hikari writes this file for the agent that owns
# the system; the agent changes it through the map tools.
`

// Store holds the map of one system and keeps its file up to date.
type Store struct {
	root string // the scope on the host
	path string // the map file on the host
	name string

	mu      sync.Mutex
	m       *Map
	modTime time.Time
	size    int64
	loadErr string
}

// Open loads the map of the system name, whose scope is root on the host. If
// the map file does not exist, Open writes an unbuilt map and reports that
// it created the file.
func Open(root, name, testCommand string) (*Store, bool, error) {
	s := &Store{root: root, path: filepath.Join(root, Dir, File), name: name}
	if _, err := os.Stat(s.path); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			return nil, false, err
		}
		s.m = New(name, testCommand)
		return s, true, s.save(s.m)
	}
	m, err := s.read()
	if err != nil {
		return nil, false, fmt.Errorf("system map %s: %w", s.path, err)
	}
	s.m = m
	return s, false, nil
}

// Root returns the scope on the host.
func (s *Store) Root() string { return s.root }

// Path returns the map file on the host.
func (s *Store) Path() string { return s.path }

// Map returns a copy of the current map.
func (s *Store) Map() *Map {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.Clone()
}

// Error returns the problem with the file on disk, when a hand edit made it
// invalid. The store then keeps the last valid map.
func (s *Store) Error() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

// Update applies fn to a copy of the map, checks the result, and writes it.
// The file never holds an invalid map.
func (s *Store) Update(fn func(*Map) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.m.Clone()
	if err := fn(next); err != nil {
		return err
	}
	if err := s.check(next); err != nil {
		return err
	}
	for i := range next.Features {
		next.Features[i].Health = Health(next.Features[i], s.hashOrEmpty(next.Features[i].Paths()))
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.m, s.loadErr = next, ""
	return nil
}

// Reload reads the file again if it changed on disk since the last read or
// write, and reports whether it did. If the file is invalid, the store keeps
// the last valid map and records the error.
func (s *Store) Reload() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	if err != nil {
		msg := fmt.Sprintf("the map file is missing: %v", err)
		changed := msg != s.loadErr
		s.loadErr = msg
		return changed
	}
	if info.ModTime().Equal(s.modTime) && info.Size() == s.size {
		return false
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
func (s *Store) Watch(ctx context.Context, interval time.Duration, changed func()) {
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

// Hash returns the content hash of paths relative to the scope.
func (s *Store) Hash(paths []string) (string, error) {
	return ContentHash(s.root, paths)
}

func (s *Store) hashOrEmpty(paths []string) string {
	h, _ := ContentHash(s.root, paths)
	return h
}

func (s *Store) check(m *Map) error {
	if m.System.Name != s.name {
		return fmt.Errorf("system.name is %q, but this system belongs to %q", m.System.Name, s.name)
	}
	return Validate(m, func(rel string) bool {
		_, err := os.Stat(filepath.Join(s.root, rel))
		return err == nil
	})
}

// read parses and checks the file, and records its size and time.
func (s *Store) read() (*Map, error) {
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
	var m Map
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := s.check(&m); err != nil {
		return nil, err
	}
	s.modTime, s.size = info.ModTime(), info.Size()
	return &m, nil
}

// save writes m with an atomic rename, so that readers never see a partial
// file.
func (s *Store) save(m *Map) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	data = append([]byte(fmt.Sprintf(header, s.name)), data...)
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

// ContentHash hashes the contents of paths relative to root. A directory
// counts as all the files below it. A missing path changes the hash too.
func ContentHash(root string, paths []string) (string, error) {
	h := sha256.New()
	sorted := slices.Sorted(slices.Values(paths))
	for _, p := range slices.Compact(sorted) {
		abs := filepath.Join(root, filepath.Clean(p))
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
			rel, _ := filepath.Rel(root, path)
			return hashFile(h, rel, path)
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
