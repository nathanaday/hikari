package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// User is the reserved name of the human operator. The user is a member of
// every hub and is driven from the web UI instead of a container.
const User = "user"

type Agent struct {
	Description  string   `yaml:"description"`
	Instructions string   `yaml:"instructions"`
	Model        string   `yaml:"model"`
	System       *System  `yaml:"system"`
	Packages     []string `yaml:"packages"`
	Setup        string   `yaml:"setup"`
	Path         []string `yaml:"path"`
}

// System is the code that one agent owns: a scope inside a repo.
type System struct {
	Repo string `yaml:"repo"`
	// Scope is relative to the repo root. Empty means the whole repo.
	Scope string   `yaml:"scope"`
	Read  []string `yaml:"read"`
	Test  string   `yaml:"test"`
	// ContainerOnly paths are relative to the scope.
	ContainerOnly []string `yaml:"container_only"`
}

// Repo is a git repo on the host. Path is absolute after Load.
type Repo struct {
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
}

// Hub is a chat room. Its orchestrator, its members, and the user can post to
// it, and any two of them can send direct messages to each other.
type Hub struct {
	Description  string   `yaml:"description"`
	Orchestrator string   `yaml:"orchestrator"`
	Members      []string `yaml:"members"`
}

type Topology struct {
	Repos  map[string]Repo  `yaml:"repos"`
	Agents map[string]Agent `yaml:"agents"`
	Hubs   map[string]Hub   `yaml:"hubs"`

	// Dir is the directory of the topology file. Relative repo paths and
	// vocabulary files start there.
	Dir string `yaml:"-"`
}

var (
	validName    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	validPackage = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)
	validBranch  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

func Load(path string) (*Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, filepath.Dir(abs))
}

// Parse reads a topology whose relative paths start at dir.
func Parse(data []byte, dir string) (*Topology, error) {
	t := Topology{Dir: dir}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("topology is empty")
	} else if err != nil {
		return nil, fmt.Errorf("parse topology: %w", err)
	}
	t.normalize()
	if err := t.validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *Topology) normalize() {
	for name, r := range t.Repos {
		r.Path = resolvePath(r.Path, t.Dir)
		t.Repos[name] = r
	}
	for name, a := range t.Agents {
		if a.System != nil {
			a.System.Scope = cleanRel(a.System.Scope)
			for i, p := range a.System.Read {
				a.System.Read[i] = cleanRel(p)
			}
			for i, p := range a.System.ContainerOnly {
				a.System.ContainerOnly[i] = cleanRel(p)
			}
		}
		t.Agents[name] = a
	}
}

func resolvePath(p, dir string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return filepath.Clean(p)
}

// cleanRel cleans a relative path and maps "." to "".
func cleanRel(p string) string {
	if p == "" {
		return ""
	}
	if c := filepath.Clean(p); c != "." {
		return c
	}
	return ""
}

func (t *Topology) validate() error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if len(t.Agents) == 0 {
		fail("topology has no agents")
	}
	if len(t.Hubs) == 0 {
		fail("topology has no hubs")
	}
	for _, name := range t.AgentNames() {
		if name == User {
			fail("agent name %q is reserved for the human operator", User)
		} else if !validName.MatchString(name) {
			fail("invalid agent name %q: use lowercase letters, digits, and dashes", name)
		}
	}

	inHub := map[string]bool{}
	for _, name := range t.HubNames() {
		h := t.Hubs[name]
		if !validName.MatchString(name) {
			fail("invalid hub name %q: use lowercase letters, digits, and dashes", name)
		}
		if _, ok := t.Agents[name]; ok || name == User {
			fail("hub %q has the same name as an agent", name)
		}
		if h.Orchestrator == "" {
			fail("hub %q has no orchestrator", name)
		}
		seen := map[string]bool{}
		for _, a := range append([]string{h.Orchestrator}, h.Members...) {
			if a == "" {
				continue
			}
			if _, ok := t.Agents[a]; !ok {
				fail("hub %q references unknown agent %q", name, a)
			}
			if seen[a] {
				fail("hub %q lists agent %q more than once", name, a)
			}
			seen[a] = true
			inHub[a] = true
		}
	}
	for _, name := range t.AgentNames() {
		if !inHub[name] {
			fail("agent %q is in no hub, so nothing can reach it", name)
		}
	}

	errs = append(errs, t.validateRepos()...)
	errs = append(errs, t.validateSystems()...)
	return errors.Join(errs...)
}

func (t *Topology) validateRepos() []error {
	var errs []error
	used := map[string]bool{}
	for _, a := range t.Agents {
		if a.System != nil {
			used[a.System.Repo] = true
		}
	}
	for _, name := range sortedKeys(t.Repos) {
		r := t.Repos[name]
		if !validName.MatchString(name) {
			errs = append(errs, fmt.Errorf("invalid repo name %q: use lowercase letters, digits, and dashes", name))
		}
		if r.Path == "" {
			errs = append(errs, fmt.Errorf("repo %q has no path", name))
		} else if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil {
			errs = append(errs, fmt.Errorf("repo %q: %s is not the top directory of a git repo", name, r.Path))
		}
		if !validBranchName(r.Branch) {
			errs = append(errs, fmt.Errorf("repo %q: %q is not a valid branch name", name, r.Branch))
		}
		if !used[name] {
			errs = append(errs, fmt.Errorf("repo %q is not used by any system", name))
		}
	}
	return errs
}

func validBranchName(b string) bool {
	return validBranch.MatchString(b) &&
		!strings.HasPrefix(b, "-") && !strings.HasPrefix(b, "/") &&
		!strings.HasSuffix(b, "/") && !strings.HasSuffix(b, ".") && !strings.HasSuffix(b, ".lock") &&
		!strings.Contains(b, "..") && !strings.Contains(b, "//")
}

func (t *Topology) validateSystems() []error {
	var errs []error
	fail := func(agent, format string, args ...any) {
		errs = append(errs, fmt.Errorf("agent %q: "+format, append([]any{agent}, args...)...))
	}
	for _, name := range t.AgentNames() {
		a := t.Agents[name]
		for _, p := range a.Packages {
			if !validPackage.MatchString(p) {
				fail(name, "invalid package name %q", p)
			}
		}
		for _, p := range a.Path {
			if !filepath.IsAbs(p) {
				fail(name, "path entry %q is not an absolute path", p)
			}
		}
		s := a.System
		if s == nil {
			continue
		}
		if len(t.OrchestratedHubs(name)) > 0 {
			fail(name, "an orchestrator cannot own a system")
		}
		repo, ok := t.Repos[s.Repo]
		if !ok {
			fail(name, "system references unknown repo %q", s.Repo)
			continue
		}
		if s.Scope != "" {
			if !filepath.IsLocal(s.Scope) {
				fail(name, "scope %q must be a relative path inside the repo", s.Scope)
			} else if !isDir(filepath.Join(repo.Path, s.Scope)) {
				fail(name, "scope %q is not a directory in repo %q", s.Scope, s.Repo)
			}
		}
		for _, p := range s.Read {
			if p == "" || !filepath.IsLocal(p) {
				fail(name, "read path %q must be a relative path inside the repo", p)
			} else if !isDir(filepath.Join(repo.Path, p)) {
				fail(name, "read path %q is not a directory in repo %q", p, s.Repo)
			}
		}
		for _, p := range s.ContainerOnly {
			if p == "" || !filepath.IsLocal(p) {
				fail(name, "container_only path %q must be a relative path inside the scope", p)
			}
		}
	}

	// Scopes in one repo must not overlap, and no read path may reach into a
	// scope.
	owners := t.Owners()
	for i, a := range owners {
		sa := t.Agents[a].System
		for _, b := range owners[i+1:] {
			sb := t.Agents[b].System
			if sa.Repo == sb.Repo && overlaps(sa.Scope, sb.Scope) {
				errs = append(errs, fmt.Errorf("agents %q and %q have overlapping scopes in repo %q", a, b, sa.Repo))
			}
		}
	}
	for _, a := range owners {
		sa := t.Agents[a].System
		for _, p := range sa.Read {
			for _, b := range owners {
				sb := t.Agents[b].System
				if sa.Repo == sb.Repo && p != "" && overlaps(p, sb.Scope) {
					fail(a, "read path %q overlaps the scope of %q", p, b)
				}
			}
		}
	}
	return errs
}

// overlaps reports whether one relative path is the other or contains it.
// The empty path is the whole repo.
func overlaps(a, b string) bool {
	if a == "" || b == "" || a == b {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// AgentNames returns the agent names in sorted order.
func (t *Topology) AgentNames() []string {
	return sortedKeys(t.Agents)
}

// HubNames returns the hub names in sorted order.
func (t *Topology) HubNames() []string {
	return sortedKeys(t.Hubs)
}

// Owners returns the agents that own a system, in sorted order.
func (t *Topology) Owners() []string {
	var out []string
	for _, name := range t.AgentNames() {
		if t.Agents[name].System != nil {
			out = append(out, name)
		}
	}
	return out
}

// OrchestratedHubs returns the hubs that agent orchestrates, in sorted order.
func (t *Topology) OrchestratedHubs(agent string) []string {
	var out []string
	for _, h := range t.HubNames() {
		if t.Hubs[h].Orchestrator == agent {
			out = append(out, h)
		}
	}
	return out
}

// VisibleSystems returns the owners that are direct members of the hubs that
// orchestrator orchestrates, in sorted order.
func (t *Topology) VisibleSystems(orchestrator string) []string {
	set := map[string]bool{}
	for _, h := range t.OrchestratedHubs(orchestrator) {
		for _, m := range t.Hubs[h].Members {
			if t.Agents[m].System != nil {
				set[m] = true
			}
		}
	}
	return sortedKeys(set)
}

// Participants returns the orchestrator, then the other members in sorted
// order, then the user.
func (t *Topology) Participants(hub string) []string {
	h, ok := t.Hubs[hub]
	if !ok {
		return nil
	}
	members := slices.Sorted(slices.Values(h.Members))
	return append(append([]string{h.Orchestrator}, members...), User)
}

func (t *Topology) InHub(node, hub string) bool {
	return slices.Contains(t.Participants(hub), node)
}

// HubsOf returns the hubs that node is in, in sorted order.
func (t *Topology) HubsOf(node string) []string {
	var out []string
	for _, h := range t.HubNames() {
		if t.InHub(node, h) {
			out = append(out, h)
		}
	}
	return out
}

// Contacts returns the nodes that share a hub with node, in sorted order.
func (t *Topology) Contacts(node string) []string {
	set := map[string]bool{}
	for _, h := range t.HubsOf(node) {
		for _, p := range t.Participants(h) {
			if p != node {
				set[p] = true
			}
		}
	}
	return sortedKeys(set)
}

func (t *Topology) CanDirect(from, to string) bool {
	return slices.Contains(t.Contacts(from), to)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
