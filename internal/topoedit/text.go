// Package topoedit changes a topology file through operations, such as
// rename or set_orchestration, and splices each change into the text so that
// every byte outside the changed nodes stays (rule TE1 of the topology API
// spec). It also reads the source model, lints a text, and finds the
// places that a rename can propagate to.
package topoedit

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// doc is a topology file's text with its node tree. Every primitive edit
// returns a new text, and the next edit parses it again.
type doc struct {
	text  []byte
	root  *yaml.Node
	lines []int // the byte offset of the start of each line
}

func parse(text []byte) (*doc, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(text, &n); err != nil {
		return nil, err
	}
	d := &doc{text: text, lines: []int{0}}
	for i, b := range text {
		if b == '\n' {
			d.lines = append(d.lines, i+1)
		}
	}
	if len(n.Content) == 0 {
		d.root = &yaml.Node{Kind: yaml.MappingNode}
	} else {
		d.root = n.Content[0]
	}
	if d.root.Kind != yaml.MappingNode {
		return nil, errors.New("the topology file is not a mapping")
	}
	return d, nil
}

// offset returns the byte offset of a 1-based line and character column.
func (d *doc) offset(line, col int) int {
	if line-1 >= len(d.lines) {
		return len(d.text)
	}
	off := d.lines[line-1]
	for i := 1; i < col && off < len(d.text) && d.text[off] != '\n'; i++ {
		_, size := utf8.DecodeRune(d.text[off:])
		off += size
	}
	return off
}

// lineEnd returns the offset just past the newline of the line that holds
// off.
func (d *doc) lineEnd(off int) int {
	if i := bytes.IndexByte(d.text[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(d.text)
}

func (d *doc) lineOf(off int) int {
	line := 1
	for i, start := range d.lines {
		if start <= off {
			line = i + 1
		}
	}
	return line
}

// indent returns the indentation of a line, or -1 for a line that holds
// only spaces.
func (d *doc) indent(line int) int {
	start := d.lines[line-1]
	end := d.lineEnd(start)
	s := strings.TrimRight(string(d.text[start:end]), "\r\n")
	trimmed := strings.TrimLeft(s, " ")
	if trimmed == "" {
		return -1
	}
	return len(s) - len(trimmed)
}

func (d *doc) isComment(line int) bool {
	start := d.lines[line-1]
	return strings.HasPrefix(strings.TrimLeft(string(d.text[start:d.lineEnd(start)]), " "), "#")
}

func (d *doc) splice(start, end int, with string) []byte {
	out := make([]byte, 0, len(d.text)+len(with))
	out = append(out, d.text[:start]...)
	out = append(out, with...)
	return append(out, d.text[end:]...)
}

// path is a parsed YAML path: keys and list indexes.
type path []string

var pathPart = regexp.MustCompile(`([^.\[\]]+)|\[(\d+)\]`)

func parsePath(p string) path {
	var out path
	for _, m := range pathPart.FindAllStringSubmatch(p, -1) {
		if m[2] != "" {
			out = append(out, "["+m[2]+"]")
		} else {
			out = append(out, m[1])
		}
	}
	return out
}

// entry is a node found by path: the node, and for a mapping value its key
// and the mapping that holds it.
type entry struct {
	key    *yaml.Node
	value  *yaml.Node
	parent *yaml.Node
	index  int // the index of the key, or of the item, in parent.Content
}

// find returns the entry at p, following no alias and no merge key.
func (d *doc) find(p path) (entry, bool) {
	e := entry{value: d.root}
	for _, part := range p {
		node := e.value
		if strings.HasPrefix(part, "[") {
			i, _ := strconv.Atoi(strings.Trim(part, "[]"))
			if node.Kind != yaml.SequenceNode || i >= len(node.Content) {
				return entry{}, false
			}
			e = entry{value: node.Content[i], parent: node, index: i}
			continue
		}
		if node.Kind != yaml.MappingNode {
			return entry{}, false
		}
		found := false
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == part {
				e = entry{key: node.Content[i], value: node.Content[i+1], parent: node, index: i}
				found = true
				break
			}
		}
		if !found {
			return entry{}, false
		}
	}
	return e, true
}

func isFlow(n *yaml.Node) bool {
	return (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) && n.Style&yaml.FlowStyle != 0
}

// flowSpan returns the span of a flow collection, from its bracket to the
// matching one.
func (d *doc) flowSpan(n *yaml.Node) (int, int, error) {
	start := d.offset(n.Line, n.Column)
	for start < len(d.text) && d.text[start] != '{' && d.text[start] != '[' {
		start++
	}
	depth, quote := 0, byte(0)
	for i := start; i < len(d.text); i++ {
		c := d.text[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0 && c == quote:
			if quote == '\'' && i+1 < len(d.text) && d.text[i+1] == '\'' {
				i++
			} else {
				quote = 0
			}
		case quote != 0:
		case c == '"' || c == '\'':
			quote = c
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return start, i + 1, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("line %d: the flow collection does not end", n.Line)
}

// scalarSpan returns the span of a scalar's text. keyIndent is the
// indentation of the key that holds a block scalar.
func (d *doc) scalarSpan(n *yaml.Node, keyIndent int) (int, int, error) {
	start := d.offset(n.Line, n.Column)
	switch n.Style {
	case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle:
		q := d.text[start]
		for i := start + 1; i < len(d.text); i++ {
			switch {
			case q == '"' && d.text[i] == '\\':
				i++
			case d.text[i] == q && q == '\'' && i+1 < len(d.text) && d.text[i+1] == '\'':
				i++
			case d.text[i] == q:
				return start, i + 1, nil
			}
		}
	case yaml.LiteralStyle, yaml.FoldedStyle:
		end := d.lineEnd(start)
		for line := d.lineOf(start) + 1; line <= len(d.lines) && d.lines[line-1] < len(d.text); line++ {
			if in := d.indent(line); in >= 0 && in <= keyIndent {
				break
			}
			end = d.lineEnd(d.lines[line-1])
		}
		// Trailing blank lines stay outside the scalar.
		for end > start && d.indent(d.lineOf(end-1)) < 0 && d.lineOf(end-1) > d.lineOf(start) {
			end = d.lines[d.lineOf(end-1)-1]
		}
		return start, end, nil
	default:
		if n.Tag == "!!null" && n.Value == "" {
			return start, start, nil
		}
		if n.Tag == "!!null" && n.Value == "~" {
			return start, start + 1, nil
		}
		if bytes.HasPrefix(d.text[start:], []byte(n.Value)) {
			return start, start + len(n.Value), nil
		}
	}
	return 0, 0, fmt.Errorf("line %d: cannot find the text of %q", n.Line, n.Value)
}

// blockEnd returns the end of the lines that belong to a block entry
// whose first line is first and whose key is at indentation keyIndent: the
// lines after it that are blank or indented deeper, and the items of a
// block sequence at the key's own indentation. Trailing blank lines and
// comments stay outside.
func (d *doc) blockEnd(first, keyIndent int, seqAtKey bool) int {
	last := first
	for line := first + 1; line <= len(d.lines) && d.lines[line-1] < len(d.text); line++ {
		in := d.indent(line)
		switch {
		case in < 0:
			continue
		case in > keyIndent && !d.isComment(line):
			last = line
		case in > keyIndent:
			continue
		case seqAtKey && in == keyIndent && bytes.HasPrefix(d.text[d.lines[line-1]+in:], []byte("- ")):
			last = line
		default:
			return d.lineEnd(d.lines[last-1])
		}
	}
	return d.lineEnd(d.lines[last-1])
}

// entrySpan returns the lines of a block mapping entry or a block
// sequence item: from the start of its first line to the end of its last.
func (d *doc) entrySpan(e entry) (int, int) {
	var first *yaml.Node
	if e.key != nil {
		first = e.key
	} else {
		first = e.value
	}
	lineStart := d.lines[first.Line-1]
	indent := first.Column - 1
	if e.key == nil {
		// An item: its dash is at the item's column minus two.
		indent = max(0, first.Column-3)
	}
	seqAtKey := e.key != nil && e.value.Kind == yaml.SequenceNode && !isFlow(e.value)
	return lineStart, d.blockEnd(first.Line, indent, seqAtKey)
}

// render writes a node as text: a flow collection on one line, or a
// scalar.
func render(n *yaml.Node) (string, error) {
	n = clean(n)
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// clean returns a copy of n in flow style, without the !!merge tag that
// the encoder would otherwise write.
func clean(n *yaml.Node) *yaml.Node {
	c := *n
	if c.Tag == "!!merge" {
		c.Tag = ""
	}
	if c.Kind == yaml.MappingNode || c.Kind == yaml.SequenceNode {
		c.Style |= yaml.FlowStyle
		c.Content = make([]*yaml.Node, len(n.Content))
		for i, k := range n.Content {
			c.Content[i] = clean(k)
		}
	}
	c.HeadComment, c.LineComment, c.FootComment = "", "", ""
	return &c
}

// scalar makes a scalar node of a string, which the encoder quotes when it
// must.
func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func list(vs []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, v := range vs {
		n.Content = append(n.Content, scalar(v))
	}
	return n
}

func mapping(kv ...any) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	for i := 0; i+1 < len(kv); i += 2 {
		v, ok := kv[i+1].(*yaml.Node)
		if !ok {
			v = scalar(kv[i+1].(string))
		}
		n.Content = append(n.Content, scalar(kv[i].(string)), v)
	}
	return n
}

// valueText renders v for the place after "key:" at keyIndent: a scalar
// with a newline becomes a literal block, a block collection the lines
// below the key, and anything else one line. The text starts with the
// space or the newline that follows the colon.
func valueText(v *yaml.Node, keyIndent int) (string, error) {
	if (v.Kind == yaml.MappingNode || v.Kind == yaml.SequenceNode) && v.Style&yaml.FlowStyle == 0 && len(v.Content) > 0 {
		return blockText(v, keyIndent+2)
	}
	text, err := lineText(v, keyIndent)
	return " " + text, err
}

// blockText renders a block collection as lines at indent, each item or
// value on one line.
func blockText(v *yaml.Node, indent int) (string, error) {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	if v.Kind == yaml.SequenceNode {
		for _, item := range v.Content {
			text, err := flowValue(item)
			if err != nil {
				return "", err
			}
			b.WriteString("\n" + pad + "- " + text)
		}
		return b.String(), nil
	}
	for i := 0; i+1 < len(v.Content); i += 2 {
		text, err := valueText(v.Content[i+1], indent)
		if err != nil {
			return "", err
		}
		b.WriteString("\n" + pad + v.Content[i].Value + ":" + text)
	}
	return b.String(), nil
}

// lineText renders a value that is not a block collection.
func lineText(v *yaml.Node, keyIndent int) (string, error) {
	if v.Kind == yaml.ScalarNode && strings.Contains(v.Value, "\n") {
		chomp := "|-"
		body := v.Value
		if strings.HasSuffix(body, "\n") {
			chomp, body = "|", strings.TrimSuffix(body, "\n")
		}
		pad := strings.Repeat(" ", keyIndent+2)
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			if l != "" {
				lines[i] = pad + l
			}
		}
		return chomp + "\n" + strings.Join(lines, "\n"), nil
	}
	return render(v)
}

// flowValue renders v inside a flow collection, where a newline must be
// escaped.
func flowValue(v *yaml.Node) (string, error) {
	if v.Kind == yaml.ScalarNode && strings.Contains(v.Value, "\n") {
		c := *v
		c.Style = yaml.DoubleQuotedStyle
		return render(&c)
	}
	return render(v)
}

// The primitive edits (decision 3 of the topology API spec). Each returns
// the new text.

// rewriteFlow writes the flow collection of e again, after fn changed a
// copy of it.
func (d *doc) rewriteFlow(n *yaml.Node, fn func(c *yaml.Node)) ([]byte, error) {
	start, end, err := d.flowSpan(n)
	if err != nil {
		return nil, err
	}
	c := *n
	c.Content = append([]*yaml.Node(nil), n.Content...)
	fn(&c)
	text, err := render(&c)
	if err != nil {
		return nil, err
	}
	return d.splice(start, end, text), nil
}

// setValue sets the value of key in the mapping at p, adding the key when
// it is missing. A new key goes first when first is set, and else last.
func (d *doc) setValue(p path, key string, v *yaml.Node, first bool) ([]byte, error) {
	before := ""
	if first {
		before = "\x00first"
	}
	return d.setValueBefore(p, key, v, before)
}

// setValueBefore is setValue for a new key that goes before the key
// before, or last when the mapping lacks it. The before key "\x00first"
// means the first key.
func (d *doc) setValueBefore(p path, key string, v *yaml.Node, before string) ([]byte, error) {
	m, ok := d.find(p)
	if !ok {
		return nil, fmt.Errorf("%s does not exist", strings.Join(p, "."))
	}
	if m.value.Kind == yaml.ScalarNode && m.value.Tag == "!!null" && m.key != nil {
		// key: (nothing) becomes key: {key: v}.
		return d.replaceValue(m, mapping(key, v))
	}
	if m.value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a mapping", strings.Join(p, "."))
	}
	if e, ok := d.find(append(append(path{}, p...), key)); ok {
		return d.replaceValue(e, v)
	}
	at := -1 // the index of the key that the new key goes before
	for i := 0; i+1 < len(m.value.Content); i += 2 {
		if before == "\x00first" || m.value.Content[i].Value == before {
			at = i
			break
		}
	}
	if isFlow(m.value) {
		return d.rewriteFlow(m.value, func(c *yaml.Node) {
			kv := []*yaml.Node{scalar(key), v}
			if at >= 0 {
				c.Content = append(append(append([]*yaml.Node{}, c.Content[:at]...), kv...), c.Content[at:]...)
			} else {
				c.Content = append(c.Content, kv...)
			}
		})
	}
	if len(m.value.Content) == 0 {
		return nil, fmt.Errorf("%s is an empty block mapping", strings.Join(p, "."))
	}
	keyNode := m.value.Content[0]
	indent := keyNode.Column - 1
	text, err := valueText(v, indent)
	if err != nil {
		return nil, err
	}
	line := strings.Repeat(" ", indent) + key + ":" + text + "\n"
	if at >= 0 {
		// The new key goes above the comment lines over the key it
		// precedes, which describe that key.
		line0 := m.value.Content[at].Line
		for line0 > 1 && d.isComment(line0-1) && d.indent(line0-1) == indent {
			line0--
		}
		return d.splice(d.lines[line0-1], d.lines[line0-1], line), nil
	}
	lastKey := m.value.Content[len(m.value.Content)-2]
	_, end := d.entrySpan(entry{key: lastKey, value: m.value.Content[len(m.value.Content)-1]})
	if end > 0 && d.text[end-1] != '\n' {
		line = "\n" + line
	}
	return d.splice(end, end, line), nil
}

// replaceValue replaces the value of an entry.
func (d *doc) replaceValue(e entry, v *yaml.Node) ([]byte, error) {
	old := e.value
	switch {
	case e.parent != nil && isFlow(e.parent) && (old.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode || strings.Contains(v.Value, "\n")):
		return d.rewriteFlow(e.parent, func(c *yaml.Node) {
			if e.key != nil {
				c.Content[e.index+1] = v
			} else {
				c.Content[e.index] = v
			}
		})
	case old.Kind == yaml.ScalarNode && old.Style != yaml.LiteralStyle && old.Style != yaml.FoldedStyle && !strings.Contains(v.Value, "\n") && v.Kind == yaml.ScalarNode:
		start, end, err := d.scalarSpan(old, 0)
		if err != nil {
			return nil, err
		}
		text, err := render(v)
		if err != nil {
			return nil, err
		}
		if old.Tag == "!!null" && old.Value == "" && e.key != nil {
			// key: (nothing): write after the colon.
			keyStart := d.offset(e.key.Line, e.key.Column)
			colon := bytes.IndexByte(d.text[keyStart:], ':')
			if colon < 0 {
				return nil, fmt.Errorf("line %d: no colon after %q", e.key.Line, e.key.Value)
			}
			start, end, text = keyStart+colon+1, keyStart+colon+1, " "+text
		}
		return d.splice(start, end, text), nil
	case isFlow(old):
		start, end, err := d.flowSpan(old)
		if err != nil {
			return nil, err
		}
		text, err := render(v)
		if err != nil {
			return nil, err
		}
		return d.splice(start, end, text), nil
	case e.key != nil:
		// A block value: everything after "key:" up to the end of the entry.
		indent := e.key.Column - 1
		keyStart := d.offset(e.key.Line, e.key.Column)
		colon := bytes.IndexByte(d.text[keyStart:], ':')
		if colon < 0 {
			return nil, fmt.Errorf("line %d: no colon after %q", e.key.Line, e.key.Value)
		}
		_, end := d.entrySpan(e)
		text, err := valueText(v, indent)
		if err != nil {
			return nil, err
		}
		return d.splice(keyStart+colon+1, end, text+"\n"), nil
	}
	return nil, errors.New("cannot replace this value")
}

// deleteKey removes key from the mapping at p. The last key of a block
// mapping leaves {}.
func (d *doc) deleteKey(p path, key string) ([]byte, error) {
	e, ok := d.find(append(append(path{}, p...), key))
	if !ok {
		return d.text, nil
	}
	if isFlow(e.parent) {
		return d.rewriteFlow(e.parent, func(c *yaml.Node) {
			c.Content = append(c.Content[:e.index:e.index], c.Content[e.index+2:]...)
		})
	}
	if len(e.parent.Content) == 2 {
		m, _ := d.find(p)
		if m.key != nil {
			return d.replaceValue(m, &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle})
		}
	}
	start, end := d.entrySpan(e)
	return d.splice(start, end, ""), nil
}

// renameKey renames key in the mapping at p.
func (d *doc) renameKey(p path, key, to string) ([]byte, error) {
	e, ok := d.find(append(append(path{}, p...), key))
	if !ok {
		return nil, fmt.Errorf("%s has no key %q", strings.Join(p, "."), key)
	}
	start, end, err := d.scalarSpan(e.key, 0)
	if err != nil {
		return nil, err
	}
	text, err := render(scalar(to))
	if err != nil {
		return nil, err
	}
	return d.splice(start, end, text), nil
}

// appendItem adds v to the list at p, and makes the list when it is
// missing.
func (d *doc) appendItem(p path, v *yaml.Node) ([]byte, error) {
	e, ok := d.find(p)
	if !ok || (e.value.Kind == yaml.ScalarNode && e.value.Tag == "!!null") {
		return d.setValue(p[:len(p)-1], p[len(p)-1], &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: []*yaml.Node{v}}, false)
	}
	if e.value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s is not a list", strings.Join(p, "."))
	}
	if isFlow(e.value) {
		return d.rewriteFlow(e.value, func(c *yaml.Node) { c.Content = append(c.Content, v) })
	}
	last := e.value.Content[len(e.value.Content)-1]
	_, end := d.entrySpan(entry{value: last, parent: e.value})
	text, err := flowValue(v)
	if err != nil {
		return nil, err
	}
	line := strings.Repeat(" ", max(0, last.Column-3)) + "- " + text + "\n"
	if end > 0 && d.text[end-1] != '\n' {
		line = "\n" + line
	}
	return d.splice(end, end, line), nil
}

// removeItem removes the item i of the list at p. The last item of a
// block list leaves [].
func (d *doc) removeItem(p path, i int) ([]byte, error) {
	e, ok := d.find(append(append(path{}, p...), "["+strconv.Itoa(i)+"]"))
	if !ok {
		return nil, fmt.Errorf("%s has no item %d", strings.Join(p, "."), i)
	}
	if isFlow(e.parent) {
		return d.rewriteFlow(e.parent, func(c *yaml.Node) {
			c.Content = append(c.Content[:i:i], c.Content[i+1:]...)
		})
	}
	if len(e.parent.Content) == 1 {
		l, _ := d.find(p)
		return d.replaceValue(l, &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle})
	}
	start, end := d.entrySpan(e)
	return d.splice(start, end, ""), nil
}

// setScalar replaces the scalar at p, where p ends in a list index.
func (d *doc) setItem(p path, v *yaml.Node) ([]byte, error) {
	e, ok := d.find(p)
	if !ok {
		return nil, fmt.Errorf("%s does not exist", strings.Join(p, "."))
	}
	return d.replaceValue(e, v)
}
