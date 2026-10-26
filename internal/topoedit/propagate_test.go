package topoedit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeStore(t *testing.T, files map[string]string) string {
	t.Helper()
	store := t.TempDir()
	for name, content := range files {
		p := filepath.Join(store, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

const story = `---
id: order-items
kind: story
name: "@dev and #eng"
---
## Purpose

Ask @dev in #eng, not @dev-ops or #engine.
@dev, then #eng.
`

func TestOccurrences(t *testing.T) {
	store := writeStore(t, map[string]string{
		"hubs/eng/wiki/order-items.md": story,
		"shared/vocabulary/eng.yaml":   "version: 1\nhub: eng\nterms:\n  - term: dev\n    definition: What @dev builds in #eng.\ninterfaces: []\n",
		"systems/dev/map.yaml":         "version: 4\nsystem:\n  name: dev\n  summary: Built by @dev.\nfeatures: []\n",
	})
	renames := []Rename{{"agent", "dev", "builder"}, {"hub", "eng", "platform"}}
	occ := Occurrences(store, renames)
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
		"hubs/eng/wiki/order-items.md:8:5", "hubs/eng/wiki/order-items.md:8:13",
		"hubs/eng/wiki/order-items.md:9:1", "hubs/eng/wiki/order-items.md:9:12",
		"shared/vocabulary/eng.yaml:5:22", "shared/vocabulary/eng.yaml:5:37",
	}
	slices.Sort(keys)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Errorf("TP1, TP2: keys %v\nwant %v", keys, want)
	}
	if unkeyed != 1 {
		t.Errorf("TP4: the map summary is listed without a key: %+v", occ)
	}

	changed, skipped, err := Propagate(store, renames, []string{"hubs/eng/wiki/order-items.md:8:5", "hubs/eng/wiki/order-items.md:8:13", "gone.md:1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []string{"hubs/eng/wiki/order-items.md"}) || !slices.Equal(skipped, []string{"gone.md:1:1"}) {
		t.Errorf("TP3: changed %v, skipped %v", changed, skipped)
	}
	data, _ := os.ReadFile(filepath.Join(store, "hubs/eng/wiki/order-items.md"))
	if !strings.Contains(string(data), "Ask @builder in #platform, not @dev-ops or #engine.\n@dev, then #eng.") ||
		!strings.Contains(string(data), `name: "@dev and #eng"`) {
		t.Errorf("TP3: only the chosen places change, and never the header:\n%s", data)
	}
}
