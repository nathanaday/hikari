package partitions

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/vault"
)

// readsReleases returns the partitions whose releases agent reads (rule
// RM7): its own, the partitions it orchestrates, and its release grants.
func (m *Manager) readsReleases(agent string) []string {
	out := m.topo.ReadableReleases(agent)
	if _, ok := m.partitionAgents[agent]; ok {
		out = append(out, agent)
	}
	slices.Sort(out)
	return out
}

// reader checks that agent reads the releases of partition.
func (m *Manager) reader(agent, partition string) error {
	if _, ok := m.partitionAgents[partition]; !ok {
		return fmt.Errorf("%w: %q is not a partition", ErrNotFound, partition)
	}
	if !slices.Contains(m.readsReleases(agent), partition) {
		return fmt.Errorf("%w: you cannot read the releases of %s; you read those of %s", ErrForbidden, partition, list(m.readsReleases(agent)))
	}
	return nil
}

// catalog returns the releases of a partition with the partitions that pin
// each (rule LS1).
func (m *Manager) catalog(partition string) []api.ReleaseEntry {
	out := []api.ReleaseEntry{}
	releases, err := m.vault.Catalog(partition)
	if err != nil {
		log.Printf("catalog of %s: %v", partition, err)
	}
	for _, r := range releases {
		by := m.vault.PinnedBy(m.topo.PartitionAgents(), partition, r.Deliverable, r.Version)
		if by == nil {
			by = []string{}
		}
		out = append(out, api.ReleaseEntry{Release: r, PinnedBy: by})
	}
	return out
}

// ListReleases lists the catalog that agent reads (rule LS1), or one
// partition of it.
func (m *Manager) ListReleases(agent, partition string) ([]api.ReleaseEntry, error) {
	partitions := m.readsReleases(agent)
	if partition != "" {
		if err := m.reader(agent, partition); err != nil {
			return nil, err
		}
		partitions = []string{partition}
	}
	out := []api.ReleaseEntry{}
	for _, p := range partitions {
		out = append(out, m.catalog(p)...)
	}
	return out, nil
}

// AllReleases lists the whole catalog, for the user.
func (m *Manager) AllReleases() []api.ReleaseEntry {
	out := []api.ReleaseEntry{}
	for _, p := range m.topo.PartitionAgents() {
		out = append(out, m.catalog(p)...)
	}
	return out
}

// Pins returns the pins of a partition (rule PN7).
func (m *Manager) Pins(partition string) []vault.Pin {
	pins, err := m.vault.LoadPins(partition)
	if err != nil {
		log.Printf("pins of %s: %v", partition, err)
	}
	if pins == nil {
		pins = []vault.Pin{}
	}
	return pins
}

// mayTag checks that agent, a holder of deliverable, may set tag (rules
// TG3 and TG4).
func (m *Manager) mayTag(agent, deliverable string, d vault.Deliverable, tag string) error {
	switch {
	case !vault.ValidTagName(tag):
		return fmt.Errorf("tag %q is not a valid tag name (a-z, then a-z, 0-9, ., _, -, up to 32 characters)", tag)
	case tag == vault.LatestTag:
		return fmt.Errorf("the tag %s is hikari's: it points at the newest version", vault.LatestTag)
	case !d.AllowsTag(tag):
		if len(d.Tags) == 0 {
			return fmt.Errorf("the deliverable %s declares no tags, so %q cannot be set; add tags to its deliverable.yaml", deliverable, tag)
		}
		return fmt.Errorf("the deliverable %s allows no tag %q; its tags match %s", deliverable, tag, strings.Join(d.Tags, ", "))
	case !m.topo.Agents[agent].Grants.MayTag(deliverable, tag, d.Tags):
		return fmt.Errorf("%w: you may set only the tags of %s that match %s", ErrForbidden, deliverable, strings.Join(m.topo.Agents[agent].Grants.Tags[deliverable], ", "))
	}
	return nil
}

// TagRelease points a tag of a deliverable that agent holds at a version
// (rule TG3).
func (m *Manager) TagRelease(agent string, req api.TagRequest) error {
	if _, err := m.holds(agent, req.Deliverable); err != nil {
		return err
	}
	d, err := m.vault.LoadDeliverable(req.Deliverable)
	if err != nil {
		return err
	}
	if err := m.mayTag(agent, req.Deliverable, d, req.Tag); err != nil {
		return err
	}
	if err := m.vault.SetTag(agent, req.Deliverable, req.Tag, req.Version); err != nil {
		return err
	}
	m.commitVault(agent, fmt.Sprintf("%s: tag %s %s as %s", agent, req.Deliverable, req.Version, req.Tag), m.vault.Rel(m.vault.TagsFile(agent, req.Deliverable)))
	m.changed(Change{Partition: agent})
	return nil
}

// TagAsUser sets or, with an empty version, removes a tag as the user
// (rule TG6), with no grant check.
func (m *Manager) TagAsUser(partition, deliverable, tag, version string) error {
	if _, ok := m.partitionAgents[partition]; !ok {
		return fmt.Errorf("%w: %q is not a partition", ErrNotFound, partition)
	}
	var err error
	message := fmt.Sprintf("user: tag %s %s %s as %s", partition, deliverable, version, tag)
	if version == "" {
		err = m.vault.RemoveTag(partition, deliverable, tag)
		message = fmt.Sprintf("user: untag %s %s %s", partition, deliverable, tag)
	} else {
		err = m.vault.SetTag(partition, deliverable, tag, version)
	}
	if err != nil {
		return err
	}
	m.commitVault("", message, m.vault.Rel(m.vault.TagsFile(partition, deliverable)))
	m.changed(Change{Partition: partition})
	return nil
}

// PullRelease gets the product of a release that agent reads (rule RM7),
// and returns the version it resolved.
func (m *Manager) PullRelease(ctx context.Context, agent string, req api.ReleaseRef) (string, error) {
	if err := m.reader(agent, req.Partition); err != nil {
		return "", err
	}
	return m.pull(ctx, req)
}

// PullAsUser gets the product of any release.
func (m *Manager) PullAsUser(ctx context.Context, req api.ReleaseRef) (string, error) {
	if _, ok := m.partitionAgents[req.Partition]; !ok {
		return "", fmt.Errorf("%w: %q is not a partition", ErrNotFound, req.Partition)
	}
	return m.pull(ctx, req)
}

func (m *Manager) pull(ctx context.Context, req api.ReleaseRef) (string, error) {
	version, err := m.vault.Resolve(req.Partition, req.Deliverable, req.Ref)
	if err != nil {
		return "", err
	}
	if err := m.vault.Pull(ctx, req.Partition, req.Deliverable, version); err != nil {
		return version, err
	}
	m.changed(Change{Partition: req.Partition})
	return version, nil
}

// RecordRelease records a release built elsewhere (rule RM8): it pulls the
// version into the staging area, then publishes it without a push.
func (m *Manager) RecordRelease(ctx context.Context, agent string, req api.PublishRequest) (vault.Manifest, error) {
	if _, err := m.holds(agent, req.Deliverable); err != nil {
		return vault.Manifest{}, err
	}
	if !vault.ValidVersion(req.Version) {
		return vault.Manifest{}, fmt.Errorf("version %q is not semver (x.y.z, with an optional -pre-release)", req.Version)
	}
	remote, err := m.vault.PullInto(ctx, agent, req.Deliverable, req.Version, m.vault.StagingOf(agent, req.Deliverable))
	if err != nil {
		return vault.Manifest{}, err
	}
	return m.publish(agent, req, remote, true)
}

// PinRelease pins a release of a partition that agent is granted (rule
// PN2), and pulls its product when it is absent. The note says what the
// pull did.
func (m *Manager) PinRelease(ctx context.Context, agent string, req api.PinRequest) (vault.Pin, string, error) {
	if _, err := m.partitionAgent(agent); err != nil {
		return vault.Pin{}, "", err
	}
	g := m.topo.Agents[agent].Grants
	if !slices.Contains(g.Releases, req.Partition) {
		return vault.Pin{}, "", fmt.Errorf("%w: the topology does not grant you the releases of %q; ask the user", ErrForbidden, req.Partition)
	}
	if !g.MayPin(req.Partition, req.Ref) {
		return vault.Pin{}, "", fmt.Errorf("%w: you may pin only the tags of %s that match %s", ErrForbidden, req.Partition, strings.Join(g.Pins[req.Partition], ", "))
	}
	return m.pin(ctx, agent, agent, req, true)
}

// PinAsUser pins for a consumer as the user (rule PN8), with the grant
// checks of the consumer.
func (m *Manager) PinAsUser(ctx context.Context, consumer string, req api.PinRequest) (vault.Pin, string, error) {
	if _, ok := m.partitionAgents[consumer]; !ok {
		return vault.Pin{}, "", fmt.Errorf("%w: %q is not a partition", ErrNotFound, consumer)
	}
	g := m.topo.Agents[consumer].Grants
	if !slices.Contains(g.Releases, req.Partition) {
		return vault.Pin{}, "", fmt.Errorf("the topology does not grant %s the releases of %q", consumer, req.Partition)
	}
	if !g.MayPin(req.Partition, req.Ref) {
		return vault.Pin{}, "", fmt.Errorf("%s may pin only the tags of %s that match %s", consumer, req.Partition, strings.Join(g.Pins[req.Partition], ", "))
	}
	return m.pin(ctx, "", consumer, req, false)
}

// pin records a pin and, for an agent, pulls its product; the user's
// command pulls on the host itself, outside the server's request.
func (m *Manager) pin(ctx context.Context, author, consumer string, req api.PinRequest, pull bool) (vault.Pin, string, error) {
	pin, err := m.vault.SetPin(consumer, req.Partition, req.Deliverable, req.Ref)
	if err != nil {
		return vault.Pin{}, "", err
	}
	who := author
	if who == "" {
		who = "user"
	}
	m.commitVault(author, fmt.Sprintf("%s: pin %s %s %s", who, req.Partition, req.Deliverable, req.Ref), m.vault.Rel(m.vault.PinsFile(consumer)))
	m.changed(Change{Partition: consumer})
	version, _ := m.vault.Resolve(req.Partition, req.Deliverable, req.Ref)
	if !pull {
		return pin, "the tag points at " + version, nil
	}
	note := "its product is on this machine"
	version, err = m.pull(ctx, api.ReleaseRef{Partition: req.Partition, Deliverable: req.Deliverable, Ref: req.Ref})
	switch {
	case err != nil:
		note = "the pull of its product failed: " + err.Error()
	case pin.Tag != "":
		note = fmt.Sprintf("the tag points at %s, whose product is on this machine", version)
	}
	return pin, note, nil
}

// UnpinRelease removes a pin of agent (rule PN3).
func (m *Manager) UnpinRelease(agent string, req api.PinRequest) error {
	if _, err := m.partitionAgent(agent); err != nil {
		return err
	}
	return m.unpin(agent, agent, req)
}

// UnpinAsUser removes a pin of consumer as the user (rule PN8).
func (m *Manager) UnpinAsUser(consumer string, req api.PinRequest) error {
	if _, ok := m.partitionAgents[consumer]; !ok {
		return fmt.Errorf("%w: %q is not a partition", ErrNotFound, consumer)
	}
	return m.unpin("", consumer, req)
}

func (m *Manager) unpin(author, consumer string, req api.PinRequest) error {
	if err := m.vault.RemovePin(consumer, req.Partition, req.Deliverable); err != nil {
		return err
	}
	who := author
	if who == "" {
		who = "user"
	}
	m.commitVault(author, fmt.Sprintf("%s: unpin %s %s", who, req.Partition, req.Deliverable), m.vault.Rel(m.vault.PinsFile(consumer)))
	m.changed(Change{Partition: consumer})
	return nil
}

// refreshCatalog brings the catalog up to date at start (rules RL4 and
// RL5): the tags and their links of every partition, and a commit of the
// records that git does not track yet.
func (m *Manager) refreshCatalog() error {
	var untracked []string
	for _, p := range m.topo.PartitionAgents() {
		wrote, err := m.vault.RefreshTags(p)
		if err != nil {
			return err
		}
		if m.projectRepo == nil {
			continue
		}
		untracked = append(untracked, wrote...)
		releases, err := m.vault.Releases(p)
		if err != nil {
			return err
		}
		for _, r := range releases {
			paths := m.vault.RecordPaths(p, r.Deliverable, r.Version)
			if !m.projectRepo.Tracked(m.vaultPath(paths[0] + "/" + vault.ManifestFile)) {
				untracked = append(untracked, paths...)
			}
		}
	}
	if len(untracked) > 0 {
		slices.Sort(untracked)
		m.commitVault("hikari", "hikari: add the records of releases", slices.Compact(untracked)...)
	}
	return nil
}

// pinWarnings says which pins cannot be met on this machine (rule PN6).
func (m *Manager) pinWarnings() []string {
	return m.vault.PinProblems(m.topo.PartitionAgents())
}
