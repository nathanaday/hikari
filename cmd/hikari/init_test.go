package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/testmap"
	"github.com/nathanaday/hikari/internal/topology"
)

func gitEnv(t *testing.T, identity bool) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if identity {
		for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
			t.Setenv(k+"_NAME", "Test User")
			t.Setenv(k+"_EMAIL", "user@example.com")
		}
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", "user.email")
		t.Setenv("GIT_CONFIG_VALUE_0", "user@example.com")
	}
}

func TestProjectName(t *testing.T) {
	for in, want := range map[string]string{"/x/P3 Project": "p3-project", "/x/my_app": "my_app", "/x/--Shop!!": "shop", "/x/日本": ""} {
		if got := projectName(in); got != want {
			t.Errorf("PR25 projectName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteProject(t *testing.T) {
	gitEnv(t, true)
	dir := filepath.Join(t.TempDir(), "p3")
	committed, err := writeProject(dir, "p3")
	if err != nil || !committed {
		t.Fatalf("writeProject = %v, %v", committed, err)
	}
	out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	if len(out) > 0 {
		t.Errorf("PR24 the new repository is not clean: %s", out)
	}
	files, _ := exec.Command("git", "-C", dir, "ls-files").Output()
	for _, f := range []string{".gitignore", ".env.example", "compose.yaml", "store/README.md", "store/.gitignore"} {
		if !strings.Contains(string(files), f+"\n") {
			t.Errorf("PR23 %s is not committed:\n%s", f, files)
		}
	}
	repo := gitrepo.Open(dir)
	for _, p := range []string{".env", ".hikari/", "repos/app/", ".obsidian/workspace.json", ".DS_Store", ".claude/settings.local.json", ".claude/skills/containerize-repo/SKILL.md"} {
		if !repo.IsIgnored(p) {
			t.Errorf("PR23 %s is not ignored", p)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/skills/containerize-repo/SKILL.md")); err != nil {
		t.Errorf("HS2 init writes the host skills: %v", err)
	}
	if repo.IsIgnored(".claude/skills/mine/SKILL.md") {
		t.Error("decision 1: a skill of your own stays versioned")
	}
	if repo.IsIgnored("e2e/p3/.hikari/private/tests.yaml") {
		t.Error("the private documents of a scope must stay versioned")
	}

	// PR26: the starter topology is valid as compose resolves it.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Log("PR26 needs docker compose; skipped")
	} else if topo, err := topology.Load(context.Background(), filepath.Join(dir, "compose.yaml")); err != nil {
		t.Errorf("PR26 %v", err)
	} else if topo.Name != "p3" || topo.Project == nil || topo.Project.Branch != "hikari" {
		t.Errorf("PR26 topology = %s, %+v", topo.Name, topo.Project)
	} else {
		// IN2: HIKARI_BRANCH gives each person their own branch.
		t.Setenv("HIKARI_BRANCH", "hikari-alex")
		if topo, err := topology.Load(context.Background(), filepath.Join(dir, "compose.yaml")); err != nil || topo.Project.Branch != "hikari-alex" {
			t.Errorf("IN2 with HIKARI_BRANCH: %+v, %v", topo.Project, err)
		}
	}

	if _, err := writeProject(dir, "p3"); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("PR23 a second init = %v", err)
	}
}

func TestWriteProjectWithoutIdentity(t *testing.T) {
	gitEnv(t, false)
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("EMAIL", "")
	dir := filepath.Join(t.TempDir(), "p")
	committed, err := writeProject(dir, "p")
	if err != nil || committed {
		t.Errorf("PR24 without an identity = %v, %v", committed, err)
	}
}

func TestStatusLines(t *testing.T) {
	lines := statusLines([]linked.SystemStatus{
		{Name: "app", Cloned: true, Commit: "0123456789abcdef", Features: 2, Health: map[string]int{"passing": 1, "failing": 1}, Results: true,
			Suites: []linked.SuiteStatus{{Name: "unit", Counts: map[string]int{"passed": 3}, Tests: []testmap.Result{{ID: "a > one", Outcome: "passed"}}}}},
		{Name: "web", Cloned: true},
		{Name: "far", Cloned: false},
	}, true)
	got := strings.Join(lines, "\n")
	for _, want := range []string{"app  described at 0123456789ab · 2 features: 1 passing, 1 failing", "  unit  ran ", "· 3 passed", "    passed  a > one", "web  not described", "  no test results on this machine", "far  not cloned"} {
		if !strings.Contains(got, want) {
			t.Errorf("SU1 to SU3: lines lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "far  not cloned\n  no test") {
		t.Error("a missing clone has no results line")
	}
}
