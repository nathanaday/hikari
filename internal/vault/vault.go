// Package vault lays out the vault: the directory that holds every hikari
// document that is not code. Its public area reaches every container; each
// partition has a read-only part, a part that its agent can edit, and a
// staging area; the orchestration agent has its own two parts.
package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The kinds of documents in the public area.
const (
	Vocabulary   = "vocabulary"
	Policies     = "policies"
	Deliverables = "deliverables"
)

// Wiki is the directory of the public wiki in the public area.
const Wiki = "wiki"

// The directories of the vault.
const (
	PublicDir        = "public"
	SkillsDir        = "skills"
	PartitionsDir    = "partitions"
	OrchestrationDir = "orchestration"
	ReadOnlyDir      = "read-only"
	CanEditDir       = "can-edit"
	StagingDir       = "staging"
	FeaturesDir      = "features"
	ReleasesDir      = "releases"
	GrantedDir       = "granted"
)

// MapName is the file of a feature map in its features directory.
const MapName = "map.yaml"

// VocabularyName is the file of the vocabulary in its directory. hikari
// ignores every other file there.
const VocabularyName = "terms.yaml"

// Vault is a vault on the host. Its functions take the names of partitions,
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

// PublicRoot is the public area, which every container reads.
func (s Vault) PublicRoot() string { return filepath.Join(s.Root, PublicDir) }

func (s Vault) Public(kind string) string { return filepath.Join(s.PublicRoot(), kind) }

// VocabularyFile is the project's vocabulary.
func (s Vault) VocabularyFile() string { return filepath.Join(s.Public(Vocabulary), VocabularyName) }

// PublicWiki is the wiki that every agent reads and the orchestration agent
// writes.
func (s Vault) PublicWiki() string { return s.Public(Wiki) }

// SkillsRoot holds your skills, which the topology grants to agents.
func (s Vault) SkillsRoot() string { return filepath.Join(s.Root, SkillsDir) }

func (s Vault) SkillDir(name string) string { return filepath.Join(s.SkillsRoot(), name) }

func (s Vault) PartitionDir(partition string) string {
	return filepath.Join(s.Root, PartitionsDir, s.PartitionID(partition))
}

// ReadOnly is the part of a partition that its partition agent and the
// orchestration agent read.
func (s Vault) ReadOnly(partition string) string {
	return filepath.Join(s.PartitionDir(partition), ReadOnlyDir)
}

// FeatureMap is the directory of a partition's feature map: its documents
// and map.yaml.
func (s Vault) FeatureMap(partition string) string {
	return filepath.Join(s.ReadOnly(partition), FeaturesDir)
}

func (s Vault) MapFile(partition string) string {
	return filepath.Join(s.FeatureMap(partition), MapName)
}

func (s Vault) Granted(partition string) string {
	return filepath.Join(s.ReadOnly(partition), GrantedDir)
}

// CanEdit is the part of a partition that its partition agent writes.
func (s Vault) CanEdit(partition string) string {
	return filepath.Join(s.PartitionDir(partition), CanEditDir)
}

func (s Vault) Staging(partition string) string {
	return filepath.Join(s.PartitionDir(partition), StagingDir)
}

func (s Vault) ReleasesOf(partition string) string {
	return filepath.Join(s.ReadOnly(partition), ReleasesDir)
}

// The parts of the orchestration agent.
func (s Vault) OrchestrationReadOnly() string {
	return filepath.Join(s.Root, OrchestrationDir, ReadOnlyDir)
}

// OrchestrationFeatureMap is the orchestration agent's feature map.
func (s Vault) OrchestrationFeatureMap() string {
	return filepath.Join(s.OrchestrationReadOnly(), FeaturesDir)
}

func (s Vault) OrchestrationCanEdit() string {
	return filepath.Join(s.Root, OrchestrationDir, CanEditDir)
}

// Rel returns a path of the vault relative to its root, with slashes.
func (s Vault) Rel(p string) string {
	rel, err := filepath.Rel(s.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// StagingOf is where a partition agent builds the product of one deliverable.
func (s Vault) StagingOf(partition, deliverable string) string {
	return filepath.Join(s.Staging(partition), deliverable)
}

func (s Vault) DeliverableReleasesDir(partition, deliverable string) string {
	return filepath.Join(s.ReleasesOf(partition), deliverable)
}

func (s Vault) ReleaseDir(partition, deliverable, version string) string {
	return filepath.Join(s.DeliverableReleasesDir(partition, deliverable), version)
}

// Prepare creates the directories of the vault that do not exist (rule V1)
// and writes its README once. The orchestration agent's parts exist only
// when the topology names one (rule V2).
func (s Vault) Prepare(partitions []string, orchestration bool) error {
	dirs := []string{s.Public(Vocabulary), s.Public(Policies), s.Public(Deliverables), s.PublicWiki(), s.SkillsRoot()}
	for _, p := range partitions {
		dirs = append(dirs, s.FeatureMap(p), s.ReleasesOf(p), s.Granted(p), s.CanEdit(p), s.Staging(p))
	}
	if orchestration {
		ro := s.OrchestrationReadOnly()
		dirs = append(dirs, s.OrchestrationFeatureMap(), filepath.Join(ro, GrantedDir), s.OrchestrationCanEdit())
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

// MigratingGuide is the guide that lists the moves from an old layout.
const MigratingGuide = "docs/guides/migrating.md"

// Migrating returns the section of the migrating guide with the anchor
// section.
func Migrating(section string) string { return MigratingGuide + "#" + section }

// OldLayout returns a problem for each directory of an earlier vault
// layout in the vault, with the move that fixes it (rule V11).
func (s Vault) OldLayout() []string {
	var out []string
	see := Migrating("the-vault-layout")
	if isDir(filepath.Join(s.Root, "shared")) {
		out = append(out, fmt.Sprintf("the vault %s holds shared/, the old layout: move shared/vocabulary, shared/wiki, shared/policies, and shared/deliverables into public/, move shared/skills to skills/ and grant each skill with grants.skills, and delete shared/agents; see %s", s.Root, see))
	}
	if isDir(filepath.Join(s.Root, "hubs")) {
		out = append(out, fmt.Sprintf("the vault %s holds hubs/, the old layout: move the rules, principles, plans, and notes of each hubs/<hub>/wiki/ to public/wiki/, and each story to orchestration/read-only/features/ as a feature; see %s", s.Root, see))
	}
	if isDir(filepath.Join(s.Root, "systems")) {
		out = append(out, fmt.Sprintf("the vault %s holds systems/, the old layout: for each systems/<id>/, move wiki/ to partitions/<id>/read-only/features/, map.yaml into that features/, releases/ to partitions/<id>/read-only/releases/, and staging/ to partitions/<id>/staging/; see %s", s.Root, see))
	}
	if voc := s.Public(Vocabulary); !exists(s.VocabularyFile()) {
		if old, _ := filepath.Glob(filepath.Join(voc, "*.yaml")); len(old) > 0 {
			out = append(out, fmt.Sprintf("the vocabulary %s holds %s and no %s, the old layout: rename the file to %s, and delete its hub: line; see %s", voc, filepath.Base(old[0]), VocabularyName, VocabularyName, see))
		}
	}
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// IgnoreFile is the vault's ignore file in a project repository (rule
// PR12): products and drafts stay out of git.
const IgnoreFile = ".gitignore"

const ignoreText = "partitions/*/staging/\npartitions/*/read-only/releases/\n"

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

    public/                     every agent reads it, at /hikari/public
      vocabulary/terms.yaml     the names the project agreed on; the orchestration agent writes it
      wiki/<id>.md              the public wiki: rules and principles that the orchestration
                                agent writes through hikari, and your plans and notes
      policies/<name>.md        rules for every agent; you write them
      deliverables/<name>/      what a release must contain and how to make it; you write them
        deliverable.yaml        the description, the layout rules, and the cloud targets
        SKILL.md                the skill that tells the partition agent how to make the product
    skills/<name>/SKILL.md      your Claude Code skills; grants.skills gives them to agents,
                                at /hikari/skills/<name>
    partitions/<id>/
      read-only/                the partition agent and the orchestration agent read it,
                                at /hikari/self and /hikari/partitions/<name>
        features/               the feature map: map.yaml, which hikari writes from the
                                documents, and the features, principles, and rules that the
                                partition agent writes through hikari, and your plans and notes
        releases/<deliverable>/<x.y.z>/   published releases, which never change
          release.yaml          the manifest; hikari writes it
          SHA256SUMS            the digest of every file of the product
          product/              the files the partition agent staged
          features/             the feature map at release time
        releases/<deliverable>/latest     the newest release of that deliverable
        granted/                files you give the partition to read
      can-edit/                 files you give the partition, which its agent can change,
                                at /hikari/can-edit
      staging/<deliverable>/    where the partition agent builds the product of its next
                                release, at /hikari/staging
    orchestration/              the orchestration agent's parts, when the project has one
      read-only/                at /hikari/self
        features/               the orchestration feature map
        granted/                files you give the orchestration agent to read
      can-edit/                 files the orchestration agent can change, at /hikari/can-edit

The directory a file is in decides who mounts it and how: each container
mounts a few of these directories, and nothing else of the vault. No
container mounts another partition's can-edit/ or staging/. A grant in the
topology mounts another partition's releases at /hikari/grants/<name>/releases.

In a project repository, git ignores staging/ and releases/: products
stay out of git, and hikari commits what agents change in the vault. Your
own changes in the wikis, can-edit/, and granted/ are committed as yours
when hikari starts and stops.
`
