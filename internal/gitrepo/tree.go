package gitrepo

import "strings"

// TreeEntry is one entry of a directory at a commit: a file or a
// directory, by its path relative to the repo, with slashes.
type TreeEntry struct {
	Path string
	Dir  bool
}

// ListTree lists the entries directly in dir at ref. dir is relative to
// the repo, with slashes; "" is the root. Submodules count as directories.
func (r *Repo) ListTree(ref, dir string) ([]TreeEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	args := []string{"ls-tree", "-z", ref}
	if dir != "" {
		args = append(args, "--", dir+"/")
	}
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	var entries []TreeEntry
	for _, l := range strings.Split(out, "\x00") {
		if l == "" {
			continue
		}
		// <mode> <type> <object>\t<path>
		meta, path, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) < 2 {
			continue
		}
		entries = append(entries, TreeEntry{Path: path, Dir: f[1] == "tree" || f[1] == "commit"})
	}
	return entries, nil
}
