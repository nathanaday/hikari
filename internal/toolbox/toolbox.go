// Package toolbox holds hikari's built-in library of policies, styles,
// skills, and profile recipes: ready-made ways for agents to work, which
// hikari toolbox lists and installs into a project's vault. Once installed,
// a file is the user's, like any policy, style, or skill they wrote.
package toolbox

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/vault"
)

//go:embed library
var library embed.FS

// The kinds of items.
const (
	Policy  = "policy"
	Style   = "style"
	Skill   = "skill"
	Profile = "profile"
)

// Kinds lists the kinds, in the order the listing shows them.
var Kinds = []string{Policy, Style, Skill, Profile}

// Item is one entry of the library.
type Item struct {
	Kind        string `yaml:"kind" json:"kind"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

// Ref names an item as kind/name.
func (it Item) Ref() string { return it.Kind + "/" + it.Name }

type index struct {
	Version int    `yaml:"version"`
	Items   []Item `yaml:"items"`
}

// Items lists the library, in the index's order.
func Items() []Item {
	data, err := library.ReadFile("library/toolbox.yaml")
	if err != nil {
		return nil
	}
	var idx index
	if yaml.Unmarshal(data, &idx) != nil {
		return nil
	}
	return idx.Items
}

// Find returns the item kind/name. kind may be its plural.
func Find(ref string) (Item, error) {
	kind, name, ok := strings.Cut(ref, "/")
	if !ok {
		return Item{}, fmt.Errorf("%q is not <kind>/<name>; the kinds are %s", ref, strings.Join(Kinds, ", "))
	}
	kind = strings.TrimSuffix(kind, "s")
	if kind == "policie" {
		kind = Policy
	}
	for _, it := range Items() {
		if it.Kind == kind && it.Name == name {
			return it, nil
		}
	}
	if !slices.Contains(Kinds, kind) {
		return Item{}, fmt.Errorf("%q is not a kind; the kinds are %s", kind, strings.Join(Kinds, ", "))
	}
	return Item{}, fmt.Errorf("the toolbox has no %s %q; run hikari toolbox list", kind, name)
}

// File is one file of an item: its path relative to where the item is
// installed, and its content.
type File struct {
	Path string
	Data []byte
}

// Files returns the files of an item, sorted by path. A policy or a style
// is one Markdown file, a skill is a directory, and a profile is one YAML
// snippet.
func Files(it Item) ([]File, error) {
	root := itemRoot(it)
	if it.Kind != Skill {
		data, err := library.ReadFile(root)
		if err != nil {
			return nil, fmt.Errorf("the toolbox lacks the file of %s", it.Ref())
		}
		return []File{{Path: path.Base(root), Data: data}}, nil
	}
	var out []File
	err := fs.WalkDir(library, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := library.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, File{Path: filepath.ToSlash(rel), Data: data})
		return nil
	})
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("the toolbox lacks the files of %s", it.Ref())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// itemRoot is the path of an item in the library: a file for a policy, a
// style, or a profile, and a directory for a skill.
func itemRoot(it Item) string {
	switch it.Kind {
	case Policy:
		return "library/policies/" + it.Name + ".md"
	case Style:
		return "library/styles/" + it.Name + ".md"
	case Profile:
		return "library/profiles/" + it.Name + ".yaml"
	}
	return "library/skills/" + it.Name
}

// Text returns the one text of a policy, a style, or a profile, or a
// skill's SKILL.md.
func Text(it Item) (string, error) {
	files, err := Files(it)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.Path == vault.SkillFile || len(files) == 1 {
			return string(f.Data), nil
		}
	}
	return string(files[0].Data), nil
}

// Dest is where an item is installed in a vault, or "" for a profile,
// which goes into the topology file by hand.
func Dest(st vault.Vault, it Item) string {
	switch it.Kind {
	case Policy:
		return filepath.Join(st.Public(vault.Policies), it.Name+".md")
	case Style:
		return st.StyleFile(it.Name)
	case Skill:
		return st.SkillDir(it.Name)
	}
	return ""
}

// Installed reports whether an item is in the vault: its file exists, or
// for a skill, its directory holds a SKILL.md.
func Installed(st vault.Vault, it Item) bool {
	dest := Dest(st, it)
	if dest == "" {
		return false
	}
	if it.Kind == Skill {
		dest = filepath.Join(dest, vault.SkillFile)
	}
	_, err := os.Stat(dest)
	return err == nil
}

// ErrExists says that an item is already in the vault.
var ErrExists = errors.New("already in the vault")

// Install writes an item into the vault and returns the paths it wrote,
// relative to the vault. It refuses to overwrite an installed item unless
// force is set, so that the user's changes to a file are never lost by
// accident. A profile is not installed: it is a snippet for the topology
// file, which Text returns.
func Install(st vault.Vault, it Item, force bool) ([]string, error) {
	if it.Kind == Profile {
		return nil, fmt.Errorf("a profile goes into x-hikari.profiles of the topology file; paste the snippet of hikari toolbox show %s", it.Ref())
	}
	dest := Dest(st, it)
	if Installed(st, it) && !force {
		return nil, fmt.Errorf("%s is %w at %s; pass -force to replace it", it.Ref(), ErrExists, st.Rel(dest))
	}
	files, err := Files(it)
	if err != nil {
		return nil, err
	}
	var written []string
	for _, f := range files {
		target := dest
		if it.Kind == Skill {
			target = filepath.Join(dest, filepath.FromSlash(f.Path))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(target, f.Data, 0o644); err != nil {
			return written, err
		}
		written = append(written, st.Rel(target))
	}
	return written, nil
}

// Grant says how an installed item reaches an agent, for the user.
func Grant(it Item) string {
	switch it.Kind {
	case Policy:
		return "every agent reads it on its next turn; nothing to grant"
	case Style:
		return fmt.Sprintf("grant it with grants.styles: [%s] on an agent, or styles: [%s] in a profile", it.Name, it.Name)
	case Skill:
		return fmt.Sprintf("grant it with grants.skills: [%s] on an agent, or skills: [%s] in a profile", it.Name, it.Name)
	case Profile:
		return "paste it under x-hikari.profiles, and give an agent profile: " + it.Name
	}
	return ""
}

// Status is an item with whether the vault has it.
type Status struct {
	Item
	Installed bool `json:"installed"`
	// Path is where it lives in the vault, relative to the vault, or ""
	// for a profile.
	Path string `json:"path,omitempty"`
}

// List returns every item with its status in st.
func List(st vault.Vault) []Status {
	var out []Status
	for _, it := range Items() {
		s := Status{Item: it, Installed: Installed(st, it)}
		if d := Dest(st, it); d != "" {
			s.Path = st.Rel(d)
		}
		out = append(out, s)
	}
	return out
}
