package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Commit is the last commit of a ref, as the repository views show it.
type Commit struct {
	SHA     string
	Subject string
	Author  string
	Time    time.Time
}

// Ref is one branch head: a local branch, or a branch of a remote.
type Ref struct {
	Name   string
	SHA    string
	Remote string // "" for a local branch
	// Upstream is the local branch's upstream, such as origin/main, or "".
	Upstream string
}

// Branches lists the local branches, each with its head and its upstream.
func (r *Repo) Branches() ([]Ref, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("for-each-ref", "--format=%(refname:short)%00%(objectname)%00%(upstream:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for _, l := range lines(out) {
		f := strings.Split(l, "\x00")
		if len(f) < 2 {
			continue
		}
		ref := Ref{Name: f[0], SHA: f[1]}
		if len(f) > 2 {
			ref.Upstream = f[2]
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// RemoteBranches lists the branches of remote as of the last fetch, without
// its HEAD pointer.
func (r *Repo) RemoteBranches(remote string) ([]Ref, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := "refs/remotes/" + remote + "/"
	out, err := r.git("for-each-ref", "--format=%(refname)%00%(objectname)", prefix)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for _, l := range lines(out) {
		name, sha, ok := strings.Cut(l, "\x00")
		if !ok || name == prefix+"HEAD" {
			continue
		}
		refs = append(refs, Ref{Name: strings.TrimPrefix(name, prefix), SHA: sha, Remote: remote})
	}
	return refs, nil
}

// DefaultBranch returns the default branch of remote: what its HEAD points
// at as of the last fetch, else main or master when one exists on the remote
// or locally, else "".
func (r *Repo) DefaultBranch(remote string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if head, err := r.git("symbolic-ref", "--short", "--quiet", "refs/remotes/"+remote+"/HEAD"); err == nil {
		return strings.TrimPrefix(head, remote+"/")
	}
	for _, name := range []string{"main", "master"} {
		if _, err := r.git("rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+name); err == nil {
			return name
		}
	}
	for _, name := range []string{"main", "master"} {
		if _, err := r.git("rev-parse", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	return ""
}

// AheadBehind counts the commits that ref has and base lacks, and the other
// way around. Both are full refs or names that git resolves.
func (r *Repo) AheadBehind(ref, base string) (ahead, behind int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("rev-list", "--left-right", "--count", ref+"..."+base)
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("git rev-list: unexpected output %q", out)
	}
	ahead, _ = strconv.Atoi(f[0])
	behind, _ = strconv.Atoi(f[1])
	return ahead, behind, nil
}

// ChangedBetween lists the paths under any of the paths that ref changed
// since it diverged from base: the diff from the merge base of base and
// ref to ref. A path of "" is the whole repo.
func (r *Repo) ChangedBetween(base, ref string, paths ...string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	args := []string{"diff", "--name-only", "--no-renames", base + "..." + ref, "--"}
	for _, p := range paths {
		args = append(args, pathspec(p))
	}
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

// IsAncestor reports whether a is reachable from b: a is merged into b.
func (r *Repo) IsAncestor(a, b string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("merge-base", "--is-ancestor", a, b)
	return err == nil
}

// LastCommit returns the last commit of ref.
func (r *Repo) LastCommit(ref string) (Commit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("log", "-1", "--format=%H%x00%s%x00%an%x00%cI", ref, "--")
	if err != nil {
		return Commit{}, err
	}
	f := strings.Split(out, "\x00")
	if len(f) != 4 {
		return Commit{}, fmt.Errorf("git log: unexpected output %q", out)
	}
	c := Commit{SHA: f[0], Subject: f[1], Author: f[2]}
	c.Time, _ = time.Parse(time.RFC3339, f[3])
	return c, nil
}

// HasRemote reports whether the repo has the named remote.
func (r *Repo) HasRemote(remote string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("remote", "get-url", remote)
	return err == nil
}

// FetchQuiet fetches remote without prompting for credentials, and gives up
// after timeout, so that a background fetch never hangs a run. It also
// updates the remote's HEAD pointer, so that DefaultBranch stays true.
func (r *Repo) FetchQuiet(ctx context.Context, remote string, timeout time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", r.Path, "fetch", "--quiet", "--prune", remote)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("git fetch %s: gave up after %s", remote, timeout)
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git fetch %s: %s", remote, firstLine(msg))
	}
	cmd = exec.CommandContext(ctx, "git", "-C", r.Path, "remote", "set-head", remote, "--auto")
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	cmd.Run() // best effort: a remote without HEAD keeps the fallbacks
	return nil
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}
