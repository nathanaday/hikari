// Package onboard holds the host skills: the skills that hikari writes into
// a project for your own Claude Code session (rules HS1 to HS5 of the
// containerize spec).
package onboard

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/nathanaday/hikari/internal/node"
)

//go:embed skills
var skills embed.FS

// borrowed maps each host skill's file to the file of the agents' skills
// that it repeats, so that both come from one source.
var borrowed = map[string]map[string]string{
	"hikify": {
		"references/wiki.md":        "skills/hikari-system/references/wiki.md",
		"references/public-wiki.md": "skills/hikari-orchestration/SKILL.md",
	},
}

// SkillsDir is the folder of a project that holds Claude Code's skills.
const SkillsDir = ".claude/skills"

// Names returns the names of the host skills, sorted.
func Names() []string {
	entries, _ := skills.ReadDir("skills")
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out
}

// selfIgnore is the .gitignore in each host skill's folder: git ignores
// the folder, this file included, with no line in the project's own
// .gitignore.
const selfIgnore = "# hikari writes this skill; hikari sync replaces it.\n*\n"

// Write writes every host skill into the project at dir, and replaces what
// is there (rule HS2 of the containerize spec).
func Write(dir string) error {
	for _, name := range Names() {
		dst := filepath.Join(dir, filepath.FromSlash(SkillsDir), name)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		root := path.Join("skills", name)
		err := fs.WalkDir(skills, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			data, err := skills.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0o644)
		})
		if err != nil {
			return err
		}
		for rel, src := range borrowed[name] {
			data, err := node.BuiltinFile(src)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(dst, ".gitignore"), []byte(selfIgnore), 0o644); err != nil {
			return err
		}
	}
	return nil
}
