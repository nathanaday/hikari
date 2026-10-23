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

// Wiki is the directory of a wiki, in a system's directory, in a hub's
// directory, and in each release.
const Wiki = "wiki"

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

// SystemWiki is the wiki of a system.
func (s Store) SystemWiki(system string) string { return filepath.Join(s.SystemDir(system), Wiki) }

func (s Store) HubDir(hub string) string { return filepath.Join(s.Root, "hubs", hub) }

// HubWiki is the wiki of a hub.
func (s Store) HubWiki(hub string) string { return filepath.Join(s.HubDir(hub), Wiki) }

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
func (s Store) Prepare(systems, hubs []string) error {
	dirs := []string{s.Shared(Vocabulary), s.Shared(Policies), s.Shared(Skills), s.Shared(Agents), s.Shared(Deliverables)}
	for _, sys := range systems {
		dirs = append(dirs, s.Staging(sys), s.ReleasesDir(sys), s.SystemWiki(sys))
	}
	for _, hub := range hubs {
		dirs = append(dirs, s.HubWiki(hub))
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
      vocabulary/<hub>.yaml     the names each hub agreed on; its orchestrator writes it
      policies/<name>.md        rules for every agent; you write them
      skills/<name>/SKILL.md    Claude Code skills for every agent; you write them
      agents/<name>.md          Claude Code subagent definitions; you write them
      deliverables/<name>/      what a release must contain and how to make it; you write them
        deliverable.yaml        the description, the layout rules, and the cloud targets
        SKILL.md                the skill that tells the owner how to make the product
    hubs/<hub>/
      wiki/<id>.md              the hub wiki: stories, rules, and principles that its orchestrator
                                writes through hikari, and your plans and notes
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
publishes only the deliverables that the topology grants it. An orchestrator
reads the maps and releases of the systems in its hubs, and the topology can
grant any agent read-only access to other systems' releases.
`
