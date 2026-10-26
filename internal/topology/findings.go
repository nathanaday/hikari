package topology

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finding is one result of a check of a compose file (rule TF1 of the
// topology API spec).
type Finding struct {
	Severity string `json:"severity"`
	Stage    string `json:"stage"`
	Message  string `json:"message"`
	// Path is the YAML path of the key at fault, in ids, such as
	// x-hikari.hubs.eng.members[1]. Line is its line in the source, or 0.
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

const (
	SeverityError   = "error"
	SeverityWarning = "warning"

	StageYAML     = "yaml"
	StageCompose  = "compose"
	StageTopology = "topology"
	StageStart    = "start"
)

// PathError is an error of a topology check with the path of the key at
// fault (rule TF2).
type PathError struct {
	Path string
	Err  error
}

func (e *PathError) Error() string { return e.Err.Error() }
func (e *PathError) Unwrap() error { return e.Err }

// ComposeError is an error that `docker compose config` reported.
type ComposeError struct{ Err error }

func (e *ComposeError) Error() string { return e.Err.Error() }
func (e *ComposeError) Unwrap() error { return e.Err }

func at(path string, format string, args ...any) error {
	return &PathError{Path: path, Err: fmt.Errorf(format, args...)}
}

// Findings splits the error of Load into findings, and gives each path its
// line in src (rule TF3). src may be nil.
func Findings(err error, src []byte) []Finding {
	if err == nil {
		return nil
	}
	var root *yaml.Node
	if src != nil {
		var doc yaml.Node
		if yaml.Unmarshal(src, &doc) == nil && len(doc.Content) > 0 {
			root = doc.Content[0]
		}
	}
	var out []Finding
	var walk func(error)
	walk = func(err error) {
		if j, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range j.Unwrap() {
				walk(e)
			}
			return
		}
		f := Finding{Severity: SeverityError, Stage: StageTopology, Message: err.Error()}
		var pe *PathError
		var ce *ComposeError
		switch {
		case errors.As(err, &pe):
			f.Path, f.Line = pe.Path, LineOf(root, pe.Path)
		case errors.As(err, &ce):
			f.Stage = StageCompose
		}
		out = append(out, f)
	}
	walk(err)
	return out
}

// SyntaxFinding checks that src is YAML, and returns the finding of its
// first syntax error with the line that the parser reports (rule TF4).
func SyntaxFinding(src []byte) *Finding {
	var doc yaml.Node
	err := yaml.Unmarshal(src, &doc)
	if err == nil {
		return nil
	}
	f := Finding{Severity: SeverityError, Stage: StageYAML, Message: strings.TrimPrefix(err.Error(), "yaml: ")}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		f.Line, _ = strconv.Atoi(m[1])
		f.Message = strings.TrimPrefix(f.Message, m[0][len("yaml: "):])
		f.Message = strings.TrimPrefix(f.Message, ": ")
	}
	return &f
}

var yamlLine = regexp.MustCompile(`yaml: line (\d+)`)

var pathPart = regexp.MustCompile(`([^.\[\]]+)|\[(\d+)\]`)

// LineOf returns the line of the node at path in the document root, or of
// the deepest node of the path that the source writes, such as the
// service whose key comes from a merge key. It returns 0 without a root.
func LineOf(root *yaml.Node, path string) int {
	if root == nil {
		return 0
	}
	node, line := root, root.Line
	for _, m := range pathPart.FindAllStringSubmatch(path, -1) {
		var next *yaml.Node
		switch {
		case m[2] != "" && node.Kind == yaml.SequenceNode:
			if i, _ := strconv.Atoi(m[2]); i < len(node.Content) {
				next = node.Content[i]
			}
		case m[1] != "" && node.Kind == yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				if node.Content[i].Value == m[1] {
					// The key's line, so that a block value points at its key.
					line = node.Content[i].Line
					next = node.Content[i+1]
					break
				}
			}
		}
		if next == nil {
			return line
		}
		node = next
		if node.Kind == yaml.ScalarNode {
			line = node.Line
		}
	}
	return line
}
