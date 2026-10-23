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
        test: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml  # optional; must write a JUnit report
      grants:
        deliverables: [python-library]  # optional; the releases this owner can publish
        releases: [inventory]     # optional; read-only access to another system's releases
```

When `hikari up` starts, it checks that each repo has no uncommitted changes, then creates the branch if needed and checks it out. Each owner's container sees only its scope at `/workspace/<scope>`, with no `.git`. The agent cannot commit by itself: hikari commits the changes inside its scope, with the agent as author and you as committer, when the agent calls `commit_changes` and at the end of every turn. You review the branch and merge it.

Four documents describe each system, and the owner is the only author of all four:

- `store/systems/<system>/features/<id>.md` is the feature wiki: one markdown document for each feature, with a property header that hikari checks and a body that says why the feature exists and what it promises. Documents link the features that work together with `[[id]]` links. A link has no direction and no kind, and there are no feature groups: the links are the structure. The owner changes the wiki only through `change_features`, which applies a set of writes as a whole or not at all.
- `store/systems/<system>/map.yaml` is the public map: a summary, the interfaces the system provides and consumes, written through `set_system_info`, and a list of the features that hikari writes from the wiki, with the health and state of each. It holds no file paths and no commands.
- `<scope>/.hikari/private/layout.yaml` is the code map: where each feature lives in the code.
- `<scope>/.hikari/private/tests.yaml` is the test map: the suites that run tests, the features that each test touches, and what no test checks. Each suite's command writes a JUnit XML report to `/hikari/private/reports/<suite>.xml`, and `record_test_run` hands it to hikari. hikari keeps the last result of each test in `results.yaml` beside it. Results and reports are never committed.

The owner edits the private documents directly. The `hikari-system` skill in its container explains each document, what counts as a feature, and how to link; it comes with three read-only agents, `hikari-feature-survey`, `hikari-feature-review`, and `hikari-test-survey`, that read the code for an owner. Health is computed from the tests that touch each feature: `failing` when one of them failed, `pending` when one has not run since its file or the code it tests changed, `untested` when no test touches the feature, and otherwise `passing`. The owner never reports a count: the test runner writes the results. State is computed too: a feature is `stale` when its code changed since the documents were marked current. On its first start, an owner reads its code and builds the documents. When the code changes, hikari tells the owner which features are stale and which changed paths belong to no feature, and the owner updates them and marks the map current. You can open the store in Obsidian: every link resolves.

An orchestrator reads the public maps of the systems in the hubs it orchestrates, and it keeps a vocabulary for each hub in `store/shared/vocabulary/<hub>.yaml`: the names that every agent uses for domain terms and interfaces. The orchestrator view flags interface names that are not in the vocabulary, and consumed interfaces that no system provides.

The UI shows each system with its public map, health, flags, and releases, and, marked private, its layout and test map. It also shows each hub's vocabulary and the shared area, with each deliverable's rules and holders.

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
    deliverables/<name>/     what a release must contain and how to make it
  systems/<system>/
    map.yaml                 the public map; hikari writes it from the wiki
    features/<id>.md         the feature wiki, one document per feature
    staging/<deliverable>/   where the owner builds the product of its next release
    releases/<deliverable>/<x.y.z>/   published releases, which never change
    releases/<deliverable>/latest     the newest release of that deliverable
```

A deliverable is a named package that says what a release must contain and how its owner makes it. You write it, in `store/shared/deliverables/<name>/`, and grant it to an owner with `grants.deliverables`. It holds two files:

- `deliverable.yaml`: a description, optional layout rules on the files of the product as glob lists `require`, `allow`, and `forbid`, and optional targets, which promise that the product is also published to `github-release`, `dockerhub`, or `huggingface`.
- `SKILL.md`: a Claude Code skill that tells the owner how to build the product. hikari installs it under the deliverable's name in each holder's container.

An owner can hold several deliverables, so one system can publish, for example, a library and a container image. To publish, the owner loads the deliverable's skill, builds the product in `staging/<deliverable>/`, calls `check_release` to check it against the layout rules, then calls `publish_release` with a version and notes. When the deliverable has targets, the owner publishes there first, with the tools and credentials that compose gives its container, and gives one certificate for each target in the same call; hikari checks each certificate's shape and never uploads anything. hikari then writes the manifest, `release.yaml`, with who published, when, from which commit, with which hikari version, the certificates, and the digest of the product, writes `SHA256SUMS`, moves the product into `releases/<deliverable>/<version>/product/` with a copy of the public map, and announces the release in the owner's hubs. The shop example's `python-library` deliverable shows the format.

The orchestrator reads the maps and releases of the systems in its hubs, and the deliverables in the shared area, so it can check each release against its deliverable. Grant any agent read-only access to another system's releases with `grants.releases`, for example so that it can integrate a library and test against it. Grants take effect when the container starts.

Every container sees its hikari documents at the same paths:

```
/hikari/private/                                 an owner's private documents
/hikari/public/export/map.yaml                   its public map (read-only)
/hikari/public/export/features/                  its feature wiki (read-only; changed through a tool)
/hikari/public/export/staging/                   its next release (read-write)
/hikari/public/export/releases/                  its published releases (read-only)
/hikari/public/readonly/shared/                  the shared area
/hikari/public/readonly/systems/<s>/releases/    releases it can read
```

The web UI shows each hub as a chat room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. Under each hub post, it shows which agents the post woke. While you write, the composer shows which agents your draft will wake.

Each turn of an agent appears in the timeline as a card, after the message that woke it. While the agent works, the card shows its steps live: what it reads, edits, runs, and writes, each with its duration. Click a tool call to see its input and output. When the turn ends, the card folds into one line, such as `26s · 14 tool calls · 3 files changed · $0.18`, which opens again on click.

The text an agent writes at the end of a turn reaches no one; its messages are the only way to reach you or another agent. The card shows that closing note, marked when the agent sent no message during the turn. A note that asks something is highlighted, because the agent may be waiting for your answer: reply to it by message.

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
- The map is a public surface. Paths, commands, and test names stay in the private documents, which only the owner and you read.
- The owner is the only author of its documents. The server applies the owner's changes to the feature wiki and checks them, so the wiki and the map never hold an invalid document.
- Containers cannot reach each other directly. Each container has its own Docker network. The only path between agents goes through the server.
- Compose owns the environment; hikari owns the boundaries. You describe each container in compose, and hikari computes every mount, network, and token that keeps agents apart.
- One binary does all jobs. `hikari up` runs on the host. hikari mounts its Linux build into each container, where it runs as `hikari node` and `hikari mcp`.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/systems`: repos, system maps, vocabularies, and who may read and change them
- `internal/sysmap`: the map and the private documents, their rules, health, and feature state
- `internal/featurewiki`: the feature documents, their links, and the change that writes them
- `internal/testmap`: the test map, JUnit reports, recorded runs, and the health of each feature
- `internal/vocab`: the hub vocabulary format
- `internal/store`: the store layout, deliverables, and releases
- `internal/gitrepo`: branch preparation and scoped commits
- `internal/node`: the driver that gives messages to Claude Code
- `internal/mcpserver`: the MCP tools for the agent
- `internal/launcher`: writes the override file and runs the compose project
- `internal/topology`: reads and checks compose files
- `internal/version`: the version of hikari, which `hikari version`, the terminal banner, and the web UI show; change it there for each release
- `web/`: the browser UI
- `examples/`: `studio` (one hub), `team` (two hubs in a tree), and `shop` (two systems in the demo repo, with a sidecar)

External:

- Docker Compose: https://docs.docker.com/compose/
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode: https://docs.claude.com/en/docs/claude-code/headless
