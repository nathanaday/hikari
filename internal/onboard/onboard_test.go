package onboard

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

// pluginSkills is the plugin's skills folder, from this package.
const pluginSkills = "../../plugin/skills"

func skillText(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(pluginSkills, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRemoveCopies checks decision 7 of the plugin spec: sync removes the
// copies that hikari wrote, and leaves a skill of your own.
func TestRemoveCopies(t *testing.T) {
	dir := t.TempDir()
	put := func(rel, text string) {
		p := filepath.Join(dir, SkillsDir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	put("hikify/SKILL.md", "old")
	put("hikify/.gitignore", selfIgnore)
	put("containerize-repo/SKILL.md", "mine, under the same name")
	put("mine/SKILL.md", "mine")
	removed, err := RemoveCopies(dir)
	if err != nil || !slices.Equal(removed, []string{"hikify"}) {
		t.Fatalf("RemoveCopies = %v, %v", removed, err)
	}
	for rel, want := range map[string]bool{"hikify": false, "containerize-repo/SKILL.md": true, "mine/SKILL.md": true} {
		if _, err := os.Stat(filepath.Join(dir, SkillsDir, rel)); (err == nil) != want {
			t.Errorf("%s kept = %v, want %v", rel, err == nil, want)
		}
	}
	if removed, _ := RemoveCopies(t.TempDir()); len(removed) != 0 {
		t.Errorf("a project with no copies: %v", removed)
	}
}

// TestBorrowed checks decision 11 of the plugin spec: hikify's borrowed
// references are copies of the agents' skills.
func TestBorrowed(t *testing.T) {
	for copy, src := range borrowed {
		want, err := os.ReadFile(filepath.Join("../driver/skills", src))
		if err != nil {
			t.Fatal(err)
		}
		if got := skillText(t, copy); !bytes.Equal([]byte(got), want) {
			t.Errorf("plugin/skills/%s differs from internal/driver/skills/%s; run make skills", copy, src)
		}
	}
}

var referenceName = regexp.MustCompile(`(?:\.\./[a-z-]+/)?references/[a-z-]+\.md`)

// TestSkillsNameEachOther checks that the host skills agree where they meet,
// and that each reference a skill names exists.
func TestSkillsNameEachOther(t *testing.T) {
	c, h := skillText(t, "containerize-repo/SKILL.md"), skillText(t, "hikify/SKILL.md")
	if !strings.Contains(c, "hikify") || !strings.Contains(h, "containerize: done") || !strings.Contains(h, "containerize-repo") {
		t.Error("C3: hikify starts where containerize ends, and each names the other")
	}
	entries, err := os.ReadDir(pluginSkills)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		text := skillText(t, name+"/SKILL.md")
		if !strings.HasPrefix(text, "---\nname: "+name+"\ndescription: ") {
			t.Errorf("%s: the front matter names the skill", name)
		}
		for _, ref := range referenceName.FindAllString(text, -1) {
			if _, err := os.Stat(filepath.Join(pluginSkills, name, ref)); err != nil {
				t.Errorf("%s names %s, which does not exist", name, ref)
			}
		}
	}
}

// TestNamesBuilt checks M4 of the Hikari agent spec: the host skills name
// only the built-in skills, the agents' tools, and hikari's commands.
func TestNamesBuilt(t *testing.T) {
	src, _ := filepath.Glob("../mcpserver/*.go")
	tools := map[string]bool{}
	for _, f := range src {
		data, _ := os.ReadFile(f)
		for _, m := range regexp.MustCompile(`Name: +"([a-z_]+)"`).FindAllStringSubmatch(string(data), -1) {
			tools[m[1]] = true
		}
	}
	main, _ := os.ReadFile("../../cmd/hikari/main.go")
	notTools := []string{"network_mode", "node_modules", "junit_family", "read_only"}
	snake := regexp.MustCompile("`([a-z]+(?:_[a-z]+)+)`")
	skill := regexp.MustCompile(`hikari-[a-z]+(?:-[a-z]+)*`)
	command := regexp.MustCompile("`hikari ([a-z]+)")
	fs.WalkDir(os.DirFS(pluginSkills), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if borrowed[p] != "" {
			return nil // the agents' own text, which names their own fields
		}
		text := skillText(t, p)
		for _, m := range snake.FindAllStringSubmatch(text, -1) {
			if !tools[m[1]] && !slices.Contains(notTools, m[1]) {
				t.Errorf("%s names %s, which is no tool", p, m[1])
			}
		}
		for _, name := range skill.FindAllString(text, -1) {
			if name != "hikari-agent" && !slices.Contains(api.BuiltinSkills, name) {
				t.Errorf("%s names %s, which is no built-in skill", p, name)
			}
		}
		for _, m := range command.FindAllStringSubmatch(text, -1) {
			if !strings.Contains(string(main), `case "`+m[1]+`"`) {
				t.Errorf("%s names hikari %s, which is no command", p, m[1])
			}
		}
		for _, old := range []string{"onboarding/wiki", "onboarding/shared"} {
			if strings.Contains(text, old) {
				t.Errorf("%s drafts in %s; drafts go in onboarding/vault/", p, old)
			}
		}
		return nil
	})
	if !tools["apply"] || len(tools) < 10 {
		t.Fatalf("read the tools from the mcpserver sources: %v", tools)
	}
}
