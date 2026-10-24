package linked

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

// Push pushes your branch of each linked repo, then of the project
// repository (rules PS1 to PS5). The project repository is pushed only when
// every linked push succeeded, because its maps describe the linked repos'
// commits.
func Push(topo *topology.Topology) []Result {
	var out []Result
	refused := false
	for _, name := range topo.Linked() {
		if slices.Contains(topo.Missing, name) {
			out = append(out, Result{name, "skipped: not cloned; run hikari sync", false})
			continue
		}
		r := push(name, topo.Repos[name])
		refused = refused || r.Failed
		out = append(out, r)
	}
	if p := topo.Project; p != nil {
		if refused {
			out = append(out, Result{topology.ProjectRepo, "not pushed: a push above was refused, and the maps here may describe commits that are not on the remote; fix that push, and run hikari push again", true})
		} else {
			out = append(out, push(topology.ProjectRepo, *p))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		if name != topology.ProjectRepo && topo.Repos[name].URL == "" {
			out = append(out, Result{name, "skipped: a path repository", false})
		}
	}
	return out
}

func push(name string, r topology.Repo) Result {
	repo := gitrepo.Open(r.Path)
	switch {
	case !repo.HasBranch(r.Branch):
		return Result{name, fmt.Sprintf("skipped: no %s yet", r.Branch), false}
	case repo.RemoteURL(gitrepo.Origin) == "":
		return Result{name, "skipped: no origin", false}
	}
	if err := repo.Fetch(gitrepo.Origin); err != nil {
		return Result{name, "fetch: " + err.Error(), true}
	}
	n, err := repo.Unpushed(r.Branch, gitrepo.Origin)
	if err != nil {
		return Result{name, err.Error(), true}
	}
	known := repo.HasRemoteBranch(gitrepo.Origin, r.Branch)
	if err := repo.Push(r.Branch, gitrepo.Origin); err != nil {
		return Result{name, fmt.Sprintf("refused: %v", err), true}
	}
	switch {
	case n > 0:
		return Result{name, fmt.Sprintf("pushed %s to origin/%s", commits(n), r.Branch), false}
	case !known:
		return Result{name, fmt.Sprintf("pushed %s to the remote", r.Branch), false}
	}
	return Result{name, fmt.Sprintf("%s is up to date", r.Branch), false}
}
