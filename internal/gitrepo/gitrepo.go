// Package gitrepo runs the git commands that hikari needs on a repo on the
// host. Only the server writes to a repo, and it does so one command at a
// time for each repo.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Repo struct {
	Path string
	mu   sync.Mutex
}

func Open(path string) *Repo {
	return &Repo{Path: path}
}

// Author returns the commit author for agent.
func Author(agent string) string {
	return fmt.Sprintf("%s (hikari) <%s>", agent, AuthorEmail(agent))
}

func AuthorEmail(agent string) string {
	return agent + "@hikari.invalid"
}

// runtimeFiles excludes the files that hikari keeps out of commits in each
// scope: test results and reports. The ignore file that does so is on the
// hikari branch, so on another branch they would look like changes.
var runtimeFiles = []string{
	":(exclude,glob)**/.hikari/private/results.yaml",
	":(exclude,glob)**/.hikari/private/reports/**",
}

// Prepare checks that the repo has no uncommitted changes, creates branch
// from HEAD if it does not exist, and checks it out. It returns the commit
// at HEAD after the checkout.
func (r *Repo) Prepare(branch string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, err := r.git(append([]string{"status", "--porcelain", "--untracked-files=all", "--", "."}, runtimeFiles...)...)
	if err != nil {
		return "", err
	}
	if status != "" {
		return "", fmt.Errorf("%s has uncommitted changes; commit or stash them before hikari starts: %s", r.Path, changedList(lines(status)))
	}
	if _, err := r.git("rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return "", fmt.Errorf("%s has no commits", r.Path)
	}
	if _, err := r.git("rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		if _, err := r.git("branch", branch); err != nil {
			return "", err
		}
	}
	if _, err := r.git("checkout", "--quiet", branch); err != nil {
		return "", err
	}
	return r.git("rev-parse", "HEAD")
}

// Branch returns the branch that is checked out, or "" when HEAD is
// detached.
func (r *Repo) Branch() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := r.git("symbolic-ref", "--short", "--quiet", "HEAD")
	return b
}

// Changed returns the paths with uncommitted changes, untracked files
// included, without the files that hikari keeps out of commits.
func (r *Repo) Changed() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, err := r.git(append([]string{"status", "--porcelain", "--untracked-files=all", "--no-renames", "--", "."}, runtimeFiles...)...)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range lines(status) {
		if len(l) > 3 {
			out = append(out, strings.Trim(l[3:], `"`))
		}
	}
	return out, nil
}

// Head returns the commit at HEAD.
func (r *Repo) Head() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.git("rev-parse", "HEAD")
}

// maxListed is the number of changed paths that a refusal names.
const maxListed = 10

// changedList names the paths of `git status --porcelain` lines, at most
// maxListed of them.
func changedList(status []string) string {
	var paths []string
	for _, l := range status {
		if len(l) > 3 {
			paths = append(paths, l[3:])
		}
	}
	if len(paths) > maxListed {
		return strings.Join(paths[:maxListed], ", ") + fmt.Sprintf(", and %d more", len(paths)-maxListed)
	}
	return strings.Join(paths, ", ")
}

// Commit stages and commits every change inside scope, and nothing outside
// it, with agent as the author. It fails if branch is not checked out. It
// returns the new commit, or "" when the scope has no changes.
func (r *Repo) Commit(branch, scope, agent, message string) (string, error) {
	return r.CommitPaths(branch, []string{pathspec(scope)}, agent, message)
}

// CommitPaths is Commit for several paths, relative to the repo. A path
// that neither exists nor is tracked is skipped. An empty agent commits
// with the user's own git identity.
func (r *Repo) CommitPaths(branch string, paths []string, agent, message string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.git("symbolic-ref", "--short", "--quiet", "HEAD")
	if err != nil || current != branch {
		return "", fmt.Errorf("branch %q is not checked out in %s (HEAD is %q); hikari commits only to %q", branch, r.Path, current, branch)
	}
	var specs []string
	for _, p := range paths {
		if _, err := os.Lstat(filepath.Join(r.Path, filepath.FromSlash(p))); err == nil {
			specs = append(specs, p)
		} else if tracked, _ := r.git("ls-files", "--", p); tracked != "" {
			specs = append(specs, p)
		}
	}
	if len(specs) == 0 {
		return "", nil
	}
	if _, err := r.git(append([]string{"add", "--all", "--"}, specs...)...); err != nil {
		return "", err
	}
	// git commit refuses a path that matches no tracked file, such as an
	// empty directory, so the commit names the staged files.
	staged, err := r.git(append([]string{"diff", "--cached", "--name-only", "--no-renames", "--"}, specs...)...)
	if err != nil {
		return "", err
	}
	if staged == "" {
		return "", nil
	}
	args := []string{"commit", "--quiet", "--no-verify", "-m", message}
	if agent != "" {
		args = append(args, "--author", Author(agent))
	}
	args = append(append(args, "--"), lines(staged)...)
	if _, err := r.git(args...); err != nil {
		return "", err
	}
	return r.git("rev-parse", "HEAD")
}

// ChangedSince returns the paths inside scope that differ from commit:
// committed changes, uncommitted changes, and untracked files. Paths are
// relative to scope. Paths under any of the exclude directories, which are
// relative to scope, are left out.
func (r *Repo) ChangedSince(commit, scope string, exclude ...string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec := pathspec(scope)
	diff, err := r.git("diff", "--name-only", "--no-renames", commit, "--", spec)
	if err != nil {
		return nil, err
	}
	untracked, err := r.git("ls-files", "--others", "--exclude-standard", "--", spec)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range append(lines(diff), lines(untracked)...) {
		rel := p
		if scope != "" {
			rel = strings.TrimPrefix(p, scope+"/")
		}
		if seen[rel] || excluded(rel, exclude) {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	return out, nil
}

// Dirty reports whether scope has uncommitted changes: modified, deleted,
// or untracked files that git does not ignore.
func (r *Repo) Dirty(scope string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("status", "--porcelain", "--untracked-files=all", "--", pathspec(scope))
	return out != "", err
}

// HasCommit reports whether commit names a commit in the repo.
func (r *Repo) HasCommit(commit string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// CountAuthored returns the number of commits by agent between since and
// HEAD.
func (r *Repo) CountAuthored(since, agent string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("rev-list", "--count", "--author="+AuthorEmail(agent), since+"..HEAD")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// IsIgnored reports whether git ignores path, which is relative to the repo.
// The path need not exist; a path that ends in a slash is a directory.
func (r *Repo) IsIgnored(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("check-ignore", "--quiet", path)
	return err == nil
}

// Show returns the content of path, relative to the repo, at ref.
func (r *Repo) Show(ref, path string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("show", ref+":"+path)
	return []byte(out), err
}

// Tracked reports whether git tracks path, which is relative to the repo.
func (r *Repo) Tracked(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, _ := r.git("ls-files", "--", path)
	return out != ""
}

// CountOn returns the number of commits between since and HEAD that
// change any of paths, by any author.
func (r *Repo) CountOn(since string, paths ...string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git(append([]string{"rev-list", "--count", since + "..HEAD", "--"}, paths...)...)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

func pathspec(scope string) string {
	if scope == "" {
		return "."
	}
	return scope
}

func excluded(p string, dirs []string) bool {
	for _, d := range dirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (r *Repo) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", r.Path}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		var exit *exec.ExitError
		if errors.As(err, &exit) && msg == "" {
			return strings.TrimRight(stdout.String(), " \t\r\n"), fmt.Errorf("git %s: exit status %d", args[0], exit.ExitCode())
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimRight(stdout.String(), " \t\r\n"), nil
}
