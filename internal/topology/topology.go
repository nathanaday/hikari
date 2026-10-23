// Package topology reads a hikari topology: a Docker compose file whose
// x-hikari keys hold the repos, the systems, the grants, and the hubs.
// Compose describes each agent's environment; hikari keeps the boundaries.
package topology

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// User is the reserved name of the human operator. The user is a member of
// every hub and is driven from the web UI instead of a container.
const User = "user"

// ProjectRepo is the reserved repo name of the project repository: the git
// repository that holds the compose file, when x-hikari.project is set.
const ProjectRepo = "project"

type Agent struct {
	Description  string  `yaml:"description"`
	Instructions string  `yaml:"instructions"`
	Model        string  `yaml:"model"`
	System       *System `yaml:"system"`
	Grants       Grants  `yaml:"grants"`
}

// Grants bind an agent to parts of the store: the releases it can read,
// and the deliverables it publishes.
type Grants struct {
	// Releases lists the systems whose releases the agent can read.
	Releases []string `yaml:"releases"`
	// Deliverables lists the deliverables that the agent publishes. Only an
	// owner can hold deliverables (rule T14).
	Deliverables []string `yaml:"deliverables"`
}

// System is the code that one agent owns: a scope inside a repo.
type System struct {
	Repo string `yaml:"repo"`
	// Scope is relative to the repo root. Empty means the whole repo.
	Scope string   `yaml:"scope"`
	Read  []string `yaml:"read"`
	Test  string   `yaml:"test"`
}

// Repo is a git repo on the host. Path is absolute after Parse.
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

// Service is what hikari reads of a compose service: an agent or a sidecar.
type Service struct {
	// Image is the image that the service runs: its image key, or the name
	// that compose gives the image it builds.
	Image string
	Build bool
	User  string
	// Serves is the agent that a sidecar serves. It is empty for an agent.
	Serves  string
	Volumes []Volume
	// Environment maps each variable to its value, or to nil when the
	// variable is passed by name and is not set.
	Environment map[string]*string
	// set holds the compose keys that the service sets.
	set map[string]bool
}

// Volume is a mount of a service, in compose's resolved form.
type Volume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

type Topology struct {
	// Name is the compose project name.
	Name string
	// Store is the directory of the shared store. It is absolute; the
	// default is store/ next to the compose file.
	Store string
	// Repos holds every repo, the project repository included under
	// ProjectRepo.
	Repos map[string]Repo
	// Project is the project repository, or nil when the topology does not
	// set x-hikari.project.
	Project  *Repo
	Agents   map[string]Agent
	Hubs     map[string]Hub
	Services map[string]Service

	// File is the compose file, when the topology came from Load.
	File string
	// Warnings are compose's warnings about the file, from Load.
	Warnings []string
	// Dir is the directory of the compose file. Relative paths in x-hikari
	// start there.
	Dir string
}

var (
	validName   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	validBranch = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

type composeFile struct {
	Name     string               `yaml:"name"`
	Services map[string]yaml.Node `yaml:"services"`
	Hikari   yaml.Node            `yaml:"x-hikari"`
}

type hikariKeys struct {
	Store   string          `yaml:"store"`
	Project *projectKeys    `yaml:"project"`
	Repos   map[string]Repo `yaml:"repos"`
	Hubs    map[string]Hub  `yaml:"hubs"`
}

type projectKeys struct {
	Branch string `yaml:"branch"`
}

type serviceKeys struct {
	Image       string             `yaml:"image"`
	Build       yaml.Node          `yaml:"build"`
	User        string             `yaml:"user"`
	Volumes     []Volume           `yaml:"volumes"`
	Environment map[string]*string `yaml:"environment"`
	Hikari      yaml.Node          `yaml:"x-hikari"`
}

type serviceHikari struct {
	Agent  `yaml:",inline"`
	Serves string `yaml:"serves"`
}

func (a Agent) empty() bool {
	return a.Description == "" && a.Instructions == "" && a.Model == "" && a.System == nil && len(a.Grants.Releases) == 0 && len(a.Grants.Deliverables) == 0
}

// Load reads a compose file through `docker compose config`, so that hikari
// sees the file as compose will run it.
func Load(ctx context.Context, file string) (*Topology, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var top map[string]yaml.Node
	if yaml.Unmarshal(data, &top) == nil {
		if _, ok := top["agents"]; ok {
			return nil, errors.New("this file uses the old topology format; hikari now reads a compose file (see the README)")
		}
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", abs, "config")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := composeMessages(stderr.String()); len(msg) > 0 {
			return nil, fmt.Errorf("docker compose config: %s", strings.Join(msg, "; "))
		}
		return nil, fmt.Errorf("docker compose config: %v", err)
	}
	t, err := Parse(out, filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	t.File, t.Warnings = abs, composeMessages(stderr.String())
	return t, nil
}

var composeMsg = regexp.MustCompile(`msg="((?:[^"\\]|\\.)*)"`)

// composeMessages returns the messages in compose's stderr, without the time
// and level that compose puts around each one.
func composeMessages(stderr string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if m := composeMsg.FindStringSubmatch(line); m != nil {
			if msg, err := strconv.Unquote(`"` + m[1] + `"`); err == nil {
				line = msg
			}
		}
		out = append(out, line)
	}
	return out
}

// Parse reads compose's resolved form of a compose file, the output of
// `docker compose config`, whose relative x-hikari paths start at dir.
func Parse(data []byte, dir string) (*Topology, error) {
	var f composeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}
	if f.Name == "" && len(f.Services) == 0 && f.Hikari.Kind == 0 {
		return nil, fmt.Errorf("compose file is empty")
	}
	t := Topology{Name: f.Name, Dir: dir, Agents: map[string]Agent{}, Services: map[string]Service{}}
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if t.Name == "" {
		fail("the compose file has no top-level name; hikari needs a stable project name")
	}
	var keys hikariKeys
	if f.Hikari.Kind == 0 {
		fail("the compose file has no top-level x-hikari key")
	} else if err := decodeStrict(&f.Hikari, &keys); err != nil {
		fail("x-hikari: %v", err)
	}
	t.Store, t.Repos, t.Hubs = keys.Store, keys.Repos, keys.Hubs
	if t.Repos == nil {
		t.Repos = map[string]Repo{}
	}
	if _, ok := t.Repos[ProjectRepo]; ok {
		fail("x-hikari.repos: the repo name %q is reserved for the project repository; rename the repo", ProjectRepo)
	}
	if keys.Project != nil {
		t.Project = &Repo{Path: dir, Branch: keys.Project.Branch}
	}

	for _, name := range sortedKeys(f.Services) {
		node := f.Services[name]
		var sk serviceKeys
		var raw map[string]any
		if err := node.Decode(&sk); err != nil {
			fail("service %q: %v", name, err)
			continue
		}
		node.Decode(&raw)
		svc := Service{Image: sk.Image, Build: sk.Build.Kind != 0, User: sk.User, Volumes: sk.Volumes, Environment: sk.Environment, set: setKeys(raw)}
		if svc.Image == "" && svc.Build {
			svc.Image = t.Name + "-" + name
		}
		if sk.Hikari.Kind == 0 {
			fail("service %q has no x-hikari key: mark it as an agent (x-hikari: {description: ...}) or as a sidecar (x-hikari: {serves: <agent>})", name)
			continue
		}
		var h serviceHikari
		if err := decodeStrict(&sk.Hikari, &h); err != nil {
			fail("service %q: x-hikari: %v", name, err)
			continue
		}
		if h.Serves != "" {
			if !h.Agent.empty() {
				fail("service %q is a sidecar, so its x-hikari holds only serves", name)
			}
			svc.Serves = h.Serves
		} else {
			t.Agents[name] = h.Agent
		}
		t.Services[name] = svc
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	t.normalize()
	if err := t.validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// decodeStrict decodes node into v and rejects unknown fields.
func decodeStrict(node *yaml.Node, v any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// setKeys returns the keys of a service that hold a value. Compose writes
// some unset keys as null, and every service without networks gets the
// default network.
func setKeys(raw map[string]any) map[string]bool {
	set := map[string]bool{}
	for k, v := range raw {
		switch v := v.(type) {
		case nil:
		case bool:
			set[k] = v
		case string:
			set[k] = v != ""
		case []any:
			set[k] = len(v) > 0
		case map[string]any:
			if k == "networks" {
				delete(v, "default")
			}
			set[k] = len(v) > 0
		default:
			set[k] = true
		}
	}
	return set
}

func (t *Topology) normalize() {
	if t.Store == "" {
		t.Store = "store"
	}
	t.Store = resolvePath(t.Store, t.Dir)
	for name, r := range t.Repos {
		r.Path = resolvePath(r.Path, t.Dir)
		t.Repos[name] = r
	}
	if t.Project != nil {
		t.Project.Path = resolvePath(t.Project.Path, t.Dir)
		t.Repos[ProjectRepo] = *t.Project
	}
	for name, a := range t.Agents {
		if a.System != nil {
			a.System.Scope = cleanRel(a.System.Scope)
			for i, p := range a.System.Read {
				a.System.Read[i] = cleanRel(p)
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
	errs = append(errs, t.validateServices()...)
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
		if name == ProjectRepo {
			errs = append(errs, t.validateProject()...)
			continue
		}
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

// validateProject checks the project repository (rules PR1 to PR3).
func (t *Topology) validateProject() []error {
	var errs []error
	p := t.Project
	if _, err := os.Stat(filepath.Join(p.Path, ".git")); err != nil {
		errs = append(errs, fmt.Errorf("x-hikari.project: the directory of the compose file, %s, is not the top directory of a git repository", p.Path))
	}
	if !validBranchName(p.Branch) {
		errs = append(errs, fmt.Errorf("x-hikari.project: %q is not a valid branch name", p.Branch))
	}
	store, root := realPath(t.Store), realPath(p.Path)
	if store == root || !within(store, root) {
		errs = append(errs, fmt.Errorf("x-hikari.project: the store %s must be a directory inside the project repository %s", t.Store, p.Path))
	}
	return errs
}

// StoreInProject returns the store's path relative to the project
// repository, with slashes, or "" when there is no project repository.
func (t *Topology) StoreInProject() string {
	if t.Project == nil {
		return ""
	}
	rel, err := filepath.Rel(realPath(t.Project.Path), realPath(t.Store))
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
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
		for _, g := range a.Grants.Releases {
			if g == name {
				fail(name, "cannot be granted its own releases")
			} else if t.Agents[g].System == nil {
				fail(name, "release grant %q is not a system in the topology", g)
			}
		}
		seen := map[string]bool{}
		for _, d := range a.Grants.Deliverables {
			switch {
			case !validName.MatchString(d):
				fail(name, "deliverable %q is not a valid name (a-z, 0-9, -, up to 32 characters)", d)
			case seen[d]:
				fail(name, "deliverable %q is granted twice", d)
			case a.System == nil:
				fail(name, "cannot hold the deliverable %q: only an agent with a system can hold deliverables", d)
			}
			seen[d] = true
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
			if s.Repo == ProjectRepo {
				fail(name, "the system is in repo %q, which needs x-hikari.project", ProjectRepo)
			} else {
				fail(name, "system references unknown repo %q", s.Repo)
			}
			continue
		}
		if s.Repo == ProjectRepo {
			if store := t.StoreInProject(); s.Scope == "" || (store != "" && overlaps(filepath.FromSlash(store), s.Scope)) {
				fail(name, "a system in the project repository needs a scope that does not overlap the store")
			}
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

// ReadableReleases returns the systems whose releases agent can read: its
// visible systems as an orchestrator and its release grants, in sorted
// order, without its own system.
func (t *Topology) ReadableReleases(agent string) []string {
	set := map[string]bool{}
	for _, s := range t.VisibleSystems(agent) {
		set[s] = true
	}
	for _, s := range t.Agents[agent].Grants.Releases {
		set[s] = true
	}
	delete(set, agent)
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
