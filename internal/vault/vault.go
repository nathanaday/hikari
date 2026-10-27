// Package store lays out the vault: the directory that holds every
// hikari document that is not code. Its shared area reaches every
// container; each partition has its public map, its staging area, and its
// releases.
package vault

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

// Wiki is the directory of a wiki, in a partition's directory, in the shared
// area, and in each release.
const Wiki = "wiki"

// VocabularyName is the file of the vocabulary in its directory. hikari
// ignores every other file there.
const VocabularyName = "terms.yaml"

// Vault is a store on the host. Its functions take the names of partitions,
// and its directories are keyed by their ids (rule ID6 of the ids spec).
// Partitions maps each name to its id; a name that it lacks is its own id.
type Vault struct {
	Root       string
	Partitions map[string]string
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

// PartitionID returns the id of a partition.
func (s Vault) PartitionID(partition string) string { return idOf(s.Partitions, partition) }

// PartitionName returns the name of the partition whose id is id.
func (s Vault) PartitionName(id string) string { return nameOf(s.Partitions, id) }

func (s Vault) SharedDir() string { return filepath.Join(s.Root, "shared") }

func (s Vault) Shared(kind string) string { return filepath.Join(s.SharedDir(), kind) }

// VocabularyFile is the project's vocabulary.
func (s Vault) VocabularyFile() string { return filepath.Join(s.Shared(Vocabulary), VocabularyName) }

// PublicWiki is the wiki that every agent reads and the orchestration agent
// writes.
func (s Vault) PublicWiki() string { return s.Shared(Wiki) }

func (s Vault) PartitionDir(partition string) string {
	return filepath.Join(s.Root, "systems", s.PartitionID(partition))
}

func (s Vault) MapFile(partition string) string {
	return filepath.Join(s.PartitionDir(partition), "map.yaml")
}

// FeatureMap is the wiki of a partition.
func (s Vault) FeatureMap(partition string) string {
	return filepath.Join(s.PartitionDir(partition), Wiki)
}

// Rel returns a path of the vault relative to its root, with slashes.
func (s Vault) Rel(p string) string {
	rel, err := filepath.Rel(s.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func (s Vault) Staging(partition string) string {
	return filepath.Join(s.PartitionDir(partition), "staging")
}

func (s Vault) ReleasesDir(partition string) string {
	return filepath.Join(s.PartitionDir(partition), "releases")
}

// StagingOf is where a partition agent builds the product of one deliverable.
func (s Vault) StagingOf(partition, deliverable string) string {
	return filepath.Join(s.Staging(partition), deliverable)
}

func (s Vault) DeliverableReleasesDir(partition, deliverable string) string {
	return filepath.Join(s.ReleasesDir(partition), deliverable)
}

func (s Vault) ReleaseDir(partition, deliverable, version string) string {
	return filepath.Join(s.DeliverableReleasesDir(partition, deliverable), version)
}

// Prepare creates the directories of the vault that do not exist (rule L1)
// and writes its README once.
func (s Vault) Prepare(partitions []string) error {
	dirs := []string{s.Shared(Vocabulary), s.Shared(Policies), s.Shared(Skills), s.Shared(Agents), s.Shared(Deliverables), s.PublicWiki()}
	for _, sys := range partitions {
		dirs = append(dirs, s.Staging(sys), s.ReleasesDir(sys), s.FeatureMap(sys))
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

// IgnoreFile is the vault's ignore file in a project repository (rule
// PR12): products and drafts stay out of git.
const IgnoreFile = ".gitignore"

const ignoreText = "systems/*/staging/\nsystems/*/releases/\n"

// WriteIgnore writes the vault's ignore file unless it already holds what
// hikari writes. It reports whether it wrote the file.
func (s Vault) WriteIgnore() (bool, error) {
	p := filepath.Join(s.Root, IgnoreFile)
	if data, err := os.ReadFile(p); err == nil && string(data) == ignoreText {
		return false, nil
	}
	return true, os.WriteFile(p, []byte(ignoreText), 0o644)
}

const readmeText = `# hikari vault

This directory holds every hikari document that is not code. hikari wrote
this file once; you can change it.

    shared/
      vocabulary/terms.yaml     the names the project agreed on; the orchestration agent writes it
      wiki/<id>.md              the public wiki: stories, rules, and principles that the
                                orchestration agent writes through hikari, and your plans and notes
      policies/<name>.md        rules for every agent; you write them
      skills/<name>/SKILL.md    Claude Code skills for every agent; you write them
      agents/<name>.md          Claude Code subagent definitions; you write them
      deliverables/<name>/      what a release must contain and how to make it; you write them
        deliverable.yaml        the description, the layout rules, and the cloud targets
        SKILL.md                the skill that tells the partition agent how to make the product
    systems/<partition>/
      map.yaml                  the partition's feature map; hikari writes it from the wiki
      wiki/<id>.md              the feature documents: features, principles, and rules that the
                                partition agent writes through hikari, and your plans and notes
      staging/<deliverable>/    where the partition agent builds the product of its next release
      releases/<deliverable>/<x.y.z>/   published releases, which never change
        release.yaml            the manifest; hikari writes it
        SHA256SUMS              the digest of every file of the product
        product/                the files the partition agent staged
        map.yaml                the feature map at release time
        wiki/                   the features, principles, and rules at release time
      releases/<deliverable>/latest     the newest release of that deliverable

In a project repository, git ignores staging/ and releases/: products
stay out of git, and hikari commits what agents change in the vault.

Every container can read shared/. A partition agent writes only its own
staging/, and publishes only the deliverables that the topology grants it.
The orchestration agent reads the feature maps and releases of every
partition, and the topology can grant any agent read-only access to other
partitions' releases.
`
