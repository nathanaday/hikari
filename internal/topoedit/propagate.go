package topoedit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Occurrence is one place where a rename can propagate (rule TP2): @name of
// an agent in a wiki body or a vocabulary description.
type Occurrence struct {
	// Key is <store path>:<line>:<column>. A map summary has no key: its
	// owner writes it (rule TP4).
	Key     string `json:"key,omitempty"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Replace string `json:"replace"`
	From    string `json:"from"`
	To      string `json:"to"`
}

func mention(name string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_-])(` + regexp.QuoteMeta("@"+name) + `)($|[^A-Za-z0-9_-])`)
}

// region is the lines of a file that propagation may change: from and to,
// 1-based and inclusive.
type region struct{ from, to int }

// Occurrences lists every place that renames can propagate to, in the
// store at store (rule TP1).
func Occurrences(store string, renames []Rename) []Occurrence {
	if store == "" || len(renames) == 0 {
		return []Occurrence{}
	}
	out := []Occurrence{}
	for _, f := range propagationFiles(store) {
		data, err := os.ReadFile(filepath.Join(store, f.rel))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, r := range f.regions(data) {
			for n := r.from; n <= r.to && n <= len(lines); n++ {
				line := lines[n-1]
				for _, rn := range renames {
					re := mention(rn.From)
					for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
						start, end := m[4], m[5]
						o := Occurrence{File: f.rel, Line: n, Text: line, From: "@" + rn.From, To: "@" + rn.To,
							Replace: line[:start] + "@" + rn.To + line[end:]}
						if f.keyed {
							o.Key = fmt.Sprintf("%s:%d:%d", f.rel, n, start+1)
						}
						out = append(out, o)
					}
				}
			}
		}
	}
	return out
}

type propagationFile struct {
	rel     string
	keyed   bool
	regions func(data []byte) []region
}

func propagationFiles(store string) []propagationFile {
	var out []propagationFile
	for _, pattern := range []string{"systems/*/wiki/*.md", "shared/wiki/*.md"} {
		matches, _ := filepath.Glob(filepath.Join(store, filepath.FromSlash(pattern)))
		for _, m := range matches {
			rel, _ := filepath.Rel(store, m)
			out = append(out, propagationFile{rel: filepath.ToSlash(rel), keyed: true, regions: bodyRegion})
		}
	}
	if _, err := os.Stat(filepath.Join(store, "shared", "vocabulary", "terms.yaml")); err == nil {
		out = append(out, propagationFile{rel: "shared/vocabulary/terms.yaml", keyed: true, regions: yamlRegions("definition", "description")})
	}
	maps, _ := filepath.Glob(filepath.Join(store, "systems", "*", "map.yaml"))
	for _, m := range maps {
		rel, _ := filepath.Rel(store, m)
		out = append(out, propagationFile{rel: filepath.ToSlash(rel), regions: yamlRegions("summary")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out
}

// bodyRegion is the body of a wiki document: the lines after its header.
func bodyRegion(data []byte) []region {
	lines := strings.Split(string(data), "\n")
	start := 1
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				start = i + 2
				break
			}
		}
	}
	return []region{{start, len(lines)}}
}

// yamlRegions returns the lines of the values of the given keys, anywhere
// in a YAML file.
func yamlRegions(keys ...string) func([]byte) []region {
	return func(data []byte) []region {
		var root yaml.Node
		if yaml.Unmarshal(data, &root) != nil {
			return nil
		}
		total := strings.Count(string(data), "\n") + 1
		var out []region
		var walk func(n *yaml.Node, end int)
		walk = func(n *yaml.Node, end int) {
			switch n.Kind {
			case yaml.DocumentNode, yaml.SequenceNode:
				for i, c := range n.Content {
					next := end
					if i+1 < len(n.Content) {
						next = n.Content[i+1].Line - 1
					}
					walk(c, next)
				}
			case yaml.MappingNode:
				for i := 0; i+1 < len(n.Content); i += 2 {
					next := end
					if i+2 < len(n.Content) {
						next = n.Content[i+2].Line - 1
					}
					k, v := n.Content[i], n.Content[i+1]
					if v.Kind == yaml.ScalarNode && slices.Contains(keys, k.Value) {
						to := v.Line
						if v.Style == yaml.LiteralStyle || v.Style == yaml.FoldedStyle {
							to = next
						}
						out = append(out, region{v.Line, max(v.Line, to)})
					} else {
						walk(v, next)
					}
				}
			}
		}
		walk(&root, total)
		return out
	}
}

// Propagate rewrites the occurrences that keys name (rule TP3). It returns
// the store paths it changed, and the keys it skipped because their text
// no longer matches.
func Propagate(store string, renames []Rename, keys []string) (changed, skipped []string, err error) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	byFile := map[string][]Occurrence{}
	for _, o := range Occurrences(store, renames) {
		if o.Key != "" && want[o.Key] {
			byFile[o.File] = append(byFile[o.File], o)
			delete(want, o.Key)
		}
	}
	for k := range want {
		skipped = append(skipped, k)
	}
	sort.Strings(skipped)
	for _, file := range sorted(byFile) {
		p := filepath.Join(store, filepath.FromSlash(file))
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		lines := strings.Split(string(data), "\n")
		occ := byFile[file]
		// From the end of each line, so that earlier columns stay right.
		sort.Slice(occ, func(i, j int) bool {
			if occ[i].Line != occ[j].Line {
				return occ[i].Line > occ[j].Line
			}
			return column(occ[i].Key) > column(occ[j].Key)
		})
		for _, o := range occ {
			l, c := lines[o.Line-1], column(o.Key)-1
			lines[o.Line-1] = l[:c] + o.To + l[c+len(o.From):]
		}
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return nil, nil, err
		}
		changed = append(changed, file)
	}
	return changed, skipped, nil
}

func column(key string) int {
	var c int
	if i := strings.LastIndexByte(key, ':'); i >= 0 {
		fmt.Sscanf(key[i+1:], "%d", &c)
	}
	return c
}
