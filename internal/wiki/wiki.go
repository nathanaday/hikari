package wiki

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

// Wiki is the documents of one system or one hub, by id. A user
// document's id is its file name.
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

// Features returns the ids of the features of w, sorted.
func (w Wiki) Features() []string {
	var out []string
	for _, id := range w.IDs() {
		if w[id].KindOf() == Feature {
			out = append(out, id)
		}
	}
	return out
}

// Neighbors returns, for each feature, every feature of w it links or that
// links it (rule K3), sorted. Documents of other kinds are not neighbors in
// the map.
func (w Wiki) Neighbors() map[string][]string {
	out := map[string][]string{}
	add := func(a, b string) {
		if a != b && !slices.Contains(out[a], b) {
			out[a] = append(out[a], b)
		}
	}
	for id, d := range w {
		if d.KindOf() != Feature {
			continue
		}
		for _, n := range d.Neighbors() {
			if w[n] != nil && w[n].KindOf() == Feature {
				add(id, n)
				add(n, id)
			}
		}
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// Check is what the rules need from outside the wiki.
type Check struct {
	// Kinds are the agent kinds that the wiki holds. nil allows every
	// agent kind.
	Kinds []string
	// Interfaces are the names the system provides or consumes (rule W3).
	// nil skips the check.
	Interfaces map[string]bool
	// Taken are the ids of the documents of every other wiki, in lower
	// case (rule W1). A user document's name counts too.
	Taken map[string]bool
	// Linkable reports whether an id names an agent document of another
	// wiki that this wiki can link (rule N2). nil links nothing outside.
	Linkable func(id string) bool
	// Known reports whether an id names any document of another wiki, for
	// the links of user documents (rule N3). nil knows nothing outside.
	Known func(id string) bool
	// Exists reports whether a path relative to the scope exists, for the
	// warning on paths in a Purpose. nil skips that warning.
	Exists func(rel string) bool
}

// allows reports whether the wiki holds agent documents of kind.
func (c Check) allows(kind string) bool {
	if c.Kinds == nil {
		return slices.Contains(AgentKinds, kind)
	}
	return slices.Contains(c.Kinds, kind)
}

// linkable reports whether an agent document of w can link id.
func (w Wiki) linkable(id string, c Check) bool {
	if d := w[id]; d != nil {
		return !d.User()
	}
	return c.Linkable != nil && c.Linkable(id)
}

// UserNameProblem says why name cannot name a user document (rule D2), or
// returns "".
func UserNameProblem(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "a document needs a name"
	case len(name) > MaxNameBytes:
		return fmt.Sprintf("a name holds at most %d bytes", MaxNameBytes)
	case name != strings.TrimSpace(name):
		return "a name cannot start or end with a space"
	case strings.HasPrefix(name, "."):
		return "a name cannot start with a dot"
	case strings.ContainsAny(name, `*"\/<>:|?#^[]`):
		return `a name cannot hold * " \ / < > : | ? # ^ [ or ]`
	}
	return ""
}

// MaxNameBytes is the longest name of a user document (rule D2).
const MaxNameBytes = 200

var (
	validID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
	// MaxDescription is the longest description, in characters.
	MaxDescription = 200
	// MaxWrites is the most writes in one change.
	MaxWrites = 100
	// ManyFeatures is the count above which the wiki warns.
	ManyFeatures = 60
)

// Validate checks the rules of the agent documents that hold for every
// write (W1 to W4, K1, K2 inside the wiki). For the documents in strict, it
// also checks the rules that a hand edit may break with only a warning
// (rule N4): every link resolves, and no other wiki uses the id.
func (w Wiki) Validate(c Check, strict map[string]bool) error {
	var errs []error
	for _, id := range w.IDs() {
		errs = append(errs, w.validateDoc(id, c, strict[id])...)
	}
	return errors.Join(errs...)
}

// validateDoc checks one document of w.
func (w Wiki) validateDoc(id string, c Check, strict bool) []error {
	d := w[id]
	if d.User() {
		return nil
	}
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf(rule+": "+format, args...))
	}
	kind := d.KindOf()
	if !c.allows(kind) {
		fail("K1", "document %q is a %s, which this wiki does not hold", id, kind)
	}
	if !validID.MatchString(id) {
		fail("W1", "document id %q must match %s", id, validID)
	}
	if d.ID != id {
		fail("W1", "document %s.md says id %q; the id is the file name", id, d.ID)
	}
	if strict && c.Taken[strings.ToLower(id)] {
		fail("W1", "another wiki of this store uses the id %q", id)
	}
	if strings.TrimSpace(d.Name) == "" {
		fail("W2", "document %q has no name", id)
	}
	desc := strings.TrimSpace(d.Description)
	switch {
	case desc == "":
		fail("W2", "document %q has no description", id)
	case len([]rune(desc)) > MaxDescription:
		fail("W2", "document %q: the description has %d characters; at most %d", id, len([]rune(desc)), MaxDescription)
	case strings.Contains(desc, "\n"):
		fail("W2", "document %q: the description must be one sentence on one line", id)
	}
	if len(d.Interfaces) > 0 && kind != Feature {
		fail("W3", "document %q is a %s; only a feature names interfaces", id, kind)
	}
	if c.Interfaces != nil {
		for _, name := range d.Interfaces {
			if !c.Interfaces[name] {
				fail("W3", "feature %q uses interface %q, which the system neither provides nor consumes", id, name)
			}
		}
	}
	if d.Key && kind != Story {
		fail("K3", "document %q is a %s; only a story can be key", id, kind)
	}
	if text, ok := section(d.Body, Purpose); !ok {
		fail("W4", "document %q has no ## Purpose section", id)
	} else if strings.TrimSpace(text) == "" {
		fail("W4", "document %q: the ## Purpose section is empty", id)
	}
	if _, err := Links(d.Body); err != nil {
		fail("K1", "document %q: %v", id, err)
	}
	for _, n := range d.Neighbors() {
		switch {
		case n == id:
			fail("K2", "document %q links itself", id)
		case strict && !w.linkable(n, c):
			fail("K2", "document %q links %q, which is not a document it can link", id, n)
		}
	}
	return errs
}

// Warnings lists what the rules allow but the author should look at, and
// what a hand edit broke (rule N4).
func (w Wiki) Warnings(c Check) []string {
	var out []string
	features := 0
	for _, id := range w.IDs() {
		d := w[id]
		switch {
		case d.User():
			if d.Kind != "" && d.KindOf() != d.Kind {
				out = append(out, fmt.Sprintf("document %q has the kind %q, which hikari does not know; it is a note", id, d.Kind))
			}
			if p := UserNameProblem(id); p != "" {
				out = append(out, fmt.Sprintf("document %q: %s", id, p))
			}
			if c.Taken[strings.ToLower(id)] {
				out = append(out, fmt.Sprintf("document %q has the name of a document in another wiki; a link to it is not clear", id))
			}
			for _, n := range d.Neighbors() {
				if w[n] == nil && (c.Known == nil || !c.Known(n)) {
					out = append(out, fmt.Sprintf("document %q links %q, which is no document", id, n))
				}
			}
			if _, err := Links(d.Body); err != nil {
				out = append(out, fmt.Sprintf("document %q: %v", id, err))
			}
			continue
		case d.KindOf() == Feature:
			features++
			if !w.linksAgentDoc(id) {
				out = append(out, fmt.Sprintf("feature %q has no neighbor; link it to the features it works with, or check the feature set", id))
			}
			if c.Exists != nil {
				purpose, _ := section(d.Body, Purpose)
				for _, p := range pathsIn(purpose, c.Exists) {
					out = append(out, fmt.Sprintf("feature %q: its Purpose names the path %s; the body cites no code", id, p))
				}
			}
		}
		if c.Taken[strings.ToLower(id)] {
			out = append(out, fmt.Sprintf("document %q has an id that another wiki of this store uses", id))
		}
		for _, n := range d.Neighbors() {
			if n != id && !w.linkable(n, c) {
				out = append(out, fmt.Sprintf("document %q links %q, which is not a document it can link", id, n))
			}
		}
	}
	if features > ManyFeatures {
		out = append(out, fmt.Sprintf("the system has %d features; above %d you are listing parts, not features", features, ManyFeatures))
	}
	return out
}

// linksAgentDoc reports whether id links, or is linked by, another agent
// document of w (rule N7).
func (w Wiki) linksAgentDoc(id string) bool {
	for _, n := range w[id].Neighbors() {
		if d := w[n]; d != nil && n != id && !d.User() {
			return true
		}
	}
	for other, d := range w {
		if other != id && !d.User() && slices.Contains(d.Neighbors(), id) {
			return true
		}
	}
	return false
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
	Kind        string   `json:"kind,omitempty"`
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
	// Redirects maps each removed id that has a redirect to it.
	Redirects map[string]string `json:"redirects,omitempty"`
	// Written lists the ids whose documents the change wrote.
	Written []string `json:"written,omitempty"`
}

// Apply applies writes to a copy of w as one change and checks the result.
// It returns the new wiki, or an error that names each write and rule that
// failed, in which case w is unchanged.
func Apply(w Wiki, writes []Write, c Check, now time.Time) (Wiki, Result, error) {
	// A document keeps whole seconds, so the wiki in memory does too, and
	// a reload changes nothing.
	now = now.UTC().Truncate(time.Second)
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
		if d := next[wr.ID]; d != nil && d.User() && wr.Op != "create" {
			return nil, res, fmt.Errorf("%s: T3: %q is the user's %s; agents never change one", where, wr.ID, d.KindOf())
		}
		switch wr.Op {
		case "create":
			if next[wr.ID] != nil {
				return nil, res, fmt.Errorf("%s: document %q exists; use modify", where, wr.ID)
			}
			kind := wr.Kind
			if kind == "" && c.allows(Feature) {
				kind = Feature
			}
			switch {
			case kind == Plan || kind == Note:
				return nil, res, fmt.Errorf("%s: T3: plans and notes are the user's; agents never write one", where)
			case !slices.Contains(AgentKinds, kind):
				return nil, res, fmt.Errorf("%s: a create needs a kind: %s", where, strings.Join(c.kinds(), ", "))
			case !c.allows(kind):
				return nil, res, fmt.Errorf("%s: this wiki holds %s, not a %s", where, strings.Join(c.kinds(), ", "), kind)
			}
			if wr.Body == nil {
				return nil, res, fmt.Errorf("%s: a create needs a body", where)
			}
			if _, ok := section(*wr.Body, Notes); ok {
				return nil, res, fmt.Errorf("%s: W4: the body has a ## Notes section, which belongs to the user", where)
			}
			next[wr.ID] = &Document{
				ID: wr.ID, Kind: kind, Name: wr.Name, Description: wr.Description, Created: now.UTC(),
				Interfaces: slices.Clone(wr.Interfaces), Related: slices.Clone(wr.Related), Body: *wr.Body,
			}
			touch(wr.ID)
			res.Applied = append(res.Applied, "created "+wr.ID)
		case "modify":
			d := next[wr.ID]
			if d == nil {
				return nil, res, fmt.Errorf("%s: no document %q", where, wr.ID)
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
				return nil, res, fmt.Errorf("%s: no document %q", where, wr.ID)
			}
			if wr.To == "" {
				return nil, res, fmt.Errorf("%s: a rename needs the new id in to", where)
			}
			if next[wr.To] != nil {
				return nil, res, fmt.Errorf("%s: document %q exists", where, wr.To)
			}
			delete(next, wr.ID)
			delete(written, wr.ID)
			d.ID = wr.To
			next[wr.To] = d
			for id, other := range next {
				if !other.User() && relink(other, wr.ID, wr.To) {
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
			for old, to := range res.Redirects {
				if to == wr.ID {
					res.Redirects[old] = wr.To
				}
			}
			res.Applied = append(res.Applied, fmt.Sprintf("renamed %s to %s", wr.ID, wr.To))
		case "remove":
			if next[wr.ID] == nil {
				return nil, res, fmt.Errorf("%s: no document %q", where, wr.ID)
			}
			if wr.Redirect != "" && !next.linkable(wr.Redirect, c) {
				return nil, res, fmt.Errorf("%s: the redirect %q is not a document this wiki can link", where, wr.Redirect)
			}
			delete(next, wr.ID)
			delete(written, wr.ID)
			for id, other := range next {
				if !other.User() && relink(other, wr.ID, wr.Redirect) {
					touch(id)
				}
			}
			res.Removed = append(res.Removed, wr.ID)
			if wr.Redirect != "" {
				if res.Redirects == nil {
					res.Redirects = map[string]string{}
				}
				res.Redirects[wr.ID] = wr.Redirect
			}
			res.Applied = append(res.Applied, "removed "+wr.ID)
		default:
			return nil, res, fmt.Errorf("%s: unknown op %q; use create, modify, rename, or remove", where, wr.Op)
		}
	}
	if err := next.Validate(c, written); err != nil {
		return nil, res, err
	}
	res.Written = slices.Sorted(maps.Keys(written))
	res.Warnings = next.Warnings(c)
	return next, res, nil
}

func (c Check) kinds() []string {
	if c.Kinds == nil {
		return AgentKinds
	}
	return c.Kinds
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

// Load reads every document of dir. A file without a property header is
// a note. A file whose header does not parse, and an agent document that
// breaks a rule that every write keeps, is a problem of that file only
// (rule D4): Load keeps its version in prev when there is one, and leaves
// it out when not. broken names the files with a problem. The rules that
// only a tool write keeps are left to Warnings (rule N4).
func Load(dir string, c Check, prev Wiki, prevRaw map[string]string) (w Wiki, raw map[string]string, problems []string, broken map[string]bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	w, raw, broken = Wiki{}, map[string]string{}, map[string]bool{}
	keep := func(name, problem string) {
		problems = append(problems, fmt.Sprintf("%s.md: %s", name, problem))
		broken[name] = true
		if d := prev[name]; d != nil {
			w[name], raw[name] = d.Clone(), prevRaw[name]
		}
	}
	c.Interfaces = nil
	for _, e := range entries {
		if e.IsDir() {
			if !strings.HasPrefix(e.Name(), ".") {
				problems = append(problems, fmt.Sprintf("%s/: a folder in a wiki is not part of it", e.Name()))
			}
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || strings.HasPrefix(name, ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			keep(name, err.Error())
			continue
		}
		d, err := Parse(string(data))
		switch {
		case errors.Is(err, ErrNoHeader):
			d = &Document{Body: string(data)}
		case err != nil:
			keep(name, err.Error())
			continue
		}
		if d.User() || d.ID == "" {
			d.ID = name
		}
		one := Wiki{name: d}
		if errs := one.validateDoc(name, c, false); len(errs) > 0 {
			keep(name, errors.Join(errs...).Error())
			continue
		}
		w[name], raw[name] = d, string(data)
	}
	return w, raw, problems, broken, nil
}
