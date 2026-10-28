// Package onboard holds what the binary keeps of the host skills, which live
// in the Hikari plugin, in plugin/skills/: the removal of the copies that
// hikari wrote into projects before the plugin (decision 7 of the plugin
// spec), and the tests that the host skills name only what hikari builds.
package onboard

import (
	"os"
	"path/filepath"
)

// Retired are the host skills that hikari wrote into a project's
// .claude/skills/ before the plugin.
var Retired = []string{"containerize-repo", "hikify"}

// SkillsDir is the folder of a project that holds Claude Code's skills.
const SkillsDir = ".claude/skills"

// selfIgnore is the .gitignore that hikari wrote into each host skill's
// folder, which marks the folder as hikari's.
const selfIgnore = "# hikari writes this skill; hikari sync replaces it.\n*\n"

// borrowed maps each of hikify's references to the file of the agents'
// skills that it copies (decision 11 of the plugin spec); make skills
// copies them.
var borrowed = map[string]string{
	"hikify/references/wiki.md":          "partition/hikari-map-features/references/wiki.md",
	"hikify/references/orchestration.md": "orchestration/hikari-map-features/SKILL.md",
}

// RemoveCopies removes each retired host skill that hikari wrote into the
// project at dir, and returns the names it removed. A folder whose
// .gitignore is not hikari's is a skill of your own, and stays.
func RemoveCopies(dir string) ([]string, error) {
	var removed []string
	for _, name := range Retired {
		folder := filepath.Join(dir, filepath.FromSlash(SkillsDir), name)
		data, err := os.ReadFile(filepath.Join(folder, ".gitignore"))
		if err != nil || string(data) != selfIgnore {
			continue
		}
		if err := os.RemoveAll(folder); err != nil {
			return removed, err
		}
		removed = append(removed, name)
	}
	return removed, nil
}
