package wiki

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Store keeps one wiki in a directory, one file per document. The server
// writes the agent documents; it reloads hand edits file by file and keeps
// a file's last valid version when an edit breaks a rule (rule D4).
type Store struct {
	dir   string
	kinds []string

	mu       sync.Mutex
	w        Wiki
	raw      map[string]string
	stamp    string
	problems []string
	broken   map[string]bool
}

// Open loads the wiki in dir, which holds the agent documents of kinds,
// creating the directory when it is missing. A file with a problem does not
// stop the wiki; Problems lists it.
func Open(dir string, kinds []string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, kinds: kinds, w: Wiki{}, raw: map[string]string{}, broken: map[string]bool{}}
	s.stamp = s.dirStamp()
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("wiki %s: %w", dir, err)
	}
	return s, nil
}

func (s *Store) load() error {
	w, raw, problems, broken, err := Load(s.dir, Check{Kinds: s.kinds}, s.w, s.raw)
	if err != nil {
		return err
	}
	s.w, s.raw, s.problems, s.broken = w, raw, problems, broken
	return nil
}

// Dir returns the directory of the wiki.
func (s *Store) Dir() string { return s.dir }

// Kinds returns the agent kinds that the wiki holds.
func (s *Store) Kinds() []string { return s.kinds }

// Wiki returns a copy of the current wiki.
func (s *Store) Wiki() Wiki {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Clone()
}

// Raw returns the text of a document as it was last read or written.
func (s *Store) Raw(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw[id]
}

// Path returns the file of a document.
func (s *Store) Path(id string) string { return filepath.Join(s.dir, id+".md") }

// Files returns the text of the documents of kinds by file name, for a
// release (rule L2).
func (s *Store) Files(kinds ...string) map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]byte{}
	for id, text := range s.raw {
		if d := s.w[id]; d != nil && slices.Contains(kinds, d.KindOf()) {
			out[id+".md"] = []byte(text)
		}
	}
	return out
}

// Problems returns the problems of the files on disk (rule D4).
func (s *Store) Problems() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.problems)
}

// Error returns the problems of the files on disk in one line, or "".
func (s *Store) Error() string { return strings.Join(s.Problems(), "; ") }

// Change applies writes as one change and writes the documents it changed,
// and the links of the user documents that a rename or a remove rewrote.
// It refuses a change that would write a document whose file is broken on
// disk (rule N5a).
func (s *Store) Change(writes []Write, c Check, now time.Time) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Kinds = s.kinds
	next, res, err := Apply(s.w, writes, c, now)
	if err != nil {
		return res, err
	}
	named := map[string]bool{}
	for _, wr := range writes {
		named[wr.ID], named[wr.To] = true, true
	}
	var written []string
	for _, id := range res.Written {
		switch {
		case s.broken[id] && named[id]:
			return Result{}, fmt.Errorf("N5a: the file of %q has a problem that a hand edit left (%s); the user must fix it first", id, s.problemOf(id))
		case s.broken[id]:
			// Its links are rewritten in the text on disk below.
			next[id] = s.w[id]
		default:
			if err := s.write(next[id]); err != nil {
				return res, err
			}
			written = append(written, id)
		}
	}
	res.Written = written
	for old := range res.Renamed {
		s.remove(old)
	}
	for _, id := range res.Removed {
		s.remove(id)
	}
	s.w = next
	users, err := s.relinkLocked(res.Renamed, removals(res), true)
	if err != nil {
		return res, err
	}
	res.Written = append(res.Written, users...)
	slices.Sort(res.Written)
	s.stamp = s.dirStamp()
	return res, nil
}

// removals maps each id that a change removed to its redirect, or "".
func removals(res Result) map[string]string {
	out := map[string]string{}
	for _, id := range res.Removed {
		out[id] = res.Redirects[id]
	}
	return out
}

func (s *Store) problemOf(id string) string {
	for _, p := range s.problems {
		if strings.HasPrefix(p, id+".md: ") {
			return strings.TrimPrefix(p, id+".md: ")
		}
	}
	return "it does not parse"
}

// Relink rewrites the links of every document to the renamed and removed
// documents of another wiki (rule N5). It returns the ids it wrote.
func (s *Store) Relink(renamed, removed map[string]string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	written, err := s.relinkLocked(renamed, removed, false)
	s.stamp = s.dirStamp()
	return written, err
}

// relinkLocked rewrites links in the files as they are on disk: an agent
// document that parses is written again from its parsed form, and a user
// document or a broken file gets only its link text replaced. With
// textOnly, it leaves the agent documents that parse to the change that ran.
func (s *Store) relinkLocked(renamed, removed map[string]string, textOnly bool) ([]string, error) {
	if len(renamed) == 0 && len(removed) == 0 {
		return nil, nil
	}
	var written []string
	for _, id := range s.w.IDs() {
		d := s.w[id]
		if textOnly && !d.User() && !s.broken[id] {
			continue
		}
		if !d.User() && !s.broken[id] {
			next := d.Clone()
			changed := false
			for old, to := range renamed {
				changed = relink(next, old, to) || changed
			}
			for old, to := range removed {
				changed = relink(next, old, to) || changed
			}
			if changed {
				if err := s.write(next); err != nil {
					return written, err
				}
				s.w[id] = next
				written = append(written, id)
			}
			continue
		}
		data, err := os.ReadFile(s.Path(id))
		if err != nil {
			continue
		}
		text := string(data)
		for old, to := range renamed {
			text = rewriteLinks(text, old, to)
		}
		for old, to := range removed {
			text = rewriteLinks(text, old, to)
		}
		if text == string(data) {
			continue
		}
		if err := s.writeText(id, text); err != nil {
			return written, err
		}
		if nd, err := Parse(text); err == nil || errors.Is(err, ErrNoHeader) {
			if nd == nil {
				nd = &Document{Body: text}
			}
			if nd.User() || nd.ID == "" {
				nd.ID = id
			}
			if !s.broken[id] {
				s.w[id] = nd
			}
		}
		written = append(written, id)
	}
	return written, nil
}

// SetDescribed records commit and now as what every agent document
// describes. A document whose file is broken on disk is left alone.
func (s *Store) SetDescribed(commit string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.w.Clone()
	for id, d := range next {
		if d.User() || s.broken[id] {
			continue
		}
		d.DescribedCommit, d.DescribedAt = commit, now.UTC().Truncate(time.Second)
		if err := s.write(d); err != nil {
			return err
		}
	}
	s.w = next
	s.stamp = s.dirStamp()
	return nil
}

// WriteDoc writes a document that the user gives in full, as a hand edit
// that must be valid (rule U8): an agent document must keep the rules of its
// kind, also the strict ones, and a user document's name must keep D2.
func (s *Store) WriteDoc(name, text string, c Check) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := Parse(text)
	switch {
	case errors.Is(err, ErrNoHeader):
		d = &Document{Body: text}
	case err != nil:
		return err
	}
	if d.User() {
		if p := UserNameProblem(name); p != "" {
			return errors.New(p)
		}
		if old := s.w[name]; old != nil && !old.User() {
			return fmt.Errorf("%q is a %s; write it with its kind", name, old.KindOf())
		}
		d.ID = name
	} else {
		if d.ID == "" {
			d.ID = name
		}
		c.Kinds = s.kinds
		next := s.w.Clone()
		next[name] = d
		if err := next.Validate(c, map[string]bool{name: true}); err != nil {
			return err
		}
	}
	if err := s.writeText(name, text); err != nil {
		return err
	}
	s.w[name] = d
	delete(s.broken, name)
	s.problems = slices.DeleteFunc(s.problems, func(p string) bool { return strings.HasPrefix(p, name+".md: ") })
	s.stamp = s.dirStamp()
	return nil
}

// RemoveUserDoc removes a plan or a note.
func (s *Store) RemoveUserDoc(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.w[name]
	switch {
	case d == nil:
		return fmt.Errorf("no document %q", name)
	case !d.User():
		return fmt.Errorf("%q is a %s; ask its author to remove it", name, d.KindOf())
	}
	if err := os.Remove(s.Path(name)); err != nil {
		return err
	}
	delete(s.w, name)
	delete(s.raw, name)
	s.stamp = s.dirStamp()
	return nil
}

// Reload reads the directory again when it changed on disk, and reports
// whether anything changed.
func (s *Store) Reload() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stamp := s.dirStamp()
	if stamp == s.stamp {
		return false
	}
	s.stamp = stamp
	if err := s.load(); err != nil {
		s.problems = []string{err.Error()}
	}
	return true
}

func (s *Store) write(d *Document) error { return s.writeText(d.ID, d.Render()) }

// writeText writes a document's file with an atomic rename.
func (s *Store) writeText(id, text string) error {
	tmp, err := os.CreateTemp(s.dir, ".doc-*.md")
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
	if err := os.Rename(tmp.Name(), s.Path(id)); err != nil {
		return err
	}
	s.raw[id] = text
	return nil
}

func (s *Store) remove(id string) {
	os.Remove(s.Path(id))
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
