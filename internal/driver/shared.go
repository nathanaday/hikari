package driver

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/vault"
)

// builtin holds hikari's own skill packages and agents, which always come
// from the binary (rule H11).
//
//go:embed skills agents
var builtin embed.FS

// sharedArea connects the vault to Claude Code in the container (rules H1
// to H10, and V7 of the vault layout spec): the skills that the agent is
// granted, the skill of each deliverable that it holds, and the policies.
type sharedArea struct {
	home string
	// public is the public area in the container, and skills the
	// directory of its granted skills.
	public string
	skills string
	// builtins are hikari's own skills that the agent's roles need.
	builtins     []string
	deliverables []string
	granted      []string
	warned       map[string]bool
}

func newSharedArea(builtins, deliverables, granted []string) (*sharedArea, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &sharedArea{home: home, public: api.PublicDir, skills: api.SkillsDir, builtins: builtins, deliverables: deliverables, granted: granted, warned: map[string]bool{}}, nil
}

// builtinSkills returns hikari's own skills that self's roles need: the
// partition skill for a partition agent, the orchestration skill for the orchestration agent.
func builtinSkills(self api.Self) []string {
	var out []string
	if self.Partition != nil {
		out = append(out, api.PartitionSkill)
	}
	if self.Orchestration {
		out = append(out, api.OrchestrationSkill)
	}
	return out
}

func (s *sharedArea) skillsDir() string { return filepath.Join(s.home, ".claude", "skills") }
func (s *sharedArea) agentsDir() string { return filepath.Join(s.home, ".claude", "agents") }

// sync links the granted skills and the skills of the agent's
// deliverables, installs the built-in skill, and returns the policies as a
// system prompt section. Problems are logged, so that a broken vault never
// stops a turn.
func (s *sharedArea) sync() string {
	for _, d := range []string{s.skillsDir(), s.agentsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Printf("vault: %v", err)
			return ""
		}
	}
	for _, name := range s.builtins {
		s.writeBuiltin(name)
	}
	for _, name := range s.granted {
		switch {
		case slices.Contains(api.BuiltinSkills, name) || strings.HasPrefix(name, api.AgentPrefix):
			s.warnOnce(name, "ignoring the skill %s; the prefix %s is reserved for hikari's own skills", name, api.AgentPrefix)
			continue
		case slices.Contains(s.deliverables, name):
			s.warnOnce(name, "ignoring the skill %s; the deliverable %s has the same name", name, name)
			continue
		}
		dir := filepath.Join(s.skills, name)
		if _, err := os.Stat(filepath.Join(dir, vault.SkillFile)); err != nil {
			s.warnOnce(name, "the granted skill %s has no %s", name, vault.SkillFile)
			continue
		}
		s.link(dir, filepath.Join(s.skillsDir(), name))
	}
	for _, name := range s.deliverables {
		s.link(filepath.Join(s.public, vault.Deliverables, name), filepath.Join(s.skillsDir(), name))
	}
	s.removeDangling(s.skillsDir())
	return s.policies()
}

func (s *sharedArea) warnOnce(key, format string, args ...any) {
	if !s.warned[key] {
		log.Printf("vault: "+format, args...)
		s.warned[key] = true
	}
}

func (s *sharedArea) files(kind, ext string) []string {
	entries, _ := os.ReadDir(filepath.Join(s.public, kind))
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out
}

// writeBuiltin installs one of hikari's own skill packages, and with the
// partition skill, hikari's agents, replacing whatever is at their paths, such
// as a link that a skill of the same name left behind.
func (s *sharedArea) writeBuiltin(skill string) {
	dir := filepath.Join(s.skillsDir(), skill)
	os.RemoveAll(dir)
	err := fs.WalkDir(builtin, "skills/"+skill, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("skills/"+skill, path)
		data, err := builtin.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		log.Printf("vault: install the %s skill: %v", skill, err)
	}
	if skill != api.PartitionSkill {
		return
	}
	agents, _ := fs.ReadDir(builtin, "agents")
	for _, a := range agents {
		data, err := builtin.ReadFile("agents/" + a.Name())
		if err == nil {
			at := filepath.Join(s.agentsDir(), a.Name())
			os.RemoveAll(at)
			err = os.WriteFile(at, data, 0o644)
		}
		if err != nil {
			log.Printf("vault: install the agent %s: %v", a.Name(), err)
		}
	}
}

// link points at to target, replacing whatever is there unless it already
// points at target.
func (s *sharedArea) link(target, at string) {
	if current, err := os.Readlink(at); err == nil && current == target {
		return
	}
	os.RemoveAll(at)
	if err := os.Symlink(target, at); err != nil {
		log.Printf("vault: link %s: %v", at, err)
	}
}

// removeDangling removes links into the vault whose target is gone.
func (s *sharedArea) removeDangling(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		at := filepath.Join(dir, e.Name())
		target, err := os.Readlink(at)
		if err != nil || !strings.HasPrefix(target, s.public+"/") && !strings.HasPrefix(target, s.skills+"/") {
			continue
		}
		if _, err := os.Stat(target); err != nil {
			os.Remove(at)
		}
	}
}

// policies returns every policy, in name order, as a system prompt
// section.
func (s *sharedArea) policies() string {
	var b strings.Builder
	for _, name := range s.files(vault.Policies, ".md") {
		data, err := os.ReadFile(filepath.Join(s.public, vault.Policies, name))
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("\n# Policies\n\nThese rules apply to every agent in this network.\n")
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", strings.TrimSuffix(name, ".md"), strings.TrimSpace(string(data)))
	}
	return b.String()
}

// BuiltinFile returns one file of hikari's own skills and agents, for the
// host skills that reuse it (decision 6 of the hikify design spec).
func BuiltinFile(name string) ([]byte, error) { return builtin.ReadFile(name) }
