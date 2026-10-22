package featurewiki

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Wiki is the feature documents of one system, by id.
type Wiki map[string]*Document

// Clone returns a deep copy of w.
func (w Wiki) Clone() Wiki {
	c := make(Wiki, len(w))
	for id, d := range w {
		c[id] = d.Clone()
	}
	return c
}

// IDs returns the ids of w, sorted.
func (w Wiki) IDs() []string {
	return slices.Sorted(maps.Keys(w))
}

// Neighbors returns, for each feature, every feature it links or that links
// it (rule K3), sorted.
func (w Wiki) Neighbors() map[string][]string {
	out := map[string][]string{}
	add := func(a, b string) {
		if a != b && !slices.Contains(out[a], b) {
			out[a] = append(out[a], b)
		}
	}
	for id, d := range w {
		for _, n := range d.Neighbors() {
			add(id, n)
			add(n, id)
		}
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// Check is what the rules need from outside the wiki.
type Check struct {
	// Interfaces are the names the system provides or consumes (rule W3).
	Interfaces map[string]bool
	// Taken are the ids that other systems use (rule W1).
	Taken map[string]bool
	// Exists reports whether a path relative to the scope exists, for the
	// warning on paths in a Purpose. nil skips that warning.
	Exists func(rel string) bool
}

var (
	validID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
	// MaxDescription is the longest description, in characters.
	MaxDescription = 200
	// MaxWrites is the most writes in one change.
	MaxWrites = 100
	// ManyFeatures is the count above which the wiki warns.
	ManyFeatures = 60
)

// Validate checks the rules W1 to W4 and K1 to K2.
func (w Wiki) Validate(c Check) error {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	for _, id := range w.IDs() {
		d := w[id]
		if !validID.MatchString(id) {
			fail("W1", "feature id %q must match %s", id, validID)
		}
		if d.ID != id {
			fail("W1", "document %s.md says id %q; the id is the file name", id, d.ID)
		}
		if c.Taken[id] {
			fail("W1", "another system of this topology uses the id %q", id)
		}
		if strings.TrimSpace(d.Name) == "" {
			fail("W2", "feature %q has no name", id)
		}
		desc := strings.TrimSpace(d.Description)
		switch {
		case desc == "":
			fail("W2", "feature %q has no description", id)
		case len([]rune(desc)) > MaxDescription:
			fail("W2", "feature %q: the description has %d characters; at most %d", id, len([]rune(desc)), MaxDescription)
		case strings.Contains(desc, "\n"):
			fail("W2", "feature %q: the description must be one sentence on one line", id)
		}
		if c.Interfaces != nil {
			for _, name := range d.Interfaces {
				if !c.Interfaces[name] {
					fail("W3", "feature %q uses interface %q, which the system neither provides nor consumes", id, name)
				}
			}
		}
		if text, ok := section(d.Body, Purpose); !ok {
			fail("W4", "feature %q has no ## Purpose section", id)
		} else if strings.TrimSpace(text) == "" {
			fail("W4", "feature %q: the ## Purpose section is empty", id)
		}
		if _, err := Links(d.Body); err != nil {
			fail("K1", "feature %q: %v", id, err)
		}
		for _, n := range d.Neighbors() {
			switch {
			case n == id:
				fail("K2", "feature %q links itself", id)
			case w[n] == nil:
				fail("K2", "feature %q links %q, which is not a feature of this system", id, n)
			}
		}
	}
	return errors.Join(errs...)
}

// Warnings lists what the rules allow but the owner should look at.
func (w Wiki) Warnings(c Check) []string {
	var out []string
	neighbors := w.Neighbors()
	for _, id := range w.IDs() {
		if len(neighbors[id]) == 0 {
			out = append(out, fmt.Sprintf("feature %q has no neighbor; link it to the features it works with, or check the feature set", id))
		}
		if c.Exists != nil {
			purpose, _ := section(w[id].Body, Purpose)
			for _, p := range pathsIn(purpose, c.Exists) {
				out = append(out, fmt.Sprintf("feature %q: its Purpose names the path %s; the body cites no code", id, p))
			}
		}
	}
	if len(w) > ManyFeatures {
		out = append(out, fmt.Sprintf("the system has %d features; above %d you are listing parts, not features", len(w), ManyFeatures))
	}
	return out
}

// pathsIn returns the words of text that name a path in the scope.
func pathsIn(text string, exists func(string) bool) []string {
	var out []string
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '`' || r == '(' || r == ')' || r == ',' || r == ';' || r == '"'
	}) {
		word = strings.TrimRight(word, ".:")
		if word == "" || word == "." || strings.HasPrefix(word, "[[") || !strings.ContainsAny(word, "/.") {
			continue
		}
		if filepath.IsLocal(word) && exists(filepath.Clean(word)) && !slices.Contains(out, word) {
			out = append(out, word)
		}
	}
	return out
}

// Write is one write of a change.
type Write struct {
	Op string `json:"op"`
	ID string `json:"id"`
	// create
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Interfaces  []string `json:"interfaces,omitempty"`
	Related     []string `json:"related,omitempty"`
	// create and modify
	Body *string `json:"body,omitempty"`
	// modify
	Fields *Fields `json:"fields,omitempty"`
	// rename
	To string `json:"to,omitempty"`
	// remove
	Redirect string `json:"redirect,omitempty"`
}

// Fields are the properties a modify changes. A field that is nil stays.
// An empty list clears interfaces or related.
type Fields struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Interfaces  *[]string `json:"interfaces,omitempty"`
	Related     *[]string `json:"related,omitempty"`
}

// Result says what a change did.
type Result struct {
	// Applied has one line per write.
	Applied []string `json:"applied"`
	// Warnings are the warnings of the wiki after the change.
	Warnings []string `json:"warnings,omitempty"`
	// Renamed maps each old id to its new id.
	Renamed map[string]string `json:"renamed,omitempty"`
	// Removed lists the removed ids.
	Removed []string `json:"removed,omitempty"`
	// Written lists the ids whose documents the change wrote.
	Written []string `json:"written,omitempty"`
}

// Apply applies writes to a copy of w as one change and checks the result.
// It returns the new wiki, or an error that names each write and rule that
// failed, in which case w is unchanged.
func Apply(w Wiki, writes []Write, c Check, now time.Time) (Wiki, Result, error) {
	var res Result
	if len(writes) == 0 {
		return nil, res, errors.New("the change has no writes")
	}
	if len(writes) > MaxWrites {
		return nil, res, fmt.Errorf("W9: the change has %d writes; at most %d", len(writes), MaxWrites)
	}
	next := w.Clone()
	written := map[string]bool{}
	touch := func(id string) {
		written[id] = true
		next[id].Updated = now.UTC()
	}
	for i, wr := range writes {
		where := fmt.Sprintf("write %d (%s %s)", i+1, wr.Op, wr.ID)
		if wr.ID == "" {
			return nil, res, fmt.Errorf("%s: the write has no id", where)
		}
		switch wr.Op {
		case "create":
			if next[wr.ID] != nil {
				return nil, res, fmt.Errorf("%s: feature %q exists; use modify", where, wr.ID)
			}
			if wr.Body == nil {
				return nil, res, fmt.Errorf("%s: a create needs a body", where)
			}
			if _, ok := section(*wr.Body, Notes); ok {
				return nil, res, fmt.Errorf("%s: W4: the body has a ## Notes section, which belongs to the user", where)
			}
			next[wr.ID] = &Document{
				ID: wr.ID, Name: wr.Name, Description: wr.Description,
				Interfaces: slices.Clone(wr.Interfaces), Related: slices.Clone(wr.Related), Body: *wr.Body,
			}
			touch(wr.ID)
			res.Applied = append(res.Applied, "created "+wr.ID)
		case "modify":
			d := next[wr.ID]
			if d == nil {
				return nil, res, fmt.Errorf("%s: no feature %q", where, wr.ID)
			}
			if wr.Fields == nil && wr.Body == nil {
				return nil, res, fmt.Errorf("%s: a modify needs fields or a body", where)
			}
			if f := wr.Fields; f != nil {
				if f.Name != nil {
					d.Name = *f.Name
				}
				if f.Description != nil {
					d.Description = *f.Description
				}
				if f.Interfaces != nil {
					d.Interfaces = slices.Clone(*f.Interfaces)
				}
				if f.Related != nil {
					d.Related = slices.Clone(*f.Related)
				}
			}
			if wr.Body != nil {
				if _, ok := section(*wr.Body, Notes); ok {
					return nil, res, fmt.Errorf("%s: W4: the body has a ## Notes section, which belongs to the user; leave it out and hikari keeps the user's notes", where)
				}
				body := strings.TrimRight(*wr.Body, "\n")
				if notes, ok := section(d.Body, Notes); ok {
					body += "\n\n## " + Notes + strings.TrimRight(notes, "\n") + "\n"
				}
				d.Body = body
			}
			touch(wr.ID)
			res.Applied = append(res.Applied, "modified "+wr.ID)
		case "rename":
			d := next[wr.ID]
			if d == nil {
				return nil, res, fmt.Errorf("%s: no feature %q", where, wr.ID)
			}
			if wr.To == "" {
				return nil, res, fmt.Errorf("%s: a rename needs the new id in to", where)
			}
			if next[wr.To] != nil {
				return nil, res, fmt.Errorf("%s: feature %q exists", where, wr.To)
			}
			delete(next, wr.ID)
			delete(written, wr.ID)
			d.ID = wr.To
			next[wr.To] = d
			for id, other := range next {
				if relink(other, wr.ID, wr.To) {
					touch(id)
				}
			}
			touch(wr.To)
			if res.Renamed == nil {
				res.Renamed = map[string]string{}
			}
			for old, to := range res.Renamed {
				if to == wr.ID {
					res.Renamed[old] = wr.To
				}
			}
			res.Renamed[wr.ID] = wr.To
			res.Applied = append(res.Applied, fmt.Sprintf("renamed %s to %s", wr.ID, wr.To))
		case "remove":
			if next[wr.ID] == nil {
				return nil, res, fmt.Errorf("%s: no feature %q", where, wr.ID)
			}
			if wr.Redirect != "" && next[wr.Redirect] == nil {
				return nil, res, fmt.Errorf("%s: the redirect %q is not a feature", where, wr.Redirect)
			}
			delete(next, wr.ID)
			delete(written, wr.ID)
			for id, other := range next {
				if relink(other, wr.ID, wr.Redirect) {
					touch(id)
				}
			}
			res.Removed = append(res.Removed, wr.ID)
			res.Applied = append(res.Applied, "removed "+wr.ID)
		default:
			return nil, res, fmt.Errorf("%s: unknown op %q; use create, modify, rename, or remove", where, wr.Op)
		}
	}
	if err := next.Validate(c); err != nil {
		return nil, res, err
	}
	res.Written = slices.Sorted(maps.Keys(written))
	res.Warnings = next.Warnings(c)
	return next, res, nil
}

// relink points every link to old in d at to, or removes the links when to
// is empty (rule K4). It reports whether d changed.
func relink(d *Document, old, to string) bool {
	changed := false
	if body := rewriteLinks(d.Body, old, to); body != d.Body {
		d.Body, changed = body, true
	}
	if i := slices.Index(d.Related, old); i >= 0 {
		changed = true
		switch {
		case to == "" || slices.Contains(d.Related, to):
			d.Related = slices.Delete(d.Related, i, i+1)
		default:
			d.Related[i] = to
		}
	}
	return changed
}

// Load reads every document of dir. A document that does not parse, or
// whose id is not its file name, is an error.
func Load(dir string) (Wiki, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	w := Wiki{}
	raw := map[string]string{}
	var errs []error
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		d, err := Parse(string(data))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		if d.ID == "" {
			d.ID = name
		}
		w[name] = d
		raw[name] = string(data)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}
	if err := w.Validate(Check{}); err != nil {
		return nil, nil, err
	}
	return w, raw, nil
}
