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
    product: {orchestrator: orchestrator, members: [writer, eng-orchestrator]}
    eng: {orchestrator: eng-orchestrator, members: [dev]}

x-agent: &agent                # a YAML anchor, so that each service repeats less
  image: hikari-agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  orchestrator:
    <<: *agent
    x-hikari: {description: Orchestrates product planning. Turns requests into tasks and reports the result.}
  writer:
    <<: *agent
    x-hikari: {description: Copywriter.}
  eng-orchestrator:
    <<: *agent
    x-hikari: {description: Orchestrates engineering.}
  dev:
    <<: *agent
    x-hikari: {description: Developer., model: haiku}
```

In this network, `eng-orchestrator` is a member of `#product` and the orchestrator of `#eng`. It is the only path between the two hubs: `dev` cannot reach `orchestrator` or `writer`. Each agent sees the `description` of the other participants in its hubs. The `instructions` field of an agent goes into the system prompt of that agent only.

Every participant of a hub receives every post. A post starts a turn at once only for:

- the orchestrator of the hub,
- each participant that the post mentions as `@name`,
- all members, when the orchestrator posts without mentions.

Other participants read the post at the start of their next turn. A direct message always starts a turn. These rules keep a room of agents from all answering at once.

Compose reads the whole file, including `x-hikari`, so write `$$` for a literal `$` in instructions.

Inside a container, every agent has the tools `post_to_hub`, `send_direct_message`, `list_hubs`, `check_inbox`, and `get_vocabulary`, and it sees its hikari documents under `/hikari`.

### Names and ids

Every agent, hub, and repo has an id, the key that the file writes, and a name. The name is the id unless a `name` field sets it, and the project's name is `x-hikari.name`:

```yaml
x-hikari:
  name: Team Planning
  hubs:
    eng: {name: platform, orchestrator: eng-orchestrator, members: [dev]}

services:
  dev:
    <<: *agent
    x-hikari: {name: builder, description: Developer.}
```

Agents and the UI use names: here you post in `#platform` and mention `@builder`. References in the file use ids, such as `members: [dev]`, so a rename changes one line. Everything outside the file is keyed by id: the store directories, the vocabulary files, the release manifests, the tester's lock, the containers, and the clones. A rename therefore moves nothing; it takes effect at the next start. Names follow the rules of ids, and no name may be another agent's or hub's id.

### Systems

Give an agent a system to make it the owner of a part of a repo:

```yaml
x-hikari:
  repos:
    shop:
      path: ~/code/shop          # relative paths start at the compose file
      branch: hikari/checkout-v2 # agents commit here; you merge it
  hubs:
    shop: {orchestrator: orchestrator, members: [orders]}

services:
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      description: Owns order intake.
      system:
        repo: shop
        scope: services/orders    # the only code orders can see or change
        read: [proto]             # optional read-only paths outside every scope
        test: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml  # optional; seeds the first suite, all
      grants:
        deliverables: [python-library]  # optional; the releases this owner can publish
        releases: [inventory]     # optional; read-only access to another system's releases
```

The `test` command seeds the owner's test map once: when the system has no `tests.yaml` yet, hikari makes the command its first suite, `all`, whose report must be `/hikari/private/reports/all.xml`. After that, the owner keeps its suites in `tests.yaml`, and a change of `test` in the compose file has no effect.

When `hikari up` starts, it checks that each repo has no uncommitted changes, then creates the branch if needed and checks it out. Each owner's container sees only its scope at `/workspace/<scope>`, with no `.git`. The agent cannot commit by itself: hikari commits the changes inside its scope, with the agent as author and you as committer, when the agent calls `commit_changes` and at the end of every turn. You review the branch and merge it.

Four documents describe each system, and the owner is the only author of all four:

- `store/systems/<system>/wiki/<id>.md` is the system wiki: one markdown document for each feature, principle, and rule of the system, with a property header that hikari checks and a body that says why it exists and what it promises. Documents link the documents that work together with `[[id]]` links. A link has no direction and no kind, and there are no groups: the links are the structure. The owner changes the wiki only through `change_wiki`, which applies a set of writes as a whole or not at all.
- `store/systems/<system>/map.yaml` is the public map: a summary, the interfaces the system provides and consumes, written through `set_system_info`, and a list of the features that hikari writes from the wiki, with the health and state of each. It holds no file paths and no commands.
- `<scope>/.hikari/private/layout.yaml` is the code map: where each feature lives in the code.
- `<scope>/.hikari/private/tests.yaml` is the test map: the suites that run tests, the features that each test touches, and what no test checks. Each suite's command writes a JUnit XML report to `/hikari/private/reports/<suite>.xml`, and `record_test_run` hands it to hikari. hikari keeps the last result of each test in `results.yaml` beside it. Results and reports are never committed.

The owner edits the private documents directly. The `hikari-system` skill in its container explains each document, what counts as a feature, and how to link; it comes with three read-only agents, `hikari-feature-survey`, `hikari-feature-review`, and `hikari-test-survey`, that read the code for an owner. Health is computed from the tests that touch each feature: `failing` when one of them failed, `pending` when one has not run since its file or the code it tests changed, `untested` when no test touches the feature, and otherwise `passing`. The owner never reports a count: the test runner writes the results. State is computed too: a feature is `stale` when its code changed since the documents were marked current. On its first start, an owner reads its code and builds the documents. When the code changes, hikari tells the owner which features are stale and which changed paths belong to no feature, and the owner updates them and marks the map current. You can open the store in Obsidian: every link resolves.

An orchestrator reads the public maps of the systems in the hubs it orchestrates, and it keeps a vocabulary for each hub in `store/shared/vocabulary/<hub>.yaml`: the names that every agent uses for domain terms and interfaces. The orchestrator view flags interface names that are not in the vocabulary, and consumed interfaces that no system provides.

The UI shows each system with its public map, health, flags, and releases, and, marked private, its layout and test map. It also shows each hub's vocabulary and the shared area, with each deliverable's rules and holders.

### Wikis

Every system and every hub has a wiki: one markdown document per entry, each with a kind.

| Kind | In | Who writes it | What it holds |
|---|---|---|---|
| `feature` | system | the owner | one thing the system does that someone relies on |
| `principle` | system, hub | the owner, the orchestrator | a lasting choice that guides the work, with its reason |
| `rule` | system, hub | the owner, the orchestrator | a statement that must always hold |
| `story` | hub | the orchestrator | a flow that an actor goes through across systems, and its outcome |
| `plan` | system, hub | you | work you intend, with a `status` |
| `note` | system, hub | you | anything else; a file with no `kind` is a note |

A hub's wiki is `store/hubs/<hub>/wiki/`. Its orchestrator writes it through `change_hub_wiki` and the `hikari-hub` skill. Every participant reads it at `/hikari/public/readonly/hubs/<hub>/wiki/`, and an orchestrator also reads the wikis of its child hubs. Ids are unique across the store, so `[[reserve-stock]]` names one document wherever it is written: a story of a hub links the features of its systems, and a feature can link the rules and principles of its hubs. The links across wikis show how the ecosystem fits together. A rename rewrites every link to the document in every wiki.

Plans and notes are yours. Write them in Obsidian, where the store is one vault, or in the UI. Agents read them and never change them, and the prompts say to act on a plan only when you ask. You can write any document by hand; one of an agent's kinds must keep the rules of its kind. A hand edit that breaks one document affects only that document: hikari keeps its last valid version and shows the problem. You mark a key story, the stories that matter most to you, in the UI. Before each turn, an agent is told which documents of the wikis it reads were changed by someone else.

`get_document` and `search_wiki` read any document of the wikis an agent can read, with the documents that link it. A hub's story or rule shows its support: the health of the features it links.

### E2E tests

A hub can have a tester: an owner whose scope is the E2E scaffold of the hub, and who proves the hub's stories, rules, and principles against the releases of its systems.

```yaml
  vote-e2e:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      description: Proves the stories of #main against the releases of its systems.
      system: {repo: project, scope: e2e/main}
      tests: main            # the tester of #main; it must be a member of the hub
```

The tester reads the releases of every system in the hub. It writes `plan.md` in its scope: the components of the test system (pinned releases, images, fixtures, simulators, and what each stands in for), the actors that drive it, the scenarios, and the gaps it cannot test. It pins the release set it tests with `pin_releases`, which writes `releases.lock` with each release's digest. Its `tests.yaml` links tests to the stories, rules, and principles of the hub, and each of them gets health from those tests, as a feature does from unit tests: a new pin makes every result pending until the tests run again, and a failing story names the systems whose features it links. hikari tells the tester when a release newer than its pin appears. The `hikari-tester` skill teaches it the plan and the procedures.

A hub with a tester gets a Tests tab: each story with its E2E health and its support, the gaps, the plan, and the release set. A gap counts only when you approve it there; a gap whose text changes needs a new approval. The tester and the hub's orchestrator read the same view with `get_test_plan`. The tester's tests reach libraries, binaries, and data where its container mounts the releases. Services run beside them in an environment.

Some stories can only be checked on real hardware. The tester writes such a check as a bench procedure in its plan: what to set up, what to do, and what proves a pass. You run it and record pass or fail on the Tests tab, with a note. A result counts for the stories it touches, like an E2E result, until the release set or the procedure changes.

#### Environments

A deliverable can make a container image: its `deliverable.yaml` names the Dockerfile in the owner's scope, and hikari builds the image when the owner publishes, from the commit that the map describes. The release records the image's tag and id; the image stays in your Docker.

```yaml
image:
  dockerfile: Dockerfile    # relative to the owner's scope
  platform: linux/arm64     # optional; the host's platform by default
```

The tester describes its test system's services in `environment.yaml`, a compose file at the root of its scope. A service runs an image release, or any image with a pinned release's product, or a directory of the tester's scope, mounted read-only:

```yaml
services:
  cloud:
    x-hikari: {release: cloud/http-service}
    healthcheck: {test: [CMD, python, -c, "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"]}
  edge:
    image: debian:bookworm-slim
    x-hikari:
      mount: {edge/linux-tarball: /opt/edge, "scope:fixtures/readings": /data}
    command: [/opt/edge/bin/edge, --cloud, http://cloud:8000, --replay, /data]
  mail:
    image: axllent/mailpit:v1.21      # stands in for the mail service
```

`start_environment(scenario)` starts the services for a scenario of the plan, whose actors set the replicas, waits until they are healthy, and connects the tester's container to their network, so the tests reach each service by name. Each service's log is in the tester's `/hikari/private/environment/logs/`. `stop_environment` removes everything; hikari also stops an environment when its scenario's minutes end and when hikari stops. hikari refuses an environment that sets networks, ports, a bind mount, privileges, or a variable of the host, and one that does not match the plan. One environment runs per hub. The Tests tab shows it, with a Stop button. `x-hikari.e2e: {cpus: "2", memory: 4g}` on the tester limits each of its services.

`make example-sensors` makes a project shaped like an IoT monitoring product in `.hikari/demo/sensors`: an edge binary, a cloud service released as an image, a web console, a thresholds data release, and a tester. Run it with `make up COMPOSE=.hikari/demo/sensors/compose.yaml`, after you copy `.env` into it.

The Map view draws the topology as a force graph: each hub, each system, the repository its code is in, and the deliverables it holds, with dashed edges where one system reads another's releases or uses its interfaces. Lenses color the systems: Activity (cyan while the owner works, muted cyan while it waits), Tests (red for a failing feature, yellow for a pending one, green when they pass), and Git (magenta for uncommitted changes). Click a system or a hub and the camera moves into it, and it opens into its wiki map: one node per document, shaped by kind, with smaller nodes for the documents of other wikis that it links or that link it. The wiki lenses are Kind, Tests (a feature's health, a story's support), and Age. Click a document to read it beside the graph, with the documents that link it, and to edit it; the New button writes a document. Each hub page has a Wiki tab that lists its documents, key stories first. Back returns to the topology.

### Each agent's container

Compose owns each agent's environment: its image or Dockerfile, its variables, env files, and secrets, its volumes, its ports, and anything else compose offers. hikari owns the boundaries. It writes an override file for each run that gives every agent its mounts, its token, its own network, and the hikari binary, and runs `docker compose` with both files.

An agent's image must meet one contract: `claude` is on its `PATH`, and its user is not root. The `hikari-agent` image meets it, so a Dockerfile can start `FROM hikari-agent` and add a toolchain. Any other image works when it installs Claude Code and sets a user. `hikari up` checks each image before it starts any container.

- **Dependencies.** A named volume inside the agent's scope keeps Linux builds out of your repo, for example `volumes: [modules:/workspace/web/node_modules]`. hikari makes the volume writable for the agent's user, and it stays between runs.
- **Secrets.** Give each agent only its own variables and env files. An agent can read every variable that its container has.
- **Sidecars.** A service with `x-hikari: {serves: orders}` runs beside orders, for example a database for its tests. It joins only orders' network, so only orders can reach it, by its service name.
- **Extra mounts.** An agent can mount other host paths, such as a dataset, but not a path in a topology repo, the store, or the Docker socket. In a project repository, only the store, `.git`, and the scopes of its systems are refused.

hikari sets an agent's `entrypoint`, `command`, `working_dir`, and networks, and it refuses a file that sets them or that gives a service host-level access, such as `privileged` or `network_mode`. Every service needs `x-hikari`, so no container runs outside the rules.

### The store and releases

The store is one directory, `store/` next to the compose file unless `x-hikari` sets `store:`. It holds every hikari document that is not code, so you can see your whole ecosystem in one place:

```
store/
  hubs/<hub>/wiki/<id>.md    the hub wiki: stories, rules, principles, and your plans and notes
  shared/                    every agent reads this
    vocabulary/<hub>.yaml    the names each hub agreed on
    policies/<name>.md       your rules for every agent; they go into every system prompt
    skills/<name>/SKILL.md   your Claude Code skills for every agent
    agents/<name>.md         your Claude Code subagent definitions
    deliverables/<name>/     what a release must contain and how to make it
  systems/<system>/
    map.yaml                 the public map; hikari writes it from the wiki
    wiki/<id>.md             the system wiki: features, principles, rules, and your plans and notes
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
/hikari/public/export/wiki/                      its system wiki (read-only; changed through a tool)
/hikari/public/readonly/hubs/<hub>/wiki/         the wiki of each hub it is in
/hikari/public/export/staging/                   its next release (read-write)
/hikari/public/export/releases/                  its published releases (read-only)
/hikari/public/readonly/shared/                  the shared area
/hikari/public/readonly/systems/<s>/releases/    releases it can read
```

### Project repositories

A project can be a git repository of its own, which holds the topology, its Dockerfiles, and the store. Make one with:

```sh
hikari init ~/code/myproject
```

This writes a starter `compose.yaml`, a `.gitignore`, an `.env.example`, and the store, and makes the first commit. Copy `.env.example` to `.env`, set a credential, and run `hikari up ~/code/myproject/compose.yaml`.

The key `x-hikari.project: {branch: hikari}` makes the compose file's directory a project repository. Then:

- The store is versioned: the maps, the wikis, the vocabularies, and your shared area. Releases and staging areas are not, and `store/.gitignore` keeps them out.
- hikari commits each change that an agent makes through a tool, with the agent as author and you as committer, on the project branch: `orders: change features: create create-order`, or `orchestrator: define order in the #shop vocabulary`. A test run changes the health in the map but makes no commit; the health goes into the system's next commit. At stop, hikari commits what each agent's paths still hold.
- hikari never commits your own files in `store/shared`: policies, skills, agents, and deliverables. Commit them yourself. Your edits in the wikis are committed with your git identity: before an agent changes the same wiki, when you make them in the UI, at stop, and at start when the project branch is checked out and only the wikis changed.
- The project repository follows the rules of every repo: it must be clean when hikari starts, and agents commit only to the project branch, which you merge. When the checkout of the branch changes the compose file, hikari reads it again.
- A system's code can live in the project repository too: `system: {repo: project, scope: app}`. The scope cannot overlap the store.
- The project repository must ignore `.env` and `/.hikari/`. Ignore `/.hikari/` at the top only: a `.hikari/` pattern also hides the private documents in each scope.

#### Linked repositories and teammates

A repo can be linked by its remote URL instead of a path. hikari keeps its clone in `repos/<name>` in the project repository, which git ignores:

```yaml
x-hikari:
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  repos:
    p3-cloud: {url: https://github.com/you/p3-cloud.git}   # its branch is the project branch
    tools: {path: ~/code/tools, branch: hikari}             # a path repo, on this machine only
```

Three commands work on the whole project:

- `hikari sync compose.yaml` clones each missing linked repo. Where the remote has your branch and you do not, it makes your branch from the remote's. It reports how each branch compares with the remote, and each map whose commit is missing because a teammate did not push it. It never merges, rebases, resets, or checks out.
- `hikari check compose.yaml` runs every check of `hikari up`, and starts nothing. `hikari up` refuses a missing clone, and names `hikari sync`.
- `hikari push compose.yaml` pushes your branch of each linked repo, then of the project repository. It never forces. When a push is refused, it does not push the project repository, whose maps describe the linked repos' commits.

`HIKARI_BRANCH` in `.env` gives each person their own branch, so that two people's agents never commit to one branch. A teammate starts like this:

1. Run `git clone <project-url> myproject`.
2. Run `hikari sync -from hikari-alex myproject/compose.yaml`. This makes your branch in each repo from `hikari-alex`, a teammate's branch.
3. Copy `.env.example` to `.env`. Set a credential, and set `HIKARI_BRANCH` to your own branch, for example `hikari-bob`.
4. Run `hikari check myproject/compose.yaml`, then `hikari up myproject/compose.yaml`.

A fresh clone has no test results and no releases, because git does not version them. The maps keep the health that the teammate's machine recorded. On its first turn, each owner runs its suites again, and publishes again each release that a tester pins. Then it tells the tester, which pins the releases again.

The web UI shows each hub as a chat room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. Under each hub post, it shows which agents the post woke. While you write, the composer shows which agents your draft will wake.

Each turn of an agent appears in the timeline as a card, after the message that woke it. While the agent works, the card shows its steps live: what it reads, edits, runs, and writes, each with its duration. Click a tool call to see its input and output. When the turn ends, the card folds into one line, such as `26s · 14 tool calls · 3 files changed · $0.18`, which opens again on click.

The text an agent writes at the end of a turn reaches no one; its messages are the only way to reach you or another agent. The card shows that closing note, marked when the agent sent no message during the turn. A note that asks something is highlighted, because the agent may be waiting for your answer: reply to it by message.

#### Bring existing repositories in

For a step-by-step guide, see [docs/guides/onboarding.md](docs/guides/onboarding.md).

`hikari init` also writes host skills into the project's `.claude/skills/`: skills for your own Claude Code session, not for an agent's container. `hikari sync` writes them again, so they always match your hikari binary. git ignores them.

`containerize-repo` wraps your repositories into the project, one system for each repository, with no change to their code. Open Claude Code in the project and run `/containerize-repo`, then give it the URL of each repository. The skill:

1. links each repository and clones it with `hikari sync`;
2. surveys each repository with a subagent: its toolchain, its tests, and the suites that cannot run in a container, such as browser tests;
3. proposes ignore lines for build products that a repository does not ignore, and adds them only with your approval;
4. writes an agent Dockerfile, a test command, and instructions for each system, and an orchestrator and a hub, then runs `hikari check`;
5. asks you to start `hikari up`, and waits until each owner has mapped its repository and recorded its tests;
6. writes the baseline, every test with its outcome, to `onboarding/baseline.md`.

It keeps its progress in `onboarding/state.md`, so a new session continues where the last one stopped.

`hikari status compose.yaml` shows what the files say of each system: the commit that its map describes, its features by health, and the last run of each suite. With `-tests`, it lists each test with its outcome. It reads files only, so it works while hikari runs and when it does not.

`hikify` starts where containerize ends, and refactors the project into isolated systems with you. Run `/hikify` in the same project. It works in stages, and keeps each one in `onboarding/`:

1. **Analysis.** A subagent reads each repository with its owner's map and the baseline. The session then writes a synthesis of the duplicated code, the boundaries it proposes, and the candidate E2E tests, and asks you its questions. A difference in behavior between two copies of code is always your decision.
2. **Target topology.** It drafts `compose.target.yaml`, explains it in `onboarding/topology.md` with a diagram, and states the reason for each boundary. `hikari check -draft compose.target.yaml` checks a target whose new repositories and scopes do not exist yet. With you, it drafts the stories, rules, and principles of each hub, and you pick the key stories.
3. **Change plan.** It writes plan documents for each hub and system, and a ledger that says where each baseline test goes.
4. **Review.** Fresh reviewer subagents look for what would break, in rounds, until a round finds only minor problems. After four rounds with serious findings, the session stops and asks you.
5. **Execution.** After you approve, the session makes the new repositories, moves code with its history, and installs the drafts and the target topology. You create each new remote. Then you start hikari, and ask the orchestrator to carry out the plan: it sends each step to its owner when the releases it needs exist. The session follows the run from files.
6. **Verification.** The session checks that every baseline test is kept, moved to the tester, or dropped with your approval, and that the key stories pass. Then it clones the project into a temporary folder, syncs, checks, and starts it, as a teammate would.

To try it, run `make example-onboard`. It writes two small repositories as local remotes, and an empty project in `.hikari/demo/onboard/project`.

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
- The owner is the only author of its documents. The server applies each agent's changes to its wiki and checks them, so the wiki and the map never hold an invalid document.
- Containers cannot reach each other directly. Each container has its own Docker network. The only path between agents goes through the server.
- Compose owns the environment; hikari owns the boundaries. You describe each container in compose, and hikari computes every mount, network, and token that keeps agents apart.
- One binary does all jobs. `hikari up` runs on the host. hikari mounts its Linux build into each container, where it runs as `hikari node` and `hikari mcp`.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/systems`: repos, system maps, vocabularies, and who may read and change them
- `internal/sysmap`: the map and the private documents, their rules, health, and feature state
- `internal/wiki`: the documents of a wiki, their kinds, links, and the change that writes them
- `internal/testmap`: the test map, JUnit reports, recorded runs, and the health of each feature
- `web/src/components/graph-engine.ts`: the canvas of the Map view: layout, drawing, input, and transitions
- `internal/vocab`: the hub vocabulary format
- `internal/store`: the store layout, deliverables, and releases
- `internal/gitrepo`: branch preparation, scoped commits, and the remote operations of sync and push
- `internal/linked`: the checks of a start, and `hikari sync` and `hikari push` across the project's repos
- `internal/e2e`, `internal/environments`, `internal/images`: the tester's plan and lock, its environments, and image releases
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
