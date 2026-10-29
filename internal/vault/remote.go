package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RemoteFile is the definition of a remote in public/remotes/<name>/.
const RemoteFile = "remote.yaml"

// DefaultRemoteTimeout bounds a remote's command when it sets none.
const DefaultRemoteTimeout = 10 * time.Minute

// Remote is how a product is pushed to and pulled from a home outside the
// vault: the project's commands, which hikari runs on the host (section 3
// of the releases spec).
type Remote struct {
	Version     int    `yaml:"version" json:"version"`
	Description string `yaml:"description" json:"description"`
	Push        string `yaml:"push,omitempty" json:"push,omitempty"`
	Pull        string `yaml:"pull,omitempty" json:"pull,omitempty"`
	Timeout     string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// Wait returns the timeout of the remote's commands.
func (r Remote) Wait() time.Duration {
	if d, err := time.ParseDuration(r.Timeout); err == nil && d > 0 {
		return d
	}
	return DefaultRemoteTimeout
}

func (s Vault) RemotesDir() string { return s.Public(Remotes) }

func (s Vault) RemoteDir(name string) string { return filepath.Join(s.RemotesDir(), name) }

// RemoteNames returns the names of the directories of public/remotes/,
// sorted.
func (s Vault) RemoteNames() ([]string, error) {
	entries, err := os.ReadDir(s.RemotesDir())
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

// LoadRemote reads and checks a remote (rule RM1).
func (s Vault) LoadRemote(name string) (Remote, error) {
	var r Remote
	if !ValidDeliverableName(name) {
		return r, fmt.Errorf("remote %q is not a valid name (a-z, 0-9, -, up to 32 characters)", name)
	}
	data, err := os.ReadFile(filepath.Join(s.RemoteDir(name), RemoteFile))
	if err != nil {
		return r, fmt.Errorf("remote %q does not exist: %s has no %s", name, s.RemoteDir(name), RemoteFile)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil && !errors.Is(err, io.EOF) {
		return r, fmt.Errorf("remote %q: %s: %v", name, RemoteFile, err)
	}
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("remote %q: "+format, append([]any{name}, args...)...))
	}
	if r.Version != 1 {
		fail("%s must have version: 1", RemoteFile)
	}
	if strings.TrimSpace(r.Description) == "" {
		fail("description is required")
	}
	if strings.TrimSpace(r.Push) == "" && strings.TrimSpace(r.Pull) == "" {
		fail("at least one of push and pull is required")
	}
	if r.Timeout != "" {
		if d, err := time.ParseDuration(r.Timeout); err != nil || d <= 0 {
			fail("timeout %q is not a duration such as 10m", r.Timeout)
		}
	}
	return r, errors.Join(errs...)
}

// RemoteEnv is what a remote's command learns from hikari (rule RM3).
type RemoteEnv struct {
	Remote, Partition, Deliverable, Version, Digest, Product, Release string
}

func (s Vault) env(e RemoteEnv) []string {
	return append(os.Environ(),
		"HIKARI_REMOTE="+e.Remote, "HIKARI_PARTITION="+e.Partition, "HIKARI_DELIVERABLE="+e.Deliverable,
		"HIKARI_VERSION="+e.Version, "HIKARI_DIGEST="+e.Digest, "HIKARI_PRODUCT="+e.Product,
		"HIKARI_RELEASE="+e.Release, "HIKARI_VAULT="+s.Root)
}

// RunRemote runs one command of a remote on the host (rules RM3 and RM4)
// and returns its output. A command that fails or runs past the timeout
// is an error that ends with the last lines of its output.
func (s Vault) RunRemote(ctx context.Context, remote string, command string, wait time.Duration, e RemoteEnv) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("remote %q has no such command", remote)
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	e.Remote = remote
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = s.Root
	cmd.Env = s.env(e)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return string(out), fmt.Errorf("remote %s: the command ran past %s%s", remote, wait, lastLines(out))
		}
		return string(out), fmt.Errorf("remote %s: %v%s", remote, err, lastLines(out))
	}
	return string(out), nil
}

func lastLines(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return ":\n" + strings.Join(lines, "\n")
}

// pulling is the directory inside a release that a pull fills before it
// becomes the product.
const pulling = ".product-pull"

// Pull gets the product of a release through its deliverable's remote and
// checks it against SHA256SUMS (rule RM6). A release whose product exists
// is left alone.
func (s Vault) Pull(ctx context.Context, partition, deliverable, version string) error {
	dir := s.ReleaseDir(partition, deliverable, version)
	if !isDir(dir) {
		return fmt.Errorf("%s %s has no release %s", partition, deliverable, version)
	}
	if isDir(filepath.Join(dir, ProductDir)) {
		return nil
	}
	name, r, err := s.pullable(deliverable)
	if err != nil {
		return err
	}
	sums, err := os.ReadFile(filepath.Join(dir, SumsFile))
	if err != nil {
		return fmt.Errorf("release %s %s %s has no %s", partition, deliverable, version, SumsFile)
	}
	want, err := parseSums(string(sums))
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, pulling)
	os.RemoveAll(tmp)
	if err := os.Mkdir(tmp, 0o755); err != nil {
		return err
	}
	m, _ := s.manifest(partition, deliverable, version)
	if _, err := s.RunRemote(ctx, name, r.Pull, r.Wait(), RemoteEnv{Partition: partition, Deliverable: deliverable, Version: version, Digest: m.Product.Digest, Product: tmp, Release: dir}); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	files, err := walkProduct(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if problems := compareSums(want, files); len(problems) > 0 {
		os.RemoveAll(tmp)
		return fmt.Errorf("the pulled product of %s %s %s does not match its %s: %s", partition, deliverable, version, SumsFile, firstFew(problems))
	}
	return os.Rename(tmp, filepath.Join(dir, ProductDir))
}

// pullable returns the remote of a deliverable when it has a pull command.
func (s Vault) pullable(deliverable string) (string, Remote, error) {
	d, err := s.LoadDeliverable(deliverable)
	if err != nil {
		return "", Remote{}, err
	}
	if d.Remote == "" {
		return "", Remote{}, fmt.Errorf("the deliverable %s has no remote, so its products cannot be pulled; its owner must publish it again on this machine", deliverable)
	}
	r, err := s.LoadRemote(d.Remote)
	if err != nil {
		return "", Remote{}, err
	}
	if strings.TrimSpace(r.Pull) == "" {
		return "", Remote{}, fmt.Errorf("the remote %s of %s has no pull command, so its products cannot be pulled; its owner must publish it again on this machine", d.Remote, deliverable)
	}
	return d.Remote, r, nil
}

// CanPull reports whether the products of a deliverable can be pulled.
func (s Vault) CanPull(deliverable string) bool {
	_, _, err := s.pullable(deliverable)
	return err == nil
}

func (s Vault) manifest(partition, deliverable, version string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(s.ReleaseDir(partition, deliverable, version), ManifestFile))
	if err != nil {
		return Manifest{}, err
	}
	m, err := ParseManifest(data)
	return s.inMemory(m), err
}

// parseSums reads SHA256SUMS into digests by product path.
func parseSums(text string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		sum, path, ok := strings.Cut(line, "  ")
		rel, under := strings.CutPrefix(path, ProductDir+"/")
		if !ok || !under || !hex64.MatchString(sum) {
			return nil, fmt.Errorf("%s has a line that is not <sha256>  %s/<path>: %q", SumsFile, ProductDir, line)
		}
		out[rel] = sum
	}
	return out, nil
}

// compareSums names each file that differs between the record and a
// pulled product.
func compareSums(want map[string]string, files []productFile) []string {
	var problems []string
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.path] = true
		switch sum, ok := want[f.path]; {
		case !ok:
			problems = append(problems, f.path+" is not in the record")
		case sum != f.sum:
			problems = append(problems, f.path+" has another digest")
		}
	}
	var missing []string
	for path := range want {
		if !seen[path] {
			missing = append(missing, path+" is missing")
		}
	}
	slices.Sort(missing)
	return append(problems, missing...)
}

// PullInto gets a version of a deliverable through its remote into dir,
// which must be empty, for record_release (rule RM8). The digest is empty:
// there is no record yet.
func (s Vault) PullInto(ctx context.Context, partition, deliverable, version, dir string) (string, error) {
	name, r, err := s.pullable(deliverable)
	if err != nil {
		return "", err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("staging/%s/ is not empty; record_release pulls the product into it, so empty it first", deliverable)
	} else if err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	_, err = s.RunRemote(ctx, name, r.Pull, r.Wait(), RemoteEnv{Partition: partition, Deliverable: deliverable, Version: version, Product: dir})
	return name, err
}

// push runs the remote's push for a publish (rule RM5), with the product
// still in the staging area.
func (s Vault) push(ctx context.Context, d Deliverable, in PublishInput, m Manifest, dest string) error {
	r, err := s.LoadRemote(d.Remote)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.Push) == "" {
		return nil
	}
	_, err = s.RunRemote(ctx, d.Remote, r.Push, r.Wait(), RemoteEnv{
		Partition: in.Partition, Deliverable: in.Deliverable, Version: in.Version, Digest: m.Product.Digest,
		Product: s.StagingOf(in.Partition, in.Deliverable), Release: dest,
	})
	return err
}
