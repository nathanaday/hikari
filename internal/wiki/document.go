// Package wiki holds one wiki, of a partition or the public wiki: one markdown
// document per entry, with a property header that the server checks and a
// body for people and agents. A document has a kind: agents write features,
// principles, rules, and stories; the user writes plans and notes.
// Documents link each other with [[id]] links, also across wikis; a link
// has no direction and no kind. An agent changes its wiki through one
// change, which applies a set of writes as a whole or not at all.
package wiki

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The kinds of documents (rule K1 of the wikis spec).
const (
	Feature   = "feature"
	Principle = "principle"
	Rule      = "rule"
	Story     = "story"
	Plan      = "plan"
	Note      = "note"
)

// AgentKinds are the kinds that agents write; the others are the user's.
var AgentKinds = []string{Feature, Principle, Rule, Story}

// Kinds names every kind.
var Kinds = []string{Feature, Principle, Rule, Story, Plan, Note}

// Document is one document of a wiki: its header and its body.
type Document struct {
	ID string
	// Kind is the kind property as written. A document without one is a
	// note; KindOf says what the document is.
	Kind        string
	Name        string
	Description string
	Interfaces  []string
	// Related holds the ids that the related property links.
	Related []string
	// Status is a plan's status, which hikari shows and never reads.
	Status string
	// Created is the time of the create write. A document from before the
	// property existed has none.
	Created time.Time
	// Updated is the time of the last write of the document.
	Updated time.Time
	// DescribedCommits and DescribedAt say which commits the document
	// describes, one <repo>@<commit> for each repo. mark_map_current sets
	// them; a write leaves them alone. A document of the format before
	// records one commit without its repo.
	DescribedCommits []string
	DescribedAt      time.Time
	// Extra holds the user's properties, which no rule names, in their order.
	Extra []Property
	// Body is the markdown below the header.
	Body string
}

// Property is a header property that the user added.
type Property struct {
	Key   string
	Value *yaml.Node
}

// The properties that code knows, in the order they are written.
var known = []string{"id", "kind", "name", "description", "interfaces", "related", "key", "status", "created", "updated", "described_commits", "described_at"}

// Owned are the properties that code owns; a write may not give them.
// described_commit is the property of the format before.
var Owned = []string{"created", "updated", "described_commits", "described_commit", "described_at"}

const timeLayout = "2006-01-02T15:04:05Z"

// KindOf returns the document's kind: a document without a kind, or with a
// kind that hikari does not know, is a note (rule K2).
func (d *Document) KindOf() string {
	if slices.Contains(Kinds, d.Kind) {
		return d.Kind
	}
	return Note
}

// User reports whether the document is the user's: a plan or a note.
func (d *Document) User() bool { k := d.KindOf(); return k == Plan || k == Note }

// ErrNoHeader is the error of Parse for a text without a property header.
// In a wiki, such a file is a note.
var ErrNoHeader = errors.New("the document does not start with a property header (---)")

// Parse reads a document from its text.
func Parse(text string) (*Document, error) {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, ErrNoHeader
	}
	header, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		if h, ok2 := strings.CutSuffix(rest, "\n---"); ok2 {
			header, body, ok = h, "", true
		}
	}
	if !ok {
		return nil, errors.New("the property header has no closing ---")
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(header), &node); err != nil {
		return nil, fmt.Errorf("the property header is not valid YAML: %w", err)
	}
	d := &Document{Body: strings.TrimPrefix(body, "\n")}
	if node.Kind == 0 {
		return d, nil
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the property header must be a mapping of properties")
	}
	m := node.Content[0]
	var relatedErr error
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i].Value, m.Content[i+1]
		var err error
		switch key {
		case "id":
			d.ID, err = scalar(val)
		case "kind":
			d.Kind, err = scalar(val)
		case "key":
			// The key flag of an older story is ignored.
		case "status":
			d.Status, err = scalar(val)
		case "name":
			d.Name, err = scalar(val)
		case "description":
			d.Description, err = scalar(val)
		case "interfaces":
			d.Interfaces, err = list(val)
		case "related":
			var links []string
			if links, err = list(val); err == nil {
				for _, l := range links {
					id, lerr := linkTarget(l)
					if lerr != nil {
						relatedErr = fmt.Errorf("property related: %w", lerr)
						continue
					}
					d.Related = append(d.Related, id)
				}
			}
		case "created":
			d.Created, err = stamp(val)
		case "updated":
			d.Updated, err = stamp(val)
		case "described_commits":
			d.DescribedCommits, err = list(val)
		case "described_commit":
			var c string
			if c, err = scalar(val); err == nil && c != "" && len(d.DescribedCommits) == 0 {
				d.DescribedCommits = []string{c}
			}
		case "described_at":
			d.DescribedAt, err = stamp(val)
		default:
			d.Extra = append(d.Extra, Property{Key: key, Value: val})
		}
		if err != nil {
			return nil, fmt.Errorf("property %s: %w", key, err)
		}
	}
	// A user document's links are its own business (rule N1).
	if relatedErr != nil && !d.User() {
		return nil, relatedErr
	}
	return d, nil
}

func scalar(n *yaml.Node) (string, error) {
	if n.Kind != yaml.ScalarNode {
		return "", errors.New("must be one value")
	}
	return n.Value, nil
}

func list(n *yaml.Node) ([]string, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!null" || n.Value == "" {
			return nil, nil
		}
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		var out []string
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, errors.New("must be a list of values")
			}
			out = append(out, item.Value)
		}
		return out, nil
	}
	return nil, errors.New("must be a list of values")
}

func stamp(n *yaml.Node) (time.Time, error) {
	s, err := scalar(n)
	if err != nil || s == "" {
		return time.Time{}, err
	}
	for _, layout := range []string{timeLayout, time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a time", s)
}

// linkTarget returns the id that a related entry names: [[id]], [[id|text]],
// or a bare id.
func linkTarget(s string) (string, error) {
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "[["); ok {
		inner, ok = strings.CutSuffix(inner, "]]")
		if !ok {
			return "", fmt.Errorf("%q is not a link", s)
		}
		target, _, _ := strings.Cut(inner, "|")
		if strings.Contains(target, "#") {
			return "", fmt.Errorf("K1: link %q names a heading; link the document", s)
		}
		return Target(target), nil
	}
	return s, nil
}

// Target returns the document that a link target names: its last path
// segment, without .md (rule N1).
func Target(t string) string {
	t = strings.TrimSpace(t)
	if i := strings.LastIndex(t, "/"); i >= 0 {
		t = t[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(t, ".md"))
}

// Render writes the document as text: the known properties in order, then
// the user's, then the body.
func (d *Document) Render() string {
	m := &yaml.Node{Kind: yaml.MappingNode}
	add := func(key string, val *yaml.Node) {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
	}
	str := func(s string) *yaml.Node {
		n := &yaml.Node{Kind: yaml.ScalarNode, Value: s}
		if needsQuotes(s) {
			n.Style = yaml.DoubleQuotedStyle
		}
		return n
	}
	flow := func(items []string) *yaml.Node {
		n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, it := range items {
			n.Content = append(n.Content, str(it))
		}
		return n
	}
	add("id", str(d.ID))
	if d.Kind != "" {
		add("kind", str(d.Kind))
	}
	add("name", str(d.Name))
	add("description", str(d.Description))
	if len(d.Interfaces) > 0 {
		add("interfaces", flow(d.Interfaces))
	}
	if len(d.Related) > 0 {
		links := make([]string, len(d.Related))
		for i, id := range d.Related {
			links[i] = "[[" + id + "]]"
		}
		add("related", flow(links))
	}
	if d.Status != "" {
		add("status", str(d.Status))
	}
	if !d.Created.IsZero() {
		add("created", str(formatTime(d.Created)))
	}
	add("updated", str(formatTime(d.Updated)))
	if len(d.DescribedCommits) > 0 || !d.DescribedAt.IsZero() || d.Kind == Feature || d.Kind == "" {
		add("described_commits", flow(d.DescribedCommits))
		add("described_at", str(formatTime(d.DescribedAt)))
	}
	for _, p := range d.Extra {
		add(p.Key, p.Value)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		// A mapping of scalars and sequences always encodes.
		panic(err)
	}
	enc.Close()
	b.WriteString("---\n\n")
	body := strings.TrimRight(d.Body, "\n")
	b.WriteString(body)
	if body != "" {
		b.WriteString("\n")
	}
	return b.String()
}

// needsQuotes reports whether a plain scalar would read back as something
// other than text, or would not parse at all. A time stays plain, so that
// Obsidian reads it as a date.
func needsQuotes(s string) bool {
	switch {
	case s == "", s != strings.TrimSpace(s), strings.ContainsAny(s, "\n\"'"):
		return true
	case strings.ContainsAny(s[:1], "[]{}&*!|>%@`#-?,"):
		return true
	case strings.Contains(s, ": "), strings.Contains(s, " #"), strings.HasSuffix(s, ":"):
		return true
	}
	tag := (&yaml.Node{Kind: yaml.ScalarNode, Value: s}).ShortTag()
	return tag != "!!str" && tag != "!!timestamp"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeLayout)
}

// Clone returns a deep copy of d.
func (d *Document) Clone() *Document {
	c := *d
	c.Interfaces = slices.Clone(d.Interfaces)
	c.Related = slices.Clone(d.Related)
	c.DescribedCommits = slices.Clone(d.DescribedCommits)
	c.Extra = slices.Clone(d.Extra)
	return &c
}

var linkRE = regexp.MustCompile(`\[\[([^\[\]|]*)(\|[^\[\]]*)?\]\]`)

// Links returns the targets of every link in the body (rule K1). It returns
// an error for a link that names a heading.
func Links(body string) ([]string, error) {
	var out []string
	var errs []error
	for _, m := range linkRE.FindAllStringSubmatch(body, -1) {
		if strings.Contains(m[1], "#") {
			errs = append(errs, fmt.Errorf("K1: link %q names a heading; link the document", m[0]))
			continue
		}
		out = append(out, Target(m[1]))
	}
	return out, errors.Join(errs...)
}

// Neighbors returns every document that d links, in the body and in
// related, in order of first mention and without repeats.
func (d *Document) Neighbors() []string {
	var out []string
	add := func(id string) {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	body, _ := Links(d.Body)
	for _, id := range body {
		add(id)
	}
	for _, id := range d.Related {
		add(id)
	}
	return out
}

// rewriteLinks replaces every link to old in the body. With to set, the
// link points at to and keeps its text; without, the link becomes its
// text, or the old id.
func rewriteLinks(body, old, to string) string {
	return linkRE.ReplaceAllStringFunc(body, func(link string) string {
		m := linkRE.FindStringSubmatch(link)
		if strings.Contains(m[1], "#") || Target(m[1]) != old {
			return link
		}
		if to != "" {
			return "[[" + to + m[2] + "]]"
		}
		if m[2] != "" {
			return m[2][1:]
		}
		return old
	})
}

var sectionRE = regexp.MustCompile(`(?m)^## +(.+?)[ \t]*$`)

// section returns the text of the level-two section titled name, without
// its heading, and whether the section exists.
func section(body, name string) (string, bool) {
	locs := sectionRE.FindAllStringSubmatchIndex(body, -1)
	for i, loc := range locs {
		if body[loc[2]:loc[3]] != name {
			continue
		}
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return body[loc[1]:end], true
	}
	return "", false
}

// withoutSection returns the body without the level-two section titled name.
func withoutSection(body, name string) string {
	locs := sectionRE.FindAllStringSubmatchIndex(body, -1)
	for i, loc := range locs {
		if body[loc[2]:loc[3]] != name {
			continue
		}
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return strings.TrimRight(body[:loc[0]], "\n") + "\n" + body[end:]
	}
	return body
}

// Notes is the section that belongs to the user.
const Notes = "Notes"

// Purpose is the section that every document must have.
const Purpose = "Purpose"
