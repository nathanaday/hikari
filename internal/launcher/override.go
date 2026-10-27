package launcher

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

// RunLabel marks every container and network of a run, so that `make clean`
// finds what a crashed run left.
const RunLabel = "hikari.run"

// OverrideInput is what the override file needs besides the topology.
type OverrideInput struct {
	RunID string
	// ServerURL is the server address as seen from inside a container.
	ServerURL string
	// EmptyDir is an empty host directory that hides paths in a container.
	EmptyDir string
	// Binaries maps each agent to the host path of the hikari binary for its
	// image's architecture.
	Binaries map[string]string
	// Users maps each agent with named volumes to the user of its container.
	Users map[string]string
}

// TokenVar names the variable that carries the token of the agent with
// service key id to compose. The override file refers to it, so that the
// token never reaches the disk.
func TokenVar(id string) string {
	return "HIKARI_TOKEN_" + strings.ToUpper(strings.ReplaceAll(id, "-", "_"))
}

// NetworkName returns the network of an agent and its sidecars, from the
// ids of the project and the agent (rule ID9).
func NetworkName(project, agent string) string { return project + "-agent-" + agent }

// AgentMounts returns every hikari mount of an agent: its code when it owns
// a system, and its part of the store.
func AgentMounts(t *topology.Topology, agent, emptyDir string) []Mount {
	var mounts []Mount
	a := t.Agents[agent]
	if s := a.System; s != nil {
		mounts = SystemMounts(t.Repos[s.Repo].Path, s.Scope, s.Read, emptyDir)
	}
	st := t.StoreOf()
	return append(mounts, StoreMounts(st, agent, a.System != nil, t.VisibleSystems(agent), t.ReadableReleases(agent), emptyDir)...)
}

type overrideFile struct {
	Services map[string]overrideService `yaml:"services"`
	Networks map[string]overrideNetwork `yaml:"networks,omitempty"`
}

type overrideService struct {
	Image       string                `yaml:"image,omitempty"`
	User        string                `yaml:"user,omitempty"`
	Entrypoint  []string              `yaml:"entrypoint,omitempty"`
	Init        bool                  `yaml:"init,omitempty"`
	WorkingDir  string                `yaml:"working_dir,omitempty"`
	NetworkMode string                `yaml:"network_mode,omitempty"`
	Networks    replaced              `yaml:"networks,omitempty"`
	ExtraHosts  []string              `yaml:"extra_hosts,omitempty"`
	Environment map[string]string     `yaml:"environment,omitempty"`
	Labels      map[string]string     `yaml:"labels,omitempty"`
	Volumes     []composeMount        `yaml:"volumes,omitempty"`
	DependsOn   map[string]dependency `yaml:"depends_on,omitempty"`
}

type composeMount struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
}

type dependency struct {
	Condition string `yaml:"condition"`
}

type overrideNetwork struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

// replaced is a list that replaces the user's list instead of merging with
// it, so that no agent keeps a network that hikari did not give it.
type replaced []string

func (r replaced) MarshalYAML() (any, error) {
	var n yaml.Node
	if err := n.Encode([]string(r)); err != nil {
		return nil, err
	}
	n.Tag = "!override"
	return &n, nil
}

// Override returns the override file of a run (rules O1 to O7).
func Override(t *topology.Topology, in OverrideInput) ([]byte, error) {
	labels := map[string]string{RunLabel: in.RunID}
	f := overrideFile{Services: map[string]overrideService{}, Networks: map[string]overrideNetwork{}}
	for _, agent := range t.AgentNames() {
		id := t.Agents[agent].ID
		network := "agent-" + id
		f.Networks[network] = overrideNetwork{Name: NetworkName(t.ID, id), Labels: labels}
		svc := overrideService{
			Entrypoint: []string{api.BinaryPath, "node"},
			Init:       true,
			Networks:   replaced{network},
			ExtraHosts: []string{"host.docker.internal:host-gateway"},
			Environment: map[string]string{
				"HIKARI_SERVER_URL":   in.ServerURL,
				"HIKARI_TOKEN":        "${" + TokenVar(id) + "}",
				"DISABLE_AUTOUPDATER": "1",
			},
			Labels:  labels,
			Volumes: []composeMount{{Type: "bind", Source: escape(in.Binaries[agent]), Target: api.BinaryPath, ReadOnly: true}},
		}
		if s := t.Agents[agent].System; s != nil {
			svc.WorkingDir = api.WorkDir(s.Scope)
		}
		for _, m := range AgentMounts(t, agent, in.EmptyDir) {
			svc.Volumes = append(svc.Volumes, composeMount{Type: "bind", Source: escape(m.Source), Target: m.Target, ReadOnly: m.ReadOnly})
		}
		if volumes := t.NamedVolumes(id); len(volumes) > 0 {
			prepare := topology.PrepareName(id)
			entry := []string{"chown", escape(in.Users[agent])}
			var mounts []composeMount
			for _, v := range volumes {
				entry = append(entry, v.Target)
				mounts = append(mounts, composeMount{Type: "volume", Source: v.Source, Target: v.Target})
			}
			f.Services[prepare] = overrideService{
				Image:       t.Services[id].Image,
				User:        "root",
				Entrypoint:  entry,
				NetworkMode: "none",
				Labels:      labels,
				Volumes:     mounts,
			}
			svc.DependsOn = map[string]dependency{prepare: {Condition: "service_completed_successfully"}}
		}
		f.Services[id] = svc
		for _, sidecar := range t.Sidecars(agent) {
			f.Services[sidecar] = overrideService{Networks: replaced{network}, Labels: labels}
		}
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// escape keeps compose from reading a $ in a value as a variable.
func escape(s string) string { return strings.ReplaceAll(s, "$", "$$") }
