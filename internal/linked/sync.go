package linked

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/topology"
)

// Result is one line of what sync or push did to a repo.
type Result struct {
	Repo   string
	Text   string
	Failed bool
}

// Failed reports whether any result failed (rules SY8 and PS4).
func Failed(results []Result) bool {
	return slices.ContainsFunc(results, func(r Result) bool { return r.Failed })
}

// Sync makes a clone of the project complete (rules SY1 to SY7): it clones
// the missing linked repos, fetches, makes your branch where the remote has
// it, and reports how each branch compares with the remote. With from set,
// a branch that neither exists nor is on the remote starts from the
// remote's from. It never merges, rebases, resets, or checks out.
func Sync(topo *topology.Topology, from string) []Result {
	var out []Result
	add := func(repo, text string, failed bool) { out = append(out, Result{repo, text, failed}) }
	if p := topo.Project; p != nil {
		repo := gitrepo.Open(p.Path)
		if repo.RemoteURL(gitrepo.Origin) == "" {
			add(topology.ProjectRepo, "has no origin; nothing to fetch", false)
		} else if err := repo.Fetch(gitrepo.Origin); err != nil {
			add(topology.ProjectRepo, "fetch: "+err.Error(), true)
		} else {
			text, failed := track(repo, p.Branch, from)
			add(topology.ProjectRepo, text, failed)
		}
		out = append(out, mapFindings(topo, topology.ProjectRepo, repo)...)
	}
	for _, name := range topo.Linked() {
		r := topo.Repos[name]
		var repo *gitrepo.Repo
		if slices.Contains(topo.Missing, name) {
			cloned, err := gitrepo.Clone(r.URL, r.Path)
			if err != nil {
				add(name, "clone: "+err.Error(), true)
				continue
			}
			repo = cloned
			add(name, "cloned "+r.URL, false)
		} else {
			repo = gitrepo.Open(r.Path)
			if origin := repo.RemoteURL(gitrepo.Origin); origin != r.URL {
				add(name, fmt.Sprintf("its origin is %q, not the topology's url %q; left alone", origin, r.URL), true)
				continue
			}
			if err := repo.Fetch(gitrepo.Origin); err != nil {
				add(name, "fetch: "+err.Error(), true)
				continue
			}
		}
		text, failed := track(repo, r.Branch, from)
		add(name, text, failed)
		out = append(out, mapFindings(topo, name, repo)...)
	}
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		if name != topology.ProjectRepo && topo.Repos[name].URL == "" {
			add(name, "is a path repository, so a teammate's clone cannot get it; give it a url to share it", false)
		}
	}
	return out
}

// track makes branch from the remote when it is missing (rule SY4), or
// says how it compares with the remote (SY5).
func track(repo *gitrepo.Repo, branch, from string) (string, bool) {
	if !repo.HasBranch(branch) {
		switch {
		case repo.HasRemoteBranch(gitrepo.Origin, branch):
			if err := repo.MakeBranch(branch, gitrepo.Origin, branch); err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("made %s from origin/%s", branch, branch), false
		case from != "" && repo.HasRemoteBranch(gitrepo.Origin, from):
			if err := repo.MakeBranch(branch, gitrepo.Origin, from); err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("made %s from origin/%s", branch, from), false
		case from != "":
			return fmt.Sprintf("origin has no %s; hikari up will make %s from HEAD", from, branch), false
		}
		return fmt.Sprintf("has no %s yet; hikari up will make it from HEAD", branch), false
	}
	if !repo.HasRemoteBranch(gitrepo.Origin, branch) {
		return fmt.Sprintf("%s is not on the remote yet", branch), false
	}
	ahead, behind, err := repo.Compare(branch, gitrepo.Origin)
	switch {
	case err != nil:
		return err.Error(), true
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("%s has diverged from origin/%s: %d ahead, %d behind; merge it with git", branch, branch, ahead, behind), false
	case ahead > 0:
		return fmt.Sprintf("%s is ahead of origin/%s by %s; hikari push sends them", branch, branch, commits(ahead)), false
	case behind > 0:
		return fmt.Sprintf("%s is behind origin/%s by %s; merge them with git", branch, branch, commits(behind)), false
	}
	return fmt.Sprintf("%s is up to date with origin/%s", branch, branch), false
}

// mapFindings reports each map of a system in repo whose commit the repo does not
// have (rule SY6).
func mapFindings(topo *topology.Topology, name string, repo *gitrepo.Repo) []Result {
	var out []Result
	commits := mapCommits(topo, name)
	for _, owner := range slices.Sorted(maps.Keys(commits)) {
		if c := commits[owner]; !repo.HasCommit(c) {
			out = append(out, Result{name, fmt.Sprintf("the map of %s describes commit %.12s, which this clone does not have; the teammate who ran %s may not have pushed it", owner, c, owner), true})
		}
	}
	return out
}

func commits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}
