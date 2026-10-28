// Package module defines a module of hikari: a piece that owns a directory
// of the vault, its tools, a skill for each kind of agent, its routes, its
// commands, and a tab of the UI, and that a project turns on in
// x-hikari.modules (rules MO1 and MO2 of the task pipeline spec). A module
// has two halves: the agent half, which a container runs with only the
// agent's Self, and the host half, which hikari up opens with a Host.
package module

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
)

// Module is one module of hikari.
type Module interface {
	// Name is the module's name in x-hikari.modules.
	Name() string
	// Skills are the module's skills for an agent of role: each name, and
	// a file system whose root is the skill's folder.
	Skills(role string) []Skill
	// Tools adds the module's MCP tools for the agent self.
	Tools(s *mcp.Server, c *api.Client, self api.Self)
	// Mounts are the module's mounts in the container of agent.
	Mounts(t *topology.Topology, agent string) []Mount
	// UserPath reports whether a path of the vault, relative to it, holds
	// your documents, which hikari commits as yours at its start.
	UserPath(rel string) bool
	// Commands are the module's commands of the hikari CLI, by name.
	Commands() map[string]Command
	// Open starts the host half for a run of hikari up.
	Open(h Host) (Instance, error)
}

// Skill is one skill of a module.
type Skill struct {
	Name  string
	Files fs.FS
}

// Mount is a bind mount of a directory of the host.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// Command runs one command of the CLI with its arguments.
type Command func(ctx context.Context, args []string) error

// Instance is the host half of a module, for one run.
type Instance interface {
	// AgentRoutes are served below /api/agent/<name>, with the calling
	// agent as caller; UIRoutes below /api/ui/<name>, with topology.User.
	AgentRoutes() Routes
	UIRoutes() Routes
	// Notices are the lines that the module adds to agent's next turn.
	Notices(agent string) []string
	// View is the module's part of the UI's snapshot.
	View() any
	// Watch follows the module's files until ctx ends.
	Watch(ctx context.Context)
}

// Routes map "METHOD /path" to a handler.
type Routes map[string]Handler

// Handler answers one request of caller. A nil answer is 204.
type Handler func(r *http.Request, caller string) (any, error)

// Host is what a module may ask of the running hikari.
type Host interface {
	Topology() *topology.Topology
	// VaultDir is the vault's directory on the host.
	VaultDir() string
	// Health is the health of the feature id in a partition's feature
	// map, or in the orchestration feature map when partition is empty.
	Health(partition, id string) (string, bool)
	// Commit commits paths of the vault, relative to it, as author: an
	// agent, topology.User, or topology.Hikari.
	Commit(author, message string, paths ...string)
	// Notify sends to a direct message from hikari.
	Notify(to, body string)
	// Changed tells the UI that the module's view changed.
	Changed()
}

// ErrForbidden and ErrNotFound make a route answer 403 and 404.
var (
	ErrForbidden = errors.New("not allowed")
	ErrNotFound  = errors.New("not found")
)
