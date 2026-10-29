package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The tags of a deliverable's releases: tags.yaml names them, and tags/
// holds one link for each (section 2 of the releases spec).
const (
	TagsFile  = "tags.yaml"
	TagsDir   = "tags"
	LatestTag = "latest"
)

// Tags maps each tag of a deliverable of a partition to a version.
type Tags struct {
	Version int               `yaml:"version"`
	Tags    map[string]string `yaml:"tags"`
}

var validTagName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,31}$`)

// ValidTagName reports whether name can name a tag (rule TG1).
func ValidTagName(name string) bool { return validTagName.MatchString(name) }

// CompileTagPatterns compiles the tag patterns of a deliverable or a grant
// (rule TG2): each is an RE2 pattern anchored at both ends, and a plain name
// matches itself. It returns one problem for each invalid pattern.
func CompileTagPatterns(patterns []string) ([]*regexp.Regexp, []error) {
	var out []*regexp.Regexp
	var errs []error
	for _, p := range patterns {
		re, err := regexp.Compile("^(?:" + p + ")$")
		if err != nil || strings.TrimSpace(p) == "" {
			errs = append(errs, fmt.Errorf("tag pattern %q is not a valid pattern", p))
			continue
		}
		out = append(out, re)
	}
	return out, errs
}

// MatchesTag reports whether tag matches one of the patterns.
func MatchesTag(patterns []string, tag string) bool {
	res, _ := CompileTagPatterns(patterns)
	return slices.ContainsFunc(res, func(re *regexp.Regexp) bool { return re.MatchString(tag) })
}

func (s Vault) TagsFile(partition, deliverable string) string {
	return filepath.Join(s.DeliverableReleasesDir(partition, deliverable), TagsFile)
}

func (s Vault) TagsDir(partition, deliverable string) string {
	return filepath.Join(s.DeliverableReleasesDir(partition, deliverable), TagsDir)
}

// LoadTags reads the tags of a deliverable of a partition. A missing file
// holds no tags.
func (s Vault) LoadTags(partition, deliverable string) (Tags, error) {
	t := Tags{Version: 1, Tags: map[string]string{}}
	data, err := os.ReadFile(s.TagsFile(partition, deliverable))
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	} else if err != nil {
		return t, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil && !errors.Is(err, io.EOF) {
		return t, fmt.Errorf("%s of %s %s: %w", TagsFile, partition, deliverable, err)
	}
	if t.Tags == nil {
		t.Tags = map[string]string{}
	}
	return t, nil
}

// WriteTags writes the tags of a deliverable and their links (rule RL5).
func (s Vault) WriteTags(partition, deliverable string, t Tags) error {
	t.Version = 1
	if t.Tags == nil {
		t.Tags = map[string]string{}
	}
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	dir := s.DeliverableReleasesDir(partition, deliverable)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.TagsFile(partition, deliverable), data, 0o644); err != nil {
		return err
	}
	return s.writeTagLinks(partition, deliverable, t)
}

// writeTagLinks makes tags/ hold one relative link for each tag and nothing
// else, and removes the latest link of an earlier layout.
func (s Vault) writeTagLinks(partition, deliverable string, t Tags) error {
	dir := s.TagsDir(partition, deliverable)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, keep := t.Tags[e.Name()]; !keep {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	for tag, version := range t.Tags {
		link := filepath.Join(dir, tag)
		target := "../" + version
		if cur, err := os.Readlink(link); err == nil && cur == target {
			continue
		}
		tmp := filepath.Join(dir, "."+tag+"-new")
		os.RemoveAll(tmp)
		if err := os.Symlink(target, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, link); err != nil {
			return err
		}
	}
	old := filepath.Join(s.DeliverableReleasesDir(partition, deliverable), LatestTag)
	if info, err := os.Lstat(old); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return os.Remove(old)
	}
	return nil
}

// SetTag points tag at version (rule TG3). Latest is hikari's own, and the
// version must be released.
func (s Vault) SetTag(partition, deliverable, tag, version string) error {
	if !ValidTagName(tag) {
		return fmt.Errorf("tag %q is not a valid tag name (a-z, then a-z, 0-9, ., _, -, up to 32 characters)", tag)
	}
	if tag == LatestTag {
		return fmt.Errorf("the tag %s is hikari's: it points at the newest version", LatestTag)
	}
	versions, err := s.Versions(partition, deliverable)
	if err != nil {
		return err
	}
	if !slices.Contains(versions, version) {
		return fmt.Errorf("%s %s has no release %s", partition, deliverable, version)
	}
	t, err := s.LoadTags(partition, deliverable)
	if err != nil {
		return err
	}
	t.Tags[tag] = version
	return s.WriteTags(partition, deliverable, t)
}

// RemoveTag removes tag (rule TG6). Latest cannot be removed.
func (s Vault) RemoveTag(partition, deliverable, tag string) error {
	if tag == LatestTag {
		return fmt.Errorf("the tag %s cannot be removed", LatestTag)
	}
	t, err := s.LoadTags(partition, deliverable)
	if err != nil {
		return err
	}
	if _, ok := t.Tags[tag]; !ok {
		return fmt.Errorf("%s %s has no tag %s", partition, deliverable, tag)
	}
	delete(t.Tags, tag)
	return s.WriteTags(partition, deliverable, t)
}

// RefreshTags writes the tags of every deliverable of partition that has
// releases (rules RL4 and RL5): latest at the newest version, a tags.yaml
// where one is missing, and the links. It returns the vault paths of the
// tags files it wrote.
func (s Vault) RefreshTags(partition string) ([]string, error) {
	deliverables, err := s.ReleasedDeliverables(partition)
	if err != nil {
		return nil, err
	}
	var wrote []string
	for _, d := range deliverables {
		versions, err := s.Versions(partition, d)
		if err != nil {
			return wrote, err
		}
		if len(versions) == 0 {
			continue
		}
		t, err := s.LoadTags(partition, d)
		if err != nil {
			return wrote, err
		}
		_, had := os.Stat(s.TagsFile(partition, d))
		changed := had != nil || t.Tags[LatestTag] != versions[0]
		t.Tags[LatestTag] = versions[0]
		for tag, v := range t.Tags {
			if !slices.Contains(versions, v) {
				delete(t.Tags, tag)
				changed = true
			}
		}
		if err := s.WriteTags(partition, d, t); err != nil {
			return wrote, err
		}
		if changed {
			wrote = append(wrote, s.Rel(s.TagsFile(partition, d)))
		}
	}
	return wrote, nil
}

// Resolve turns a version or a tag of a deliverable of a partition into a
// released version.
func (s Vault) Resolve(partition, deliverable, ref string) (string, error) {
	versions, err := s.Versions(partition, deliverable)
	if err != nil {
		return "", err
	}
	if ValidVersion(ref) {
		if !slices.Contains(versions, ref) {
			return "", fmt.Errorf("%s %s has no release %s", partition, deliverable, ref)
		}
		return ref, nil
	}
	t, err := s.LoadTags(partition, deliverable)
	if err != nil {
		return "", err
	}
	if v, ok := t.Tags[ref]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%s %s has no tag %s", partition, deliverable, ref)
}
