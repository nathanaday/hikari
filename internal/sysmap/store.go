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
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Store holds the map of one system and keeps its file up to date.
type Store struct {
	root string // the scope on the host
	path string // the map file on the host
	name string

	mu          sync.Mutex
	m           *Map
	modTime     time.Time
	size        int64
	loadErr     string
	privateSeen string
	note        string
}

// Open loads the map of the system name from mapPath. root is the system's
// scope on the host, where its private documents live. Open writes an
// unbuilt map and empty private documents for those that do not exist, and
// reports whether it created the map. testCommand, when set, becomes the
// first suite of a new test plan.
func Open(root, mapPath, name, testCommand string) (*Store, bool, error) {
	s := &Store{root: root, path: mapPath, name: name}
	if err := writePrivateSkeletons(root, testCommand); err != nil {
		return nil, false, err
	}
	s.privateSeen = s.privateStamp()
	if _, err := os.Stat(s.path); errors.Is(err, fs.ErrNotExist) {
		s.m = New(name)
		return s, true, s.save(s.m)
	}
	if v, err := fileVersion(s.path); err == nil && v != version {
		// A map from an earlier hikari is not converted (rule Q4).
		if err := os.Remove(s.path); err != nil {
			return nil, false, err
		}
		s.note = fmt.Sprintf("deleted %s: it is a version %d map, and this hikari writes version %d; the owner describes the system again", s.path, v, version)
		s.m = New(name)
		return s, true, s.save(s.m)
	}
	m, err := s.read()
	if err != nil {
		return nil, false, fmt.Errorf("system map %s: %w", s.path, err)
	}
	s.m = m
	return s, false, nil
}

// Note returns what Open did to a map file from an earlier version, or "".
func (s *Store) Note() string { return s.note }

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
	priv, _ := s.Private()
	for i := range next.Features {
		f := &next.Features[i]
		f.Health = Health(*f, priv.Tested(f.ID), s.hashOrEmpty(priv.Paths(f.ID)))
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.m, s.loadErr = next, ""
	return nil
}

// Reload reads the map again if it changed on disk since the last read or
// write, and reports whether the map or a private document changed. If the
// map file is invalid, the store keeps the last valid map and records the
// error.
func (s *Store) Reload() bool {
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

// Private reads the private documents.
func (s *Store) Private() (Private, error) {
	return LoadPrivate(s.root)
}

// PrivateErrors returns the problems of the private documents, checked
// against the features of m.
func (s *Store) PrivateErrors(m *Map) []string {
	p, err := s.Private()
	if err == nil {
		err = p.Validate(m.IDs(), s.exists)
	}
	if err == nil {
		return nil
	}
	return strings.Split(err.Error(), "\n")
}

// FeatureHash returns the current content hash of a feature's files, as the
// private documents list them.
func (s *Store) FeatureHash(id string) (string, error) {
	p, err := s.Private()
	if err != nil {
		return "", err
	}
	return ContentHash(s.root, p.Paths(id))
}

func (s *Store) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(s.root, rel))
	return err == nil
}

// privateStamp identifies the current version of the private documents.
func (s *Store) privateStamp() string {
	var b strings.Builder
	for _, file := range []string{LayoutFile, TestFile} {
		if info, err := os.Stat(privatePath(s.root, file)); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", file, info.ModTime().UnixNano(), info.Size())
		}
	}
	return b.String()
}

func (s *Store) hashOrEmpty(paths []string) string {
	h, _ := ContentHash(s.root, paths)
	return h
}

func (s *Store) check(m *Map) error {
	if m.System.Name != s.name {
		return fmt.Errorf("system.name is %q, but this system belongs to %q", m.System.Name, s.name)
	}
	return Validate(m)
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
func (s *Store) save(m *Map) error {
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
