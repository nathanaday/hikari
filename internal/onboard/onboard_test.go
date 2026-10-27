package onboard

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

func TestWrite(t *testing.T) {
	if got := Names(); !slices.Equal(got, []string{"containerize-repo", "hikify"}) {
		t.Fatalf("HS1 Names() = %v", got)
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "--quiet")
	stale := filepath.Join(dir, ".claude/skills/hikify/old.md")
	os.MkdirAll(filepath.Dir(stale), 0o755)
	os.WriteFile(stale, []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".claude/skills/mine"), 0o755)
	os.WriteFile(filepath.Join(dir, ".claude/skills/mine/SKILL.md"), []byte("mine"), 0o644)
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("HS2 a write replaces what is there")
	}
	if got := git("status", "--porcelain", "--untracked-files=all"); got != "?? .claude/skills/mine/SKILL.md\n" {
		t.Errorf("HS3 git sees only the user's own skill:\n%s", got)
	}
	for _, f := range []string{"SKILL.md", "references/principles.md", "references/review.md", "references/wiki.md", "references/orchestration.md"} {
		if _, err := os.Stat(filepath.Join(dir, ".claude/skills/hikify", f)); err != nil {
			t.Errorf("hikify lacks %s: %v", f, err)
		}
	}
	public, _ := os.ReadFile(filepath.Join(dir, ".claude/skills/hikify/references/orchestration.md"))
	if !strings.Contains(string(public), "## The document") {
		t.Error("decision 6: the orchestration reference comes from the agents' skill")
	}
}

func TestSkillsNameEachOther(t *testing.T) {
	c, _ := skills.ReadFile("skills/containerize-repo/SKILL.md")
	h, _ := skills.ReadFile("skills/hikify/SKILL.md")
	if !strings.Contains(string(c), "hikify") || !strings.Contains(string(h), "containerize: done") || !strings.Contains(string(h), "containerize-repo") {
		t.Error("C3: hikify starts where containerize ends, and each names the other")
	}
	for name, data := range map[string][]byte{"containerize-repo": c, "hikify": h} {
		if !strings.HasPrefix(string(data), "---\nname: "+name+"\ndescription: ") {
			t.Errorf("%s: the front matter names the skill", name)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "references/") {
				for _, f := range strings.Fields(line) {
					if i := strings.Index(f, "references/"); i >= 0 {
						ref := strings.Trim(f[i:], "`.,:;)")
						if _, err := skills.ReadFile("skills/" + name + "/" + ref); err != nil && borrowed[name][ref] == "" {
							t.Errorf("%s names %s, which it lacks", name, ref)
						}
					}
				}
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
	fs.WalkDir(skills, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := skills.ReadFile(p)
		text := string(data)
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
