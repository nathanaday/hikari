// Package linked works on a project and its repositories as a whole: it
// checks that the project can start, syncs the clones of linked repos, and
// pushes the hikari branches (the linked repositories spec).
package linked

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
)

// Finding is one result of a check.
type Finding struct {
	Text    string
	Warning bool
}

// Check runs the checks of rule CK1 that follow the topology's own: the
// missing clones, the old layouts, the granted skills, the project's
// ignore rules, uncommitted changes, and the credentials. It writes nothing and makes no network call (CK2).
func Check(topo *topology.Topology, credentials bool) []Finding {
	var out []Finding
	fail := func(format string, args ...any) { out = append(out, Finding{Text: fmt.Sprintf(format, args...)}) }
	for _, name := range topo.Missing {
		fail("the linked repo %s is not cloned; run: hikari sync %s", name, Shown(topo.File))
	}
	for _, f := range OldLayout(topo) {
		fail("%s", f)
	}
	out = append(out, checkSkills(topo)...)
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
			if !topo.ServiceOf(n).Credentials() {
				out = append(out, Finding{Text: fmt.Sprintf("agent %s has neither ANTHROPIC_API_KEY nor CLAUDE_CODE_OAUTH_TOKEN set", n), Warning: true})
			}
		}
	}
	return out
}

// OldLayout returns a problem for each part of an earlier layout of the
// vault and the private files, with the move that fixes it (rule V11 of
// the vault layout spec). hikari moves nothing itself.
func OldLayout(topo *topology.Topology) []string {
	var out []string
	if topo.VaultDefault && isDir(filepath.Join(topo.Dir, "store")) && !exists(topo.Vault) {
		out = append(out, fmt.Sprintf("the topology file sets no x-hikari.vault, and %s holds store/ and no %s/, the old layout: rename store/ to %s/, and move its files to the new layout; see %s", topo.Dir, topology.DefaultVault, topology.DefaultVault, vault.MigratingGuide))
	}
	out = append(out, topo.VaultOf().OldLayout()...)
	for _, name := range topo.PartitionAgents() {
		s := topo.Agents[name].Partition
		if slices.Contains(topo.Missing, s.Repo) {
			continue
		}
		dir := filepath.Join(topo.Repos[s.Repo].Path, s.Scope, featuremap.Dir, featuremap.PrivateDir)
		if exists(filepath.Join(dir, featuremap.OldCodeMapFile)) && !exists(filepath.Join(dir, featuremap.CodeMapFile)) {
			out = append(out, fmt.Sprintf("the private directory %s of %s holds %s and no %s, the old layout: rename %s to %s; see %s", dir, name, featuremap.OldCodeMapFile, featuremap.CodeMapFile, featuremap.OldCodeMapFile, featuremap.CodeMapFile, vault.MigratingGuide))
		}
	}
	return out
}

// checkSkills checks each granted skill (rule V7): it is a directory of the
// vault's skills/ with a SKILL.md, and no deliverable has its name.
func checkSkills(topo *topology.Topology) []Finding {
	var out []Finding
	st := topo.VaultOf()
	deliverables := map[string]bool{}
	for _, a := range topo.Agents {
		for _, d := range a.Grants.Deliverables {
			deliverables[d] = true
		}
	}
	for _, name := range topo.AgentNames() {
		for _, sk := range topo.Agents[name].Grants.Skills {
			dir := st.SkillDir(sk)
			var problem string
			switch {
			case deliverables[sk] || isDir(st.DeliverableDir(sk)):
				problem = "has the name of a deliverable; rename one of them"
			case !isDir(dir):
				problem = "does not exist: " + dir + " is not a directory"
			case !exists(filepath.Join(dir, vault.SkillFile)):
				problem = "has no " + vault.SkillFile + " in " + dir
			default:
				continue
			}
			out = append(out, Finding{Text: fmt.Sprintf("agent %s is granted the skill %s, which %s", name, sk, problem)})
		}
	}
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
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
	if name == topology.ProjectRepo && repo.Branch() == r.Branch && !slices.ContainsFunc(changed, func(p string) bool { return !partitions.IsUserPath(topo.VaultInProject(), p) }) {
		return nil
	}
	return changed
}

// Shown returns path relative to the working directory when it is inside
// it, for a command that a person copies.
func Shown(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && filepath.IsLocal(rel) {
			return rel
		}
	}
	return path
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

// mapCommits returns the commit that each partition's map describes, for the
// partitions of repo. In a project repository, the maps are read from your
// branch, which hikari up checks out; otherwise from the vault's files.
func mapCommits(topo *topology.Topology, repo string) map[string]string {
	out := map[string]string{}
	st := topo.VaultOf()
	var project *gitrepo.Repo
	if p := topo.Project; p != nil && topo.VaultInProject() != "" {
		if r := gitrepo.Open(p.Path); r.HasBranch(p.Branch) {
			project = r
		}
	}
	for _, partitionAgent := range topo.PartitionAgents() {
		if topo.Agents[partitionAgent].Partition.Repo != repo {
			continue
		}
		var data []byte
		var err error
		if project != nil {
			data, err = project.Show("refs/heads/"+topo.Project.Branch, topo.VaultInProject()+"/"+st.Rel(st.MapFile(partitionAgent)))
		} else {
			data, err = os.ReadFile(st.MapFile(partitionAgent))
		}
		if err != nil {
			continue
		}
		if m, err := featuremap.Decode(data); err == nil && m.Partition.DescribedState.Commit != "" {
			out[partitionAgent] = m.Partition.DescribedState.Commit
		}
	}
	return out
}
