// Package store lays out the shared store: the directory that holds every
// hikari document that is not code. Its shared area reaches every
// container; each system has its public map, its staging area, and its
// releases.
package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The kinds of documents in the shared area.
const (
	Vocabulary   = "vocabulary"
	Policies     = "policies"
	Skills       = "skills"
	Agents       = "agents"
	Deliverables = "deliverables"
)

// Wiki is the directory of a wiki, in a system's directory, in the shared
// area, and in each release.
const Wiki = "wiki"

// VocabularyName is the file of the vocabulary in its directory. hikari
// ignores every other file there.
const VocabularyName = "terms.yaml"

// Store is a store on the host. Its functions take the names of systems,
// and its directories are keyed by their ids (rule ID6 of the ids spec).
// Systems maps each name to its id; a name that it lacks is its own id.
type Store struct {
	Root    string
	Systems map[string]string
}

func idOf(ids map[string]string, name string) string {
	if id, ok := ids[name]; ok {
		return id
	}
	return name
}

func nameOf(ids map[string]string, id string) string {
	for name, v := range ids {
		if v == id {
			return name
		}
	}
	return id
}

// SystemID returns the id of a system.
func (s Store) SystemID(system string) string { return idOf(s.Systems, system) }

// SystemName returns the name of the system whose id is id.
func (s Store) SystemName(id string) string { return nameOf(s.Systems, id) }

func (s Store) SharedDir() string { return filepath.Join(s.Root, "shared") }

func (s Store) Shared(kind string) string { return filepath.Join(s.SharedDir(), kind) }

// VocabularyFile is the project's vocabulary.
func (s Store) VocabularyFile() string { return filepath.Join(s.Shared(Vocabulary), VocabularyName) }

// PublicWiki is the wiki that every agent reads and the orchestrator
// writes.
func (s Store) PublicWiki() string { return s.Shared(Wiki) }

func (s Store) SystemDir(system string) string {
	return filepath.Join(s.Root, "systems", s.SystemID(system))
}

func (s Store) MapFile(system string) string { return filepath.Join(s.SystemDir(system), "map.yaml") }

// SystemWiki is the wiki of a system.
func (s Store) SystemWiki(system string) string { return filepath.Join(s.SystemDir(system), Wiki) }

// Rel returns a path of the store relative to its root, with slashes.
func (s Store) Rel(p string) string {
	rel, err := filepath.Rel(s.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func (s Store) Staging(system string) string { return filepath.Join(s.SystemDir(system), "staging") }

func (s Store) ReleasesDir(system string) string {
	return filepath.Join(s.SystemDir(system), "releases")
}

// StagingOf is where an owner builds the product of one deliverable.
func (s Store) StagingOf(system, deliverable string) string {
	return filepath.Join(s.Staging(system), deliverable)
}

func (s Store) DeliverableReleasesDir(system, deliverable string) string {
	return filepath.Join(s.ReleasesDir(system), deliverable)
}

func (s Store) ReleaseDir(system, deliverable, version string) string {
	return filepath.Join(s.DeliverableReleasesDir(system, deliverable), version)
}

// Prepare creates the directories of the store that do not exist (rule L1)
// and writes its README once.
func (s Store) Prepare(systems []string) error {
	dirs := []string{s.Shared(Vocabulary), s.Shared(Policies), s.Shared(Skills), s.Shared(Agents), s.Shared(Deliverables), s.PublicWiki()}
	for _, sys := range systems {
		dirs = append(dirs, s.Staging(sys), s.ReleasesDir(sys), s.SystemWiki(sys))
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	readme := filepath.Join(s.Root, "README.md")
	if _, err := os.Stat(readme); errors.Is(err, fs.ErrNotExist) {
		return os.WriteFile(readme, []byte(readmeText), 0o644)
	}
	return nil
}

// IgnoreFile is the store's ignore file in a project repository (rule
// PR12): products and drafts stay out of git.
const IgnoreFile = ".gitignore"

const ignoreText = "systems/*/staging/\nsystems/*/releases/\n"

// WriteIgnore writes the store's ignore file unless it already holds what
// hikari writes. It reports whether it wrote the file.
func (s Store) WriteIgnore() (bool, error) {
	p := filepath.Join(s.Root, IgnoreFile)
	if data, err := os.ReadFile(p); err == nil && string(data) == ignoreText {
		return false, nil
	}
	return true, os.WriteFile(p, []byte(ignoreText), 0o644)
}

const readmeText = `# hikari store

This directory holds every hikari document that is not code. hikari wrote
this file once; you can change it.

    shared/
      vocabulary/terms.yaml     the names the project agreed on; the orchestrator writes it
      wiki/<id>.md              the public wiki: stories, rules, and principles that the
                                orchestrator writes through hikari, and your plans and notes
      policies/<name>.md        rules for every agent; you write them
      skills/<name>/SKILL.md    Claude Code skills for every agent; you write them
      agents/<name>.md          Claude Code subagent definitions; you write them
      deliverables/<name>/      what a release must contain and how to make it; you write them
        deliverable.yaml        the description, the layout rules, and the cloud targets
        SKILL.md                the skill that tells the owner how to make the product
    systems/<system>/
      map.yaml                  the public map of the system; hikari writes it from the wiki
      wiki/<id>.md              the system wiki: features, principles, and rules that its owner
                                writes through hikari, and your plans and notes
      staging/<deliverable>/    where the owner builds the product of its next release
      releases/<deliverable>/<x.y.z>/   published releases, which never change
        release.yaml            the manifest; hikari writes it
        SHA256SUMS              the digest of every file of the product
        product/                the files the owner staged
        map.yaml                the public map at release time
        wiki/                   the features, principles, and rules at release time
      releases/<deliverable>/latest     the newest release of that deliverable

In a project repository, git ignores staging/ and releases/: products
stay out of git, and hikari commits what agents change in the store.

Every container can read shared/. An owner writes only its own staging/, and
publishes only the deliverables that the topology grants it. The orchestrator
reads the maps and releases of every system, and the topology can grant any
agent read-only access to other systems' releases.
`
