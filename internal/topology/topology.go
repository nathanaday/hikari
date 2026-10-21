package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// User is the reserved name of the human operator. The user is a member of
// every hub and is driven from the web UI instead of a container.
const User = "user"

type Agent struct {
	Description  string `yaml:"description"`
	Instructions string `yaml:"instructions"`
	Model        string `yaml:"model"`
}

// Hub is a chat room. Its orchestrator, its members, and the user can post to
// it, and any two of them can send direct messages to each other.
type Hub struct {
	Description  string   `yaml:"description"`
	Orchestrator string   `yaml:"orchestrator"`
	Members      []string `yaml:"members"`
}

type Topology struct {
	Agents map[string]Agent `yaml:"agents"`
	Hubs   map[string]Hub   `yaml:"hubs"`
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
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("topology is empty")
	} else if err != nil {
		return nil, fmt.Errorf("parse topology: %w", err)
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *Topology) validate() error {
	if len(t.Agents) == 0 {
		return fmt.Errorf("topology has no agents")
	}
	if len(t.Hubs) == 0 {
		return fmt.Errorf("topology has no hubs")
	}
	for name := range t.Agents {
		if name == User {
			return fmt.Errorf("agent name %q is reserved for the human operator", User)
		}
		if !validName.MatchString(name) {
			return fmt.Errorf("invalid agent name %q: use lowercase letters, digits, and dashes", name)
		}
	}
	inHub := map[string]bool{}
	for name, h := range t.Hubs {
		if !validName.MatchString(name) {
			return fmt.Errorf("invalid hub name %q: use lowercase letters, digits, and dashes", name)
		}
		if _, ok := t.Agents[name]; ok || name == User {
			return fmt.Errorf("hub %q has the same name as an agent", name)
		}
		if h.Orchestrator == "" {
			return fmt.Errorf("hub %q has no orchestrator", name)
		}
		seen := map[string]bool{}
		for _, a := range append([]string{h.Orchestrator}, h.Members...) {
			if _, ok := t.Agents[a]; !ok {
				return fmt.Errorf("hub %q references unknown agent %q", name, a)
			}
			if seen[a] {
				return fmt.Errorf("hub %q lists agent %q more than once", name, a)
			}
			seen[a] = true
			inHub[a] = true
		}
	}
	for name := range t.Agents {
		if !inHub[name] {
			return fmt.Errorf("agent %q is in no hub, so nothing can reach it", name)
		}
	}
	return nil
}

// AgentNames returns the agent names in sorted order.
func (t *Topology) AgentNames() []string {
	return sortedKeys(t.Agents)
}

// HubNames returns the hub names in sorted order.
func (t *Topology) HubNames() []string {
	return sortedKeys(t.Hubs)
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
