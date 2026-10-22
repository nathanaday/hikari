package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

// The files that hikari writes into each release.
const (
	ManifestFile = "release.yaml"
	SumsFile     = "SHA256SUMS"
	ProductDir   = "product"
)

// Manifest describes a release. hikari writes every field when it
// publishes (section 6 of the deliverables spec).
type Manifest struct {
	Deliverable  string        `yaml:"deliverable" json:"deliverable"`
	Version      string        `yaml:"version" json:"version"`
	System       string        `yaml:"system" json:"system"`
	Released     time.Time     `yaml:"released" json:"released"`
	Notes        string        `yaml:"notes" json:"notes"`
	Hikari       string        `yaml:"hikari" json:"hikari"`
	SourceCommit string        `yaml:"source_commit,omitempty" json:"source_commit,omitempty"`
	Hubs         []string      `yaml:"hubs" json:"hubs"`
	Product      ProductInfo   `yaml:"product" json:"product"`
	Map          string        `yaml:"map" json:"map"`
	Certificates []Certificate `yaml:"certificates" json:"certificates"`
}

// ProductInfo summarizes the product of a release.
type ProductInfo struct {
	Path   string `yaml:"path" json:"path"`
	Files  int    `yaml:"files" json:"files"`
	Bytes  int64  `yaml:"bytes" json:"bytes"`
	Digest string `yaml:"digest" json:"digest"`
}

// ParseManifest reads a manifest, rejecting unknown fields.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return m, nil
}

// PublishInput is what the server knows when an owner publishes.
type PublishInput struct {
	System       string
	Deliverable  string
	Version      string
	Notes        string
	Certificates []Certificate
	// SourceCommit is the commit that the system's map describes.
	SourceCommit string
	// Map is the public map at release time, as YAML.
	Map []byte
	// Hubs are the hubs where the release is announced.
	Hubs []string
	// Hikari is the version of the server.
	Hikari string
	Now    time.Time
}

// ProductCheck is what Check reports about a staged product that meets
// its deliverable.
type ProductCheck struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type productFile struct {
	path string
	size int64
	sum  string
}

// Check checks the staged product of a deliverable against the deliverable
// (rule R15, without the map checks). It returns every problem.
func (s Store) Check(system, deliverable string) (ProductCheck, error) {
	_, files, err := s.checkProduct(system, deliverable)
	if err != nil {
		return ProductCheck{}, err
	}
	return summarize(files), nil
}

func summarize(files []productFile) ProductCheck {
	var c ProductCheck
	for _, f := range files {
		c.Files++
		c.Bytes += f.size
	}
	return c
}

func (s Store) checkProduct(system, deliverable string) (Deliverable, []productFile, error) {
	var errs []error
	d, err := s.LoadDeliverable(deliverable)
	if err != nil {
		errs = append(errs, err)
	}
	dir := s.StagingOf(system, deliverable)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		errs = append(errs, fmt.Errorf("there is no staged product: %s is not a directory", dir))
		return d, nil, errors.Join(errs...)
	}
	files, err := walkProduct(dir)
	if err != nil {
		return d, nil, errors.Join(append(errs, err)...)
	}
	if len(errs) > 0 {
		return d, files, errors.Join(errs...)
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.path
	}
	errs = append(errs, d.Layout.Check(paths)...)
	if len(files) == 0 && len(d.Targets) == 0 {
		errs = append(errs, errors.New("the product has no files, and the deliverable has no targets"))
	}
	return d, files, errors.Join(errs...)
}

// walkProduct lists the regular files of a product with their sizes and
// digests, sorted by path (rule G5). A link is a problem.
func walkProduct(dir string) ([]productFile, error) {
	var files []productFile
	var links []string
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		switch {
		case e.Type()&fs.ModeSymlink != 0:
			links = append(links, rel)
		case e.Type().IsRegular():
			sum, size, err := digestFile(p)
			if err != nil {
				return err
			}
			files = append(files, productFile{path: rel, size: size, sum: sum})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		return nil, fmt.Errorf("the product must not contain links: %s", firstFew(links))
	}
	slices.SortFunc(files, func(a, b productFile) int { return strings.Compare(a.path, b.path) })
	return files, nil
}

func digestFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Publish checks the staged product of a deliverable and turns it into a
// release (rules R16, R17, R19). On any problem it returns every problem
// and changes nothing.
func (s Store) Publish(in PublishInput) (Manifest, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if !ValidVersion(in.Version) {
		fail("version %q is not semver (x.y.z, with an optional -pre-release)", in.Version)
	} else {
		published, err := s.Versions(in.System, in.Deliverable)
		if err != nil {
			return Manifest{}, err
		}
		for _, v := range published {
			if CompareVersions(in.Version, v) <= 0 {
				fail("version %s is not greater than the published version %s of %s", in.Version, v, in.Deliverable)
				break
			}
		}
	}
	if strings.TrimSpace(in.Notes) == "" {
		fail("notes are required: say what changed in this version")
	}
	d, files, err := s.checkProduct(in.System, in.Deliverable)
	if err != nil {
		errs = append(errs, err)
	} else {
		errs = append(errs, CheckCertificates(d.Targets, in.Certificates)...)
	}
	if len(errs) > 0 {
		return Manifest{}, errors.Join(errs...)
	}

	dest := s.ReleaseDir(in.System, in.Deliverable, in.Version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Manifest{}, err
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return Manifest{}, err
	}
	product := filepath.Join(dest, ProductDir)
	if err := os.Mkdir(product, 0o755); err != nil {
		return Manifest{}, err
	}
	// Move the entries, not the directory: the owner's container mounts
	// staging/, and a renamed directory would stay writable to it.
	staging := s.StagingOf(in.System, in.Deliverable)
	entries, err := os.ReadDir(staging)
	if err != nil {
		return Manifest{}, err
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(staging, e.Name()), filepath.Join(product, e.Name())); err != nil {
			return Manifest{}, err
		}
	}

	sums := sumsText(files)
	if err := os.WriteFile(filepath.Join(dest, SumsFile), []byte(sums), 0o644); err != nil {
		return Manifest{}, err
	}
	digest := sha256.Sum256([]byte(sums))
	summary := summarize(files)
	certs := in.Certificates
	if certs == nil {
		certs = []Certificate{}
	}
	hubs := in.Hubs
	if hubs == nil {
		hubs = []string{}
	}
	m := Manifest{
		Deliverable: in.Deliverable, Version: in.Version, System: in.System,
		Released: in.Now.UTC(), Notes: in.Notes, Hikari: in.Hikari, SourceCommit: in.SourceCommit,
		Hubs: hubs,
		Product: ProductInfo{
			Path: ProductDir + "/", Files: summary.Files, Bytes: summary.Bytes,
			Digest: "sha256:" + hex.EncodeToString(digest[:]),
		},
		Map: "map.yaml", Certificates: certs,
	}
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
	return m, s.pointLatest(in.System, in.Deliverable)
}

// sumsText renders the product's digests in sha256sum format (rule R12).
func sumsText(files []productFile) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "%s  %s/%s\n", f.sum, ProductDir, f.path)
	}
	return b.String()
}

// pointLatest points releases/<deliverable>/latest at the newest version,
// replacing the link atomically.
func (s Store) pointLatest(system, deliverable string) error {
	versions, err := s.Versions(system, deliverable)
	if err != nil || len(versions) == 0 {
		return err
	}
	dir := s.DeliverableReleasesDir(system, deliverable)
	tmp := filepath.Join(dir, ".latest-new")
	os.Remove(tmp)
	if err := os.Symlink(versions[0], tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "latest"))
}

// Versions returns the published versions of a deliverable of a system,
// newest first.
func (s Store) Versions(system, deliverable string) ([]string, error) {
	entries, err := os.ReadDir(s.DeliverableReleasesDir(system, deliverable))
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

// ReleasedDeliverables returns the deliverables of a system that have
// releases, sorted by name.
func (s Store) ReleasedDeliverables(system string) ([]string, error) {
	entries, err := os.ReadDir(s.ReleasesDir(system))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && ValidDeliverableName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// OldReleases returns the directories of a system's releases in the layout
// before deliverables, releases/<x.y.z>/, which hikari ignores (rule O1).
func (s Store) OldReleases(system string) []string {
	entries, _ := os.ReadDir(s.ReleasesDir(system))
	var out []string
	for _, e := range entries {
		if e.IsDir() && ValidVersion(e.Name()) {
			out = append(out, filepath.Join(s.ReleasesDir(system), e.Name()))
		}
	}
	return out
}

// Releases returns the manifests of a system's releases: by deliverable
// name, and newest first within each.
func (s Store) Releases(system string) ([]Manifest, error) {
	deliverables, err := s.ReleasedDeliverables(system)
	if err != nil {
		return nil, err
	}
	out := []Manifest{}
	for _, d := range deliverables {
		versions, err := s.Versions(system, d)
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			data, err := os.ReadFile(filepath.Join(s.ReleaseDir(system, d, v), ManifestFile))
			if err != nil {
				continue
			}
			if m, err := ParseManifest(data); err == nil {
				out = append(out, m)
			}
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
