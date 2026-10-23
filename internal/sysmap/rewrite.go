package sysmap

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/nathanaday/hikari/internal/testmap"
	"gopkg.in/yaml.v3"
)

// RewriteIDs follows a change to the wiki in the private documents of root
// (rules E7 to E9). renamed maps old ids to new ones. removed maps each
// removed id to its redirect, or to "" when it has none. It rewrites the
// links and gaps of the test map, and the feature keys of the code map on a
// rename, through the YAML node tree so that comments and order survive. It
// returns a note for each file it could not rewrite.
func RewriteIDs(root string, renamed, removed map[string]string) []string {
	if len(renamed) == 0 && len(removed) == 0 {
		return nil
	}
	var notes []string
	for _, f := range []struct {
		file string
		fn   func(*yaml.Node) bool
	}{
		{testmap.MapFile, func(doc *yaml.Node) bool { return rewriteTestMap(doc, renamed, removed) }},
		{LayoutFile, func(doc *yaml.Node) bool { return renameKeys(value(doc, "features"), renamed, nil) }},
	} {
		if err := rewriteFile(privatePath(root, f.file), f.fn); err != nil {
			notes = append(notes, fmt.Sprintf("%s could not be rewritten to follow the change (%v); fix its feature ids by hand", f.file, err))
		}
	}
	return notes
}

func rewriteFile(path string, fn func(*yaml.Node) bool) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 || !fn(doc.Content[0]) {
		return nil
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// rewriteTestMap rewrites the links under tests and the keys under gaps.
// A key whose list a remove empties is dropped, so that its tests show as
// unmapped (rule E8).
func rewriteTestMap(doc *yaml.Node, renamed, removed map[string]string) bool {
	changed := renameKeys(value(doc, "gaps"), renamed, removed)
	tests := value(doc, "tests")
	if tests == nil || tests.Kind != yaml.MappingNode {
		return changed
	}
	var kept []*yaml.Node
	for i := 0; i+1 < len(tests.Content); i += 2 {
		key, list := tests.Content[i], tests.Content[i+1]
		if list.Kind != yaml.SequenceNode {
			kept = append(kept, key, list)
			continue
		}
		was := len(list.Content)
		var items []*yaml.Node
		var seen []string
		for _, item := range list.Content {
			id, err := testmap.LinkID(item.Value)
			if err == nil {
				if to, ok := renamed[id]; ok {
					item.Value, changed = testmap.Link(to), true
				} else if to, ok := removed[id]; ok {
					changed = true
					if to == "" {
						continue
					}
					item.Value = testmap.Link(to)
				}
			}
			if slices.Contains(seen, item.Value) {
				changed = true
				continue
			}
			seen = append(seen, item.Value)
			items = append(items, item)
		}
		list.Content = items
		if was > 0 && len(items) == 0 {
			continue
		}
		kept = append(kept, key, list)
	}
	if len(kept) != len(tests.Content) {
		changed = true
	}
	tests.Content = kept
	return changed
}

// renameKeys renames the keys of a mapping that renamed names, and drops
// those that removed names.
func renameKeys(m *yaml.Node, renamed, removed map[string]string) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	changed := false
	var kept []*yaml.Node
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i]
		if to, ok := renamed[key.Value]; ok {
			key.Value, changed = to, true
		} else if _, ok := removed[key.Value]; ok {
			changed = true
			continue
		}
		kept = append(kept, key, m.Content[i+1])
	}
	m.Content = kept
	return changed
}

// value returns the value of a key of a mapping node, or nil.
func value(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
