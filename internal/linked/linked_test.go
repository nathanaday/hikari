package linked

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/onboard"
	"github.com/nathanaday/hikari/internal/topology"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test User")
		t.Setenv(k+"_EMAIL", "user@example.com")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// remote makes a bare repository from files, and returns its URL.
func remote(t *testing.T, files map[string]string) string {
	t.Helper()
	src := t.TempDir()
	for name, content := range files {
		write(t, src, name, content)
	}
	git(t, src, "init", "--quiet", "--initial-branch=main")
	git(t, src, "add", "--all")
	git(t, src, "commit", "--quiet", "-m", "initial")
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, src, "clone", "--quiet", "--bare", src, bare)
	return "file://" + bare
}

// topo is a project whose partition agent dev owns src in the linked repo app, on
// branch.
func topo(t *testing.T, dir, branch, url string) *topology.Topology {
	t.Helper()
	src := `name: test
x-hikari:
  project: {branch: ` + branch + `}
  repos:
    app: {url: "` + url + `"}
  orchestration: lead
services:
  lead: {x-hikari: {}}
  dev: {x-hikari: {partition: {repo: app, scope: src}}}
`
	tp, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	tp.File = filepath.Join(dir, "topology.yaml")
	return tp
}

var projectFiles = map[string]string{
	".gitignore":           ".env\n/.hikari/\n/repos/\n",
	"topology.yaml":        "# see topo()\n",
	"vault/README.md":      "store\n",
	"vault/public/note.md": "x\n",
}

// clone clones a URL into a new directory.
func clone(t *testing.T, url string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "project")
	git(t, filepath.Dir(dir), "clone", "--quiet", url, dir)
	return dir
}

func texts(rs []Result) string {
	var b strings.Builder
	for _, r := range rs {
		mark := "ok"
		if r.Failed {
			mark = "FAIL"
		} else if r.Warning {
			mark = "WARN"
		}
		b.WriteString(mark + " " + r.Repo + ": " + r.Text + "\n")
	}
	return b.String()
}

func want(t *testing.T, rs []Result, failed bool, parts ...string) {
	t.Helper()
	got := texts(rs)
	if Failed(rs) != failed {
		t.Errorf("Failed = %v, want %v:\n%s", Failed(rs), failed, got)
	}
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Errorf("results lack %q:\n%s", p, got)
		}
	}
}

// work makes a commit by dev in app on branch, records it in dev's map,
// and commits the map in the project, as a run of hikari would.
func work(t *testing.T, project string, tp *topology.Topology, file string) {
	t.Helper()
	app := gitrepo.Open(tp.Repos["app"].Path)
	pr := gitrepo.Open(project)
	for _, r := range []*gitrepo.Repo{pr, app} {
		if _, err := r.Prepare(tp.Project.Branch); err != nil {
			t.Fatal(err)
		}
	}
	write(t, app.Path, "src/"+file, file+"\n")
	commit, err := app.CommitPaths(tp.Project.Branch, []string{"src"}, "dev", "dev: "+file)
	if err != nil {
		t.Fatal(err)
	}
	write(t, project, "vault/partitions/dev/read-only/features/map.yaml", "version: 4\npartition:\n  name: dev\n  described_state: {commit: "+commit+"}\nfeatures: []\n")
	if _, err := pr.CommitPaths(tp.Project.Branch, []string{"vault"}, "dev", "dev: map"); err != nil {
		t.Fatal(err)
	}
}

func TestTeammateGetsTheSameProject(t *testing.T) {
	isolate(t)
	appURL := remote(t, map[string]string{"src/main.go": "package main\n"})
	projectURL := remote(t, projectFiles)

	// Alex syncs a fresh project: SY1 clones app.
	alex := clone(t, projectURL)
	tp := topo(t, alex, "hikari-alex", appURL)
	if f := Check(tp, false); len(f) != 1 || !strings.Contains(f[0].Text, "app is not cloned; run: hikari sync") {
		t.Errorf("CK1 a missing clone: %+v", f)
	}
	want(t, Sync(tp, "", skills), false, "ok project: wrote the host skills containerize-repo, hikify into .claude/skills", "ok app: cloned "+appURL, "app: has no hikari-alex yet", "project: has no hikari-alex yet")
	tp = topo(t, alex, "hikari-alex", appURL)
	if f := Check(tp, false); len(f) != 0 {
		t.Errorf("CK1 after sync: %+v", f)
	}

	// Alex's agents work, and Alex pushes: app first, then the project.
	work(t, alex, tp, "a.go")
	rs := Push(tp)
	want(t, rs, false, "app: pushed 1 commit to origin/hikari-alex", "project: pushed 1 commit to origin/hikari-alex")
	if rs[0].Repo != "app" || rs[1].Repo != topology.ProjectRepo {
		t.Errorf("decision 7: the project is pushed last:\n%s", texts(rs))
	}
	want(t, Push(tp), false, "app: hikari-alex is up to date")

	// Bob starts from Alex's work (decision 6).
	bob := clone(t, projectURL)
	bt := topo(t, bob, "hikari-bob", appURL)
	want(t, Sync(bt, "hikari-alex", skills), false, "project: made hikari-bob from origin/hikari-alex", "app: made hikari-bob from origin/hikari-alex")
	if a, b := git(t, filepath.Join(bob, "repos/app"), "rev-parse", "hikari-bob"), git(t, filepath.Join(alex, "repos/app"), "rev-parse", "hikari-alex"); a != b {
		t.Error("Bob's app branch starts from Alex's")
	}
	if git(t, bob, "branch", "--show-current") != "main" {
		t.Error("SY5 sync never checks out")
	}

	// Alex pushes the map but not the code: Bob's sync names the missing
	// commit (SY6), and the project is behind (SY5).
	bt = topo(t, bob, "hikari-bob", appURL)
	work(t, alex, tp, "b.go")
	if err := gitrepo.Open(alex).Push("hikari-alex", gitrepo.Origin); err != nil {
		t.Fatal(err)
	}
	ct := topo(t, clone(t, projectURL), "hikari-alex", appURL)
	want(t, Sync(ct, "", skills), true, "project: made hikari-alex from origin/hikari-alex", "the map of dev describes commit", "may not have pushed it")
	want(t, Sync(bt, "", skills), false, "project: hikari-bob is not on the remote yet")
}

func TestSyncReports(t *testing.T) {
	isolate(t)
	appURL := remote(t, map[string]string{"src/main.go": "package main\n"})
	projectURL := remote(t, projectFiles)
	dir := clone(t, projectURL)
	tp := topo(t, dir, "hikari", appURL)
	Sync(tp, "", skills)
	tp = topo(t, dir, "hikari", appURL)
	work(t, dir, tp, "a.go")
	want(t, Sync(tp, "", skills), false, "app: hikari is not on the remote yet")
	want(t, Push(tp), false)
	work(t, dir, tp, "b.go")
	want(t, Sync(tp, "", skills), false, "app: hikari is ahead of origin/hikari by 1 commit; hikari push sends them")

	// SY2: a clone with another origin is left alone.
	other := remote(t, map[string]string{"x": "x\n"})
	git(t, filepath.Join(dir, "repos/app"), "remote", "set-url", "origin", other)
	want(t, Sync(tp, "", skills), true, "its origin is", "left alone")
	git(t, filepath.Join(dir, "repos/app"), "remote", "set-url", "origin", appURL)

	// SY1: a failed clone is reported, and sync goes on.
	gone := topo(t, clone(t, projectURL), "hikari", "file:///nowhere/app.git")
	want(t, Sync(gone, "", skills), true, "app: clone:")
}

func TestPushRefused(t *testing.T) {
	isolate(t)
	appURL := remote(t, map[string]string{"src/main.go": "package main\n"})
	projectURL := remote(t, projectFiles)
	alex := clone(t, projectURL)
	tp := topo(t, alex, "hikari", appURL)
	Sync(tp, "", skills)
	tp = topo(t, alex, "hikari", appURL)
	work(t, alex, tp, "a.go")
	want(t, Push(tp), false)

	// Someone else pushes app's hikari first.
	other := clone(t, appURL)
	git(t, other, "checkout", "--quiet", "hikari")
	write(t, other, "src/o.go", "o\n")
	git(t, other, "add", "--all")
	git(t, other, "commit", "--quiet", "-m", "other")
	git(t, other, "push", "--quiet", "origin", "hikari")

	work(t, alex, tp, "b.go")
	rs := Push(tp)
	want(t, rs, true, "app: refused:", "rejected", "project: not pushed: a push above was refused")
	if strings.Contains(texts(rs), "hint:") {
		t.Errorf("PS3 the reason has no hints:\n%s", texts(rs))
	}
}

func TestCheck(t *testing.T) {
	isolate(t)
	appURL := remote(t, map[string]string{"src/main.go": "package main\n"})
	dir := clone(t, remote(t, projectFiles))
	tp := topo(t, dir, "hikari", appURL)
	Sync(tp, "", skills)
	tp = topo(t, dir, "hikari", appURL)
	if f := Check(tp, true); len(f) != 2 || !f[0].Warning || !strings.Contains(f[0].Text, "agent dev has neither") {
		t.Errorf("CK1 credentials are warnings: %+v", f)
	}

	write(t, filepath.Join(dir, "repos/app"), "src/new.go", "x\n")
	if f := Check(tp, false); len(f) != 1 || !strings.Contains(f[0].Text, "repo app") || !strings.Contains(f[0].Text, "src/new.go") {
		t.Errorf("CK1 a dirty linked repo: %+v", f)
	}
	os.Remove(filepath.Join(dir, "repos/app/src/new.go"))

	// On the project branch, wiki edits are not a finding; anything else is.
	if _, err := gitrepo.Open(dir).Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "vault/partitions/dev/read-only/features/plan.md", "# plan\n")
	if f := Check(tp, false); len(f) != 0 {
		t.Errorf("CK1 wiki edits on the project branch: %+v", f)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/skills/containerize-repo/SKILL.md")); err != nil {
		t.Errorf("HS2 sync writes the host skills: %v", err)
	}
	write(t, dir, ".gitignore", ".env\n")
	f := Check(tp, false)
	if len(f) != 2 || !strings.Contains(f[0].Text, "must ignore /.hikari/ and /repos/") || !strings.Contains(f[1].Text, ".gitignore") {
		t.Errorf("CK1 the ignore rules: %+v", f)
	}
}

func TestStatus(t *testing.T) {
	isolate(t)
	appURL := remote(t, map[string]string{"src/main.go": "package main\n"})
	dir := clone(t, remote(t, projectFiles))
	tp := topo(t, dir, "hikari", appURL)
	if s := Status(tp); len(s) != 1 || s[0].Cloned {
		t.Errorf("SU4 a missing clone: %+v", s)
	}
	Sync(tp, "", skills)
	tp = topo(t, dir, "hikari", appURL)
	if s := Status(tp)[0]; !s.Cloned || s.Commit != "" || s.Results {
		t.Errorf("SU1 before any run: %+v", s)
	}
	work(t, dir, tp, "a.go")
	write(t, filepath.Join(dir, "repos/app/src"), ".hikari/private/results.yaml", `version: 1
runs:
  unit:
    ran: 2026-10-23T10:00:00Z
    tests:
      - {id: "b > two", outcome: failed, message: boom}
      - {id: "a > one", outcome: passed}
`)
	s := Status(tp)[0]
	if s.Commit == "" || !s.Results || len(s.Suites) != 1 {
		t.Fatalf("SU1, SU2: %+v", s)
	}
	su := s.Suites[0]
	if su.Name != "unit" || su.Counts["passed"] != 1 || su.Counts["failed"] != 1 || su.Tests[0].ID != "a > one" {
		t.Errorf("SU2, SU3: %+v", su)
	}
}

// skills writes the host skills as hikari sync does.
func skills(dir string) Result {
	if err := onboard.Write(dir); err != nil {
		return Result{Repo: topology.ProjectRepo, Text: "write the host skills: " + err.Error(), Failed: true}
	}
	return Result{Repo: topology.ProjectRepo, Text: "wrote the host skills " + strings.Join(onboard.Names(), ", ") + " into " + onboard.SkillsDir}
}

// TestOldLayout checks rule V11: each old layout fails the start checks,
// with the move that fixes it.
func TestOldLayout(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "shop")
	git(t, dir, "init", "--quiet", "shop")
	write(t, repo, "orders/.hikari/private/layout.yaml", "version: 1\n")
	write(t, dir, "store/shared/vocabulary/terms.yaml", "version: 1\n")
	src := "name: shop\nx-hikari:\n  repos: {shop: {path: shop, branch: hikari}}\nservices:\n  orders: {x-hikari: {partition: {repo: shop, scope: orders}}}\n"
	tp, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := OldLayout(tp)
	if len(got) != 2 || !strings.Contains(got[0], "rename store/ to vault/") || !strings.Contains(got[1], "rename layout.yaml to code-map.yaml") || !strings.Contains(got[1], "docs/guides/migrating.md") {
		t.Errorf("V11 store/ and layout.yaml: %q", got)
	}
	os.Rename(filepath.Join(dir, "store"), filepath.Join(dir, "vault"))
	write(t, repo, "orders/.hikari/private/code-map.yaml", "version: 1\n")
	got = OldLayout(tp)
	if len(got) != 1 || !strings.Contains(got[0], "holds shared/, the old layout: move shared/vocabulary") {
		t.Errorf("V11 shared/: %q", got)
	}
	os.RemoveAll(filepath.Join(dir, "vault", "shared"))
	if got := OldLayout(tp); len(got) != 0 {
		t.Errorf("V11 the new layout: %q", got)
	}
}

// TestCheckSkills checks rule V7.
func TestCheckSkills(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "--quiet", "shop")
	os.MkdirAll(filepath.Join(dir, "shop", "orders"), 0o755)
	write(t, dir, "vault/skills/style/SKILL.md", "# style\n")
	write(t, dir, "vault/skills/empty/notes.md", "x\n")
	write(t, dir, "vault/skills/lib/SKILL.md", "# lib\n")
	src := "name: shop\nx-hikari:\n  repos: {shop: {path: shop, branch: hikari}}\nservices:\n  orders: {x-hikari: {partition: {repo: shop, scope: orders}, grants: {deliverables: [lib], skills: [style, empty, ghost, lib]}}}\n"
	tp, err := topology.Parse([]byte(src), dir)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, f := range checkSkills(tp) {
		texts = append(texts, f.Text)
	}
	all := strings.Join(texts, "\n")
	if len(texts) != 3 || !strings.Contains(all, "skill empty, which has no SKILL.md") || !strings.Contains(all, "skill ghost, which does not exist") || !strings.Contains(all, "skill lib, which has the name of a deliverable") {
		t.Errorf("V7: %s", all)
	}
}
