// Package modules lists hikari's modules (decision 2 of the task pipeline
// spec), so that the server, the MCP server, the driver, the launcher, and
// the commands reach each one through the module interface.
package modules

import (
	"slices"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/modules/tasks"
	"github.com/nathanaday/hikari/internal/topology"
)

// All are hikari's modules, in the order of topology.KnownModules.
var All = []module.Module{tasks.Module{}}

// Named returns the modules of names, in the order of All.
func Named(names []string) []module.Module {
	var out []module.Module
	for _, m := range All {
		if slices.Contains(names, m.Name()) {
			out = append(out, m)
		}
	}
	return out
}

// Of returns the modules that t turns on.
func Of(t *topology.Topology) []module.Module { return Named(t.Modules) }

// Pack is the pack of built-in skills of an agent of role, with the skills
// of the modules names (rule MO4 of the task pipeline spec).
func Pack(role string, names []string) []string {
	pack := slices.Clone(api.Packs[role])
	for _, m := range Named(names) {
		for _, s := range m.Skills(role) {
			pack = append(pack, s.Name)
		}
	}
	return pack
}

// Packs are the packs of every role, for the modules names.
func Packs(names []string) map[string][]string {
	out := map[string][]string{}
	for role := range api.Packs {
		out[role] = Pack(role, names)
	}
	return out
}

// BuiltinSkills are the names of every built-in skill, the modules' included.
func BuiltinSkills() []string {
	out := slices.Clone(api.BuiltinSkills)
	for _, m := range All {
		for _, role := range []string{api.RolePartition, api.RoleOrchestration} {
			for _, s := range m.Skills(role) {
				if !slices.Contains(out, s.Name) {
					out = append(out, s.Name)
				}
			}
		}
	}
	return out
}

// UserPath reports whether a path of the vault holds your documents of a
// module that t turns on.
func UserPath(t *topology.Topology, rel string) bool {
	return slices.ContainsFunc(Of(t), func(m module.Module) bool { return m.UserPath(rel) })
}

// Command returns the CLI command name of any module, enabled or not.
func Command(name string) (module.Command, bool) {
	for _, m := range All {
		if c, ok := m.Commands()[name]; ok {
			return c, true
		}
	}
	return nil, false
}
