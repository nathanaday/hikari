// Package coverage finds the code of a repository that no partition owns:
// the tracked paths outside every writable folder (the uncovered code of
// the topology). A path is listed at the highest directory that holds no
// partition folder, so that a repository whose services/ holds three
// partitions and two orphans lists the two orphans, not every file.
package coverage

import (
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

// Lister lists the entries directly in a directory of a repository.
type Lister interface {
	ListTree(ref, dir string) ([]gitrepo.TreeEntry, error)
}

// Uncovered returns the tracked paths of a repository at ref that no
// writable folder covers. writable holds the folders' paths, relative to
// the repo, with slashes; "" is the whole repo, which covers everything. A
// directory is listed with a trailing slash. Without any writable folder,
// every path is uncovered, so the result is nil: the repository is not
// partitioned, which is another statement.
func Uncovered(repo Lister, ref string, writable []string) ([]string, error) {
	if len(writable) == 0 || slices.Contains(writable, "") {
		return nil, nil
	}
	var out []string
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := repo.ListTree(ref, dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			switch {
			case covered(e.Path, writable):
			case e.Dir && holdsFolder(e.Path, writable):
				if err := walk(e.Path); err != nil {
					return err
				}
			case e.Dir:
				out = append(out, e.Path+"/")
			default:
				out = append(out, e.Path)
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	return out, nil
}

// covered reports whether p is a writable folder or inside one.
func covered(p string, writable []string) bool {
	for _, w := range writable {
		if p == w || strings.HasPrefix(p, w+"/") {
			return true
		}
	}
	return false
}

// holdsFolder reports whether a writable folder is strictly inside dir.
func holdsFolder(dir string, writable []string) bool {
	for _, w := range writable {
		if strings.HasPrefix(w, dir+"/") {
			return true
		}
	}
	return false
}

// WritableFolders returns the paths of the writable folders that the
// partitions of topo have in repo, for Uncovered.
func WritableFolders(topo *topology.Topology, repo string) []string {
	var out []string
	for _, name := range topo.PartitionAgents() {
		for _, f := range topo.Agents[name].Partition.Writable() {
			if f.Repo == repo {
				out = append(out, f.Path)
			}
		}
	}
	return out
}
