package toolbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/wiki"
)

// TestLibrary checks that the index and the files agree: every item has
// its files, every file is an item, each skill's SKILL.md names itself, and
// every description is one line.
func TestLibrary(t *testing.T) {
	items := Items()
	if len(items) < 10 {
		t.Fatalf("the toolbox has %d items", len(items))
	}
	seen := map[string]bool{}
	roots := map[string]bool{}
	for _, it := range items {
		if seen[it.Ref()] {
			t.Errorf("%s is listed twice", it.Ref())
		}
		seen[it.Ref()] = true
		if !slices.Contains(Kinds, it.Kind) {
			t.Errorf("%s: unknown kind", it.Ref())
		}
		if strings.TrimSpace(it.Description) == "" || strings.Contains(it.Description, "\n") {
			t.Errorf("%s: the description is one line", it.Ref())
		}
		files, err := Files(it)
		if err != nil || len(files) == 0 {
			t.Errorf("%s: %v", it.Ref(), err)
			continue
		}
		roots[itemRoot(it)] = true
		text, _ := Text(it)
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s is empty", it.Ref())
		}
		if it.Kind == Skill {
			fm, _ := frontmatter(text)
			if fm["name"] != it.Name || fm["description"] == "" {
				t.Errorf("%s: SKILL.md names %q", it.Ref(), fm["name"])
			}
		}
		if it.Kind == Profile && !strings.Contains(text, it.Name+":") {
			t.Errorf("%s: the snippet does not define %s", it.Ref(), it.Name)
		}
	}
	fs.WalkDir(library, "library", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "library/toolbox.yaml" {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		if roots[p] || roots[dir] || roots[filepath.ToSlash(filepath.Dir(dir))] {
			return nil
		}
		t.Errorf("%s is in the library but not in toolbox.yaml", p)
		return nil
	})
	if _, err := Find("style/terse"); err != nil {
		t.Error(err)
	}
	if _, err := Find("styles/terse"); err != nil {
		t.Error("the plural is accepted:", err)
	}
	if _, err := Find("policies/no-secrets"); err != nil {
		t.Error("policies is accepted:", err)
	}
	for _, bad := range []string{"terse", "gadget/terse", "style/ghost"} {
		if _, err := Find(bad); err == nil {
			t.Errorf("Find(%q) succeeds", bad)
		}
	}
}

// frontmatter reads the key: value lines between the --- fences.
func frontmatter(text string) (map[string]string, bool) {
	out := map[string]string{}
	if !strings.HasPrefix(text, "---\n") {
		return out, false
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return out, false
	}
	for _, l := range strings.Split(rest[:end], "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out, true
}

// TestInstall checks that an item is written where the vault keeps its
// kind, that an installed item is refused without force, and that a
// profile is a snippet and not a file.
func TestInstall(t *testing.T) {
	st := vault.Vault{Root: t.TempDir()}
	for _, ref := range []string{"policy/no-secrets", "style/terse", "skill/tdd"} {
		it, err := Find(ref)
		if err != nil {
			t.Fatal(err)
		}
		if Installed(st, it) {
			t.Errorf("%s is installed before the install", ref)
		}
		written, err := Install(st, it, false)
		if err != nil {
			t.Fatal(err)
		}
		if !Installed(st, it) || len(written) == 0 {
			t.Errorf("%s: written %v, installed %v", ref, written, Installed(st, it))
		}
		if _, err := Install(st, it, false); !errors.Is(err, ErrExists) {
			t.Errorf("%s: a second install is refused: %v", ref, err)
		}
		if _, err := Install(st, it, true); err != nil {
			t.Errorf("%s: -force replaces: %v", ref, err)
		}
	}
	if _, err := os.Stat(filepath.Join(st.Root, "public", "policies", "no-secrets.md")); err != nil {
		t.Error("the policy is in public/policies:", err)
	}
	if _, err := os.Stat(st.StyleFile("terse")); err != nil {
		t.Error("the style is in styles/:", err)
	}
	if _, err := os.Stat(filepath.Join(st.SkillDir("tdd"), vault.SkillFile)); err != nil {
		t.Error("the skill is in skills/tdd:", err)
	}
	if !slices.Contains(st.StyleNames(), "terse") || !slices.Contains(st.SkillNames(), "tdd") {
		t.Error("the vault lists the installed style and skill")
	}
	profile, _ := Find("profile/careful")
	if _, err := Install(st, profile, false); err == nil {
		t.Error("a profile is not installed")
	}
	list := List(st)
	installed := 0
	for _, s := range list {
		if s.Installed {
			installed++
		}
		if s.Kind != Profile && s.Path == "" {
			t.Errorf("%s has no path", s.Ref())
		}
	}
	if installed != 3 {
		t.Errorf("%d installed, want 3", installed)
	}
	// A record-a-decision skill talks about wiki kinds that exist.
	text, _ := Text(Item{Kind: Skill, Name: "record-a-decision"})
	for _, k := range []string{wiki.Principle, wiki.Note} {
		if !strings.Contains(text, "`"+k+"`") {
			t.Errorf("record-a-decision does not name the kind %s", k)
		}
	}
}
