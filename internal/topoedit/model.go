package topoedit

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/topology"
)

// Model is the topology as the source writes it (rules TM1 to TM4): ids,
// names, references, and lines, read without the checks, so that a file
// with a broken reference still has one.
type Model struct {
	Project  Project   `json:"project"`
	Repos    []Repo    `json:"repos"`
	Hubs     []Hub     `json:"hubs"`
	Agents   []Agent   `json:"agents"`
	Sidecars []Sidecar `json:"sidecars"`
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Store is x-hikari.store as written, or "".
	Store string `json:"store"`
	// Repo is whether x-hikari.project is set, and Branch is its branch.
	Repo   bool   `json:"repo"`
	Branch string `json:"branch"`
}

type Repo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Line     int    `json:"line"`
	IDLocked bool   `json:"id_locked"`
}

type Hub struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Orchestrator string   `json:"orchestrator"`
	Members      []string `json:"members"`
	Line         int      `json:"line"`
	IDLocked     bool     `json:"id_locked"`
}

type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Model        string   `json:"model"`
	System       *System  `json:"system"`
	Releases     []string `json:"releases"`
	Deliverables []string `json:"deliverables"`
	// Image and Build describe the container: the image key, and the
	// build's context and Dockerfile.
	Image string `json:"image"`
	Build string `json:"build"`
	// Template is the anchor that the service merges, such as agent for
	// <<: *agent.
	Template string `json:"template"`
	// Compose lists the service's other compose keys, which the YAML view
	// edits.
	Compose  []string `json:"compose"`
	Line     int      `json:"line"`
	IDLocked bool     `json:"id_locked"`
}

type System struct {
	Repo  string   `json:"repo"`
	Scope string   `json:"scope"`
	Read  []string `json:"read"`
	Test  string   `json:"test"`
}

type Sidecar struct {
	ID     string `json:"id"`
	Serves string `json:"serves"`
	Image  string `json:"image"`
	Line   int    `json:"line"`
}

type srcFile struct {
	Name     string                `yaml:"name"`
	Hikari   srcHikari             `yaml:"x-hikari"`
	Services map[string]srcService `yaml:"services"`
}

type srcHikari struct {
	Name    string `yaml:"name"`
	Store   string `yaml:"store"`
	Project *struct {
		Branch string `yaml:"branch"`
	} `yaml:"project"`
	Repos map[string]srcRepo `yaml:"repos"`
	Hubs  map[string]srcHub  `yaml:"hubs"`
}

type srcRepo struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
}

type srcHub struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Orchestrator string   `yaml:"orchestrator"`
	Members      []string `yaml:"members"`
}

type srcService struct {
	Image  string    `yaml:"image"`
	Build  yaml.Node `yaml:"build"`
	Hikari *struct {
		Name         string  `yaml:"name"`
		Description  string  `yaml:"description"`
		Instructions string  `yaml:"instructions"`
		Model        string  `yaml:"model"`
		System       *System `yaml:"system"`
		Grants       struct {
			Releases     []string `yaml:"releases"`
			Deliverables []string `yaml:"deliverables"`
		} `yaml:"grants"`
		Serves string `yaml:"serves"`
	} `yaml:"x-hikari"`
}

// ReadModel reads the source model of text, whose file is in dir. It
// returns nil for a text with a YAML syntax error (rule TM4).
func ReadModel(text []byte, dir string) *Model {
	d, err := parse(text)
	if err != nil {
		return nil
	}
	// A value of the wrong type, such as a list for a mapping, is an error
	// that the decoder reports after it read everything else.
	var f srcFile
	yaml.Unmarshal(text, &f)
	m := &Model{Repos: []Repo{}, Hubs: []Hub{}, Agents: []Agent{}, Sidecars: []Sidecar{}}
	m.Project = Project{ID: f.Name, Name: f.Name, Store: f.Hikari.Store}
	if f.Hikari.Name != "" {
		m.Project.Name = f.Hikari.Name
	}
	if f.Hikari.Project != nil {
		m.Project.Repo, m.Project.Branch = true, f.Hikari.Project.Branch
	}
	store := storeDir(f.Hikari.Store, dir)
	line := func(p string) int { return topology.LineOf(d.root, p) }

	for _, id := range sorted(f.Hikari.Repos) {
		r := f.Hikari.Repos[id]
		m.Repos = append(m.Repos, Repo{
			ID: id, Name: nameOr(r.Name, id), URL: r.URL, Path: r.Path, Branch: r.Branch, Line: line("x-hikari.repos." + id),
			IDLocked: r.URL != "" && exists(filepath.Join(dir, topology.CloneDir, id)),
		})
	}
	for _, id := range sorted(f.Hikari.Hubs) {
		h := f.Hikari.Hubs[id]
		m.Hubs = append(m.Hubs, Hub{
			ID: id, Name: nameOr(h.Name, id), Description: freeText(h.Description), Orchestrator: h.Orchestrator,
			Members: nonNil(h.Members), Line: line("x-hikari.hubs." + id),
			IDLocked: store != "" && (exists(filepath.Join(store, "hubs", id)) || exists(filepath.Join(store, "shared", "vocabulary", id+".yaml"))),
		})
	}
	for _, id := range sorted(f.Services) {
		s := f.Services[id]
		h := s.Hikari
		if h != nil && h.Serves != "" {
			m.Sidecars = append(m.Sidecars, Sidecar{ID: id, Serves: h.Serves, Image: s.Image, Line: line("services." + id)})
			continue
		}
		a := Agent{ID: id, Name: id, Image: s.Image, Build: buildText(&s.Build), Line: line("services." + id),
			Releases: []string{}, Deliverables: []string{}, Compose: composeKeys(d, id), Template: template(d, id)}
		if h != nil {
			a.Name = nameOr(h.Name, id)
			a.Description, a.Instructions, a.Model = freeText(h.Description), freeText(h.Instructions), h.Model
			a.System = h.System
			a.Releases, a.Deliverables = nonNil(h.Grants.Releases), nonNil(h.Grants.Deliverables)
			if a.System != nil {
				a.System.Read = nonNil(a.System.Read)
			}
		}
		a.IDLocked = store != "" && exists(filepath.Join(store, "systems", id))
		m.Agents = append(m.Agents, a)
	}
	return m
}

// storeDir resolves x-hikari.store as topology does, or returns "" when
// it is an expression.
func storeDir(store, dir string) string {
	if store == "" {
		store = "store"
	}
	if strings.Contains(store, "$") {
		return ""
	}
	if store == "~" || strings.HasPrefix(store, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			store = filepath.Join(home, strings.TrimPrefix(store, "~"))
		}
	}
	if !filepath.IsAbs(store) {
		store = filepath.Join(dir, store)
	}
	return store
}

var interpolation = regexp.MustCompile(`\$[{A-Za-z_]`)

// IsExpression reports whether compose interpolates a value (rule TM2).
func IsExpression(v string) bool {
	return interpolation.MatchString(strings.ReplaceAll(v, "$$", ""))
}

// freeText reads a text field as compose resolves it, $$ as $, unless it
// holds an expression, which shows as written.
func freeText(v string) string {
	if IsExpression(v) {
		return v
	}
	return strings.ReplaceAll(v, "$$", "$")
}

// escapeText writes a text field so that compose keeps every $.
func escapeText(v string) string { return strings.ReplaceAll(v, "$", "$$") }

// composeKeys lists the keys of a service other than x-hikari and the
// merge key, as the source writes them.
func composeKeys(d *doc, id string) []string {
	out := []string{}
	e, ok := d.find(path{"services", id})
	if !ok || e.value.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(e.value.Content); i += 2 {
		if k := e.value.Content[i].Value; k != "x-hikari" && k != "<<" {
			out = append(out, k)
		}
	}
	return out
}

// template returns the anchor that a service merges, or "".
func template(d *doc, id string) string {
	e, ok := d.find(path{"services", id, "<<"})
	if ok && e.value.Kind == yaml.AliasNode {
		return e.value.Value
	}
	return ""
}

func buildText(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value
	case yaml.MappingNode:
		var b struct {
			Context    string `yaml:"context"`
			Dockerfile string `yaml:"dockerfile"`
		}
		n.Decode(&b)
		if b.Dockerfile != "" {
			return filepath.Join(b.Context, b.Dockerfile)
		}
		return b.Context
	}
	return ""
}

func nameOr(name, id string) string {
	if name == "" {
		return id
	}
	return name
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
