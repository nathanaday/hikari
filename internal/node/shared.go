package node

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
	"github.com/nathanaday/hikari/internal/store"
)

// builtin holds hikari's own skill packages and agents, which always come
// from the binary (rule H11).
//
//go:embed skills agents
var builtin embed.FS

// sharedArea connects the store's shared area to Claude Code in the
// container (rules H1 to H10): the shared skills and agents, the policies,
// and the skill of each deliverable that the agent holds.
type sharedArea struct {
	home string
	dir  string // the shared area in the container
	// builtins are hikari's own skills that the agent's roles need.
	builtins     []string
	deliverables []string
	warned       map[string]bool
}

func newSharedArea(builtins, deliverables []string) (*sharedArea, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &sharedArea{home: home, dir: api.SharedDir, builtins: builtins, deliverables: deliverables, warned: map[string]bool{}}, nil
}

// builtinSkills returns hikari's own skills that self's roles need: the
// system skill for an owner, the hub skill for an orchestrator.
func builtinSkills(self api.Self) []string {
	var out []string
	if self.System != nil {
		out = append(out, api.SystemSkill)
	}
	if len(self.Orchestrates) > 0 {
		out = append(out, api.HubSkill)
	}
	return out
}

func (s *sharedArea) skillsDir() string { return filepath.Join(s.home, ".claude", "skills") }
func (s *sharedArea) agentsDir() string { return filepath.Join(s.home, ".claude", "agents") }

// sync links the shared skills and agents and the skills of the agent's
// deliverables, installs the built-in skill, and returns the policies as a
// system prompt section. Problems are logged, so that a broken shared area
// never stops a turn.
func (s *sharedArea) sync() string {
	for _, d := range []string{s.skillsDir(), s.agentsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Printf("shared area: %v", err)
			return ""
		}
	}
	for _, name := range s.builtins {
		s.writeBuiltin(name)
	}
	for _, name := range s.sharedSkills() {
		switch {
		case slices.Contains(api.BuiltinSkills, name):
			s.warnOnce(name, "ignoring skills/%s; hikari's own %s skill always comes from hikari", name, name)
			continue
		case slices.Contains(s.deliverables, name):
			s.warnOnce(name, "ignoring skills/%s; the deliverable %s has the same name", name, name)
			continue
		}
		s.link(filepath.Join(s.dir, "skills", name), filepath.Join(s.skillsDir(), name))
	}
	for _, name := range s.deliverables {
		s.link(filepath.Join(s.dir, store.Deliverables, name), filepath.Join(s.skillsDir(), name))
	}
	for _, name := range s.files("agents", ".md") {
		if strings.HasPrefix(name, api.AgentPrefix) {
			s.warnOnce("agent:"+name, "ignoring agents/%s; the prefix %s is reserved for hikari's own agents", name, api.AgentPrefix)
			continue
		}
		s.link(filepath.Join(s.dir, "agents", name), filepath.Join(s.agentsDir(), name))
	}
	s.removeDangling(s.skillsDir())
	s.removeDangling(s.agentsDir())
	return s.policies()
}

func (s *sharedArea) warnOnce(key, format string, args ...any) {
	if !s.warned[key] {
		log.Printf("shared area: "+format, args...)
		s.warned[key] = true
	}
}

// sharedSkills returns the names of the shared skill directories that hold
// a SKILL.md.
func (s *sharedArea) sharedSkills() []string {
	entries, _ := os.ReadDir(filepath.Join(s.dir, "skills"))
	var out []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(s.dir, "skills", e.Name(), "SKILL.md")); err == nil {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *sharedArea) files(kind, ext string) []string {
	entries, _ := os.ReadDir(filepath.Join(s.dir, kind))
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
// system skill hikari's agents, replacing whatever is at their paths, such
// as a link that a shared skill of the same name left behind.
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
		log.Printf("shared area: install the %s skill: %v", skill, err)
	}
	if skill != api.SystemSkill {
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
			log.Printf("shared area: install the agent %s: %v", a.Name(), err)
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
		log.Printf("shared area: link %s: %v", at, err)
	}
}

// removeDangling removes links into the shared area whose target is gone.
func (s *sharedArea) removeDangling(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		at := filepath.Join(dir, e.Name())
		target, err := os.Readlink(at)
		if err != nil || !strings.HasPrefix(target, s.dir+"/") {
			continue
		}
		if _, err := os.Stat(target); err != nil {
			os.Remove(at)
		}
	}
}

// policies returns every shared policy, in name order, as a system prompt
// section.
func (s *sharedArea) policies() string {
	var b strings.Builder
	for _, name := range s.files("policies", ".md") {
		data, err := os.ReadFile(filepath.Join(s.dir, "policies", name))
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
