// Package workspace maps a partition's workspace, the folders that its
// container sees under /workspace, to the folders of the repos on the host.
// Paths in the code map and the test map are workspace paths, relative to
// /workspace, with slashes.
package workspace

import (
	"path"
	"path/filepath"
	"strings"
)

// Folder is one folder of a partition.
type Folder struct {
	// Place is where the container sees the folder, relative to /workspace.
	Place string
	// Repo is the repo's name, and Path the folder in it ("" for the whole
	// repo).
	Repo string
	Path string
	// Host is the folder on the host.
	Host     string
	ReadOnly bool
}

// Workspace is the folders of one partition, in the order of the topology.
type Workspace []Folder

// Home returns the first writable folder, which holds the private
// documents.
func (w Workspace) Home() Folder {
	for _, f := range w {
		if !f.ReadOnly {
			return f
		}
	}
	return Folder{}
}

// Writable returns the writable folders, in order.
func (w Workspace) Writable() []Folder {
	var out []Folder
	for _, f := range w {
		if !f.ReadOnly {
			out = append(out, f)
		}
	}
	return out
}

// Repos returns the repos of the writable folders, each once, in the order
// of their first folder.
func (w Workspace) Repos() []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range w.Writable() {
		if !seen[f.Repo] {
			seen[f.Repo] = true
			out = append(out, f.Repo)
		}
	}
	return out
}

// InRepo returns the writable folders of repo.
func (w Workspace) InRepo(repo string) []Folder {
	var out []Folder
	for _, f := range w.Writable() {
		if f.Repo == repo {
			out = append(out, f)
		}
	}
	return out
}

// Find returns the folder that holds the workspace path p, and p relative
// to that folder.
func (w Workspace) Find(p string) (Folder, string, bool) {
	p = path.Clean(p)
	for _, f := range w {
		if p == f.Place {
			return f, "", true
		}
		if rest, ok := strings.CutPrefix(p, f.Place+"/"); ok {
			return f, rest, true
		}
	}
	return Folder{}, "", false
}

// Host returns the host path of the workspace path p, or false when p is
// in no folder.
func (w Workspace) Host(p string) (string, bool) {
	f, rest, ok := w.Find(p)
	if !ok {
		return "", false
	}
	return filepath.Join(f.Host, filepath.FromSlash(rest)), true
}

// Commit is one entry of a list of described commits: <repo>@<commit>.
func Commit(repo, commit string) string { return repo + "@" + commit }

// CommitOf returns the commit that commits records for repo, or "".
func CommitOf(commits []string, repo string) string {
	for _, c := range commits {
		if r, commit, ok := strings.Cut(c, "@"); ok && r == repo {
			return commit
		}
	}
	return ""
}
