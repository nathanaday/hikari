package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// The files of a deliverable package in public/deliverables/<name>/, and
// of a skill in skills/<name>/.
const (
	DeliverableFile = "deliverable.yaml"
	SkillFile       = "SKILL.md"
)

// Deliverable is a named package of instructions: what a product must
// contain, where it must also be published, and, in its SKILL.md, how the
// partition agent makes it.
type Deliverable struct {
	Version     int      `yaml:"version" json:"version"`
	Description string   `yaml:"description" json:"description"`
	Layout      Layout   `yaml:"layout,omitempty" json:"layout"`
	Targets     []Target `yaml:"targets,omitempty" json:"targets"`
	// Remote names the remote of public/remotes/ that holds the products
	// (rule RM2).
	Remote string `yaml:"remote,omitempty" json:"remote,omitempty"`
	// Tags are the patterns of the tags a release of it can carry (rule
	// TG2).
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// AllowsTag reports whether tag matches one of the deliverable's tag
// patterns (rule TG2).
func (d Deliverable) AllowsTag(tag string) bool { return MatchesTag(d.Tags, tag) }

// Layout holds the rules on the files of a product: glob patterns that must
// match, that alone may match, and that must not match.
type Layout struct {
	Require []string `yaml:"require,omitempty" json:"require,omitempty"`
	Allow   []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	Forbid  []string `yaml:"forbid,omitempty" json:"forbid,omitempty"`
}

// Target promises that the product is also published to a cloud service.
// Its fields other than Type pin where it goes; an empty pin leaves the
// certificate to say.
type Target struct {
	Type       string `yaml:"type" json:"type"`
	Repo       string `yaml:"repo,omitempty" json:"repo,omitempty"`
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty"`
	Kind       string `yaml:"kind,omitempty" json:"kind,omitempty"`
}

// Certificate records one target met. It is flat: each target type uses
// its own fields.
type Certificate struct {
	Type       string  `yaml:"type" json:"type" jsonschema:"the target type: github-release, dockerhub, or huggingface"`
	Repo       string  `yaml:"repo,omitempty" json:"repo,omitempty" jsonschema:"github-release and huggingface: the repo, as owner/name"`
	Repository string  `yaml:"repository,omitempty" json:"repository,omitempty" jsonschema:"dockerhub: the repository, as owner/name"`
	Tag        string  `yaml:"tag,omitempty" json:"tag,omitempty" jsonschema:"github-release and dockerhub: the tag"`
	URL        string  `yaml:"url,omitempty" json:"url,omitempty" jsonschema:"github-release and huggingface: the https URL of what was published"`
	Digest     string  `yaml:"digest,omitempty" json:"digest,omitempty" jsonschema:"dockerhub: the image digest, as sha256:<64 hex>"`
	Kind       string  `yaml:"kind,omitempty" json:"kind,omitempty" jsonschema:"huggingface: model, dataset, or space"`
	Revision   string  `yaml:"revision,omitempty" json:"revision,omitempty" jsonschema:"huggingface: the 40-hex commit of the upload"`
	Assets     []Asset `yaml:"assets,omitempty" json:"assets,omitempty" jsonschema:"github-release, optional: the uploaded files and their digests"`
}

// Asset is one file of a GitHub release.
type Asset struct {
	Name   string `yaml:"name" json:"name" jsonschema:"the file name"`
	SHA256 string `yaml:"sha256" json:"sha256" jsonschema:"the SHA-256 of the file, as 64 hex characters"`
}

// Target types (section 5 of the deliverables spec).
const (
	TargetGitHubRelease = "github-release"
	TargetDockerHub     = "dockerhub"
	TargetHuggingFace   = "huggingface"
)

var targetTypes = []string{TargetGitHubRelease, TargetDockerHub, TargetHuggingFace}

var (
	validDeliverableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	hex64                = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40                = regexp.MustCompile(`^[0-9a-f]{40}$`)
	partitionAgentName   = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
)

// ValidDeliverableName reports whether name can name a deliverable.
func ValidDeliverableName(name string) bool { return validDeliverableName.MatchString(name) }

func (s Vault) DeliverablesDir() string { return s.Public(Deliverables) }

func (s Vault) DeliverableDir(name string) string {
	return filepath.Join(s.DeliverablesDir(), name)
}

// DeliverableNames returns the names of the directories in the public
// deliverables area, sorted, valid or not.
func (s Vault) DeliverableNames() ([]string, error) {
	entries, err := os.ReadDir(s.DeliverablesDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// SkillNames returns the names of the skills of skills/ that hold a
// SKILL.md, sorted.
func (s Vault) SkillNames() []string {
	entries, _ := os.ReadDir(s.SkillsRoot())
	var out []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(s.SkillDir(e.Name()), SkillFile)); err == nil {
			out = append(out, e.Name())
		}
	}
	return out
}

// LoadDeliverable reads and checks a deliverable (rules D1 to D5, T13). On
// any problem it returns every problem.
func (s Vault) LoadDeliverable(name string) (Deliverable, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if !ValidDeliverableName(name) {
		return Deliverable{}, fmt.Errorf("deliverable %q is not a valid name (a-z, 0-9, -, up to 32 characters)", name)
	}
	dir := s.DeliverableDir(name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return Deliverable{}, fmt.Errorf("deliverable %q does not exist: %s is not a directory", name, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, DeliverableFile))
	if err != nil {
		fail("deliverable %q has no %s", name, DeliverableFile)
	}
	var d Deliverable
	if err == nil {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&d); err != nil && !errors.Is(err, io.EOF) {
			fail("deliverable %q: %s: %v", name, DeliverableFile, err)
		} else {
			errs = append(errs, d.check(name)...)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, SkillFile)); err != nil || info.IsDir() {
		fail("deliverable %q has no %s", name, SkillFile)
	} else if info.Size() == 0 {
		fail("deliverable %q: %s is empty", name, SkillFile)
	}
	if d.Remote != "" {
		if _, err := s.LoadRemote(d.Remote); err != nil {
			fail("deliverable %q: remote: %v", name, err)
		}
	}
	return d, errors.Join(errs...)
}

func (d Deliverable) check(name string) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("deliverable %q: "+format, append([]any{name}, args...)...))
	}
	if d.Version != 1 {
		fail("%s must have version: 1", DeliverableFile)
	}
	if strings.TrimSpace(d.Description) == "" {
		fail("description is required")
	}
	for list, patterns := range map[string][]string{"require": d.Layout.Require, "allow": d.Layout.Allow, "forbid": d.Layout.Forbid} {
		for _, p := range patterns {
			if !doublestar.ValidatePattern(p) || p == "" {
				fail("layout.%s pattern %q is not a valid glob", list, p)
			}
		}
	}
	seen := map[string]bool{}
	for i, t := range d.Targets {
		if !slices.Contains(targetTypes, t.Type) {
			fail("targets[%d].type must be one of %s", i, strings.Join(targetTypes, ", "))
			continue
		}
		if seen[t.Type] {
			fail("targets has two entries of type %s; at most one is allowed", t.Type)
		}
		seen[t.Type] = true
		for field, value := range t.pins() {
			if value != "" && !slices.Contains(pinFields[t.Type], field) {
				fail("targets[%d] (%s) cannot pin %s", i, t.Type, field)
			}
		}
		if t.Kind != "" && !slices.Contains(hfKinds, t.Kind) {
			fail("targets[%d].kind must be one of %s", i, strings.Join(hfKinds, ", "))
		}
	}
	if _, bad := CompileTagPatterns(d.Tags); len(bad) > 0 {
		for _, err := range bad {
			fail("tags: %v", err)
		}
	}
	if d.Remote != "" && !ValidDeliverableName(d.Remote) {
		fail("remote %q is not a valid name", d.Remote)
	}
	return errs
}

// pins returns the pin fields of a target by name.
func (t Target) pins() map[string]string {
	return map[string]string{"repo": t.Repo, "repository": t.Repository, "kind": t.Kind}
}

var pinFields = map[string][]string{
	TargetGitHubRelease: {"repo"},
	TargetDockerHub:     {"repository"},
	TargetHuggingFace:   {"repo", "kind"},
}

var hfKinds = []string{"model", "dataset", "space"}

// Match reports whether a product path matches a layout pattern (rule G4):
// `**` spans directories, and a pattern without `/` matches at the top
// level only.
func Match(pattern, path string) bool {
	ok, err := doublestar.Match(pattern, path)
	return err == nil && ok
}

// Check applies the layout rules to the paths of a product (rules G1 to
// G3, G7) and returns one problem for each failed rule.
func (l Layout) Check(paths []string) []error {
	var errs []error
	for _, p := range l.Require {
		if !slices.ContainsFunc(paths, func(path string) bool { return Match(p, path) }) {
			errs = append(errs, fmt.Errorf("layout.require: no file matches %q", p))
		}
	}
	if len(l.Allow) > 0 {
		var stray []string
		for _, path := range paths {
			if !slices.ContainsFunc(l.Allow, func(p string) bool { return Match(p, path) }) {
				stray = append(stray, path)
			}
		}
		if len(stray) > 0 {
			errs = append(errs, fmt.Errorf("layout.allow: these files match no allowed pattern: %s", firstFew(stray)))
		}
	}
	for _, p := range l.Forbid {
		var hits []string
		for _, path := range paths {
			if Match(p, path) {
				hits = append(hits, path)
			}
		}
		if len(hits) > 0 {
			errs = append(errs, fmt.Errorf("layout.forbid: %q matches %s", p, firstFew(hits)))
		}
	}
	return errs
}

func firstFew(paths []string) string {
	const limit = 5
	if len(paths) <= limit {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(paths[:limit], ", "), len(paths)-limit)
}

// CheckCertificates checks the certificates of a publish against the
// targets of its deliverable (rules C1 to C4).
func CheckCertificates(targets []Target, certs []Certificate) []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	byType := map[string]int{}
	for i, c := range certs {
		if !slices.Contains(targetTypes, c.Type) {
			fail("certificates[%d].type must be one of %s", i, strings.Join(targetTypes, ", "))
			continue
		}
		if _, dup := byType[c.Type]; dup {
			fail("certificates has two entries of type %s", c.Type)
			continue
		}
		byType[c.Type] = i
		t := slices.IndexFunc(targets, func(t Target) bool { return t.Type == c.Type })
		if t < 0 {
			fail("certificates[%d]: the deliverable has no %s target", i, c.Type)
			continue
		}
		errs = append(errs, c.check(i, targets[t])...)
	}
	for _, t := range targets {
		if _, ok := byType[t.Type]; !ok {
			fail("the deliverable promises a %s target, and no certificate of that type was given", t.Type)
		}
	}
	return errs
}

func (c Certificate) check(i int, t Target) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("certificates[%d] (%s): "+format, append([]any{i, c.Type}, args...)...))
	}
	fields := map[string]string{
		"repo": c.Repo, "repository": c.Repository, "tag": c.Tag, "url": c.URL,
		"digest": c.Digest, "kind": c.Kind, "revision": c.Revision,
	}
	var required []string
	switch c.Type {
	case TargetGitHubRelease:
		required = []string{"repo", "tag", "url"}
	case TargetDockerHub:
		required = []string{"repository", "tag", "digest"}
	case TargetHuggingFace:
		required = []string{"repo", "kind", "revision", "url"}
	}
	for field, value := range fields {
		if value != "" && !slices.Contains(required, field) {
			fail("%s is not a field of this type", field)
		}
	}
	if len(c.Assets) > 0 && c.Type != TargetGitHubRelease {
		fail("assets is not a field of this type")
	}
	for _, field := range required {
		if strings.TrimSpace(fields[field]) == "" {
			fail("%s is required", field)
		}
	}
	for field, pin := range t.pins() {
		if pin != "" && fields[field] != "" && fields[field] != pin {
			fail("%s is %q, and the deliverable pins it to %q", field, fields[field], pin)
		}
	}
	for _, field := range []string{"repo", "repository"} {
		if v := fields[field]; v != "" && !partitionAgentName.MatchString(v) {
			fail("%s %q is not owner/name", field, v)
		}
	}
	if c.URL != "" {
		if u, err := url.Parse(c.URL); err != nil || u.Scheme != "https" || u.Host == "" {
			fail("url %q is not an absolute https URL", c.URL)
		}
	}
	if c.Digest != "" && (!strings.HasPrefix(c.Digest, "sha256:") || !hex64.MatchString(strings.TrimPrefix(c.Digest, "sha256:"))) {
		fail("digest %q is not sha256:<64 hex>", c.Digest)
	}
	if c.Kind != "" && !slices.Contains(hfKinds, c.Kind) {
		fail("kind must be one of %s", strings.Join(hfKinds, ", "))
	}
	if c.Revision != "" && !hex40.MatchString(c.Revision) {
		fail("revision %q is not 40 hex characters", c.Revision)
	}
	for j, a := range c.Assets {
		if a.Name == "" || !hex64.MatchString(a.SHA256) {
			fail("assets[%d] needs a name and a 64-hex sha256", j)
		}
	}
	return errs
}
