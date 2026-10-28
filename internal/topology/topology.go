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
	"path"
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
	"github.com/nathanaday/hikari/internal/workspace"
)

// User is the reserved name of the human operator. The user is in the room
// and is driven from the web UI instead of a container.
const User = "user"

// Hikari is the reserved name of hikari itself, which sends the messages of
// its modules (decision 3 of the task pipeline spec). It has no turn and no
// inbox.
const Hikari = "hikari"

// KnownModules are the modules that x-hikari.modules can turn on (rule MO3
// of the task pipeline spec). A test holds it equal to the registry.
var KnownModules = []string{"tasks"}

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

// Partition is the folders that one agent's container mounts as its
// workspace. Test runs in /workspace.
type Partition struct {
	Folders []Folder `yaml:"folders"`
	Test    string   `yaml:"test"`
}

// Folder is a path in a repo. Repo holds the repo's id in the file and its
// name after Parse. Path is relative to the repo's root, with slashes; empty
// means the whole repo.
type Folder struct {
	Repo     string `yaml:"repo"`
	Path     string `yaml:"path"`
	ReadOnly bool   `yaml:"read_only"`
	At       string `yaml:"at"`
	// Place is where the folder mounts, relative to /workspace: At, or the
	// repo's name and Path.
	Place string `yaml:"-"`
}

// Writable returns the folders without read_only, in order.
func (p *Partition) Writable() []Folder {
	var out []Folder
	for _, f := range p.Folders {
		if !f.ReadOnly {
			out = append(out, f)
		}
	}
	return out
}

// Uses reports whether a folder of p is in repo.
func (p *Partition) Uses(repo string) bool {
	return slices.ContainsFunc(p.Folders, func(f Folder) bool { return f.Repo == repo })
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
	// Modules are the modules that x-hikari.modules turns on.
	Modules []string

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
	// Modules turns on modules by name.
	Modules []string `yaml:"modules"`
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

// LoadDraft is Load for a target topology: repos and folders that do not
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
	} else if old := oldKeys(&f.Hikari, oldProjectKeys); len(old) > 0 {
		for _, o := range old {
			fail("x-hikari."+o[0], "%s", o[1])
		}
	} else if err := decodeStrict(&f.Hikari, &keys); err != nil {
		fail("x-hikari", "x-hikari: %v", err)
	}
	t.Vault, t.Repos, t.Orchestration = keys.Vault, keys.Repos, keys.Orchestration
	for _, m := range keys.Modules {
		switch {
		case !slices.Contains(KnownModules, m):
			fail("x-hikari.modules", "x-hikari.modules: %q is not a module; the modules are %s", m, strings.Join(KnownModules, ", "))
		case slices.Contains(t.Modules, m):
			fail("x-hikari.modules", "x-hikari.modules: %q is listed twice", m)
		default:
			t.Modules = append(t.Modules, m)
		}
	}
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
		if old := oldKeys(&sk.Hikari, oldServiceKeys); len(old) > 0 {
			for _, o := range old {
				fail("services."+name+".x-hikari."+o[0], "service %q: %s", name, o[1])
			}
			continue
		}
		if oldPartition(&sk.Hikari) {
			fail("services."+name+".x-hikari.partition", "service %q: the partition uses the old keys repo, scope, and read; write it as folders: [{repo: <repo>, path: <scope>, at: <scope>}], with read_only: true for each read path; see %s", name, vault.Migrating("partitions"))
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

// The keys of x-hikari from before the shared language, each with the move
// that replaces it.
var (
	oldProjectKeys = map[string]string{
		"store": "x-hikari.store is now x-hikari.vault: rename the key, or remove it to use vault/ next to the topology file, and rename the directory store/ to vault/; see " + vault.Migrating("the-vault"),
		"hubs":  "x-hikari.hubs is gone: a project has one room, so name its orchestration agent with x-hikari.orchestration: <service>, and give every other agent a partition or remove it; see " + vault.Migrating("the-orchestration-agent"),
	}
	oldServiceKeys = map[string]string{
		"tests":  "x-hikari.tests is gone with the hub's tester: remove the key, and remove the service or give it a partition; its suites stay in their folder, left for integration; see " + vault.Migrating("the-orchestration-agent"),
		"system": "x-hikari.system is now x-hikari.partition, a list of folders: write partition: {folders: [{repo: <repo>, path: <scope>, at: <scope>}], test: cd <scope> && <test>}, with a folder of read_only: true for each read path; see " + vault.Migrating("partitions"),
	}
)

// oldKeys returns each key of the x-hikari node n that old names, with its
// move, in the order of the file.
func oldKeys(n *yaml.Node, old map[string]string) [][2]string {
	var out [][2]string
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if move, ok := old[n.Content[i].Value]; ok {
			out = append(out, [2]string{n.Content[i].Value, move})
		}
	}
	return out
}

// oldPartition reports whether an x-hikari node holds a partition of the
// form before folders (decision 5 of the partition folders spec).
func oldPartition(n *yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if p := n.Content[i+1]; n.Content[i].Value == "partition" && p.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(p.Content); j += 2 {
				if k := p.Content[j].Value; k == "scope" || k == "read" || k == "repo" {
					return true
				}
			}
		}
	}
	return false
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
			for i, f := range a.Partition.Folders {
				f.Path, f.At = cleanSlash(f.Path), cleanSlash(f.At)
				f.Place = f.At
				if f.Place == "" {
					repo := f.Repo
					if r, ok := t.Repos[f.Repo]; ok {
						repo = r.Name
					}
					f.Place = path.Join(repo, f.Path)
				}
				a.Partition.Folders[i] = f
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
			p := *s
			p.Folders = slices.Clone(s.Folders)
			for i := range p.Folders {
				p.Folders[i].Repo = repo(p.Folders[i].Repo)
			}
			a.Partition = &p
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

// cleanSlash cleans a relative path, writes it with slashes, and maps "."
// to "".
func cleanSlash(p string) string {
	if p == "" {
		return ""
	}
	if c := path.Clean(filepath.ToSlash(p)); c != "." {
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
		} else if name == Hikari {
			fail("services."+name, "agent name %q is reserved for hikari itself", Hikari)
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
			for _, f := range a.Partition.Folders {
				used[f.Repo] = true
			}
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
		if a.Partition == nil {
			continue
		}
		if name == t.Orchestration {
			fail(name, "partition", "the orchestration agent cannot have a partition")
		}
		errs = append(errs, t.validateFolders(name, a.Partition)...)
	}

	// Writable folders of two partitions must not overlap in a repo, and no
	// folder may reach into another partition's writable folder (rule F3).
	partitionAgents := t.PartitionAgents()
	for _, a := range partitionAgents {
		pa := t.Agents[a].Partition
		for i, fa := range pa.Folders {
			for _, b := range partitionAgents {
				if a == b || (!fa.ReadOnly && b < a) {
					continue
				}
				for _, fb := range t.Agents[b].Partition.Writable() {
					if fa.Repo != fb.Repo || !overlaps(fa.Path, fb.Path) {
						continue
					}
					if fa.ReadOnly {
						fail(a, fmt.Sprintf("partition.folders[%d]", i), "the read-only folder %s overlaps the writable folder %s of %q in repo %q", folderName(fa), folderName(fb), b, fa.Repo)
					} else {
						fail(a, fmt.Sprintf("partition.folders[%d]", i), "the writable folders %s of %q and %s of %q overlap in repo %q", folderName(fa), a, folderName(fb), b, fa.Repo)
					}
				}
			}
		}
	}
	return errs
}

// validateFolders checks the folders of one partition (rules F1 to F3).
func (t *Topology) validateFolders(name string, p *Partition) []error {
	var errs []error
	fail := func(key, format string, args ...any) {
		errs = append(errs, at("services."+name+".x-hikari."+key, "agent %q: "+format, append([]any{name}, args...)...))
	}
	if len(p.Writable()) == 0 {
		fail("partition.folders", "the partition needs at least one folder without read_only: its private documents and its commits live there")
	}
	for i, f := range p.Folders {
		key := fmt.Sprintf("partition.folders[%d]", i)
		repo, ok := t.Repos[f.Repo]
		switch {
		case f.Repo == "":
			fail(key+".repo", "folder %d has no repo", i+1)
			continue
		case !ok && f.Repo == ProjectRepo:
			fail(key+".repo", "the folder %s is in repo %q, which needs x-hikari.project", folderName(f), ProjectRepo)
			continue
		case !ok:
			fail(key+".repo", "folder %d references unknown repo %q", i+1, f.Repo)
			continue
		}
		if f.Path != "" && !filepath.IsLocal(f.Path) {
			fail(key+".path", "path %q must be a relative path inside the repo", f.Path)
			continue
		}
		if raw := p.Folders[i].At; raw != "" && !filepath.IsLocal(raw) {
			fail(key+".at", "at %q must be a relative path under %s", raw, api.WorkspaceRoot)
		} else if under(f.Place, ".hikari") || under(f.Place, ".git") {
			fail(key, "the folder %s has the place %s/%s, which hikari keeps for itself", folderName(f), api.WorkspaceRoot, f.Place)
		}
		if f.Repo == ProjectRepo {
			if vault := t.VaultInProject(); f.Path == "" || (vault != "" && overlaps(vault, f.Path)) {
				fail(key+".path", "a folder in the project repository needs a path that does not overlap the vault")
			} else if overlaps(CloneDir, f.Path) {
				fail(key+".path", "a folder in the project repository needs a path that does not overlap the clone folder %s/", CloneDir)
			}
		}
		if f.Path != "" && !slices.Contains(t.Missing, f.Repo) && !t.draft && !isDir(filepath.Join(repo.Path, filepath.FromSlash(f.Path))) {
			fail(key+".path", "path %q is not a directory in repo %q", f.Path, f.Repo)
		}
		for j, g := range p.Folders[:i] {
			switch {
			case overlaps(f.Place, g.Place):
				fail(key, "the folders %d and %d have the places %s and %s, which nest; give one of them at:", j+1, i+1, g.Place, f.Place)
			case f.Repo == g.Repo && overlaps(f.Path, g.Path):
				fail(key, "the folders %d and %d overlap in repo %q; one folder of a repo cannot hold another", j+1, i+1, f.Repo)
			}
		}
	}
	return errs
}

// folderName names a folder in an error: its repo and its path.
func folderName(f Folder) string {
	if f.Path == "" {
		return f.Repo + " (the whole repo)"
	}
	return f.Repo + "/" + f.Path
}

// overlaps reports whether one relative slash path is the other or
// contains it. The empty path is the whole repo.
func overlaps(a, b string) bool {
	if a == "" || b == "" || a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
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
	case name == Hikari:
		return "hikari"
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

// Workspace returns the folders of agent's partition with their host
// paths, or nil when agent has no partition.
func (t *Topology) Workspace(agent string) workspace.Workspace {
	p := t.Agents[agent].Partition
	if p == nil {
		return nil
	}
	out := make(workspace.Workspace, len(p.Folders))
	for i, f := range p.Folders {
		out[i] = workspace.Folder{
			Place: f.Place, Repo: f.Repo, Path: f.Path, ReadOnly: f.ReadOnly,
			Host: filepath.Join(t.Repos[f.Repo].Path, filepath.FromSlash(f.Path)),
		}
	}
	return out
}

// VaultOf returns the topology's vault, which maps the names of partitions to
// their ids.
func (t *Topology) VaultOf() vault.Vault {
	return vault.Vault{Root: t.Vault, Partitions: t.PartitionIDs()}
}

// HasModule reports whether x-hikari.modules turns on the module name.
func (t *Topology) HasModule(name string) bool { return slices.Contains(t.Modules, name) }
