package tasks

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Version is the schema version of the task and queue documents (B5).
const Version = 1

// The stages of a task, and the kinds of the module's documents (rules TK1
// and TK5).
const (
	StageStub  = "stub"
	StageScope = "scope"
	KindTask   = "task"
	KindQueue  = "queue"
)

// The statuses of a scope and of a part (E2). A draft scope has no status.
const (
	Queued     = "queued"
	Waiting    = "waiting"
	Started    = "started"
	Blocked    = "blocked"
	Testing    = "testing"
	Validating = "validating"
	Done       = "done"
)

// The states of a queue (decision 7).
const (
	Running = "running"
	Paused  = "paused"
)

// Task is one document of the pipeline: a stub or a scope.
type Task struct {
	Version    int
	ID         string
	Kind       string
	Stage      string
	Name       string
	Author     string
	Created    time.Time
	Updated    time.Time
	Partitions []string
	// Features and Needs hold the ids that their [[id]] links name.
	Features []string
	Needs    []string
	// Status, Queue, and Parts are hikari's (rule TK7). Block is the
	// reason of a block that the scope itself has, not one of its parts.
	Status string
	Block  string
	// Wait says why a waiting scope cannot start yet.
	Wait  string
	Queue string
	Parts []Part
	// Extra holds the properties that hikari does not name, in their
	// order (B4).
	Extra []Property
	Body  string
}

// Part is one partition's share of a scope, with its status.
type Part struct {
	Partition string
	Status    string
}

// Queue is an ordered list of scopes (rule TK5).
type Queue struct {
	Version int
	ID      string
	Kind    string
	Name    string
	State   string
	Created time.Time
	Updated time.Time
	// Scopes holds the ids of the scopes, in order.
	Scopes []string
	Extra  []Property
	Body   string
}

// Property is a header property that hikari does not name.
type Property struct {
	Key   string
	Value *yaml.Node
}

var (
	taskKeys  = []string{"version", "id", "kind", "stage", "name", "author", "created", "updated", "partitions", "features", "needs", "status", "block", "wait", "queue", "parts"}
	queueKeys = []string{"version", "id", "kind", "name", "state", "created", "updated", "scopes"}
	validID   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	link      = regexp.MustCompile(`^\[\[([^\]|#]+)(?:[|#][^\]]*)?\]\]$`)
)

const timeLayout = "2006-01-02T15:04:05Z"

// split cuts a document into its header properties and its body.
func split(text string) (*yaml.Node, string, error) {
	rest, ok := strings.CutPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "---\n")
	if !ok {
		return nil, "", errors.New("the document does not start with a property header (---)")
	}
	header, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		if h, ok2 := strings.CutSuffix(rest, "\n---"); ok2 {
			header, body, ok = h, "", true
		} else if h, ok2 := strings.CutPrefix(rest, "---\n"); ok2 {
			header, body, ok = "", h, true
		}
	}
	if !ok {
		return nil, "", errors.New("the property header has no closing ---")
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(header), &node); err != nil {
		return nil, "", fmt.Errorf("the property header is not valid YAML: %w", err)
	}
	m := &yaml.Node{Kind: yaml.MappingNode}
	if node.Kind != 0 {
		if node.Kind != yaml.DocumentNode || len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
			return nil, "", errors.New("the property header must be a mapping of properties")
		}
		m = node.Content[0]
	}
	return m, strings.TrimPrefix(body, "\n"), nil
}

func scalar(key string, n *yaml.Node) (string, error) {
	if n.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s must be text", key)
	}
	return n.Value, nil
}

func number(key string, n *yaml.Node) (int, error) {
	var v int
	if n.Kind != yaml.ScalarNode || n.Decode(&v) != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return v, nil
}

func timestamp(key string, n *yaml.Node) (time.Time, error) {
	s, err := scalar(key, n)
	if err != nil || s == "" {
		return time.Time{}, err
	}
	for _, layout := range []string{timeLayout, time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%s must be a time such as 2026-10-27T09:00:00Z", key)
}

// list reads a list of text, or one text as a list of one.
func list(key string, n *yaml.Node) ([]string, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value == "" || n.Tag == "!!null" {
			return nil, nil
		}
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("%s must be a list of text", key)
			}
			out = append(out, item.Value)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s must be a list", key)
}

// links reads a list of [[id]] links, or of bare ids, as ids.
func links(key string, n *yaml.Node) ([]string, error) {
	items, err := list(key, n)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if m := link.FindStringSubmatch(it); m != nil {
			it = strings.TrimSpace(m[1])
		}
		out = append(out, it)
	}
	return out, nil
}

// ParseTask reads a task from its text.
func ParseTask(text string) (*Task, error) {
	m, body, err := split(text)
	if err != nil {
		return nil, err
	}
	t := &Task{Body: body}
	var errs []error
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i].Value, m.Content[i+1]
		var err error
		switch key {
		case "version":
			t.Version, err = number(key, val)
		case "id":
			t.ID, err = scalar(key, val)
		case "kind":
			t.Kind, err = scalar(key, val)
		case "stage":
			t.Stage, err = scalar(key, val)
		case "name":
			t.Name, err = scalar(key, val)
		case "author":
			t.Author, err = scalar(key, val)
		case "created":
			t.Created, err = timestamp(key, val)
		case "updated":
			t.Updated, err = timestamp(key, val)
		case "partitions":
			t.Partitions, err = list(key, val)
		case "features":
			t.Features, err = links(key, val)
		case "needs":
			t.Needs, err = links(key, val)
		case "status":
			t.Status, err = scalar(key, val)
		case "block":
			t.Block, err = scalar(key, val)
		case "wait":
			t.Wait, err = scalar(key, val)
		case "queue":
			var q []string
			if q, err = links(key, val); err == nil && len(q) > 0 {
				t.Queue = q[0]
			}
		case "parts":
			t.Parts, err = parts(val)
		default:
			t.Extra = append(t.Extra, Property{Key: key, Value: val})
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return t, errors.Join(errs...)
}

func parts(n *yaml.Node) ([]Part, error) {
	if n.Kind == yaml.ScalarNode && (n.Value == "" || n.Tag == "!!null") {
		return nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, errors.New("parts must map each partition to its status")
	}
	var out []Part
	for i := 0; i+1 < len(n.Content); i += 2 {
		s, err := scalar("parts", n.Content[i+1])
		if err != nil {
			return nil, errors.New("parts must map each partition to its status")
		}
		out = append(out, Part{Partition: n.Content[i].Value, Status: s})
	}
	return out, nil
}

// ParseQueue reads a queue from its text.
func ParseQueue(text string) (*Queue, error) {
	m, body, err := split(text)
	if err != nil {
		return nil, err
	}
	q := &Queue{Body: body}
	var errs []error
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i].Value, m.Content[i+1]
		var err error
		switch key {
		case "version":
			q.Version, err = number(key, val)
		case "id":
			q.ID, err = scalar(key, val)
		case "kind":
			q.Kind, err = scalar(key, val)
		case "name":
			q.Name, err = scalar(key, val)
		case "state":
			q.State, err = scalar(key, val)
		case "created":
			q.Created, err = timestamp(key, val)
		case "updated":
			q.Updated, err = timestamp(key, val)
		case "scopes":
			q.Scopes, err = links(key, val)
		default:
			q.Extra = append(q.Extra, Property{Key: key, Value: val})
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return q, errors.Join(errs...)
}

// header writes properties in order, then the extra ones.
type header struct{ m *yaml.Node }

func newHeader() *header { return &header{m: &yaml.Node{Kind: yaml.MappingNode}} }

func (h *header) add(key string, val *yaml.Node) {
	h.m.Content = append(h.m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}

func str(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: s}
	// A value that YAML would read as another type, or that starts a
	// structure, is quoted.
	if n.ShortTag() != "!!str" || strings.ContainsAny(s, ":#[]{}") || s == "" {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

func num(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprint(v), Tag: "!!int"}
}

func seq(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, it := range items {
		n.Content = append(n.Content, str(it))
	}
	return n
}

func linkSeq(ids []string) *yaml.Node {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "[[" + id + "]]"
	}
	return seq(out)
}

func (h *header) text(extra []Property, body string) string {
	for _, p := range extra {
		h.add(p.Key, p.Value)
	}
	out, _ := yaml.Marshal(h.m)
	text := "---\n" + string(out) + "---\n"
	if body != "" {
		text += "\n" + strings.TrimLeft(body, "\n")
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
	}
	return text
}

// Format writes a task as its document.
func (t *Task) Format() string {
	h := newHeader()
	h.add("version", num(t.Version))
	h.add("id", str(t.ID))
	h.add("kind", str(t.Kind))
	h.add("stage", str(t.Stage))
	h.add("name", str(t.Name))
	if t.Author != "" {
		h.add("author", str(t.Author))
	}
	if !t.Created.IsZero() {
		h.add("created", str(t.Created.UTC().Format(timeLayout)))
	}
	if !t.Updated.IsZero() {
		h.add("updated", str(t.Updated.UTC().Format(timeLayout)))
	}
	if t.Stage == StageScope || len(t.Partitions) > 0 {
		h.add("partitions", seq(t.Partitions))
	}
	if t.Stage == StageScope || len(t.Features) > 0 {
		h.add("features", linkSeq(t.Features))
	}
	if len(t.Needs) > 0 {
		h.add("needs", linkSeq(t.Needs))
	}
	if t.Status != "" {
		h.add("status", str(t.Status))
	}
	if t.Block != "" {
		h.add("block", str(t.Block))
	}
	if t.Wait != "" {
		h.add("wait", str(t.Wait))
	}
	if t.Queue != "" {
		h.add("queue", str("[["+t.Queue+"]]"))
	}
	if len(t.Parts) > 0 {
		m := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		for _, p := range t.Parts {
			m.Content = append(m.Content, str(p.Partition), str(p.Status))
		}
		h.add("parts", m)
	}
	return h.text(t.Extra, t.Body)
}

// Format writes a queue as its document.
func (q *Queue) Format() string {
	h := newHeader()
	h.add("version", num(q.Version))
	h.add("id", str(q.ID))
	h.add("kind", str(q.Kind))
	h.add("name", str(q.Name))
	h.add("state", str(q.State))
	if !q.Created.IsZero() {
		h.add("created", str(q.Created.UTC().Format(timeLayout)))
	}
	if !q.Updated.IsZero() {
		h.add("updated", str(q.Updated.UTC().Format(timeLayout)))
	}
	h.add("scopes", linkSeq(q.Scopes))
	return h.text(q.Extra, q.Body)
}

// PartStatus returns the status of partition's part, or "".
func (t *Task) PartStatus(partition string) string {
	for _, p := range t.Parts {
		if p.Partition == partition {
			return p.Status
		}
	}
	return ""
}

// SetPart sets the status of partition's part, in the order of Partitions.
func (t *Task) SetPart(partition, status string) {
	m := map[string]string{}
	for _, p := range t.Parts {
		m[p.Partition] = p.Status
	}
	m[partition] = status
	t.Parts = nil
	for _, name := range t.Partitions {
		if s, ok := m[name]; ok {
			t.Parts = append(t.Parts, Part{Partition: name, Status: s})
		}
	}
}

// Slug makes an id from a name: lowercase letters, digits, and dashes.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
}

// The sections of a scope's body (rule TK3).
const (
	SectionRequest  = "Request"
	SectionGoal     = "Goal"
	SectionParts    = "Parts"
	SectionDoneWhen = "Done when"
	SectionAnswers  = "Answers"
	SectionLog      = "Log"
)

// Section returns the text under "## name" in body, without its heading.
func Section(body, name string) (string, bool) {
	start, end, ok := sectionBounds(body, name)
	if !ok {
		return "", false
	}
	text := body[start:end]
	_, rest, _ := strings.Cut(text, "\n")
	return strings.Trim(rest, "\n"), true
}

func sectionBounds(body, name string) (int, int, bool) {
	heading := "## " + name
	lines := strings.SplitAfter(body, "\n")
	pos, start := 0, -1
	for _, l := range lines {
		trimmed := strings.TrimRight(l, "\n")
		if start < 0 && strings.EqualFold(strings.TrimSpace(trimmed), heading) {
			start = pos
		} else if start >= 0 && strings.HasPrefix(trimmed, "## ") {
			return start, pos, true
		}
		pos += len(l)
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, len(body), true
}

// appendLog adds one line to the Log section, which it makes when missing.
func appendLog(body, line string) string {
	_, end, ok := sectionBounds(body, SectionLog)
	if !ok {
		body = strings.TrimRight(body, "\n")
		if body != "" {
			body += "\n\n"
		}
		return body + "## " + SectionLog + "\n\n- " + line + "\n"
	}
	before := strings.TrimRight(body[:end], "\n")
	return before + "\n- " + line + "\n" + strings.TrimLeft(body[end:], "\n")
}

// scopeBody writes the sections of a scope, in the order of rule TK3.
func scopeBody(request, goal string, parts map[string]string, order []string, doneWhen, answers, log string) string {
	var b strings.Builder
	sec := func(name, text string) {
		fmt.Fprintf(&b, "## %s\n\n", name)
		if t := strings.TrimSpace(text); t != "" {
			b.WriteString(t + "\n\n")
		}
	}
	sec(SectionRequest, request)
	sec(SectionGoal, goal)
	fmt.Fprintf(&b, "## %s\n\n", SectionParts)
	for _, p := range order {
		fmt.Fprintf(&b, "### %s\n\n", p)
		if t := strings.TrimSpace(parts[p]); t != "" {
			b.WriteString(t + "\n\n")
		}
	}
	sec(SectionDoneWhen, doneWhen)
	sec(SectionAnswers, answers)
	if strings.TrimSpace(log) != "" {
		sec(SectionLog, log)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// active reports whether a status holds its partitions: a started scope
// that is not done (D4).
func active(status string) bool {
	return slices.Contains([]string{Started, Blocked, Testing, Validating}, status)
}
