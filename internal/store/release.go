package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the manifest of a release, in the staging area and in
// each release.
const ManifestFile = "release.yaml"

// Manifest describes a release. The owner writes the product, runtime,
// docs, and interfaces; the server adds the rest when it publishes.
type Manifest struct {
	Version      string    `yaml:"version,omitempty" json:"version"`
	System       string    `yaml:"system,omitempty" json:"system"`
	Released     time.Time `yaml:"released,omitempty" json:"released,omitzero"`
	SourceCommit string    `yaml:"source_commit,omitempty" json:"source_commit,omitempty"`
	Notes        string    `yaml:"notes,omitempty" json:"notes,omitempty"`
	Product      Product   `yaml:"product" json:"product"`
	Runtime      Runtime   `yaml:"runtime,omitempty" json:"runtime"`
	Docs         Docs      `yaml:"docs" json:"docs"`
	Interfaces   []string  `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	Map          string    `yaml:"map,omitempty" json:"map,omitempty"`
}

type Product struct {
	Kind     string    `yaml:"kind" json:"kind"`
	Path     string    `yaml:"path,omitempty" json:"path,omitempty"`
	Location *Location `yaml:"location,omitempty" json:"location,omitempty"`
}

// Location is where a remote product is hosted.
type Location struct {
	Type string `yaml:"type" json:"type"`
	Ref  string `yaml:"ref" json:"ref"`
}

type Runtime struct {
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
	Install  string   `yaml:"install,omitempty" json:"install,omitempty"`
	Run      string   `yaml:"run,omitempty" json:"run,omitempty"`
}

type Docs struct {
	Index string `yaml:"index" json:"index"`
}

// Product kinds and remote location types.
const (
	KindFiles  = "files"
	KindRemote = "remote"
)

var locationTypes = []string{"docker", "github-release", "huggingface", "url"}

// ParseManifest reads a manifest, rejecting unknown fields.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return m, nil
}

// PublishInput is what the server knows when an owner publishes.
type PublishInput struct {
	System  string
	Version string
	Notes   string
	// SourceCommit is the commit that the system's map describes.
	SourceCommit string
	// Provides holds the interfaces that the system's map provides.
	Provides map[string]bool
	// Map is the public map at release time, as YAML.
	Map []byte
	Now time.Time
}

// Publish checks the staging area of a system and turns it into a release
// (rules R1 and R3 to R5). On any problem it returns every problem and
// changes nothing.
func (s Store) Publish(in PublishInput) (Manifest, error) {
	staging := s.Staging(in.System)
	m, err := s.checkStaging(in)
	if err != nil {
		return Manifest{}, err
	}

	dest := s.ReleaseDir(in.System, in.Version)
	if err := os.Mkdir(dest, 0o755); err != nil {
		return Manifest{}, err
	}
	// Move the entries, not the directory: the owner's container mounts
	// staging/, and a renamed staging/ would stay writable to it.
	entries, err := os.ReadDir(staging)
	if err != nil {
		return Manifest{}, err
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(staging, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return Manifest{}, err
		}
	}

	m.Version, m.System, m.Notes = in.Version, in.System, in.Notes
	m.Released, m.SourceCommit, m.Map = in.Now.UTC(), in.SourceCommit, "map.yaml"
	data, err := yaml.Marshal(m)
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, ManifestFile), data, 0o644); err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, "map.yaml"), in.Map, 0o644); err != nil {
		return Manifest{}, err
	}
	return m, s.pointLatest(in.System)
}

func (s Store) checkStaging(in PublishInput) (Manifest, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	staging := s.Staging(in.System)

	if !ValidVersion(in.Version) {
		fail("version %q is not semver (x.y.z, with an optional -pre-release)", in.Version)
	} else {
		published, err := s.Versions(in.System)
		if err != nil {
			return Manifest{}, err
		}
		for _, v := range published {
			if CompareVersions(in.Version, v) <= 0 {
				fail("version %s is not greater than the published version %s", in.Version, v)
				break
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(staging, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		fail("the staging area has no %s", ManifestFile)
		return Manifest{}, errors.Join(errs...)
	} else if err != nil {
		return Manifest{}, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		fail("%v", err)
		return Manifest{}, errors.Join(errs...)
	}

	inStaging := func(p string) (fs.FileInfo, bool) {
		if p == "" || !filepath.IsLocal(filepath.Clean(p)) {
			return nil, false
		}
		info, err := os.Stat(filepath.Join(staging, filepath.Clean(p)))
		return info, err == nil
	}
	switch m.Product.Kind {
	case KindFiles:
		if _, ok := inStaging(m.Product.Path); !ok {
			fail("product.path %q does not exist in the staging area", m.Product.Path)
		}
	case KindRemote:
		if m.Product.Location == nil || !slices.Contains(locationTypes, m.Product.Location.Type) {
			fail("product.location.type must be one of %s", strings.Join(locationTypes, ", "))
		} else if m.Product.Location.Ref == "" {
			fail("product.location.ref is empty")
		}
	default:
		fail("product.kind must be %q or %q", KindFiles, KindRemote)
	}
	if info, ok := inStaging(m.Docs.Index); !ok || info.IsDir() {
		fail("docs.index %q is not a file in the staging area", m.Docs.Index)
	} else if info.Size() == 0 {
		fail("docs.index %q is empty", m.Docs.Index)
	}
	for _, name := range m.Interfaces {
		if !in.Provides[name] {
			fail("interface %q is not one that the map provides", name)
		}
	}
	return m, errors.Join(errs...)
}

// pointLatest points releases/latest at the newest version, replacing the
// link atomically.
func (s Store) pointLatest(system string) error {
	versions, err := s.Versions(system)
	if err != nil || len(versions) == 0 {
		return err
	}
	dir := s.ReleasesDir(system)
	tmp := filepath.Join(dir, ".latest-new")
	os.Remove(tmp)
	if err := os.Symlink(versions[0], tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "latest"))
}

// Versions returns the published versions of a system, newest first.
func (s Store) Versions(system string) ([]string, error) {
	entries, err := os.ReadDir(s.ReleasesDir(system))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && ValidVersion(e.Name()) {
			out = append(out, e.Name())
		}
	}
	slices.SortFunc(out, func(a, b string) int { return CompareVersions(b, a) })
	return out, nil
}

// Releases returns the manifests of a system's releases, newest first.
func (s Store) Releases(system string) ([]Manifest, error) {
	versions, err := s.Versions(system)
	if err != nil {
		return nil, err
	}
	out := []Manifest{}
	for _, v := range versions {
		data, err := os.ReadFile(filepath.Join(s.ReleaseDir(system, v), ManifestFile))
		if err != nil {
			continue
		}
		if m, err := ParseManifest(data); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// ValidVersion reports whether v is a semver version without build
// metadata.
func ValidVersion(v string) bool { return semver.MatchString(v) }

// CompareVersions compares two valid versions by semver precedence. It
// returns a negative number when a < b, zero when they are equal, and a
// positive number when a > b.
func CompareVersions(a, b string) int {
	ma, mb := semver.FindStringSubmatch(a), semver.FindStringSubmatch(b)
	for i := 1; i <= 3; i++ {
		if c := compareNumbers(ma[i], mb[i]); c != 0 {
			return c
		}
	}
	switch pa, pb := ma[4], mb[4]; {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	default:
		return comparePreRelease(strings.Split(pa, "."), strings.Split(pb, "."))
	}
}

func comparePreRelease(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		na, ea := strconv.Atoi(a[i])
		nb, eb := strconv.Atoi(b[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				return na - nb
			}
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return len(a) - len(b)
}

func compareNumbers(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}
