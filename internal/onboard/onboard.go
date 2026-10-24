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
)

//go:embed skills
var skills embed.FS

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

// IgnorePath is the path, relative to the project, that git must ignore
// for the host skill name.
func IgnorePath(name string) string { return SkillsDir + "/" + name + "/" }

// IgnoreLine is the .gitignore line that ignores the host skill name.
func IgnoreLine(name string) string { return "/" + IgnorePath(name) }

// Write writes every host skill into the project at dir, and replaces what
// is there. It writes a skill only when ignored reports that git ignores
// its folder, and returns the names it skipped.
func Write(dir string, ignored func(rel string) bool) (skipped []string, err error) {
	for _, name := range Names() {
		if !ignored(IgnorePath(name)) {
			skipped = append(skipped, name)
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(SkillsDir), name)
		if err := os.RemoveAll(dst); err != nil {
			return skipped, err
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
			return skipped, err
		}
	}
	return skipped, nil
}
