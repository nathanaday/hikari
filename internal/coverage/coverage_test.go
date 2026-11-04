package coverage

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

func repoWith(t *testing.T, files ...string) *gitrepo.Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f+"\n"), 0o644)
	}
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "add", "--all"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "initial"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return gitrepo.Open(dir)
}

func TestUncovered(t *testing.T) {
	repo := repoWith(t,
		"README.md", "Makefile",
		"services/api/main.py", "services/api/tests/test_main.py",
		"services/web/index.html",
		"services/README.md",
		"libs/money/money.py",
		"libs/dates/dates.py",
		"scripts/deploy.sh",
	)
	got, err := Uncovered(repo, "HEAD", []string{"services/api", "libs/money"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Makefile", "README.md", "libs/dates/", "scripts/", "services/README.md", "services/web/"}
	if !slices.Equal(got, want) {
		t.Errorf("Uncovered = %v, want %v", got, want)
	}

	// The whole repo as a folder covers everything; no folder says nothing.
	for _, folders := range [][]string{{""}, nil, {"services/api", ""}} {
		if got, err := Uncovered(repo, "HEAD", folders); err != nil || got != nil {
			t.Errorf("Uncovered(%q) = %v, %v; want nil", folders, got, err)
		}
	}
	if got, _ := Uncovered(repo, "HEAD", []string{"services", "libs", "scripts"}); !slices.Equal(got, []string{"Makefile", "README.md"}) {
		t.Errorf("root files = %v", got)
	}
}

func TestWritableFolders(t *testing.T) {
	topo := &topology.Topology{Agents: map[string]topology.Agent{
		"api":  {Partition: &topology.Partition{Folders: []topology.Folder{{Repo: "backend", Path: "services/api"}, {Repo: "backend", Path: "libs/money", ReadOnly: true}}}},
		"web":  {Partition: &topology.Partition{Folders: []topology.Folder{{Repo: "frontend", Path: ""}}}},
		"lead": {},
	}}
	if got := WritableFolders(topo, "backend"); !slices.Equal(got, []string{"services/api"}) {
		t.Errorf("backend = %v", got)
	}
	if got := WritableFolders(topo, "frontend"); !slices.Equal(got, []string{""}) {
		t.Errorf("frontend = %v", got)
	}
	if got := WritableFolders(topo, "other"); got != nil {
		t.Errorf("other = %v", got)
	}
}
