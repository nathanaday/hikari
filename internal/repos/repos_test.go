package repos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/forge"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

func run(t *testing.T, dir string, args ...string) string {
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
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cloneOf makes a bare remote with one commit on main, whose origin URL is
// a GitHub URL as far as git config says, and a clone of it.
func cloneOf(t *testing.T) (*gitrepo.Repo, string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test User")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "user@example.com")
	}
	src := t.TempDir()
	write(t, src, "README.md", "shop\n")
	run(t, src, "init", "--quiet", "--initial-branch=main")
	run(t, src, "add", "--all")
	run(t, src, "commit", "--quiet", "-m", "initial")
	bare := filepath.Join(t.TempDir(), "remote.git")
	run(t, src, "clone", "--quiet", "--bare", src, bare)
	dir := filepath.Join(t.TempDir(), "shop")
	run(t, src, "clone", "--quiet", bare, dir)
	// The clone's origin names GitHub, but pushes go to the bare repo.
	run(t, dir, "remote", "set-url", "origin", "https://github.com/acme/shop.git")
	run(t, dir, "remote", "set-url", "--push", "origin", bare)
	run(t, dir, "config", "url."+bare+".insteadOf", "https://github.com/acme/shop.git")
	return gitrepo.Open(dir), bare
}

// fake is an in-memory forge.
type fake struct {
	mu     sync.Mutex
	remote forge.Remote
	prs    []forge.PullRequest
	checks map[string]forge.Checks
	merged []int
	fail   error
}

func (f *fake) Remote() forge.Remote { return f.remote }
func (f *fake) DefaultBranch(context.Context) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	return "main", nil
}
func (f *fake) PullRequests(context.Context) ([]forge.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.prs), f.fail
}
func (f *fake) Checks(_ context.Context, sha string) (forge.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks[sha], nil
}
func (f *fake) CreatePullRequest(_ context.Context, in forge.NewPullRequest) (forge.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pr := forge.PullRequest{Number: len(f.prs) + 1, Title: in.Title, Body: in.Body, Head: in.Head, Base: in.Base, State: forge.Open, SHA: "sha" + fmt.Sprint(len(f.prs)+1), URL: "https://github.com/acme/shop/pull/" + fmt.Sprint(len(f.prs)+1)}
	f.prs = append(f.prs, pr)
	return pr, nil
}
func (f *fake) Merge(_ context.Context, number int, method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.prs {
		if f.prs[i].Number == number {
			f.prs[i].State, f.prs[i].Merged = forge.Merged, time.Now()
			f.merged = append(f.merged, number)
			return nil
		}
	}
	return errors.New("no such pull request")
}

func newManager(t *testing.T, repo *gitrepo.Repo, f *fake) (*Manager, *[]api.RepoView) {
	t.Helper()
	topo := &topology.Topology{Repos: map[string]topology.Repo{"shop": {ID: "shop", Name: "shop", Path: repo.Path, Branch: "hikari"}}}
	var mu sync.Mutex
	var changes []api.RepoView
	m := New(topo, map[string]*gitrepo.Repo{"shop": repo}, Options{
		NewForge:     func(r forge.Remote) (forge.Forge, error) { f.remote = r; return f, nil },
		PartitionsOf: func(string) []string { return []string{"orders"} },
		OnChange: func(v api.RepoView) {
			mu.Lock()
			defer mu.Unlock()
			changes = append(changes, v)
		},
		Now: func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) },
	})
	return m, &changes
}

func branch(v api.RepoView, name string) api.BranchView {
	for _, b := range v.Branches {
		if b.Name == name {
			return b
		}
	}
	return api.BranchView{}
}

func TestViewsBranchesAndForge(t *testing.T) {
	repo, _ := cloneOf(t)
	if _, err := repo.Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	write(t, repo.Path, "a.txt", "a\n")
	if _, err := repo.CommitPaths("hikari", []string{"a.txt"}, "orders", "orders: add a"); err != nil {
		t.Fatal(err)
	}
	f := &fake{checks: map[string]forge.Checks{}}
	m, changes := newManager(t, repo, f)
	v, ok := m.View("shop")
	if !ok {
		t.Fatal("no view")
	}
	if v.Forge == nil || v.Forge.Slug() != "acme/shop" || v.URL != "https://github.com/acme/shop.git" {
		t.Errorf("forge = %+v, url %q", v.Forge, v.URL)
	}
	if v.Default != "main" || !slices.Equal(v.Partitions, []string{"orders"}) || v.Dirty {
		t.Errorf("view = default %q, partitions %v, dirty %v", v.Default, v.Partitions, v.Dirty)
	}
	yours := branch(v, "hikari")
	if !yours.Yours || !yours.Local || yours.Remote || yours.Ahead != 1 || yours.Behind != 0 || yours.Unpushed != 1 || yours.Merged || yours.Subject != "orders: add a" {
		t.Errorf("your branch = %+v", yours)
	}
	if def := branch(v, "main"); !def.Default || !def.Local || !def.Remote || def.Ahead != 0 || !def.Merged == false && false {
		t.Errorf("main = %+v", def)
	}
	if v.Branches[0].Name != "hikari" || v.Branches[1].Name != "main" {
		t.Errorf("order = %v", v.Branches)
	}
	if len(*changes) == 0 {
		t.Error("the first view is published")
	}

	// A pull request without a push is refused.
	if _, err := m.OpenPullRequest(context.Background(), "shop", api.NewPullRequestRequest{Title: "Add a"}); err == nil || !strings.Contains(err.Error(), "push it first") {
		t.Errorf("OpenPullRequest before the push: %v", err)
	}
	if err := m.Push(context.Background(), "shop"); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View("shop")
	if yours := branch(v, "hikari"); !yours.Remote || yours.Unpushed != 0 {
		t.Errorf("after the push: %+v", yours)
	}
	f.checks["sha1"] = forge.Summarize([]forge.Check{{Name: "go test", State: forge.Passing}})
	pr, err := m.OpenPullRequest(context.Background(), "shop", api.NewPullRequestRequest{Title: "Add a", Body: "Closes #3"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 1 || pr.Head != "hikari" || pr.Base != "main" {
		t.Errorf("opened %+v", pr.PullRequest)
	}
	v, _ = m.View("shop")
	if len(v.PullRequests) != 1 || v.PullRequests[0].Checks.State != forge.Passing || v.Synced.IsZero() {
		t.Errorf("after the PR: %+v, synced %v", v.PullRequests, v.Synced)
	}
	if branch(v, "hikari").PullRequest != 1 {
		t.Errorf("your branch's pull request = %d", branch(v, "hikari").PullRequest)
	}
	if err := m.Merge(context.Background(), "shop", 1, forge.MergeSquash); err != nil {
		t.Fatal(err)
	}
	v, _ = m.View("shop")
	if v.PullRequests[0].State != forge.Merged || branch(v, "hikari").PullRequest != 0 {
		t.Errorf("after the merge: %+v", v.PullRequests[0].State)
	}
	if err := m.Merge(context.Background(), "shop", 9, ""); err == nil {
		t.Error("merging a missing PR fails")
	}
	if _, err := m.OpenPullRequest(context.Background(), "nope", api.NewPullRequestRequest{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown repo: %v", err)
	}
	s := Summary(m.Views())
	for _, want := range []string{"shop: your branch is hikari", "acme/shop on github", "1 ahead of main", "pushed", "no open pull request"} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary lacks %q:\n%s", want, s)
		}
	}
}

func TestForgeErrorAndRefresh(t *testing.T) {
	repo, _ := cloneOf(t)
	if _, err := repo.Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	f := &fake{fail: errors.New("github: Bad credentials")}
	m, _ := newManager(t, repo, f)
	err := m.Refresh(context.Background(), "shop")
	if err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("Refresh reports the forge's error: %v", err)
	}
	v, _ := m.View("shop")
	if v.Error == "" || v.Fetched.IsZero() || v.GitError != "" {
		t.Errorf("view = error %q, fetched %v, git error %q", v.Error, v.Fetched, v.GitError)
	}
	f.fail = nil
	if err := m.Refresh(context.Background(), "shop"); err != nil {
		t.Error(err)
	}
	v, _ = m.View("shop")
	if v.Error != "" {
		t.Errorf("the error clears: %q", v.Error)
	}
	if err := m.Refresh(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
}

func TestRepoWithoutForge(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	write(t, dir, "a.txt", "a\n")
	run(t, dir, "init", "--quiet", "--initial-branch=main")
	run(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "add", "--all")
	run(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "initial")
	repo := gitrepo.Open(dir)
	topo := &topology.Topology{Repos: map[string]topology.Repo{"tools": {ID: "tools", Name: "tools", Path: dir, Branch: "main"}}}
	m := New(topo, map[string]*gitrepo.Repo{"tools": repo}, Options{NewForge: func(forge.Remote) (forge.Forge, error) { t.Fatal("no origin, no forge"); return nil, nil }})
	v, _ := m.View("tools")
	if v.Forge != nil || v.URL != "" || v.Default != "main" || len(v.Branches) != 1 || !v.Branches[0].Yours {
		t.Errorf("view = %+v", v)
	}
	if err := m.Push(context.Background(), "tools"); err == nil {
		t.Error("a repo without origin cannot push")
	}
	if _, err := m.OpenPullRequest(context.Background(), "tools", api.NewPullRequestRequest{Title: "x"}); err == nil || !strings.Contains(err.Error(), "no origin") {
		t.Errorf("OpenPullRequest without origin: %v", err)
	}
	if _, ok := m.Tracker("tools"); ok {
		t.Error("no tracker without a forge")
	}
	if !strings.Contains(Summary(m.Views()), "tools: your branch is main") {
		t.Error(Summary(m.Views()))
	}
}
