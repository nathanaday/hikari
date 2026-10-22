package featurewiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store keeps the wiki of one system in a directory, one file per feature.
// The server is the only writer; it reloads hand edits and keeps the last
// valid wiki when an edit breaks a rule.
type Store struct {
	dir string

	mu      sync.Mutex
	w       Wiki
	raw     map[string]string
	stamp   string
	loadErr string
}

// Open loads the wiki in dir, creating the directory when it is missing.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, w: Wiki{}, raw: map[string]string{}}
	s.stamp = s.dirStamp()
	w, raw, err := Load(dir)
	if err != nil {
		return nil, fmt.Errorf("feature wiki %s: %w", dir, err)
	}
	s.w, s.raw = w, raw
	return s, nil
}

// Dir returns the directory of the wiki.
func (s *Store) Dir() string { return s.dir }

// Wiki returns a copy of the current wiki.
func (s *Store) Wiki() Wiki {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Clone()
}

// Raw returns the text of a document as it is on disk.
func (s *Store) Raw(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw[id]
}

// Files returns every document's text by file name, for a release.
func (s *Store) Files() map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]byte{}
	for id, text := range s.raw {
		out[id+".md"] = []byte(text)
	}
	return out
}

// Error returns the problem with the files on disk, when a hand edit made
// the wiki invalid.
func (s *Store) Error() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

// Change applies writes as one change and writes the documents it changed.
func (s *Store) Change(writes []Write, c Check, now time.Time) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, res, err := Apply(s.w, writes, c, now)
	if err != nil {
		return res, err
	}
	for _, id := range res.Written {
		if err := s.write(next[id]); err != nil {
			return res, err
		}
	}
	for old := range res.Renamed {
		s.remove(old)
	}
	for _, id := range res.Removed {
		s.remove(id)
	}
	s.w, s.loadErr = next, ""
	s.stamp = s.dirStamp()
	return res, nil
}

// SetDescribed records commit and now as what every document describes.
func (s *Store) SetDescribed(commit string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.w.Clone()
	for _, d := range next {
		d.DescribedCommit, d.DescribedAt = commit, now.UTC()
		if err := s.write(d); err != nil {
			return err
		}
	}
	s.w = next
	s.stamp = s.dirStamp()
	return nil
}

// Reload reads the directory again when it changed on disk, and reports
// whether anything changed. An invalid wiki is kept as an error, and the
// last valid wiki stays.
func (s *Store) Reload() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stamp := s.dirStamp()
	if stamp == s.stamp {
		return false
	}
	s.stamp = stamp
	w, raw, err := Load(s.dir)
	if err != nil {
		s.loadErr = err.Error()
		return true
	}
	s.w, s.raw, s.loadErr = w, raw, ""
	return true
}

func (s *Store) write(d *Document) error {
	text := d.Render()
	path := filepath.Join(s.dir, d.ID+".md")
	tmp, err := os.CreateTemp(s.dir, ".feature-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	s.raw[d.ID] = text
	return nil
}

func (s *Store) remove(id string) {
	os.Remove(filepath.Join(s.dir, id+".md"))
	delete(s.raw, id)
}

// dirStamp identifies the current files of the directory.
func (s *Store) dirStamp() string {
	entries, _ := os.ReadDir(s.dir)
	var b strings.Builder
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.ModTime().UnixNano(), info.Size())
		}
	}
	return b.String()
}
