# hikari

Status: in development.

## About

Hikari runs each Claude Code agent in its own Docker container and connects the agents through hubs that you define. A hub is a chat room with one orchestrator and its members. An agent can send messages only to the hubs it is in and to the agents it shares a hub with.

You are a member of every hub, so you can post in any hub and send a direct message to any agent.

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

This command builds the `hikari` binary and the `hikari-node` image. Then it starts the server and one container for each agent. Open the web UI URL that the command prints. The URL contains a secret token, so do not share it. Press Ctrl-C to stop all containers.

To use a different network:

```sh
make up TOPOLOGY=examples/team.yaml
```

## Usage

A topology file names the agents and the hubs. The name `user` always means you.

```yaml
agents:
  lead:
    description: Product lead. Turns requests into tasks and reports the result.
  writer:
    description: Copywriter.
  eng-lead:
    description: Engineering lead.
  dev:
    description: Developer.
    model: haiku

hubs:
  product:
    orchestrator: lead
    members: [writer, eng-lead]
  eng:
    orchestrator: eng-lead
    members: [dev]
```

In this network, `eng-lead` is a member of `#product` and the orchestrator of `#eng`. It is the only path between the two hubs: `dev` cannot reach `lead` or `writer`. Each agent sees the `description` of the other participants in its hubs. The `instructions` field of an agent goes into the system prompt of that agent only.

Every participant of a hub receives every post. A post starts a turn at once only for:

- the orchestrator of the hub,
- each participant that the post mentions as `@name`,
- all members, when the orchestrator posts without mentions.

Other participants read the post at the start of their next turn. A direct message always starts a turn. These rules keep a room of agents from all answering at once.

Inside a container, the agent has four tools: `post_to_hub`, `send_direct_message`, `list_hubs`, and `check_inbox`.

The web UI shows each hub as a chat room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. The server also writes each message as one JSON line to `.hikari/<run-id>.jsonl`.

## Patterns and conventions

The design notes in [docs/design.md](docs/design.md) give the full reasons.

- The server is the only authority on the topology. It checks every message against the hubs. A container cannot change who it is or who it can reach.
- Containers cannot reach each other directly. Each container has its own Docker network. The only path between agents goes through the server.
- One binary does all jobs. `hikari up` runs on the host. `hikari node` and `hikari mcp` run inside the containers.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/node`: the driver that gives messages to Claude Code
- `internal/mcpserver`: the MCP tools for the agent
- `internal/launcher`: starts and removes containers
- `internal/topology`: reads and checks topology files
- `examples/`: `studio.yaml` (one hub) and `team.yaml` (two hubs in a tree)

External:

- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode: https://docs.claude.com/en/docs/claude-code/headless
