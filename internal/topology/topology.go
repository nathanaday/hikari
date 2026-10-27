// Package topology reads a hikari topology: a Docker Compose file whose
// x-hikari keys hold the repos, the partitions, the grants, and the orchestration agent.
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
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/vault"
)

// User is the reserved name of the human operator. The user is in the room
// and is driven from the web UI instead of a container.
const User = "user"

// ProjectRepo is the reserved repo name of the project repository: the git
// repository that holds the topology file, when x-hikari.project is set.
const ProjectRepo = "project"

type Agent struct {
	// ID is the agent's service key, and Name is what people and agents
	// call it (rules ID1 and ID4 of the ids spec). Name is the ID unless
	// x-hikari sets it.
	ID           string     `yaml:"-"`
	Name         string     `yaml:"name"`
	Description  string     `yaml:"description"`
	Instructions string     `yaml:"instructions"`
	Model        string     `yaml:"model"`
	Partition    *Partition `yaml:"partition"`
	Grants       Grants     `yaml:"grants"`
}

// Grants bind an agent to parts of the vault: the releases it can read,
// the deliverables it publishes, and the skills it uses.
type Grants struct {
	// Releases lists the partitions whose releases the agent can read.
	Releases []string `yaml:"releases"`
	// Deliverables lists the deliverables that the agent publishes. Only a
	// partition agent can hold deliverables (rule T14).
	Deliverables []string `yaml:"deliverables"`
	// Skills lists skills of the vault's skills/ (rule V7).
	Skills []string `yaml:"skills"`
}

// Partition is the code that one agent owns: a scope inside a repo.
type Partition struct {
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

// Topology is keyed by name: Agents and Repos map each name to its
// entity, and every reference holds a name. Services stays keyed by
// service key, because it describes compose. Outside the file, hikari keys
// by id: the vault, compose, and the clones (rules ID6 to ID10).
type Topology struct {
	// ID is the compose project name, and Name is x-hikari.name, or the ID.
	ID   string
	Name string
	// Vault is the directory of the vault. It is absolute; the
	// default is vault/ next to the topology file.
	Vault string
	// VaultDefault is whether x-hikari.vault is unset.
	VaultDefault bool
	// Repos holds every repo, the project repository included under
	// ProjectRepo.
	Repos map[string]Repo
	// Project is the project repository, or nil when the topology does not
	// set x-hikari.project.
	Project  *Repo
	Agents   map[string]Agent
	Services map[string]Service
	// Orchestration is the name of the agent that x-hikari.orchestration
	// names, or empty when the project has no orchestration agent.
	Orchestration string

	// File is the topology file, when the topology came from Load.
	File string
	// Warnings are compose's warnings about the file, from Load, and
	// hikari's own.
	Warnings []string
	// Dir is the directory of the topology file. Relative paths in x-hikari
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

// DefaultVault is the vault's directory, next to the topology file, when
// x-hikari.vault is unset.
const DefaultVault = "vault"

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
	Vault   string          `yaml:"vault"`
	Project *projectKeys    `yaml:"project"`
	Repos   map[string]Repo `yaml:"repos"`
	// Orchestration is the service id of the orchestration agent.
	Orchestration string `yaml:"orchestration"`
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
	return a.Name == "" && a.Description == "" && a.Instructions == "" && a.Model == "" && a.Partition == nil && len(a.Grants.Releases) == 0 && len(a.Grants.Deliverables) == 0 && len(a.Grants.Skills) == 0
}

// Load reads a topology file through `docker compose config`, so that hikari
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
	return loadText(ctx, data, abs, draft)
}

// LoadText is Load for a text that is not on disk yet, as file would hold
// it: compose reads it on stdin, with relative paths and .env from the
// file's directory (decision 2 of the topology API spec).
func LoadText(ctx context.Context, data []byte, file string) (*Topology, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	return loadText(ctx, data, abs, false)
}

func loadText(ctx context.Context, data []byte, abs string, draft bool) (*Topology, error) {
	var top map[string]yaml.Node
	if yaml.Unmarshal(data, &top) == nil {
		if _, ok := top["agents"]; ok {
			return nil, errors.New("this file uses the old topology format; hikari now reads a topology file (see the README)")
		}
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", "-", "--project-directory", filepath.Dir(abs), "config")
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := composeMessages(stderr.String()); len(msg) > 0 {
			return nil, &ComposeError{fmt.Errorf("docker compose config: %s", strings.Join(msg, "; "))}
		}
		return nil, &ComposeError{fmt.Errorf("docker compose config: %v", err)}
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

// Parse reads compose's resolved form of a topology file, the output of
// `docker compose config`, whose relative x-hikari paths start at dir.
func Parse(data []byte, dir string) (*Topology, error) { return parse(data, dir, false) }

func parse(data []byte, dir string, draft bool) (*Topology, error) {
	var f composeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse topology file: %w", err)
	}
	if f.Name == "" && len(f.Services) == 0 && f.Hikari.Kind == 0 {
		return nil, fmt.Errorf("topology file is empty")
	}
	t := Topology{ID: f.Name, Name: f.Name, Dir: dir, Agents: map[string]Agent{}, Services: map[string]Service{}, draft: draft}
	var errs []error
	fail := func(path, format string, args ...any) { errs = append(errs, at(path, format, args...)) }

	if t.ID == "" {
		fail("name", "the topology file has no top-level name; hikari needs a stable project name")
	}
	var keys hikariKeys
	if f.Hikari.Kind == 0 {
		fail("", "the topology file has no top-level x-hikari key")
	} else if err := decodeStrict(&f.Hikari, &keys); err != nil {
		fail("x-hikari", "x-hikari: %v", err)
	}
	t.Vault, t.Repos, t.Orchestration = keys.Vault, keys.Repos, keys.Orchestration
	if keys.Name != "" {
		t.Name = keys.Name
	}
	if t.Repos == nil {
		t.Repos = map[string]Repo{}
	}
	t.raw = maps.Clone(t.Repos)
	if _, ok := t.Repos[ProjectRepo]; ok {
		fail("x-hikari.repos."+ProjectRepo, "x-hikari.repos: the repo name %q is reserved for the project repository; rename the repo", ProjectRepo)
	}
	if keys.Project != nil {
		t.Project = &Repo{Path: dir, Branch: keys.Project.Branch}
	}

	for _, name := range sortedKeys(f.Services) {
		node := f.Services[name]
		var sk serviceKeys
		var raw map[string]any
		if err := node.Decode(&sk); err != nil {
			fail("services."+name, "service %q: %v", name, err)
			continue
		}
		node.Decode(&raw)
		svc := Service{Image: sk.Image, Build: sk.Build.Kind != 0, User: sk.User, Volumes: sk.Volumes, Environment: sk.Environment, set: setKeys(raw)}
		if svc.Image == "" && svc.Build {
			svc.Image = t.ID + "-" + name
		}
		if sk.Hikari.Kind == 0 {
			fail("services."+name, "service %q has no x-hikari key: mark it as an agent (x-hikari: {description: ...}) or as a sidecar (x-hikari: {serves: <agent>})", name)
			continue
		}
		var h serviceHikari
		if err := decodeStrict(&sk.Hikari, &h); err != nil {
			fail("services."+name+".x-hikari", "service %q: x-hikari: %v", name, err)
			continue
		}
		if h.Serves != "" {
			if !h.Agent.empty() {
				fail("services."+name+".x-hikari", "service %q is a sidecar, so its x-hikari holds only serves", name)
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
	if t.Vault == "" {
		t.Vault, t.VaultDefault = DefaultVault, true
	}
	t.Vault = resolvePath(t.Vault, t.Dir)
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
	for name, a := range t.Agents {
		a.ID = name
		if a.Name == "" {
			a.Name = name
		}
		if a.Partition != nil {
			a.Partition.Scope = cleanRel(a.Partition.Scope)
			for i, p := range a.Partition.Read {
				a.Partition.Read[i] = cleanRel(p)
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
	repo := func(id string) string {
		if r, ok := t.Repos[id]; ok {
			return r.Name
		}
		return id
	}
	agents := map[string]Agent{}
	for _, a := range t.Agents {
		if s := a.Partition; s != nil {
			sys := *s
			sys.Repo = repo(s.Repo)
			a.Partition = &sys
		}
		a.Grants.Releases = mapNames(a.Grants.Releases, agent)
		agents[a.Name] = a
	}
	if t.Orchestration != "" {
		t.Orchestration = agent(t.Orchestration)
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
	t.Agents, t.Repos = agents, repos
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

// PartitionIDs maps the name of each partition to its id, for the vault (rule
// ID6).
func (t *Topology) PartitionIDs() map[string]string {
	out := map[string]string{}
	for name, a := range t.Agents {
		if a.Partition != nil {
			out[name] = a.ID
		}
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
	fail := func(path, format string, args ...any) { errs = append(errs, at(path, format, args...)) }

	if len(t.Agents) == 0 {
		fail("services", "topology has no agents")
	}
	if o := t.Orchestration; o != "" {
		if _, ok := t.Agents[o]; !ok {
			fail("x-hikari.orchestration", "x-hikari.orchestration: %q is not an agent's service id", o)
		}
	}
	for _, name := range t.AgentNames() {
		if name == User {
			fail("services."+name, "agent name %q is reserved for the human operator", User)
		} else if !validName.MatchString(name) {
			fail("services."+name, "invalid agent name %q: use lowercase letters, digits, and dashes", name)
		}
	}

	for _, name := range t.AgentNames() {
		if name != t.Orchestration && t.Agents[name].Partition == nil {
			fail("services."+name, "agent %q has no partition and is not the orchestration agent", name)
		}
	}

	errs = append(errs, t.validateNames()...)
	errs = append(errs, t.validateRepos()...)
	errs = append(errs, t.validatePartitions()...)
	errs = append(errs, t.validateServices()...)
	return errors.Join(errs...)
}

// validProjectName checks the project's name: one line of 1 to 64
// characters (rule ID2 of the ids spec).
func validProjectName(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 64 && !strings.ContainsAny(s, "\r\n")
}

// validateNames checks the names that x-hikari sets (rules ID2 and ID4).
// An entity without a name field has its id as its name, and the id is
// checked as before.
func (t *Topology) validateNames() []error {
	var errs []error
	fail := func(path, format string, args ...any) { errs = append(errs, at(path, format, args...)) }
	if t.Name != t.ID && !validProjectName(t.Name) {
		fail("x-hikari.name", "x-hikari.name: the project's name must be one line of 1 to 64 characters")
	}

	type entity struct{ kind, id, name, path string }
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
					fail(e.path, "%s: the name %q is reserved", label, reserved)
				case !validName.MatchString(e.name):
					fail(e.path, "%s: invalid name %q: use lowercase letters, digits, and dashes", label, e.name)
				}
				if other, ok := ids[e.name]; ok && other != e {
					fail(e.path, "%s is named %q, which is the id of %s %q", label, e.name, other.kind, other.id)
				}
			}
			if other, ok := names[e.name]; ok {
				fail(e.path, "%s %q and %s have the same name %q", other.kind, other.id, label, e.name)
			}
			names[e.name] = e
		}
	}
	var nodes []entity
	for _, id := range sortedKeys(t.Agents) {
		nodes = append(nodes, entity{"agent", id, t.Agents[id].Name, "services." + id + ".x-hikari.name"})
	}
	check(nodes, User)
	var repos []entity
	for _, id := range sortedKeys(t.Repos) {
		if id != ProjectRepo {
			repos = append(repos, entity{"repo", id, t.Repos[id].Name, "x-hikari.repos." + id + ".name"})
		}
	}
	check(repos, ProjectRepo)
	return errs
}

func (t *Topology) validateRepos() []error {
	var errs []error
	used := map[string]bool{}
	for _, a := range t.Agents {
		if a.Partition != nil {
			used[a.Partition.Repo] = true
		}
	}
	for _, name := range sortedKeys(t.Repos) {
		r := t.Repos[name]
		if name == ProjectRepo {
			errs = append(errs, t.validateProject()...)
			continue
		}
		path := "x-hikari.repos." + name
		if !validName.MatchString(name) {
			errs = append(errs, at(path, "invalid repo name %q: use lowercase letters, digits, and dashes", name))
		}
		raw := t.raw[name]
		switch {
		case raw.URL != "" && raw.Path != "":
			errs = append(errs, at(path, "repo %q has both url and path; a linked repo's clone is always %s/%s", name, CloneDir, name))
		case raw.URL != "" && t.Project == nil:
			errs = append(errs, at(path+".url", "repo %q has a url, which needs x-hikari.project: the clone lives in the project repository", name))
		case raw.URL != "" && !validURL(raw.URL):
			errs = append(errs, at(path+".url", "repo %q: %q is not a valid url: it must not be empty, hold spaces, or start with -", name, raw.URL))
		case raw.URL != "":
			if _, err := os.Stat(r.Path); os.IsNotExist(err) {
				t.Missing = append(t.Missing, name)
			} else if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil {
				errs = append(errs, at(path, "repo %q: %s exists but is not the top directory of a git repo; move it away, and run hikari sync", name, r.Path))
			}
		case r.Path == "":
			errs = append(errs, at(path, "repo %q has neither url nor path", name))
		default:
			if _, err := os.Stat(filepath.Join(r.Path, ".git")); err != nil && !(t.draft && os.IsNotExist(err)) {
				errs = append(errs, at(path+".path", "repo %q: %s is not the top directory of a git repo", name, r.Path))
			}
		}
		if !validBranchName(r.Branch) {
			errs = append(errs, at(path+".branch", "repo %q: %q is not a valid branch name", name, r.Branch))
		}
		switch {
		case used[name]:
		case r.URL != "":
			// Onboarding links the repos before it writes their partitions.
			t.Warnings = append(t.Warnings, fmt.Sprintf("the linked repo %s is not used by any partition yet", name))
		default:
			errs = append(errs, at(path, "repo %q is not used by any partition", name))
		}
	}
	return errs
}

// validateProject checks the project repository (rules PR1 to PR3).
func (t *Topology) validateProject() []error {
	var errs []error
	p := t.Project
	if _, err := os.Stat(filepath.Join(p.Path, ".git")); err != nil {
		errs = append(errs, at("x-hikari.project", "x-hikari.project: the directory of the topology file, %s, is not the top directory of a git repository", p.Path))
	}
	if !validBranchName(p.Branch) {
		errs = append(errs, at("x-hikari.project.branch", "x-hikari.project: %q is not a valid branch name", p.Branch))
	}
	vault, root := realPath(t.Vault), realPath(p.Path)
	if vault == root || !within(vault, root) {
		errs = append(errs, at("x-hikari.vault", "x-hikari.project: the vault %s must be a directory inside the project repository %s", t.Vault, p.Path))
	}
	return errs
}

// VaultInProject returns the vault's path relative to the project
// repository, with slashes, or "" when there is no project repository.
func (t *Topology) VaultInProject() string {
	if t.Project == nil {
		return ""
	}
	rel, err := filepath.Rel(realPath(t.Project.Path), realPath(t.Vault))
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

func (t *Topology) validatePartitions() []error {
	var errs []error
	fail := func(agent, key, format string, args ...any) {
		path := "services." + agent + ".x-hikari"
		if key != "" {
			path += "." + key
		}
		errs = append(errs, at(path, "agent %q: "+format, append([]any{agent}, args...)...))
	}
	for _, name := range t.AgentNames() {
		a := t.Agents[name]
		for _, g := range a.Grants.Releases {
			if g == name {
				fail(name, "grants.releases", "cannot be granted its own releases")
			} else if t.Agents[g].Partition == nil {
				fail(name, "grants.releases", "release grant %q is not a partition in the topology", g)
			}
		}
		seen := map[string]bool{}
		for _, d := range a.Grants.Deliverables {
			switch {
			case !validName.MatchString(d):
				fail(name, "grants.deliverables", "deliverable %q is not a valid name (a-z, 0-9, -, up to 32 characters)", d)
			case seen[d]:
				fail(name, "grants.deliverables", "deliverable %q is granted twice", d)
			case a.Partition == nil:
				fail(name, "grants.deliverables", "cannot hold the deliverable %q: only an agent with a partition can hold deliverables", d)
			}
			seen[d] = true
		}
		seen = map[string]bool{}
		for _, sk := range a.Grants.Skills {
			switch {
			case !validName.MatchString(sk):
				fail(name, "grants.skills", "skill %q is not a valid name (a-z, 0-9, -, up to 32 characters)", sk)
			case seen[sk]:
				fail(name, "grants.skills", "skill %q is granted twice", sk)
			case strings.HasPrefix(sk, api.AgentPrefix):
				fail(name, "grants.skills", "skill %q: the prefix %s is reserved for hikari's own skills; rename it", sk, api.AgentPrefix)
			}
			seen[sk] = true
		}
		s := a.Partition
		if s == nil {
			continue
		}
		if name == t.Orchestration {
			fail(name, "partition", "the orchestration agent cannot have a partition")
		}
		repo, ok := t.Repos[s.Repo]
		if !ok {
			if s.Repo == ProjectRepo {
				fail(name, "partition.repo", "the partition is in repo %q, which needs x-hikari.project", ProjectRepo)
			} else {
				fail(name, "partition.repo", "partition references unknown repo %q", s.Repo)
			}
			continue
		}
		if s.Repo == ProjectRepo {
			if vault := t.VaultInProject(); s.Scope == "" || (vault != "" && overlaps(filepath.FromSlash(vault), s.Scope)) {
				fail(name, "partition.scope", "a partition in the project repository needs a scope that does not overlap the vault")
			} else if overlaps(CloneDir, s.Scope) {
				fail(name, "partition.scope", "a partition in the project repository needs a scope that does not overlap the clone folder %s/", CloneDir)
			}
		}
		if slices.Contains(t.Missing, s.Repo) || t.draft {
			if s.Scope != "" && !filepath.IsLocal(s.Scope) {
				fail(name, "partition.scope", "scope %q must be a relative path inside the repo", s.Scope)
			}
			for _, p := range s.Read {
				if p == "" || !filepath.IsLocal(p) {
					fail(name, "partition.read", "read path %q must be a relative path inside the repo", p)
				}
			}
			continue
		}
		if s.Scope != "" {
			if !filepath.IsLocal(s.Scope) {
				fail(name, "partition.scope", "scope %q must be a relative path inside the repo", s.Scope)
			} else if !isDir(filepath.Join(repo.Path, s.Scope)) {
				fail(name, "partition.scope", "scope %q is not a directory in repo %q", s.Scope, s.Repo)
			}
		}
		for _, p := range s.Read {
			if p == "" || !filepath.IsLocal(p) {
				fail(name, "partition.read", "read path %q must be a relative path inside the repo", p)
			} else if !isDir(filepath.Join(repo.Path, p)) {
				fail(name, "partition.read", "read path %q is not a directory in repo %q", p, s.Repo)
			}
		}
	}

	// Scopes in one repo must not overlap, and no read path may reach into a
	// scope.
	partitionAgents := t.PartitionAgents()
	for i, a := range partitionAgents {
		sa := t.Agents[a].Partition
		for _, b := range partitionAgents[i+1:] {
			sb := t.Agents[b].Partition
			if sa.Repo == sb.Repo && overlaps(sa.Scope, sb.Scope) {
				errs = append(errs, at("services."+b+".x-hikari.partition.scope", "agents %q and %q have overlapping scopes in repo %q", a, b, sa.Repo))
			}
		}
	}
	for _, a := range partitionAgents {
		sa := t.Agents[a].Partition
		for _, p := range sa.Read {
			for _, b := range partitionAgents {
				sb := t.Agents[b].Partition
				if sa.Repo == sb.Repo && p != "" && overlaps(p, sb.Scope) {
					fail(a, "partition.read", "read path %q overlaps the scope of %q", p, b)
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

// PartitionAgents returns the agents that have a partition, in sorted order.
func (t *Topology) PartitionAgents() []string {
	var out []string
	for _, name := range t.AgentNames() {
		if t.Agents[name].Partition != nil {
			out = append(out, name)
		}
	}
	return out
}

// VisiblePartitions returns the partitions whose directories agent can read:
// every partition for the orchestration agent, and none for any other agent.
func (t *Topology) VisiblePartitions(agent string) []string {
	if agent == "" || agent != t.Orchestration {
		return nil
	}
	return t.PartitionAgents()
}

// ReadableReleases returns the partitions whose releases agent can read: its
// visible partitions as an orchestration agent and its release grants, in sorted
// order, without its own partition.
func (t *Topology) ReadableReleases(agent string) []string {
	set := map[string]bool{}
	for _, s := range t.VisiblePartitions(agent) {
		set[s] = true
	}
	for _, s := range t.Agents[agent].Grants.Releases {
		set[s] = true
	}
	delete(set, agent)
	return sortedKeys(set)
}

// Participants returns everyone in the room: the orchestration agent, then the
// other agents in sorted order, then the user.
func (t *Topology) Participants() []string {
	var out []string
	if t.Orchestration != "" {
		out = append(out, t.Orchestration)
	}
	for _, name := range t.AgentNames() {
		if name != t.Orchestration {
			out = append(out, name)
		}
	}
	return append(out, User)
}

// Role returns what name is in the room: "orchestration", "partition", or
// "operator" for the user.
func (t *Topology) Role(name string) string {
	switch {
	case name == User:
		return "operator"
	case name == t.Orchestration:
		return "orchestration"
	}
	return "partition"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// VaultOf returns the topology's vault, which maps the names of partitions to
// their ids.
func (t *Topology) VaultOf() vault.Vault {
	return vault.Vault{Root: t.Vault, Partitions: t.PartitionIDs()}
}
