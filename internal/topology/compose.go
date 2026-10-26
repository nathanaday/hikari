package topology

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
)

// hikari sets these keys of an agent service in the override file, or they
// would cross a boundary (rule C7).
var agentOnlyHikari = []string{"entrypoint", "command", "working_dir"}

var boundaryKeys = []string{"networks", "network_mode", "links", "external_links", "volumes_from",
	"privileged", "cap_add", "devices", "pid", "ipc", "userns_mode"}

// PrepareName returns the name of the service that makes the named volumes
// of agent writable (rule O6).
func PrepareName(agent string) string { return agent + "-prepare" }

// ServiceOf returns the compose service of the agent named agent. Services
// are keyed by id, and agents by name.
func (t *Topology) ServiceOf(agent string) Service { return t.Services[t.Agents[agent].ID] }

// Sidecars returns the sidecars of agent, in sorted order.
func (t *Topology) Sidecars(agent string) []string {
	var out []string
	for _, name := range sortedKeys(t.Services) {
		if t.Services[name].Serves == agent {
			out = append(out, name)
		}
	}
	return out
}

// NamedVolumes returns the named volumes of a service.
func (t *Topology) NamedVolumes(service string) []Volume {
	var out []Volume
	for _, v := range t.Services[service].Volumes {
		if v.Type == "volume" {
			out = append(out, v)
		}
	}
	return out
}

// validateServices checks the compose part of each service (rules C6 to
// C10, C14, and C15).
func (t *Topology) validateServices() []error {
	var errs []error
	fail := func(service, key, format string, args ...any) {
		path := "services." + service
		if key != "" {
			path += "." + key
		}
		errs = append(errs, at(path, "service %q: "+format, append([]any{service}, args...)...))
	}
	guarded := []string{t.Store}
	for _, name := range sortedKeys(t.Repos) {
		if name == ProjectRepo {
			guarded = append(guarded, t.projectGuarded()...)
		} else {
			guarded = append(guarded, t.Repos[name].Path)
		}
	}
	for i, p := range guarded {
		guarded[i] = realPath(p)
	}

	for _, name := range sortedKeys(t.Services) {
		s := t.Services[name]
		_, isAgent := t.Agents[name]
		if !isAgent {
			if _, ok := t.Agents[s.Serves]; !ok {
				fail(name, "x-hikari.serves", "serves %q, which is not an agent", s.Serves)
			}
		}
		if owner, ok := strings.CutSuffix(name, "-prepare"); ok && len(t.NamedVolumes(owner)) > 0 {
			if _, ok := t.Agents[owner]; ok {
				fail(name, "", "the name is taken by the prepare service of agent %q", owner)
			}
		}

		keys := boundaryKeys
		if isAgent {
			keys = append(append([]string{}, agentOnlyHikari...), boundaryKeys...)
		}
		for _, k := range keys {
			if s.set[k] {
				fail(name, k, "sets %s, which hikari controls", k)
			}
		}

		for _, v := range s.Volumes {
			if v.Type != "bind" {
				continue
			}
			src := realPath(v.Source)
			if filepath.Base(src) == "docker.sock" {
				fail(name, "volumes", "mounts the Docker socket %s", v.Source)
			}
			for _, g := range guarded {
				if within(src, g) || within(g, src) {
					fail(name, "volumes", "mounts %s, which overlaps %s; hikari mounts the repos and the store itself", v.Source, g)
				}
			}
		}

		if !isAgent {
			continue
		}
		var scope string
		if sys := t.Agents[name].System; sys != nil {
			scope = api.WorkDir(sys.Scope)
		}
		for _, v := range s.Volumes {
			target := path.Clean(v.Target)
			switch {
			case under(target, api.HikariRoot):
				fail(name, "volumes", "mounts %s; hikari owns %s", v.Target, api.HikariRoot)
			case under(target, api.WorkspaceRoot) && !scopeVolume(v.Type, target, scope):
				fail(name, "volumes", "mounts %s; under %s, only named volumes and tmpfs inside the agent's own scope are allowed", v.Target, api.WorkspaceRoot)
			}
		}
		for k := range s.Environment {
			if strings.HasPrefix(k, "HIKARI_") {
				fail(name, "environment."+k, "sets %s; hikari sets the HIKARI_ variables", k)
			}
		}
	}
	return errs
}

// projectGuarded returns the parts of the project repository that no
// service can bind mount (rule PR7): its .git and the scopes of its
// systems. The store is guarded for every topology.
func (t *Topology) projectGuarded() []string {
	out := []string{filepath.Join(t.Project.Path, ".git")}
	for _, name := range t.Owners() {
		if s := t.Agents[name].System; s.Repo == ProjectRepo {
			out = append(out, filepath.Join(t.Project.Path, s.Scope))
		}
	}
	return out
}

// scopeVolume reports whether a mount of type kind at target is allowed
// under /workspace: a named volume or tmpfs strictly inside scope, and not
// in its .hikari or, for a whole-repo scope, .git.
func scopeVolume(kind, target, scope string) bool {
	if scope == "" || (kind != "volume" && kind != "tmpfs") || target == scope || !under(target, scope) {
		return false
	}
	for _, hidden := range []string{".hikari", ".git"} {
		if under(target, path.Join(scope, hidden)) {
			return false
		}
	}
	return true
}

// under reports whether the slash path p is dir or inside it.
func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// within reports whether the host path p is dir or inside it.
func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// realPath resolves the links in the longest part of p that exists, so that
// two spellings of one host path compare equal.
func realPath(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for dir := p; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		if dir == filepath.Dir(dir) {
			return p
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
	}
}

// Credentials reports whether a service has a Claude credential set.
func (s Service) Credentials() bool {
	for _, k := range []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if v := s.Environment[k]; v != nil && *v != "" {
			return true
		}
	}
	return false
}
