# hikari

Status: in development.

## About

An agent that can reach everything is easy to distract. An agent that holds
every message in its context loses focus. hikari runs each Claude Code agent
in its own Docker container and connects the agents in a network that you
define. An agent can send messages only to its neighbors. To reach a node
farther away, an agent must ask a neighbor to pass the message on.

You see every message in one log, as in a group chat. You are also a node in
the network, so you can send messages to the agents that you connect to.

## Quickstart

### Prerequisites

- Go 1.25 or later
- Docker (Docker Desktop on macOS)
- A Claude subscription or an Anthropic API key

### Setup

1. Make a long-lived token for the containers:

   ```sh
   claude setup-token
   ```

2. Write the token to `.env` in the repository root. Git ignores this file.

   ```sh
   CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-...
   ```

   You can use `ANTHROPIC_API_KEY=...` instead.

### Start

```sh
make up
```

This command builds the `hikari` binary and the `hikari-node` image. Then it
starts the hub and one container for each agent. Open the web UI URL that the
command prints. The URL contains a secret token, so do not share it. Press
Ctrl-C to stop all containers.

To use a different network:

```sh
make up TOPOLOGY=path/to/network.yaml
```

## Usage

A topology file names the agents and the connections between them. The name
`user` always means you.

```yaml
nodes:
  alice:
    description: Coordinator. Takes requests from the user and delegates work to bob.
    instructions: |
      When the user asks for something, decide what part bob should do and ask bob.
      When bob replies, combine the result and report to the user.
  bob:
    description: Specialist. Does focused tasks that alice gives.
    model: haiku

edges:
  - [user, alice]
  - [alice, bob]
```

In this network you can talk only to alice. Bob can talk only to alice. Each
node sees the `description` of its neighbors. The `instructions` text goes
into the system prompt of that node only.

Inside a container, the agent has three tools: `send_message`,
`list_neighbors`, and `check_inbox`. A message to an agent starts a new turn
in the same Claude Code session. The agent replies with `send_message`.

The hub writes each message as one JSON line to `.hikari/<run-id>.jsonl`.

## Patterns and conventions

The design notes in [docs/design.md](docs/design.md) give the full reasons.

- The hub is the only authority on the topology. It checks every message
  against the edges. A container cannot change who it is or who it can reach.
- Containers cannot reach each other directly. Each container has its own
  Docker network. The only path between agents goes through the hub.
- One binary does all jobs. `hikari up` runs on the host. `hikari node` and
  `hikari mcp` run inside the containers.

## Layout

- `cmd/hikari` — the command-line entry point
- `internal/hub` — message routing, the log, the HTTP API, and the web UI
- `internal/node` — the driver that gives messages to Claude Code
- `internal/mcpserver` — the MCP tools for the agent
- `internal/launcher` — starts and removes containers
- `internal/topology` — reads and checks topology files

External:

- MCP Go SDK — https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode — https://docs.claude.com/en/docs/claude-code/headless
