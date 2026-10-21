# hikari

Status: in development.

## About

Hikari runs each Claude Code agent in its own Docker container and connects the agents through hubs that you define. A hub is a chat room with one orchestrator and its members. An agent can send messages only to the hubs it is in and to the agents it shares a hub with.

An agent can own a system: a directory of a git repo on your machine, mounted into its container. The owner is the only agent that can read or change that code, and it keeps a map of what the system does. Orchestrators never see code. They plan from the maps and ask the owners for changes.

You are a member of every hub, so you can post in any hub and send a direct message to any agent.

## Quickstart

### Prerequisites

- Go 1.25 or later
- Node.js 22.12 or later, to build the web UI
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

This command builds the web UI, the `hikari` binary, and the `hikari-node` image. Then it starts the server and one container for each agent. Open the web UI URL that the command prints. The URL contains a secret token, so do not share it. Press Ctrl-C to stop all containers.

To use a different network:

```sh
make up TOPOLOGY=examples/team.yaml
```

To try systems on a small demo repo:

```sh
make demo-repo
make up TOPOLOGY=examples/systems.yaml
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

Inside a container, every agent has the tools `post_to_hub`, `send_direct_message`, `list_hubs`, `check_inbox`, and `get_vocabulary`.

### Systems

Give an agent a system to make it the owner of a part of a repo:

```yaml
repos:
  shop:
    path: ~/code/shop          # relative paths start at the topology file
    branch: hikari/checkout-v2 # agents commit here; you merge it

agents:
  lead:
    description: Product lead. Plans changes from the system maps.
  orders:
    description: Owns order intake.
    system:
      repo: shop
      scope: services/orders    # the only code orders can see or change
      read: [proto]             # optional read-only paths outside every scope
      test: python3 -m unittest discover -s tests
      container_only: [.venv]   # optional; kept in a Docker volume, not in your repo
    packages: [python3]         # Debian packages for the agent's image
    setup: |                    # optional; runs as root when the image is built
      pip install --break-system-packages tox
    path: [/opt/tools/bin]      # optional; added to PATH

hubs:
  shop: {orchestrator: lead, members: [orders]}
```

When `hikari up` starts, it checks that each repo has no uncommitted changes, then creates the branch if needed and checks it out. Each owner's container sees only its scope at `/workspace/<scope>`, with no `.git`. The agent cannot commit by itself: hikari commits the changes inside its scope, with the agent as author and you as committer, when the agent calls `commit_changes` and at the end of every turn. You review the branch and merge it.

Three documents describe each system, and the owner writes all three:

- `<scope>/.hikari/map.yaml` is the public surface of the system: a summary, the interfaces it provides and consumes, and its features with their test health. It holds no file paths and no commands. The owner changes it only through map tools, and the server checks each change.
- `<scope>/.hikari/private/layout.yaml` says where each feature lives in the code.
- `<scope>/.hikari/private/test.yaml` is the test plan: the suites that run tests, and for each feature what its tests cover, which files hold them, and what they leave out.

The owner edits the private documents directly. A `hikari-system` skill in its container explains their formats. Health is computed: `passing`, `failing`, `untested`, or `stale` when the feature's code or test files changed after its last test run. On its first start, an owner reads its code and builds the documents. When the code changes, the owner updates them and marks the map current.

An orchestrator reads the public maps of the systems in the hubs it orchestrates, and it keeps a vocabulary for each hub in `vocabulary/<hub>.yaml` next to the topology file: the names that every agent uses for domain terms and interfaces. The orchestrator view flags interface names that are not in the vocabulary, and consumed interfaces that no system provides.

The UI shows each system with its public map, health, and flags, and, marked private, its layout and test plan. It also shows each hub's vocabulary.

The web UI shows each hub as a chat room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. Under each hub post, it shows which agents the post woke. While you write, the composer shows which agents your draft will wake. The server also writes each message as one JSON line to `.hikari/<run-id>.jsonl`.

### Work on the UI

The UI is a Vite, React, and shadcn/ui app in `web/`. The `hikari` binary embeds the built UI. To change the UI with hot reload, start a network with `make up`, then run:

```sh
make dev-ui
```

Open http://localhost:5173/?token=... with the token that `make up` printed. The dev server sends API requests to the running hikari server. `make test` runs the Go tests and the UI tests.

## Patterns and conventions

The design notes in [docs/design.md](docs/design.md) give the full reasons.

- The server is the only authority on the topology. It checks every message against the hubs. A container cannot change who it is or who it can reach.
- Only the server writes to git. Agents never get `.git`, so they cannot commit outside their scope, rewrite history, or switch branches.
- The map is a public surface. Paths, commands, and test plans stay in the private documents, which only the owner and you read.
- The owner is the only author of its documents. The server applies the owner's map changes and checks them, so the map file never holds an invalid map.
- Containers cannot reach each other directly. Each container has its own Docker network. The only path between agents goes through the server.
- One binary does all jobs. `hikari up` runs on the host. `hikari node` and `hikari mcp` run inside the containers.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/systems`: repos, system maps, vocabularies, and who may read and change them
- `internal/sysmap`: the map and the private documents, their rules, and health
- `internal/vocab`: the hub vocabulary format
- `internal/gitrepo`: branch preparation and scoped commits
- `internal/node`: the driver that gives messages to Claude Code
- `internal/mcpserver`: the MCP tools for the agent
- `internal/launcher`: starts and removes containers
- `internal/topology`: reads and checks topology files
- `web/`: the browser UI
- `examples/`: `studio.yaml` (one hub), `team.yaml` (two hubs in a tree), and `systems.yaml` (two systems in the demo repo)

External:

- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode: https://docs.claude.com/en/docs/claude-code/headless
