// Package gitrepo runs the git commands that hikari needs on a repo on the
// host. Only the server writes to a repo, and it does so one command at a
// time for each repo.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
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

// Prepare checks that the repo has no uncommitted changes, creates branch
// from HEAD if it does not exist, and checks it out. It returns the commit
// at HEAD after the checkout.
func (r *Repo) Prepare(branch string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, err := r.git("status", "--porcelain")
	if err != nil {
		return "", err
	}
	if status != "" {
		return "", fmt.Errorf("%s has uncommitted changes; commit or stash them before hikari starts", r.Path)
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

// Head returns the commit at HEAD.
func (r *Repo) Head() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.git("rev-parse", "HEAD")
}

// Commit stages and commits every change inside scope, and nothing outside
// it, with agent as the author. It fails if branch is not checked out. It
// returns the new commit, or "" when the scope has no changes.
func (r *Repo) Commit(branch, scope, agent, message string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.git("symbolic-ref", "--short", "--quiet", "HEAD")
	if err != nil || current != branch {
		return "", fmt.Errorf("branch %q is not checked out in %s (HEAD is %q); hikari commits only to %q", branch, r.Path, current, branch)
	}
	spec := pathspec(scope)
	if _, err := r.git("add", "--all", "--", spec); err != nil {
		return "", err
	}
	if _, err := r.git("diff", "--cached", "--quiet", "--", spec); err == nil {
		return "", nil
	}
	if _, err := r.git("commit", "--quiet", "--no-verify", "--author", Author(agent), "-m", message, "--", spec); err != nil {
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
func (r *Repo) IsIgnored(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("check-ignore", "--quiet", path)
	return err == nil
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
			return strings.TrimSpace(stdout.String()), fmt.Errorf("git %s: exit status %d", args[0], exit.ExitCode())
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
