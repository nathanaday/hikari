// Package tasks is the task pipeline, hikari's first module: stubs that the
// user plants, scopes that the orchestration agent writes from them, queues
// that order the scopes, and the status of each scope and its parts while a
// queue runs (the task pipeline spec).
package tasks

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/nathanaday/hikari/internal/module"
	"github.com/nathanaday/hikari/internal/topology"
)

// Name is the module's name in x-hikari.modules.
const Name = "tasks"

// Dir is the module's directory in the vault, and ContainerDir where the
// orchestration agent reads it (rule TK20).
const (
	Dir          = "tasks"
	ContainerDir = "/hikari/tasks"
)

// Module is the agent half of the task pipeline.
type Module struct{}

func (Module) Name() string { return Name }

func (Module) Mounts(t *topology.Topology, agent string) []module.Mount {
	if agent != t.Orchestration {
		return nil
	}
	return []module.Mount{{Source: filepath.Join(t.Vault, Dir), Target: ContainerDir, ReadOnly: true}}
}

// UserPath reports whether rel is a task or a queue: your documents, which
// hikari commits as yours (rule TK7).
func (Module) UserPath(rel string) bool {
	rest, ok := strings.CutPrefix(rel, Dir+"/")
	return ok && path.Ext(rest) == ".md"
}

func (Module) Open(h module.Host) (module.Instance, error) { return open(h) }
