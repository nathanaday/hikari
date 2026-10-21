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
	Vocabulary = "vocabulary"
	Policies   = "policies"
	Skills     = "skills"
	Agents     = "agents"
)

// Store is a store on the host.
type Store struct {
	Root string
}

func (s Store) SharedDir() string { return filepath.Join(s.Root, "shared") }

func (s Store) Shared(kind string) string { return filepath.Join(s.SharedDir(), kind) }

func (s Store) VocabularyFile(hub string) string {
	return filepath.Join(s.Shared(Vocabulary), hub+".yaml")
}

func (s Store) SystemDir(system string) string { return filepath.Join(s.Root, "systems", system) }

func (s Store) MapFile(system string) string { return filepath.Join(s.SystemDir(system), "map.yaml") }

func (s Store) Staging(system string) string { return filepath.Join(s.SystemDir(system), "staging") }

func (s Store) Releases(system string) string { return filepath.Join(s.SystemDir(system), "releases") }

func (s Store) Release(system, version string) string {
	return filepath.Join(s.Releases(system), version)
}

// Prepare creates the directories of the store that do not exist (rule L1)
// and writes its README once.
func (s Store) Prepare(systems []string) error {
	dirs := []string{s.Shared(Vocabulary), s.Shared(Policies), s.Shared(Skills), s.Shared(Agents)}
	for _, sys := range systems {
		dirs = append(dirs, s.Staging(sys), s.Releases(sys))
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

const readmeText = `# hikari store

This directory holds every hikari document that is not code. hikari wrote
this file once; you can change it.

    shared/
      vocabulary/<hub>.yaml     the names each hub agreed on; its orchestrator writes it
      policies/<name>.md        rules for every agent; you write them
      skills/<name>/SKILL.md    Claude Code skills for every agent; you write them
      agents/<name>.md          Claude Code subagent definitions; you write them
    systems/<system>/
      map.yaml                  the public map of the system; its owner writes it through hikari
      staging/                  where the owner prepares its next release
      releases/<x.y.z>/         published releases, which never change
        release.yaml            what the release is and how to use it
        product/                the deliverable, unless it is hosted elsewhere
        docs/                   user-facing docs
        map.yaml                the public map at release time
      releases/latest           the newest release

Every container can read shared/. An owner writes only its own staging/. An
orchestrator reads the maps and releases of the systems in its hubs, and the
topology can grant any agent read-only access to other systems' releases.
`
