package topoedit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeVault(t *testing.T, files map[string]string) string {
	t.Helper()
	vault := t.TempDir()
	for name, content := range files {
		p := filepath.Join(vault, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return vault
}

const story = `---
id: order-items
kind: story
name: "@dev and @qa"
---
## Purpose

Ask @dev and @qa, not @dev-ops or #dev.
@dev, then @qa.
`

func TestOccurrences(t *testing.T) {
	vault := writeVault(t, map[string]string{
		"shared/wiki/order-items.md":   story,
		"shared/vocabulary/terms.yaml": "version: 1\nterms:\n  - term: dev\n    definition: What @dev builds for @qa.\ninterfaces: []\n",
		"shared/vocabulary/eng.yaml":   "version: 1\nterms:\n  - term: dev\n    definition: What @dev builds.\ninterfaces: []\n",
		"systems/dev/map.yaml":         "version: 4\npartition:\n  name: dev\n  summary: Built by @dev.\nfeatures: []\n",
	})
	renames := []Rename{{"agent", "dev", "builder"}, {"agent", "qa", "checker"}}
	occ := Occurrences(vault, renames)
	var keys []string
	var unkeyed int
	for _, o := range occ {
		if o.Key == "" {
			unkeyed++
		} else {
			keys = append(keys, o.Key)
		}
	}
	want := []string{
		"shared/wiki/order-items.md:8:5", "shared/wiki/order-items.md:8:14",
		"shared/wiki/order-items.md:9:1", "shared/wiki/order-items.md:9:12",
		"shared/vocabulary/terms.yaml:4:22", "shared/vocabulary/terms.yaml:4:38",
	}
	slices.Sort(keys)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Errorf("TP1, TP2: keys %v\nwant %v", keys, want)
	}
	if unkeyed != 1 {
		t.Errorf("TP4: the map summary is listed without a key: %+v", occ)
	}

	changed, skipped, err := Propagate(vault, renames, []string{"shared/wiki/order-items.md:8:5", "shared/wiki/order-items.md:8:14", "gone.md:1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []string{"shared/wiki/order-items.md"}) || !slices.Equal(skipped, []string{"gone.md:1:1"}) {
		t.Errorf("TP3: changed %v, skipped %v", changed, skipped)
	}
	data, _ := os.ReadFile(filepath.Join(vault, "shared/wiki/order-items.md"))
	if !strings.Contains(string(data), "Ask @builder and @checker, not @dev-ops or #dev.\n@dev, then @qa.") ||
		!strings.Contains(string(data), `name: "@dev and @qa"`) {
		t.Errorf("TP3: only the chosen places change, and never the header:\n%s", data)
	}
}
