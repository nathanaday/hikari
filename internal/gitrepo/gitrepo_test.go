package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newRepo makes a git repo with one commit that holds the given files, and
// isolates git from the user's configuration.
func newRepo(t *testing.T, files map[string]string) *Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test User")
	t.Setenv("GIT_AUTHOR_EMAIL", "user@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "user@example.com")
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	run(t, dir, "init", "--quiet", "--initial-branch=main")
	run(t, dir, "add", "--all")
	run(t, dir, "commit", "--quiet", "-m", "initial")
	return Open(dir)
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

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

var shop = map[string]string{
	"services/orders/create.py":   "def create(): pass\n",
	"services/inventory/stock.py": "def reserve(): pass\n",
	"README.md":                   "shop\n",
}

func TestPrepareRefusesUncommittedChanges(t *testing.T) {
	r := newRepo(t, shop)
	write(t, r.Path, "services/orders/new.py", "x\n")
	if _, err := r.Prepare("hikari/work"); err == nil || !strings.Contains(err.Error(), "uncommitted changes") || !strings.Contains(err.Error(), "services/orders/new.py") {
		t.Fatalf("PR10 Prepare with an untracked file = %v", err)
	}
	for i := range 12 {
		write(t, r.Path, "more/f"+string(rune('a'+i)), "x\n")
	}
	if _, err := r.Prepare("hikari/work"); err == nil || !strings.Contains(err.Error(), "and 3 more") {
		t.Errorf("PR10 names at most ten paths: %v", err)
	}
}

func TestPrepareIgnoresResultsOnAnotherBranch(t *testing.T) {
	r := newRepo(t, shop)
	write(t, r.Path, "services/orders/.hikari/private/results.yaml", "version: 1\n")
	write(t, r.Path, "services/orders/.hikari/private/reports/unit.xml", "<testsuites/>\n")
	if _, err := r.Prepare("hikari/work"); err != nil {
		t.Errorf("results and reports are not changes: %v", err)
	}
	write(t, r.Path, "services/orders/.hikari/private/tests.yaml", "version: 1\n")
	if _, err := r.Prepare("hikari/work"); err == nil {
		t.Error("the test map is a change")
	}
}

func TestCommitPaths(t *testing.T) {
	r := newRepo(t, map[string]string{"store/systems/a/map.yaml": "v1\n", "store/shared/policies/p.md": "p\n", "other.txt": "o\n"})
	r.Prepare("hikari")
	write(t, r.Path, "store/systems/a/map.yaml", "v2\n")
	write(t, r.Path, "store/systems/a/features/x.md", "x\n")
	write(t, r.Path, "store/shared/policies/p.md", "p2\n")
	write(t, r.Path, "other.txt", "o2\n")
	commit, err := r.CommitPaths("hikari", []string{"store/systems/a/map.yaml", "store/systems/a/features", "store/shared/vocabulary/h.yaml"}, "a", "a: change")
	if err != nil || commit == "" {
		t.Fatalf("CommitPaths = %q, %v", commit, err)
	}
	files := run(t, r.Path, "show", "--name-only", "--format=", "HEAD")
	if files != "store/systems/a/features/x.md\nstore/systems/a/map.yaml" {
		t.Errorf("committed %q; the policy and other.txt must stay, the missing vocabulary is skipped", files)
	}
	if got := run(t, r.Path, "status", "--porcelain"); got != "M other.txt\n M store/shared/policies/p.md" {
		t.Errorf("status = %q", got)
	}
	// A removed file is a change of a tracked path.
	os.RemoveAll(filepath.Join(r.Path, "store/systems/a/features"))
	if commit, err := r.CommitPaths("hikari", []string{"store/systems/a/features"}, "a", "a: remove"); err != nil || commit == "" {
		t.Errorf("a removal = %q, %v", commit, err)
	}
	if commit, err := r.CommitPaths("hikari", []string{"store/systems/a/features", "nope"}, "a", "a: nothing"); err != nil || commit != "" {
		t.Errorf("nothing to commit = %q, %v", commit, err)
	}
	if n, err := r.CountOn(run(t, r.Path, "rev-parse", "main"), "store/systems/a"); err != nil || n != 2 {
		t.Errorf("CountOn = %d, %v", n, err)
	}
}

func TestIsIgnoredPathsThatDoNotExist(t *testing.T) {
	r := newRepo(t, map[string]string{".gitignore": ".env\n.hikari/\n"})
	for _, p := range []string{".env", ".hikari/", ".hikari/runs/x"} {
		if !r.IsIgnored(p) {
			t.Errorf("%s is not ignored", p)
		}
	}
	if r.IsIgnored("compose.yaml") || r.IsIgnored("store/") {
		t.Error("compose.yaml and store/ are not ignored")
	}
}

func TestPrepareCreatesAndChecksOutBranch(t *testing.T) {
	r := newRepo(t, shop)
	head := run(t, r.Path, "rev-parse", "HEAD")
	start, err := r.Prepare("hikari/work")
	if err != nil {
		t.Fatal(err)
	}
	if start != head {
		t.Errorf("start = %s, want %s", start, head)
	}
	if got := run(t, r.Path, "branch", "--show-current"); got != "hikari/work" {
		t.Errorf("checked out %q", got)
	}
	// A second run reuses the branch with its commits.
	write(t, r.Path, "services/orders/create.py", "def create(): return 1\n")
	if _, err := r.Commit("hikari/work", "services/orders", "orders", "change"); err != nil {
		t.Fatal(err)
	}
	run(t, r.Path, "checkout", "--quiet", "main")
	if _, err := r.Prepare("hikari/work"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, r.Path, "log", "-1", "--format=%s"); got != "change" {
		t.Errorf("existing branch lost its commit; HEAD is %q", got)
	}
}

func TestCommitStaysInScope(t *testing.T) {
	r := newRepo(t, shop)
	if _, err := r.Prepare("hikari/work"); err != nil {
		t.Fatal(err)
	}
	write(t, r.Path, "services/orders/create.py", "def create(): return 1\n")
	write(t, r.Path, "services/orders/cancel.py", "def cancel(): pass\n")
	write(t, r.Path, "services/inventory/stock.py", "def reserve(): return 1\n")
	os.Remove(filepath.Join(r.Path, "README.md"))
	// Another owner's staged change must not enter this commit.
	run(t, r.Path, "add", "services/inventory/stock.py")

	commit, err := r.Commit("hikari/work", "services/orders", "orders", "Add cancel")
	if err != nil {
		t.Fatal(err)
	}
	files := strings.Fields(run(t, r.Path, "show", "--name-only", "--format=", commit))
	if want := []string{"services/orders/cancel.py", "services/orders/create.py"}; !slices.Equal(files, want) {
		t.Errorf("commit has %v, want %v", files, want)
	}
	if got := run(t, r.Path, "log", "-1", "--format=%an <%ae>|%cn <%ce>|%s"); got != "orders (hikari) <orders@hikari.invalid>|Test User <user@example.com>|Add cancel" {
		t.Errorf("author|committer|subject = %q", got)
	}

	// The other owner commits its own change, including the deletion only
	// when the scope is the whole repo.
	if _, err := r.Commit("hikari/work", "services/inventory", "inventory", "Reserve"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, r.Path, "status", "--porcelain"); got != "D README.md" {
		t.Errorf("status after scoped commits = %q, want only the README deletion", got)
	}
	if _, err := r.Commit("hikari/work", "", "whole", "Remove README"); err != nil {
		t.Fatal(err)
	}
	if got := run(t, r.Path, "status", "--porcelain"); got != "" {
		t.Errorf("status after whole-repo commit = %q", got)
	}
}

func TestCommitWithoutChanges(t *testing.T) {
	r := newRepo(t, shop)
	r.Prepare("hikari/work")
	commit, err := r.Commit("hikari/work", "services/orders", "orders", "nothing")
	if err != nil || commit != "" {
		t.Fatalf("Commit with no changes = %q, %v", commit, err)
	}
}

func TestCommitRefusesOtherBranch(t *testing.T) {
	r := newRepo(t, shop)
	r.Prepare("hikari/work")
	run(t, r.Path, "checkout", "--quiet", "main")
	write(t, r.Path, "services/orders/create.py", "changed\n")
	if _, err := r.Commit("hikari/work", "services/orders", "orders", "x"); err == nil || !strings.Contains(err.Error(), `branch "hikari/work" is not checked out`) {
		t.Fatalf("Commit on main = %v", err)
	}
}

func TestChangedSince(t *testing.T) {
	r := newRepo(t, shop)
	start, _ := r.Prepare("hikari/work")
	write(t, r.Path, "services/orders/create.py", "v2\n")
	r.Commit("hikari/work", "services/orders", "orders", "v2")
	write(t, r.Path, "services/orders/cancel.py", "new\n")           // untracked
	write(t, r.Path, "services/orders/.hikari/map.yaml", "x: 1\n")   // excluded
	write(t, r.Path, "services/inventory/stock.py", "other scope\n") // other scope

	got, err := r.ChangedSince(start, "services/orders", ".hikari")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"cancel.py", "create.py"}; !slices.Equal(got, want) {
		t.Errorf("ChangedSince = %v, want %v", got, want)
	}
	if !r.HasCommit(start) || r.HasCommit("0000000000000000000000000000000000000000") {
		t.Error("HasCommit is wrong")
	}
	if n, _ := r.CountAuthored(start, "orders"); n != 1 {
		t.Errorf("CountAuthored(orders) = %d, want 1", n)
	}
	if n, _ := r.CountAuthored(start, "inventory"); n != 0 {
		t.Errorf("CountAuthored(inventory) = %d, want 0", n)
	}
}

func TestIsIgnored(t *testing.T) {
	r := newRepo(t, map[string]string{".gitignore": ".hikari/\n", "a/b.txt": "x"})
	if !r.IsIgnored("a/.hikari/map.yaml") {
		t.Error("a/.hikari/map.yaml should be ignored")
	}
	if r.IsIgnored("a/b.txt") {
		t.Error("a/b.txt is not ignored")
	}
}

func TestDirty(t *testing.T) {
	r := newRepo(t, map[string]string{"a/x.py": "x\n", "b/y.py": "y\n", ".gitignore": "*.log\n"})
	check := func(scope string, want bool) {
		t.Helper()
		if got, err := r.Dirty(scope); err != nil || got != want {
			t.Errorf("Dirty(%q) = %v, %v; want %v", scope, got, err, want)
		}
	}
	check("a", false)
	write(t, r.Path, "a/debug.log", "ignored")
	check("a", false)
	write(t, r.Path, "a/new.py", "untracked")
	check("a", true)
	check("b", false)
	write(t, r.Path, "b/y.py", "changed\n")
	check("b", true)
	os.Remove(filepath.Join(r.Path, "a/new.py"))
	os.Remove(filepath.Join(r.Path, "a/x.py"))
	check("a", true)
	check("", true)
}
