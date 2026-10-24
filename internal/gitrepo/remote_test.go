package gitrepo

import (
	"path/filepath"
	"strings"
	"testing"
)

// bareRemote makes a bare repository from a repo with the given files, and
// returns its file:// URL.
func bareRemote(t *testing.T, files map[string]string) string {
	t.Helper()
	src := newRepo(t, files)
	bare := filepath.Join(t.TempDir(), "remote.git")
	run(t, src.Path, "clone", "--quiet", "--bare", src.Path, bare)
	return "file://" + bare
}

func TestCloneAndOrigin(t *testing.T) {
	url := bareRemote(t, shop)
	dir := filepath.Join(t.TempDir(), "repos", "shop")
	r, err := Clone(url, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.RemoteURL(Origin); got != url {
		t.Errorf("RemoteURL = %q, want %q", got, url)
	}
	if r.RemoteURL("nope") != "" {
		t.Error("a missing remote has no URL")
	}
	if _, err := Clone(url, dir); err == nil {
		t.Error("cloning into an existing directory must fail")
	}
	if _, err := Clone("--upload-pack=touch /tmp/x", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("an option-like URL is a URL, and it does not exist")
	}
}

func TestTrackCompareAndPush(t *testing.T) {
	url := bareRemote(t, shop)
	alex, err := Clone(url, filepath.Join(t.TempDir(), "alex"))
	if err != nil {
		t.Fatal(err)
	}
	if alex.HasBranch("hikari") {
		t.Fatal("a fresh clone has no hikari branch")
	}
	if _, err := alex.Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	write(t, alex.Path, "a.txt", "a\n")
	if _, err := alex.CommitPaths("hikari", []string{"a.txt"}, "dev", "dev: a"); err != nil {
		t.Fatal(err)
	}
	if n, err := alex.Unpushed("hikari", Origin); err != nil || n != 1 {
		t.Errorf("Unpushed = %d, %v; want 1", n, err)
	}
	if err := alex.Push("hikari", Origin); err != nil {
		t.Fatal(err)
	}
	if up := run(t, alex.Path, "rev-parse", "--abbrev-ref", "hikari@{upstream}"); up != "origin/hikari" {
		t.Errorf("PS5 upstream = %q", up)
	}
	if n, _ := alex.Unpushed("hikari", Origin); n != 0 {
		t.Errorf("after the push, Unpushed = %d", n)
	}

	bob, err := Clone(url, filepath.Join(t.TempDir(), "bob"))
	if err != nil {
		t.Fatal(err)
	}
	if !bob.HasRemoteBranch(Origin, "hikari") || bob.HasRemoteBranch(Origin, "nope") {
		t.Fatal("HasRemoteBranch")
	}
	if err := bob.MakeBranch("hikari", Origin, "hikari"); err != nil {
		t.Fatal(err)
	}
	if up := run(t, bob.Path, "rev-parse", "--abbrev-ref", "hikari@{upstream}"); up != "origin/hikari" {
		t.Errorf("SY4 a tracking branch has upstream %q", up)
	}
	if err := bob.MakeBranch("hikari-bob", Origin, "hikari"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitIn(bob.Path, "rev-parse", "--abbrev-ref", "hikari-bob@{upstream}"); err == nil {
		t.Error("SY4 a branch made from a teammate's has no upstream")
	}
	if run(t, bob.Path, "rev-parse", "hikari-bob") != run(t, alex.Path, "rev-parse", "hikari") {
		t.Error("hikari-bob starts from alex's work")
	}

	// Bob pushes first, so alex's next push is not a fast-forward.
	if _, err := bob.Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	write(t, bob.Path, "b.txt", "b\n")
	if _, err := bob.CommitPaths("hikari", []string{"b.txt"}, "dev", "dev: b"); err != nil {
		t.Fatal(err)
	}
	if err := bob.Push("hikari", Origin); err != nil {
		t.Fatal(err)
	}
	write(t, alex.Path, "c.txt", "c\n")
	if _, err := alex.CommitPaths("hikari", []string{"c.txt"}, "dev", "dev: c"); err != nil {
		t.Fatal(err)
	}
	if err := alex.Fetch(Origin); err != nil {
		t.Fatal(err)
	}
	if ahead, behind, err := alex.Compare("hikari", Origin); err != nil || ahead != 1 || behind != 1 {
		t.Errorf("Compare = %d ahead, %d behind, %v; want 1 and 1", ahead, behind, err)
	}
	err = alex.Push("hikari", Origin)
	if err == nil || !strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), "hint:") {
		t.Errorf("PS3 a push that is not a fast-forward = %v", err)
	}
}

func gitIn(dir string, args ...string) (string, error) {
	return Open(dir).git(args...)
}

// A modified file's status line starts with a space, which the first line
// of git's output must keep.
func TestChangedKeepsTheFirstPath(t *testing.T) {
	r := newRepo(t, shop)
	write(t, r.Path, "README.md", "changed\n")
	if got, err := r.Changed(); err != nil || len(got) != 1 || got[0] != "README.md" {
		t.Errorf("Changed = %q, %v", got, err)
	}
	if _, err := r.Prepare("hikari"); err == nil || !strings.HasSuffix(err.Error(), ": README.md") {
		t.Errorf("PR10 names the modified file: %v", err)
	}
}
