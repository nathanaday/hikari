package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// PinsFile holds the pins of a partition, in its read-only part (section 4
// of the releases spec).
const PinsFile = "pins.yaml"

// Pin is a partition's declaration of a release of another partition that
// it uses: a version, or a tag that it follows.
type Pin struct {
	Partition   string `yaml:"partition" json:"partition"`
	Deliverable string `yaml:"deliverable" json:"deliverable"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Tag         string `yaml:"tag,omitempty" json:"tag,omitempty"`
}

// Ref is the version or the tag of the pin.
func (p Pin) Ref() string {
	if p.Tag != "" {
		return p.Tag
	}
	return p.Version
}

// Pins is pins.yaml.
type Pins struct {
	Version int   `yaml:"version"`
	Pins    []Pin `yaml:"pins"`
}

func (s Vault) PinsFile(partition string) string {
	return filepath.Join(s.ReadOnly(partition), PinsFile)
}

// LoadPins reads the pins of a partition (rule PN1). A missing file holds
// none. The partitions in it are named by id on disk.
func (s Vault) LoadPins(partition string) ([]Pin, error) {
	data, err := os.ReadFile(s.PinsFile(partition))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var p Pins
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s of %s: %w", PinsFile, partition, err)
	}
	for i := range p.Pins {
		p.Pins[i].Partition = s.PartitionName(p.Pins[i].Partition)
	}
	return p.Pins, nil
}

// WritePins writes the pins of a partition, sorted by partition and
// deliverable.
func (s Vault) WritePins(partition string, pins []Pin) error {
	out := slices.Clone(pins)
	for i := range out {
		out[i].Partition = s.PartitionID(out[i].Partition)
	}
	slices.SortFunc(out, func(a, b Pin) int {
		return strings.Compare(a.Partition+"/"+a.Deliverable, b.Partition+"/"+b.Deliverable)
	})
	if out == nil {
		out = []Pin{}
	}
	data, err := yaml.Marshal(Pins{Version: 1, Pins: out})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.ReadOnly(partition), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.PinsFile(partition), data, 0o644)
}

// SetPin adds or replaces the pin of consumer on a deliverable of a
// partition (rule PN2): ref is a released version or a tag of tags.yaml.
// It returns the pin.
func (s Vault) SetPin(consumer, partition, deliverable, ref string) (Pin, error) {
	if _, err := s.Resolve(partition, deliverable, ref); err != nil {
		return Pin{}, err
	}
	pin := Pin{Partition: partition, Deliverable: deliverable}
	if ValidVersion(ref) {
		pin.Version = ref
	} else {
		pin.Tag = ref
	}
	pins, err := s.LoadPins(consumer)
	if err != nil {
		return Pin{}, err
	}
	pins = slices.DeleteFunc(pins, func(p Pin) bool { return p.Partition == partition && p.Deliverable == deliverable })
	return pin, s.WritePins(consumer, append(pins, pin))
}

// RemovePin removes the pin of consumer on a deliverable of a partition
// (rule PN3).
func (s Vault) RemovePin(consumer, partition, deliverable string) error {
	pins, err := s.LoadPins(consumer)
	if err != nil {
		return err
	}
	rest := slices.DeleteFunc(slices.Clone(pins), func(p Pin) bool { return p.Partition == partition && p.Deliverable == deliverable })
	if len(rest) == len(pins) {
		return fmt.Errorf("%s has no pin on %s %s", consumer, partition, deliverable)
	}
	return s.WritePins(consumer, rest)
}

// PinnedBy returns the partitions, among consumers, that pin a release:
// by its version, or by a tag that points at it.
func (s Vault) PinnedBy(consumers []string, partition, deliverable, version string) []string {
	tags, _ := s.LoadTags(partition, deliverable)
	var out []string
	for _, c := range consumers {
		pins, _ := s.LoadPins(c)
		for _, p := range pins {
			if p.Partition != partition || p.Deliverable != deliverable {
				continue
			}
			if p.Version == version || (p.Tag != "" && tags.Tags[p.Tag] == version) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// PinProblems says which pins of the consumers cannot be met on this
// machine (rule PN6): a pin that the catalog does not resolve, and a pin
// whose product is absent, with what gets it.
func (s Vault) PinProblems(consumers []string) []string {
	var out []string
	for _, consumer := range consumers {
		pins, err := s.LoadPins(consumer)
		if err != nil {
			out = append(out, err.Error())
			continue
		}
		for _, p := range pins {
			version, err := s.Resolve(p.Partition, p.Deliverable, p.Ref())
			if err != nil {
				out = append(out, fmt.Sprintf("%s pins %s %s %s, which is not in the catalog: %v", consumer, p.Partition, p.Deliverable, p.Ref(), err))
				continue
			}
			if !isDir(filepath.Join(s.ReleaseDir(p.Partition, p.Deliverable, version), ProductDir)) {
				fix := "run hikari sync to pull it"
				if !s.CanPull(p.Deliverable) {
					fix = "its deliverable has no remote to pull from, so " + p.Partition + " must publish it again on this machine"
				}
				out = append(out, fmt.Sprintf("%s pins %s %s %s, whose product is absent on this machine; %s", consumer, p.Partition, p.Deliverable, p.Ref(), fix))
			}
		}
	}
	return out
}

// PullPinned pulls the product of every pin of the consumers that is absent
// (rule PN5). For each pin it returns the consumer, the pin, what was done,
// and whether that failed or is only a warning.
func (s Vault) PullPinned(ctx context.Context, consumers []string) []PullResult {
	var out []PullResult
	for _, consumer := range consumers {
		pins, err := s.LoadPins(consumer)
		if err != nil {
			out = append(out, PullResult{Consumer: consumer, Text: err.Error(), Failed: true})
			continue
		}
		for _, p := range pins {
			r := PullResult{Consumer: consumer, Pin: p}
			version, err := s.Resolve(p.Partition, p.Deliverable, p.Ref())
			switch {
			case err != nil:
				r.Text, r.Warning = "is not in the catalog: "+err.Error(), true
			case isDir(filepath.Join(s.ReleaseDir(p.Partition, p.Deliverable, version), ProductDir)):
				r.Text = "present at " + version
			case !s.CanPull(p.Deliverable):
				r.Text, r.Warning = "cannot be pulled: its deliverable has no remote with a pull command, so "+p.Partition+" must publish it again on this machine", true
			default:
				if err := s.Pull(ctx, p.Partition, p.Deliverable, version); err != nil {
					r.Text, r.Failed = "pull: "+err.Error(), true
				} else {
					r.Text = "pulled " + version
				}
			}
			out = append(out, r)
		}
	}
	return out
}

// PullResult is what PullPinned did for one pin.
type PullResult struct {
	Consumer string
	Pin      Pin
	Text     string
	Failed   bool
	Warning  bool
}
