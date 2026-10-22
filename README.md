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

This command builds the web UI, the `hikari` binaries, and the `hikari-agent` image. Then it starts the server and the example's compose project, with one container for each agent. The terminal shows each start-up step, then keeps the web UI address, the state of each agent, and the cost so far pinned at the bottom, with one line for each message above them. Open the web UI address. It contains a secret token, so do not share it. Press Ctrl-C to stop all containers.

Each run keeps its records in `.hikari/runs/<run-id>/`: `hikari.log` with everything the containers and compose print, including each tool call of each agent, `messages.jsonl` with one JSON line for each message, when there are messages, and the run's compose override file.

To use a different network:

```sh
make up COMPOSE=examples/team/compose.yaml
```

To try systems on a small demo repo:

```sh
make demo-repo
make up COMPOSE=examples/shop/compose.yaml
```

## Usage

The topology is a Docker compose file. Compose describes each agent's container, and the `x-hikari` keys, which compose ignores, describe the network: the hubs at the top, and each agent's role in its service. The name `user` always means you.

```yaml
name: team                     # required; it names the containers, networks, and volumes

x-hikari:
  hubs:
    product: {orchestrator: lead, members: [writer, eng-lead]}
    eng: {orchestrator: eng-lead, members: [dev]}

x-agent: &agent                # a YAML anchor, so that each service repeats less
  image: hikari-agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  lead:
    <<: *agent
    x-hikari: {description: Product lead. Turns requests into tasks and reports the result.}
  writer:
    <<: *agent
    x-hikari: {description: Copywriter.}
  eng-lead:
    <<: *agent
    x-hikari: {description: Engineering lead.}
  dev:
    <<: *agent
    x-hikari: {description: Developer., model: haiku}
```

In this network, `eng-lead` is a member of `#product` and the orchestrator of `#eng`. It is the only path between the two hubs: `dev` cannot reach `lead` or `writer`. Each agent sees the `description` of the other participants in its hubs. The `instructions` field of an agent goes into the system prompt of that agent only.

Every participant of a hub receives every post. A post starts a turn at once only for:

- the orchestrator of the hub,
- each participant that the post mentions as `@name`,
- all members, when the orchestrator posts without mentions.

Other participants read the post at the start of their next turn. A direct message always starts a turn. These rules keep a room of agents from all answering at once.

Compose reads the whole file, including `x-hikari`, so write `$$` for a literal `$` in instructions.

Inside a container, every agent has the tools `post_to_hub`, `send_direct_message`, `list_hubs`, `check_inbox`, and `get_vocabulary`, and it sees its hikari documents under `/hikari`.

### Systems

Give an agent a system to make it the owner of a part of a repo:

```yaml
x-hikari:
  repos:
    shop:
      path: ~/code/shop          # relative paths start at the compose file
      branch: hikari/checkout-v2 # agents commit here; you merge it
  hubs:
    shop: {orchestrator: lead, members: [orders]}

services:
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      description: Owns order intake.
      system:
        repo: shop
        scope: services/orders    # the only code orders can see or change
        read: [proto]             # optional read-only paths outside every scope
        test: python3 -m unittest discover -s tests
      grants:
        releases: [inventory]     # optional; read-only access to another system's releases
```

When `hikari up` starts, it checks that each repo has no uncommitted changes, then creates the branch if needed and checks it out. Each owner's container sees only its scope at `/workspace/<scope>`, with no `.git`. The agent cannot commit by itself: hikari commits the changes inside its scope, with the agent as author and you as committer, when the agent calls `commit_changes` and at the end of every turn. You review the branch and merge it.

Three documents describe each system, and the owner writes all three:

- `store/systems/<system>/map.yaml` is the public surface of the system: a summary, the interfaces it provides and consumes, and its features with their test health. It holds no file paths and no commands. The owner changes it only through map tools, and the server checks each change.
- `<scope>/.hikari/private/layout.yaml` says where each feature lives in the code.
- `<scope>/.hikari/private/test.yaml` is the test plan: the suites that run tests, and for each feature what its tests cover, which files hold them, and what they leave out.

The owner edits the private documents directly. A `hikari-system` skill in its container explains their formats. Health is computed: `passing`, `failing`, `untested`, or `stale` when the feature's code or test files changed after its last test run. On its first start, an owner reads its code and builds the documents. When the code changes, the owner updates them and marks the map current.

An orchestrator reads the public maps of the systems in the hubs it orchestrates, and it keeps a vocabulary for each hub in `store/shared/vocabulary/<hub>.yaml`: the names that every agent uses for domain terms and interfaces. The orchestrator view flags interface names that are not in the vocabulary, and consumed interfaces that no system provides.

The UI shows each system with its public map, health, flags, and releases, and, marked private, its layout and test plan. It also shows each hub's vocabulary and the shared area.

### Each agent's container

Compose owns each agent's environment: its image or Dockerfile, its variables, env files, and secrets, its volumes, its ports, and anything else compose offers. hikari owns the boundaries. It writes an override file for each run that gives every agent its mounts, its token, its own network, and the hikari binary, and runs `docker compose` with both files.

An agent's image must meet one contract: `claude` is on its `PATH`, and its user is not root. The `hikari-agent` image meets it, so a Dockerfile can start `FROM hikari-agent` and add a toolchain. Any other image works when it installs Claude Code and sets a user. `hikari up` checks each image before it starts any container.

- **Dependencies.** A named volume inside the agent's scope keeps Linux builds out of your repo, for example `volumes: [modules:/workspace/web/node_modules]`. hikari makes the volume writable for the agent's user, and it stays between runs.
- **Secrets.** Give each agent only its own variables and env files. An agent can read every variable that its container has.
- **Sidecars.** A service with `x-hikari: {serves: orders}` runs beside orders, for example a database for its tests. It joins only orders' network, so only orders can reach it, by its service name.
- **Extra mounts.** An agent can mount other host paths, such as a dataset, but not a path in a topology repo, the store, or the Docker socket.

hikari sets an agent's `entrypoint`, `command`, `working_dir`, and networks, and it refuses a file that sets them or that gives a service host-level access, such as `privileged` or `network_mode`. Every service needs `x-hikari`, so no container runs outside the rules.

### The store and releases

The store is one directory, `store/` next to the compose file unless `x-hikari` sets `store:`. It holds every hikari document that is not code, so you can see your whole ecosystem in one place:

```
store/
  shared/                    every agent reads this
    vocabulary/<hub>.yaml    the names each hub agreed on
    policies/<name>.md       your rules for every agent; they go into every system prompt
    skills/<name>/SKILL.md   your Claude Code skills for every agent
    agents/<name>.md         your Claude Code subagent definitions
  systems/<system>/
    map.yaml                 the public map
    staging/                 where the owner prepares its next release
    releases/<x.y.z>/        published releases, which never change
    releases/latest          the newest release
```

An owner publishes a release in three steps. It builds the deliverable in its staging area, writes the docs with the built-in `release-docs` skill, and calls `publish_release`. hikari checks the release, moves it into `releases/<version>/` with the source commit and a copy of the public map, and announces it in the owner's hubs. A release holds `release.yaml`, which says how to install and run it, the product or a pointer to where it is hosted, and user-facing docs. To change how release docs read, write your own `store/shared/skills/release-docs/SKILL.md`.

The orchestrator reads the maps and releases of the systems in its hubs, so it can check each release. Grant any agent read-only access to another system's releases with `grants.releases`, for example so that it can integrate a library and test against it. Grants take effect when the container starts.

Every container sees its hikari documents at the same paths:

```
/hikari/private/                                 an owner's private documents
/hikari/public/export/map.yaml                   its public map (read-only)
/hikari/public/export/staging/                   its next release (read-write)
/hikari/public/export/releases/                  its published releases (read-only)
/hikari/public/readonly/shared/                  the shared area
/hikari/public/readonly/systems/<s>/releases/    releases it can read
```

The web UI shows each hub as a chat room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. Under each hub post, it shows which agents the post woke. While you write, the composer shows which agents your draft will wake.

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
- Compose owns the environment; hikari owns the boundaries. You describe each container in compose, and hikari computes every mount, network, and token that keeps agents apart.
- One binary does all jobs. `hikari up` runs on the host. hikari mounts its Linux build into each container, where it runs as `hikari node` and `hikari mcp`.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/systems`: repos, system maps, vocabularies, and who may read and change them
- `internal/sysmap`: the map and the private documents, their rules, and health
- `internal/vocab`: the hub vocabulary format
- `internal/store`: the store layout and releases
- `internal/gitrepo`: branch preparation and scoped commits
- `internal/node`: the driver that gives messages to Claude Code
- `internal/mcpserver`: the MCP tools for the agent
- `internal/launcher`: writes the override file and runs the compose project
- `internal/topology`: reads and checks compose files
- `web/`: the browser UI
- `examples/`: `studio` (one hub), `team` (two hubs in a tree), and `shop` (two systems in the demo repo, with a sidecar)

External:

- Docker Compose: https://docs.docker.com/compose/
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode: https://docs.claude.com/en/docs/claude-code/headless
