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
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/store"
)

// User is the reserved name of the human operator. The user is a member of
// every hub and is driven from the web UI instead of a container.
const User = "user"

// ProjectRepo is the reserved repo name of the project repository: the git
// repository that holds the compose file, when x-hikari.project is set.
const ProjectRepo = "project"

type Agent struct {
	// ID is the agent's service key, and Name is what people and agents
	// call it (rules ID1 and ID4 of the ids spec). Name is the ID unless
	// x-hikari sets it.
	ID           string  `yaml:"-"`
	Name         string  `yaml:"name"`
	Description  string  `yaml:"description"`
	Instructions string  `yaml:"instructions"`
	Model        string  `yaml:"model"`
	System       *System `yaml:"system"`
	Grants       Grants  `yaml:"grants"`
	// Tests names the hub whose stories this owner checks: it is the hub's
	// tester (rules TS1 to TS3 of the tester spec).
	Tests string `yaml:"tests"`
	// E2E limits each service of a tester's environment (rule ST7 of the
	// environments spec).
	E2E *E2ELimits `yaml:"e2e"`
}

// E2ELimits are the most CPUs and memory that a service of an environment
// may use.
type E2ELimits struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
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

// Repo is a git repo on the host. Path is absolute after Parse. A linked
// repo has a URL, and its path is in the clone folder (rules LR1 to LR4 of
// the linked repositories spec).
type Repo struct {
	ID     string `yaml:"-"`
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
}

// CloneDir is the folder of the project repository that holds the clones of
// linked repos.
const CloneDir = "repos"

// Hub is a chat room. Its orchestrator, its members, and the user can post to
// it, and any two of them can send direct messages to each other.
type Hub struct {
	ID           string   `yaml:"-"`
	Name         string   `yaml:"name"`
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
	// Serves is the name of the agent that a sidecar serves. It is empty
	// for an agent.
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

// Topology is keyed by name: Agents, Hubs, and Repos map each name to its
// entity, and every reference holds a name. Services stays keyed by
// service key, because it describes compose. Outside the file, hikari keys
// by id: the store, compose, and the clones (rules ID6 to ID10).
type Topology struct {
	// ID is the compose project name, and Name is x-hikari.name, or the ID.
	ID   string
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
	// Warnings are compose's warnings about the file, from Load, and
	// hikari's own.
	Warnings []string
	// Dir is the directory of the compose file. Relative paths in x-hikari
	// start there.
	Dir string
	// Missing lists the linked repos whose clones do not exist yet (rule
	// LR6).
	Missing []string
	// raw holds the repos as the file writes them.
	raw map[string]Repo
	// draft skips the checks of paths that a target topology creates later
	// (rule DR1 of the hikify design spec).
	draft bool
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
	Name    string          `yaml:"name"`
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
	return a.Name == "" && a.Description == "" && a.Instructions == "" && a.Model == "" && a.System == nil && len(a.Grants.Releases) == 0 && len(a.Grants.Deliverables) == 0 && a.Tests == "" && a.E2E == nil
}

// Load reads a compose file through `docker compose config`, so that hikari
// sees the file as compose will run it.
func Load(ctx context.Context, file string) (*Topology, error) { return load(ctx, file, false) }

// LoadDraft is Load for a target topology: repos and scopes that do not
// exist yet are not errors (rule DR1).
func LoadDraft(ctx context.Context, file string) (*Topology, error) { return load(ctx, file, true) }

func load(ctx context.Context, file string, draft bool) (*Topology, error) {
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
	t, err := parse(out, filepath.Dir(abs), draft)
	if err != nil {
		return nil, err
	}
	t.File, t.Warnings = abs, append(composeMessages(stderr.String()), t.Warnings...)
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
func Parse(data []byte, dir string) (*Topology, error) { return parse(data, dir, false) }

func parse(data []byte, dir string, draft bool) (*Topology, error) {
	var f composeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}
	if f.Name == "" && len(f.Services) == 0 && f.Hikari.Kind == 0 {
		return nil, fmt.Errorf("compose file is empty")
	}
	t := Topology{ID: f.Name, Name: f.Name, Dir: dir, Agents: map[string]Agent{}, Services: map[string]Service{}, draft: draft}
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if t.ID == "" {
		fail("the compose file has no top-level name; hikari needs a stable project name")
	}
	var keys hikariKeys
	if f.Hikari.Kind == 0 {
		fail("the compose file has no top-level x-hikari key")
	} else if err := decodeStrict(&f.Hikari, &keys); err != nil {
		fail("x-hikari: %v", err)
	}
	t.Store, t.Repos, t.Hubs = keys.Store, keys.Repos, keys.Hubs
	if keys.Name != "" {
		t.Name = keys.Name
	}
	if t.Repos == nil {
		t.Repos = map[string]Repo{}
	}
	t.raw = maps.Clone(t.Repos)
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
			svc.Image = t.ID + "-" + name
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
	t.rekey()
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
	if t.Project != nil {
		t.Project.Path = resolvePath(t.Project.Path, t.Dir)
		t.Project.ID, t.Project.Name = ProjectRepo, ProjectRepo
	}
	for name, r := range t.Repos {
		r.ID = name
		if r.Name == "" {
			r.Name = name
		}
		if r.URL != "" && r.Path == "" && t.Project != nil {
			r.Path = filepath.Join(t.Project.Path, CloneDir, name)
			if r.Branch == "" {
				r.Branch = t.Project.Branch
			}
		} else {
			r.Path = resolvePath(r.Path, t.Dir)
		}
		t.Repos[name] = r
	}
	if t.Project != nil {
		t.Repos[ProjectRepo] = *t.Project
	}
	for name, h := range t.Hubs {
		h.ID = name
		if h.Name == "" {
			h.Name = name
		}
		t.Hubs[name] = h
	}
	for name, a := range t.Agents {
		a.ID = name
		if a.Name == "" {
			a.Name = name
		}
		if a.System != nil {
			a.System.Scope = cleanRel(a.System.Scope)
			for i, p := range a.System.Read {
				a.System.Read[i] = cleanRel(p)
			}
		}
		t.Agents[name] = a
	}
}

// rekey keys the checked topology by name, and resolves every reference
// from an id to a name (decision 3 of the ids spec).
func (t *Topology) rekey() {
	agent := func(id string) string {
		if a, ok := t.Agents[id]; ok {
			return a.Name
		}
		return id
	}
	hub := func(id string) string {
		if h, ok := t.Hubs[id]; ok {
			return h.Name
		}
		return id
	}
	repo := func(id string) string {
		if r, ok := t.Repos[id]; ok {
			return r.Name
		}
		return id
	}
	agents := map[string]Agent{}
	for _, a := range t.Agents {
		if s := a.System; s != nil {
			sys := *s
			sys.Repo = repo(s.Repo)
			a.System = &sys
		}
		a.Grants.Releases = mapNames(a.Grants.Releases, agent)
		if a.Tests != "" {
			a.Tests = hub(a.Tests)
		}
		agents[a.Name] = a
	}
	hubs := map[string]Hub{}
	for _, h := range t.Hubs {
		h.Orchestrator = agent(h.Orchestrator)
		h.Members = mapNames(h.Members, agent)
		hubs[h.Name] = h
	}
	repos := map[string]Repo{}
	for _, r := range t.Repos {
		repos[r.Name] = r
	}
	for i, id := range t.Missing {
		t.Missing[i] = repo(id)
	}
	for id, svc := range t.Services {
		if svc.Serves != "" {
			svc.Serves = agent(svc.Serves)
			t.Services[id] = svc
		}
	}
	t.Agents, t.Hubs, t.Repos = agents, hubs, repos
}

func mapNames(ids []string, name func(string) string) []string {
	if ids == nil {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = name(id)
	}
	return out
}

// AgentOfService returns the name of the agent whose service key is id.
func (t *Topology) AgentOfService(id string) (string, bool) {
	for name, a := range t.Agents {
		if a.ID == id {
			return name, true
		}
	}
	return "", false
}

// SystemIDs and HubIDs map the name of each system and hub to its id, for
// the store (rule ID6).
func (t *Topology) SystemIDs() map[string]string {
	out := map[string]string{}
	for name, a := range t.Agents {
		if a.System != nil {
			out[name] = a.ID
		}
	}
	return out
}

func (t *Topology) HubIDs() map[string]string {
	out := map[string]string{}
	for name, h := range t.Hubs {
		out[name] = h.ID
	}
	return out
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

	errs = append(errs, t.validateNames()...)
	errs = append(errs, t.validateRepos()...)
	errs = append(errs, t.validateSystems()...)
	errs = append(errs, t.validateServices()...)
	return errors.Join(errs...)
}

// validateNames checks the names that x-hikari sets (rules ID2 and ID4).
// An entity without a name field has its id as its name, and the id is
// checked as before.
func (t *Topology) validateNames() []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if t.Name != t.ID && (strings.TrimSpace(t.Name) == "" || len(t.Name) > 64 || strings.ContainsAny(t.Name, "\r\n")) {
		fail("x-hikari.name: the project's name must be one line of 1 to 64 characters")
	}

	type entity struct{ kind, id, name string }
	check := func(entities []entity, reserved string) {
		ids, names := map[string]entity{}, map[string]entity{}
		for _, e := range entities {
			ids[e.id] = e
		}
		for _, e := range entities {
			label := fmt.Sprintf("%s %q", e.kind, e.id)
			if e.name != e.id {
				switch {
				case e.name == reserved:
					fail("%s: the name %q is reserved", label, reserved)
				case !validName.MatchString(e.name):
					fail("%s: invalid name %q: use lowercase letters, digits, and dashes", label, e.name)
				}
				if other, ok := ids[e.name]; ok && other != e {
					fail("%s is named %q, which is the id of %s %q", label, e.name, other.kind, other.id)
				}
			}
			if other, ok := names[e.name]; ok {
				fail("%s %q and %s have the same name %q", other.kind, other.id, label, e.name)
			}
			names[e.name] = e
		}
	}
	var nodes []entity
	for _, id := range sortedKeys(t.Agents) {
		nodes = append(nodes, entity{"agent", id, t.Agents[id].Name})
	}
	for _, id := range sortedKeys(t.Hubs) {
		nodes = append(nodes, entity{"hub", id, t.Hubs[id].Name})
	}
	check(nodes, User)
	var repos []entity
	for _, id := range sortedKeys(t.Repos) {
		if id != ProjectRepo {
			repos = append(repos, entity{"repo", id, t.Repos[id].Name})
		}
	}
	check(repos, ProjectRepo)
	return errs
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
		raw := t.raw[name]
		switch {
		case raw.URL != "" && raw.Path != "":
			errs = append(errs, fmt.Errorf("repo %q has both url and path; a linked repo's clone is always %s/%s", name, CloneDir, name))
		case raw.URL != "" && t.Project == nil:
			errs = append(errs, fmt.Errorf("repo %q has a url, which needs x-hikari.project: the clone lives in the project repository", name))
		case raw.URL != "" && !validURL(raw.URL):
			errs = append(errs, fmt.Errorf("repo %q: %q is not a valid url: it must not be empty, hold spaces, or start with -", name, raw.URL))
		case raw.URL != "":
			if _, err := os.Stat(r.Path); os.IsNotExist(err) {
				t.Missing = append(t.Missing, name)
			} else if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil {
				errs = append(errs, fmt.Errorf("repo %q: %s exists but is not the top directory of a git repo; move it away, and run hikari sync", name, r.Path))
			}
		case r.Path == "":
			errs = append(errs, fmt.Errorf("repo %q has neither url nor path", name))
		default:
			if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil && !(t.draft && os.IsNotExist(err)) {
				errs = append(errs, fmt.Errorf("repo %q: %s is not the top directory of a git repo", name, r.Path))
			}
		}
		if !validBranchName(r.Branch) {
			errs = append(errs, fmt.Errorf("repo %q: %q is not a valid branch name", name, r.Branch))
		}
		switch {
		case used[name]:
		case r.URL != "":
			// Onboarding links the repos before it writes their systems.
			t.Warnings = append(t.Warnings, fmt.Sprintf("the linked repo %s is not used by any system yet", name))
		default:
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

// validURL checks a linked repo's url (rule LR3). git gets it after --,
// so the leading dash is refused only to keep the error clear.
func validURL(u string) bool {
	return u != "" && !strings.HasPrefix(u, "-") && !strings.ContainsFunc(u, func(r rune) bool { return r <= ' ' || r == 0x7f })
}

// Linked returns the names of the linked repos, sorted.
func (t *Topology) Linked() []string {
	var out []string
	for _, name := range sortedKeys(t.Repos) {
		if t.Repos[name].URL != "" {
			out = append(out, name)
		}
	}
	return out
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
		if a.E2E != nil && a.Tests == "" {
			fail(name, "sets e2e limits, which only a tester has")
		}
		if a.Tests != "" {
			h, ok := t.Hubs[a.Tests]
			switch {
			case a.System == nil:
				fail(name, "tests #%s, but only an agent with a system can be a tester", a.Tests)
			case !ok:
				fail(name, "tests the hub %q, which is not in the topology", a.Tests)
			case !slices.Contains(h.Members, name):
				fail(name, "tests #%s, so it must be a member of #%s", a.Tests, a.Tests)
			}
			for _, other := range t.AgentNames() {
				if other < name && t.Agents[other].Tests == a.Tests {
					fail(name, "tests #%s, which %q already tests; a hub has one tester", a.Tests, other)
				}
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
			} else if overlaps(CloneDir, s.Scope) {
				fail(name, "a system in the project repository needs a scope that does not overlap the clone folder %s/", CloneDir)
			}
		}
		if slices.Contains(t.Missing, s.Repo) || t.draft {
			if s.Scope != "" && !filepath.IsLocal(s.Scope) {
				fail(name, "scope %q must be a relative path inside the repo", s.Scope)
			}
			for _, p := range s.Read {
				if p == "" || !filepath.IsLocal(p) {
					fail(name, "read path %q must be a relative path inside the repo", p)
				}
			}
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
	for _, s := range t.TestedSystems(agent) {
		set[s] = true
	}
	delete(set, agent)
	return sortedKeys(set)
}

// TestedSystems returns the systems of the hub that agent tests: the
// owners that are members of it, without agent (rule TS4).
func (t *Topology) TestedSystems(agent string) []string {
	hub := t.Agents[agent].Tests
	if hub == "" {
		return nil
	}
	var out []string
	for _, m := range t.Hubs[hub].Members {
		if m != agent && t.Agents[m].System != nil {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return out
}

// Tester returns the tester of hub, or "".
func (t *Topology) Tester(hub string) string {
	for _, name := range t.AgentNames() {
		if t.Agents[name].Tests == hub {
			return name
		}
	}
	return ""
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

// ChildHubs returns the hubs whose orchestrator is a member of hub, in
// sorted order.
func (t *Topology) ChildHubs(hub string) []string {
	var out []string
	for _, h := range t.HubNames() {
		if h != hub && slices.Contains(t.Hubs[hub].Members, t.Hubs[h].Orchestrator) {
			out = append(out, h)
		}
	}
	return out
}

// ParentHubs returns the hubs that have the orchestrator of hub as a
// member, in sorted order.
func (t *Topology) ParentHubs(hub string) []string {
	var out []string
	for _, h := range t.HubNames() {
		if h != hub && slices.Contains(t.Hubs[h].Members, t.Hubs[hub].Orchestrator) {
			out = append(out, h)
		}
	}
	return out
}

// MemberHubs returns the hubs that agent is a member of, not the
// orchestrator of, in sorted order.
func (t *Topology) MemberHubs(agent string) []string {
	var out []string
	for _, h := range t.HubNames() {
		if slices.Contains(t.Hubs[h].Members, agent) {
			out = append(out, h)
		}
	}
	return out
}

// ReadableHubWikis returns the hubs whose wikis agent reads (rules A1 and
// A2 of the wikis spec): the hubs it is in, and the child hubs of the hubs
// it orchestrates, in sorted order.
func (t *Topology) ReadableHubWikis(agent string) []string {
	set := map[string]bool{}
	for _, h := range t.HubsOf(agent) {
		set[h] = true
	}
	for _, h := range t.OrchestratedHubs(agent) {
		for _, c := range t.ChildHubs(h) {
			set[c] = true
		}
	}
	return sortedKeys(set)
}

// StoreOf returns the topology's store, which maps the names of systems
// and hubs to their ids.
func (t *Topology) StoreOf() store.Store {
	return store.Store{Root: t.Store, Systems: t.SystemIDs(), Hubs: t.HubIDs()}
}
