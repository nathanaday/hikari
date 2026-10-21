package topology

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// User is the reserved name of the human operator node. It exists in every
// topology and is driven from the web UI instead of a container.
const User = "user"

type Node struct {
	Description  string `yaml:"description"`
	Instructions string `yaml:"instructions"`
	Model        string `yaml:"model"`
}

type Topology struct {
	Nodes map[string]Node `yaml:"nodes"`
	Edges [][2]string     `yaml:"edges"`

	adj map[string]map[string]bool
}

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

func Load(path string) (*Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Topology, error) {
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse topology: %w", err)
	}
	if err := t.init(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *Topology) init() error {
	if len(t.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}
	if _, ok := t.Nodes[User]; ok {
		return fmt.Errorf("node name %q is reserved for the human operator", User)
	}
	for name := range t.Nodes {
		if !validName.MatchString(name) {
			return fmt.Errorf("invalid node name %q: use lowercase letters, digits, and dashes", name)
		}
	}
	t.adj = map[string]map[string]bool{User: {}}
	for name := range t.Nodes {
		t.adj[name] = map[string]bool{}
	}
	for _, e := range t.Edges {
		a, b := e[0], e[1]
		if a == b {
			return fmt.Errorf("edge %s-%s connects a node to itself", a, b)
		}
		for _, n := range e {
			if _, ok := t.adj[n]; !ok {
				return fmt.Errorf("edge %s-%s references unknown node %q", a, b, n)
			}
		}
		t.adj[a][b] = true
		t.adj[b][a] = true
	}
	return nil
}

// Agents returns the container node names in sorted order.
func (t *Topology) Agents() []string {
	names := make([]string, 0, len(t.Nodes))
	for n := range t.Nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (t *Topology) Has(name string) bool {
	_, ok := t.adj[name]
	return ok
}

func (t *Topology) Connected(a, b string) bool {
	return t.adj[a][b]
}

func (t *Topology) Neighbors(name string) []string {
	var out []string
	for n := range t.adj[name] {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
