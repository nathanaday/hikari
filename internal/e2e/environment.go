package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvironmentFile is the environment of a tester's scope (section 2 of the
// environments spec).
const EnvironmentFile = "environment.yaml"

// Environment is environment.yaml: a compose file of services, read as
// YAML so that hikari sees exactly what the tester wrote.
type Environment struct {
	Services map[string]EnvService
}

// EnvService is what hikari reads of one service: its image, its hikari
// keys, and every key it sets.
type EnvService struct {
	Image   string
	Release string
	// Mounts maps each source to its container path: a release
	// (<system>/<deliverable>) or a directory of the tester's scope
	// (scope:<path>).
	Mounts map[string]string
	keys   map[string]*yaml.Node
}

// ScopePrefix starts a mount source that is a directory of the tester's
// scope.
const ScopePrefix = "scope:"

// refusedKeys cross a boundary, or are hikari's to set (rule EN3).
var refusedKeys = []string{"networks", "network_mode", "ports", "privileged", "cap_add", "devices", "pid", "ipc",
	"userns_mode", "volumes_from", "env_file", "build", "external_links", "links", "extra_hosts", "security_opt", "cgroup_parent"}

var (
	serviceName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	// hostVariable finds compose's interpolation of a host variable: a $
	// that $$ does not escape.
	hostVariable = regexp.MustCompile(`(^|[^$])\$(\{|[A-Za-z_])`)
)

// ParseEnvironment reads environment.yaml and checks the rules that need
// nothing outside it (EN1 to EN3).
func ParseEnvironment(data []byte) (Environment, error) {
	var errs []error
	fail := func(rule, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s: "+format, append([]any{rule, EnvironmentFile}, args...)...))
	}
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(data, &top); err != nil {
		return Environment{}, fmt.Errorf("EN1: %s: %w", EnvironmentFile, err)
	}
	for key := range top {
		if key != "services" {
			fail("EN1", "the top-level key %q is hikari's or not allowed; an environment holds only services", key)
		}
	}
	if hostVariable.Match(data) {
		fail("CR1", "it reads a variable of the host, such as ${NAME}; write the value, or $$ for a literal $")
	}
	env := Environment{Services: map[string]EnvService{}}
	services := top["services"]
	var raw map[string]map[string]yaml.Node
	if services.Kind != 0 {
		if err := services.Decode(&raw); err != nil {
			return Environment{}, fmt.Errorf("EN1: %s: services: %w", EnvironmentFile, err)
		}
	}
	if len(raw) == 0 {
		fail("EN1", "it has no services")
	}
	for _, name := range slices.Sorted(maps.Keys(raw)) {
		keys := map[string]*yaml.Node{}
		for k, n := range raw[name] {
			keys[k] = &n
		}
		svc := EnvService{Mounts: map[string]string{}, keys: keys}
		if !serviceName.MatchString(name) {
			fail("EN1", "service name %q must be lowercase letters, digits, dashes, and underscores", name)
		}
		for _, k := range refusedKeys {
			if keys[k] != nil {
				fail("EN3", "service %q sets %s, which hikari controls or refuses", name, k)
			}
		}
		if n := keys["image"]; n != nil {
			n.Decode(&svc.Image)
		}
		if n := keys["x-hikari"]; n != nil {
			var h struct {
				Release string            `yaml:"release"`
				Mount   map[string]string `yaml:"mount"`
			}
			dec := yaml.NewDecoder(strings.NewReader(nodeText(n)))
			dec.KnownFields(true)
			if err := dec.Decode(&h); err != nil {
				fail("EN2", "service %q: x-hikari: %v", name, err)
			}
			svc.Release = h.Release
			if h.Mount != nil {
				svc.Mounts = h.Mount
			}
		}
		switch {
		case svc.Release != "" && svc.Image != "":
			fail("EN2", "service %q sets both a release and an image", name)
		case svc.Release == "" && svc.Image == "":
			fail("EN2", "service %q needs an image, or a release in x-hikari", name)
		}
		if svc.Release != "" && !validRelease(svc.Release) {
			fail("EN2", "service %q: release %q must be <system>/<deliverable>", name, svc.Release)
		}
		for _, src := range slices.Sorted(maps.Keys(svc.Mounts)) {
			target := svc.Mounts[src]
			switch rel, ok := strings.CutPrefix(src, ScopePrefix); {
			case ok && (rel == "" || !filepath.IsLocal(filepath.Clean(rel))):
				fail("EN2", "service %q mounts %q, which is not a directory inside your scope", name, src)
			case !ok && !validRelease(src):
				fail("EN2", "service %q mounts %q; a source is <system>/<deliverable> or scope:<path>", name, src)
			}
			if !path.IsAbs(target) {
				fail("EN2", "service %q mounts %s at %q, which is not an absolute path", name, src, target)
			}
		}
		for _, v := range volumes(keys["volumes"]) {
			if v.bind {
				fail("EN3", "service %q bind mounts %s; mount a release or a directory of your scope in x-hikari", name, v.source)
			}
		}
		if hostEnv(keys["environment"]) {
			fail("CR1", "service %q passes a variable of the host; give it a value", name)
		}
		env.Services[name] = svc
	}
	return env, errors.Join(errs...)
}

func validRelease(r string) bool {
	sys, del, ok := strings.Cut(r, "/")
	return ok && sys != "" && del != "" && !strings.Contains(del, "/")
}

type volume struct {
	source string
	bind   bool
}

// volumes reads a service's volumes, in their short or long form.
func volumes(n *yaml.Node) []volume {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []volume
	for _, item := range n.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			src, _, _ := strings.Cut(item.Value, ":")
			bind := strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") || strings.HasPrefix(src, "~")
			out = append(out, volume{source: src, bind: bind})
		case yaml.MappingNode:
			var v struct {
				Type   string `yaml:"type"`
				Source string `yaml:"source"`
			}
			item.Decode(&v)
			out = append(out, volume{source: v.Source, bind: v.Type == "bind"})
		}
	}
	return out
}

// hostEnv reports whether an environment key passes a variable of the host:
// a list entry without =, or a map entry without a value.
func hostEnv(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if !strings.Contains(item.Value, "=") {
				return true
			}
		}
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			if v := n.Content[i]; v.Tag == "!!null" {
				return true
			}
		}
	}
	return false
}

func nodeText(n *yaml.Node) string {
	data, _ := yaml.Marshal(n)
	return string(data)
}

// Names returns the services, sorted.
func (e Environment) Names() []string { return slices.Sorted(maps.Keys(e.Services)) }

// PlanFindings checks the environment against the plan (rule EN4). images
// says which releases are image releases.
func (e Environment) PlanFindings(p Plan, images map[string]bool) []string {
	var out []string
	for _, name := range e.Names() {
		if _, ok := p.Components[name]; !ok {
			out = append(out, fmt.Sprintf("the service %q of %s is no component of the plan", name, EnvironmentFile))
		}
		if r := e.Services[name].Release; r != "" && !images[r] {
			out = append(out, fmt.Sprintf("the service %q runs %s, which is no image release; mount its product into an image instead", name, r))
		}
	}
	for _, name := range sorted(p.Components) {
		c := p.Components[name]
		service := c.Image != "" || c.Simulates != "" || (c.Release != "" && images[c.Release])
		if _, ok := e.Services[name]; service && !ok {
			out = append(out, fmt.Sprintf("the component %q runs, so %s needs a service of that name", name, EnvironmentFile))
		}
	}
	return out
}

// Replicas returns the replicas of each service for a scenario (rule EN5):
// an actor that runs a service sets its count, times the scenario's count
// of that actor; every other service runs once.
func (e Environment) Replicas(p Plan, scenario string) (map[string]int, error) {
	sc, ok := p.Scenarios[scenario]
	if !ok {
		return nil, fmt.Errorf("the plan has no scenario %q; it has %s", scenario, strings.Join(sorted(p.Scenarios), ", "))
	}
	out := map[string]int{}
	for _, name := range e.Names() {
		out[name] = 1
	}
	for actor, n := range sc.Actors {
		a := p.Actors[actor]
		if _, ok := e.Services[a.Runs]; ok {
			out[a.Runs] = a.Count * n
		}
	}
	return out, nil
}

// OverrideInput is what the override of an environment needs from hikari.
type OverrideInput struct {
	// Network is the name of the environment's network.
	Network string
	// Labels go on every container and the network.
	Labels map[string]string
	// Images maps each image release that a service runs to its tag.
	Images map[string]string
	// Products maps each release that a service mounts to its product's
	// directory on the host.
	Products map[string]string
	// Scope is the tester's scope on the host.
	Scope string
	// CPUs and Memory limit each service, when set (rule ST7).
	CPUs   string
	Memory string
}

// Override returns the compose override of an environment (rule ST2):
// the network, the mounts, the images of image releases, the labels, and
// the limits.
func (e Environment) Override(in OverrideInput) ([]byte, error) {
	type mount struct {
		Type     string `yaml:"type"`
		Source   string `yaml:"source"`
		Target   string `yaml:"target"`
		ReadOnly bool   `yaml:"read_only"`
	}
	type limits struct {
		CPUs   string `yaml:"cpus,omitempty"`
		Memory string `yaml:"memory,omitempty"`
	}
	type deploy struct {
		Resources struct {
			Limits limits `yaml:"limits"`
		} `yaml:"resources"`
	}
	type service struct {
		Image    string            `yaml:"image,omitempty"`
		Networks []string          `yaml:"networks"`
		Labels   map[string]string `yaml:"labels"`
		Volumes  []mount           `yaml:"volumes,omitempty"`
		Deploy   *deploy           `yaml:"deploy,omitempty"`
	}
	type network struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	}
	out := struct {
		Services map[string]service `yaml:"services"`
		Networks map[string]network `yaml:"networks"`
	}{Services: map[string]service{}, Networks: map[string]network{"e2e": {Name: in.Network, Labels: in.Labels}}}
	for _, name := range e.Names() {
		svc := e.Services[name]
		s := service{Networks: []string{"e2e"}, Labels: in.Labels}
		if svc.Release != "" {
			tag, ok := in.Images[svc.Release]
			if !ok {
				return nil, fmt.Errorf("the service %q runs %s, which has no image", name, svc.Release)
			}
			s.Image = tag
		}
		for _, src := range slices.Sorted(maps.Keys(svc.Mounts)) {
			host, ok := in.Products[src]
			if rel, scope := strings.CutPrefix(src, ScopePrefix); scope {
				host, ok = filepath.Join(in.Scope, filepath.FromSlash(rel)), true
			}
			if !ok {
				return nil, fmt.Errorf("the service %q mounts %s, which the lock does not pin", name, src)
			}
			s.Volumes = append(s.Volumes, mount{Type: "bind", Source: escape(host), Target: svc.Mounts[src], ReadOnly: true})
		}
		if in.CPUs != "" || in.Memory != "" {
			d := &deploy{}
			d.Resources.Limits = limits{CPUs: in.CPUs, Memory: in.Memory}
			s.Deploy = d
		}
		out.Services[name] = s
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// escape keeps compose from reading a $ in a path as a variable.
func escape(s string) string { return strings.ReplaceAll(s, "$", "$$") }
