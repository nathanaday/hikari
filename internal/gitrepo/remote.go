package gitrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Origin is the remote that hikari syncs and pushes.
const Origin = "origin"

// Clone clones url into dir, which must not exist. The url comes after --,
// so that git never reads it as an option.
func Clone(url, dir string) (*Repo, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	r := Open(filepath.Dir(dir))
	if _, err := r.git("clone", "--quiet", "--", url, filepath.Base(dir)); err != nil {
		return nil, err
	}
	return Open(dir), nil
}

// RemoteURL returns the URL of remote, or "" when the repo has no such
// remote.
func (r *Repo) RemoteURL(remote string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("remote", "get-url", remote)
	if err != nil {
		return ""
	}
	return out
}

// Fetch fetches remote.
func (r *Repo) Fetch(remote string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("fetch", "--quiet", remote)
	return err
}

// HasBranch reports whether the local branch exists.
func (r *Repo) HasBranch(branch string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// HasRemoteBranch reports whether the last fetch of remote saw branch.
func (r *Repo) HasRemoteBranch(remote, branch string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.git("rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+branch)
	return err == nil
}

// MakeBranch makes the local branch from remote's from. When from is branch,
// the new branch tracks it; otherwise it has no upstream until it is
// pushed.
func (r *Repo) MakeBranch(branch, remote, from string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	track := "--no-track"
	if from == branch {
		track = "--track"
	}
	_, err := r.git("branch", track, branch, "refs/remotes/"+remote+"/"+from)
	return err
}

// Compare counts the commits that branch has and remote's branch lacks
// (ahead), and the other way around (behind).
func (r *Repo) Compare(branch, remote string) (ahead, behind int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("rev-list", "--left-right", "--count", "refs/heads/"+branch+"...refs/remotes/"+remote+"/"+branch)
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

// Unpushed counts the commits of branch that no branch of remote has, as of
// the last fetch.
func (r *Repo) Unpushed(branch, remote string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out, err := r.git("rev-list", "--count", "refs/heads/"+branch, "--not", "--remotes="+remote)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// Push pushes branch to the same name on remote, never forced, and sets the
// branch's upstream. A refusal names git's reason.
func (r *Repo) Push(branch, remote string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref := "refs/heads/" + branch
	_, err := r.git("push", "--quiet", "--set-upstream", remote, ref+":"+ref)
	if err != nil {
		return fmt.Errorf("%s", pushReason(err.Error()))
	}
	return nil
}

// pushReason keeps the line of git's output that says why it refused a
// push, without the hints.
func pushReason(msg string) string {
	for _, l := range strings.Split(msg, "\n") {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "[rejected]") || strings.Contains(l, "[remote rejected]") {
			return l
		}
	}
	for _, l := range strings.Split(msg, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "fatal:") || strings.HasPrefix(l, "error:") {
			return l
		}
	}
	return msg
}
