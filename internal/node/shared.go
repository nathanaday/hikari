package node

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/store"
)

//go:embed skills/*.md
var builtinSkills embed.FS

// systemSkill is hikari's own skill, which always comes from the binary.
const systemSkill = api.SystemSkill

// sharedArea connects the store's shared area to Claude Code in the
// container (rules H1 to H10): the shared skills and agents, the policies,
// and the skill of each deliverable that the agent holds.
type sharedArea struct {
	home         string
	dir          string // the shared area in the container
	owner        bool
	deliverables []string
	warned       map[string]bool
}

func newSharedArea(owner bool, deliverables []string) (*sharedArea, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &sharedArea{home: home, dir: api.SharedDir, owner: owner, deliverables: deliverables, warned: map[string]bool{}}, nil
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
	if s.owner {
		s.writeBuiltin(systemSkill)
	}
	for _, name := range s.sharedSkills() {
		switch {
		case name == systemSkill:
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

// writeBuiltin installs a built-in skill, replacing a link that a shared
// skill of the same name left behind.
func (s *sharedArea) writeBuiltin(name string) {
	dir := filepath.Join(s.skillsDir(), name)
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		os.Remove(dir)
	}
	data, err := builtinSkills.ReadFile("skills/" + name + ".md")
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0o644)
	}
	if err != nil {
		log.Printf("shared area: install the %s skill: %v", name, err)
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
