// Package linked works on a project and its repositories as a whole: it
// checks that the project can start, syncs the clones of linked repos, and
// pushes the hikari branches (the linked repositories spec).
package linked

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/store"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
)

// Finding is one result of a check.
type Finding struct {
	Text    string
	Warning bool
}

// Check runs the checks of rule CK1 that follow the topology's own: the
// missing clones, the project's ignore rules, uncommitted changes, and the
// credentials. It writes nothing and makes no network call (CK2).
func Check(topo *topology.Topology, credentials bool) []Finding {
	var out []Finding
	fail := func(format string, args ...any) { out = append(out, Finding{Text: fmt.Sprintf(format, args...)}) }
	for _, name := range topo.Missing {
		fail("the linked repo %s is not cloned; run: hikari sync %s", name, topo.File)
	}
	if p := topo.Project; p != nil {
		repo := gitrepo.Open(p.Path)
		// Each path to check, with the .gitignore line that ignores it.
		need := [][2]string{{".env", ".env"}, {".hikari/", "/.hikari/"}}
		if len(topo.Linked()) > 0 {
			need = append(need, [2]string{topology.CloneDir + "/", "/" + topology.CloneDir + "/"})
		}
		var lines []string
		for _, n := range need {
			if !repo.IsIgnored(n[0]) {
				lines = append(lines, n[1])
			}
		}
		if len(lines) > 0 {
			fail("the project repository %s must ignore %s; add these lines to its .gitignore: %s", p.Path, strings.Join(lines, " and "), strings.Join(lines, ", "))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		if slices.Contains(topo.Missing, name) {
			continue
		}
		if changed := Uncommitted(topo, name); len(changed) > 0 {
			fail("%s has uncommitted changes; commit or stash them before hikari starts: %s", label(topo, name), listed(changed))
		}
	}
	if credentials {
		for _, n := range topo.AgentNames() {
			if !topo.Services[n].Credentials() {
				out = append(out, Finding{Text: fmt.Sprintf("agent %s has neither ANTHROPIC_API_KEY nor CLAUDE_CODE_OAUTH_TOKEN set", n), Warning: true})
			}
		}
	}
	return out
}

// Uncommitted returns the changed paths of a repo that would stop hikari
// up. In the project repository on its branch, edits only to the wikis do
// not count: hikari up commits them for you (rule C5 of the wikis spec).
func Uncommitted(topo *topology.Topology, name string) []string {
	r := topo.Repos[name]
	repo := gitrepo.Open(r.Path)
	changed, err := repo.Changed()
	if err != nil {
		return []string{err.Error()}
	}
	if name == topology.ProjectRepo && repo.Branch() == r.Branch && !slices.ContainsFunc(changed, func(p string) bool { return !systems.IsWikiPath(topo.StoreInProject(), p) }) {
		return nil
	}
	return changed
}

func label(topo *topology.Topology, name string) string {
	if name == topology.ProjectRepo {
		return "the project repository " + topo.Repos[name].Path
	}
	return "repo " + name + " (" + topo.Repos[name].Path + ")"
}

func listed(paths []string) string {
	const max = 10
	if len(paths) > max {
		return strings.Join(paths[:max], ", ") + fmt.Sprintf(", and %d more", len(paths)-max)
	}
	return strings.Join(paths, ", ")
}

// mapCommits returns the commit that each system's map describes, for the
// systems of repo. In a project repository, the maps are read from your
// branch, which hikari up checks out; otherwise from the store's files.
func mapCommits(topo *topology.Topology, repo string) map[string]string {
	out := map[string]string{}
	st := store.Store{Root: topo.Store}
	var project *gitrepo.Repo
	if p := topo.Project; p != nil && topo.StoreInProject() != "" {
		if r := gitrepo.Open(p.Path); r.HasBranch(p.Branch) {
			project = r
		}
	}
	for _, owner := range topo.Owners() {
		if topo.Agents[owner].System.Repo != repo {
			continue
		}
		var data []byte
		var err error
		if project != nil {
			data, err = project.Show("refs/heads/"+topo.Project.Branch, topo.StoreInProject()+"/systems/"+owner+"/map.yaml")
		} else {
			data, err = os.ReadFile(st.MapFile(owner))
		}
		if err != nil {
			continue
		}
		var m sysmap.Map
		if yaml.Unmarshal(data, &m) == nil && m.System.DescribedState.Commit != "" {
			out[owner] = m.System.DescribedState.Commit
		}
	}
	return out
}
