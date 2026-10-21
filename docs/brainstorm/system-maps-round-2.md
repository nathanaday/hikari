# System maps: round 2

Round 1 settled these points:

- This spec covers mounts and ownership, the system map, the map query tools, and a read-only Systems view. Work orders, change logs, and the validation agent wait for the next spec.
- A system is a repo plus an optional subdirectory scope, with optional read-only paths. Scopes in one repo must not overlap.
- The map file lives inside the system.
- A feature records which tests cover it and their last results, reported by the owner on the honor system. The orchestrator's view leaves out code and test file paths.
- The worker builds the map on first start. The driver tells it what changed since the map was last updated.
- Only orchestrators read other systems' maps. A worker with a blocker asks its orchestrator.
- An agent owns one system or none. Orchestrators own none.
- Each agent declares the packages its tests need.
- Open internet is acceptable for now.

Your answers raise new questions. Each one has my recommendation.

---

## A. Git and commits

**A1. Local git needs no credentials.** Commit, branch, and merge are all local operations. Credentials matter only for `push` and `fetch`. So "git in the container" and "credentials" are separate questions. An agent can commit to a local branch without any credentials, and you push when you are ready.

**A2. Who runs the commit?** I recommend that the server runs git on the host, not the agent in the container:

- The agent calls an MCP tool, `commit_changes(message)`, when it finishes a unit of work.
- The server stages and commits only the files inside that agent's scope: `git add -A -- <scope>`, then `git commit -- <scope>`. A commit can never include another agent's files, even when two agents share a repo.
- At the end of each turn, the server commits any changes that the agent left uncommitted. Nothing is lost if the agent forgets to commit.
- The container gets no `.git` directory, so the agent cannot commit, reset, or switch branches by itself. See C1.

This removes the need for a second agent on the host that applies change logs. Do you agree?

**A3. Commit identity.** I recommend that the agent is the author and you are the committer. For example, the author is `api (hikari) <api@hikari.invalid>`, and the committer comes from your host git config. `git log` then shows which agent wrote each change, and that you accepted it into your repo. Is this what you want?

**A4. Merges.** I recommend that agents never merge. You merge the work branch into `main` yourself. Do you agree?

## B. Branches

**B1. One branch per repo, not per agent.** One working tree can have only one branch checked out. Agents that share a repo must therefore share its branch. I recommend that the topology declares repos once, and each system points at one:

```yaml
repos:
  shop:
    path: ~/code/shop
    branch: hikari/checkout-v2
agents:
  api:
    system: {repo: shop, scope: services/api}
  web:
    system: {repo: shop, scope: apps/web}
```

**B2. What `hikari up` does with the branch.** I recommend:

1. If the repo has uncommitted changes, refuse to start. This keeps your own unfinished work out of agent commits.
2. If the branch does not exist, create it from the current `HEAD`.
3. Check out the branch.
4. On shutdown, leave the branch checked out and print how many commits each agent made.

Do you agree with each step? In particular, should step 4 switch back to your previous branch instead?

## C. Mounts

**C1. What the agent sees.** I recommend that the container sees only its scope and its read-only paths, at the same relative paths as in the repo:

- `/workspace/services/api`, read-write (the scope)
- `/workspace/proto`, read-only (a `read:` path)
- `/workspace/services/api/.hikari/map.yaml`, read-only (changed only through the map tools; see D1)

The rest of the repo and its `.git` directory are not in the container. Without `.git`, `git status` inside the container would show every file outside the scope as deleted, which would mislead the agent. Do you agree that the agent works without git history? A later spec can add a read-only history tool if agents need `git log`.

**C2. Dependency directories.** When an agent runs `npm install` or creates a Python virtual environment inside its scope, the files land in your repo on the host. They are Linux builds, so native modules can break your own `npm run` on macOS. I recommend an optional list of directories that stay inside the container:

```yaml
system: {repo: shop, scope: apps/web, container_only: [node_modules]}
```

Hikari mounts an empty Docker volume over each listed path. The agent can install into it, and your host copy stays untouched. Do you want this now, or later when a repo needs it?

## D. The map

**D1. Clarification: the worker owns the map.** I agree that only the worker can write an accurate map. In round 1, "the server is its only writer" meant only the mechanics of writing the file:

- The worker is the only author. Only the owner agent can call the write tools. The orchestrator can only read.
- The server turns each tool call into a change to `map.yaml`. It checks the change against the schema, writes the file, and tells the UI.

Writing through tools, instead of editing the YAML file directly, has three benefits:

- A typo cannot corrupt the map.
- The server knows at once when the map changed.
- The redacted view for the orchestrator is computed from structured data, so it can't leak paths.

Does this match what you meant by "the worker should own it"?

**D2. Orchestrators in a tree of hubs.** In `team.yaml`, `lead` orchestrates `#product`, and `eng-lead` is a member of `#product` and the orchestrator of `#eng`. I recommend that an orchestrator sees only the systems of members of the hubs it orchestrates. `lead` then does not see the systems inside `#eng`. `eng-lead` represents them, the way an engineering manager speaks for their team. Do you agree, or should a parent orchestrator see every system below it?

**D3. Links between systems.** Workers cannot read other maps, so a worker cannot know the feature IDs of another system. I recommend that interface names are the contract between systems:

- Each system lists the interfaces it provides and consumes, for example `POST /orders` or the event `order.created`.
- The orchestrator's view matches names across systems: "web consumes `POST /orders`, which api provides". It also flags a consumed interface that no system provides.
- `related` links in a feature point only to features of the same system.

The orchestrator can ask owners to align names that don't match. Do you agree, or does the orchestrator need its own store of links between systems?

**D4. Health comes from test results.** I recommend that the server computes health, so the owner reports only facts:

- `untested`: the feature lists no tests.
- `failing`: at least one listed test failed in the last run.
- `passing`: all listed tests passed in the last run.
- `stale`: the code in the feature's paths changed after the last test run.

Do you agree that owners cannot set health directly?

**D5. Draft schema.** Please review the fields and their names:

```yaml
# services/api/.hikari/map.yaml
# Written by hikari for the agent that owns this system. Change it through
# the map tools.
version: 1
system:
  name: api                       # the owning agent's name
  summary: Takes orders from the web app and stores them.
  responsibilities:
    - Validate and store orders.
    - Publish an event for each new order.
  interfaces:
    provides:
      - {name: POST /orders, kind: http, description: Creates an order.}
      - {name: order.created, kind: event, description: Sent after an order is stored.}
    consumes:
      - {name: inventory.reserve, kind: grpc, description: Reserves stock before an order is stored.}
  layout:                         # the owner's view only
    - {path: cmd/api, description: Server entry point.}
    - {path: internal/orders, description: Order validation and storage.}
  test_command: go test ./...
  described_state:                # the code that this map describes
    commit: 4e8ada5
    uncommitted_hash: ""          # hash of uncommitted changes in scope, empty when clean
    updated: 2026-10-21T09:30:00Z
features:
  - id: create-order              # unique in this system; referenced as api/create-order
    name: Create an order
    description: Validates an order, reserves stock, stores it, and sends order.created.
    interfaces: [POST /orders, inventory.reserve, order.created]
    code: [internal/orders/create.go, internal/orders/validate.go]   # owner's view only
    tests:
      refs: [internal/orders/create_test.go]                         # owner's view only
      passed: 12
      failed: 0
      ran: 2026-10-21T09:28:00Z
    health: passing               # computed by the server; see D4
    findings:
      - A client retry can create a duplicate order.
    plans:
      - Add idempotency keys to POST /orders.
    related: [cancel-order]       # features in this system only
```

Questions about the schema:

- (a) Are findings and plans enough as plain text lists for now? The next spec can give them IDs and status when work orders arrive.
- (b) Should the guidance for agents be "a feature is something a user or another system relies on, usually 5 to 30 per system"?
- (c) Is anything from the canvas missing?

## E. Packages

**E1. How packages get into the container.** Debian's packaged language toolchains are often old. For example, Debian bookworm ships Go 1.19. I recommend two optional fields for each agent:

```yaml
api:
  packages: [build-essential]     # Debian packages
  setup: |                        # runs as root when the image is built
    curl -fsSL https://go.dev/dl/go1.25.3.linux-arm64.tar.gz | tar -C /usr/local -xz
```

`hikari up` builds an image for each agent, `FROM hikari-node`, with these packages and this setup script. It tags the image with a hash of both fields, so it rebuilds only when they change. Project dependencies, such as `go mod download` or `npm install`, are not installed this way: the agent runs those itself at run time. Do you agree?

## F. Small decisions I will make unless you object

- The system name is the owning agent's name.
- A system must be inside a git repo.
- While the agent builds its map on first start, the UI shows its state as `indexing`. Messages to it wait until the map is built.
- The orchestrator's tools are `list_systems`, `get_system`, `search_features`, and `get_feature`, all with the redacted view.
- The owner's tools are `get_my_system`, `upsert_feature`, `remove_feature`, `set_system_info`, `record_test_results`, and `commit_changes`.
- You can see every map in full in the UI, but you cannot edit maps there in this spec.
- Claude sessions still start fresh on each `hikari up`. The map and the git history are what an agent remembers between runs.
