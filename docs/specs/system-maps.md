# Spec: systems, system maps, and hub vocabularies

Status: built. Changes made during the build, with their reasons in `docs/design.md`:

- Owners add features in batches with `upsert_features` (see section 8).
- hikari commits what each owner left when hikari stops, in addition to rule G13, so that rule G1 does not block the next start.
- A change to a map or a vocabulary also updates the UI views of the systems whose flags and providers depend on it.
- `hikari up` opens its port before it prepares any repo, so that a busy port leaves the repos untouched.

Sources: `docs/brainstorm/system-maps.md` (round 1), `system-maps-answers.md`, `system-maps-round-2.md` and its answers, `system-maps-round-3.md` (approved as written), and the canvas "Containerized Agents".

## Summary

An agent can own a **system**: a directory of a git repo on the host, mounted into the agent's container. The owner is the only agent that can read or change that code. It keeps a **system map** of its system: purpose, interfaces, and features with their test health. Orchestrators own no code. They plan from the maps of the systems in their hubs, and they keep a **vocabulary** of shared names for each hub. The server commits each owner's work to a configured branch.

## Changes from the approved brainstorm

Writing exact requirements showed six points where the brainstorm was incomplete or would not work as written. Please approve or reject each one.

1. **No `uncommitted_hash` in the map.** The map cannot list which files changed after it was described if it records uncommitted changes only as a hash. Instead, `mark_map_current` first commits the pending changes in the scope, then records the commit. `described_state` holds `commit` and `updated` only.
2. **A new field, `tests.content_hash`.** D4 marks a feature `stale` when its code changed after its last test run. A git commit cannot show this: the server commits after the tests run, so the commit would always differ. The server instead records a hash of the contents of the feature's code and test paths when the results arrive, and compares it with the current contents.
3. **A new owner tool, `mark_map_current`.** Some step must declare "the map now describes the code". Every write cannot do this, because the owner updates the map in several steps.
4. **A new agent field, `path`.** A toolchain installed by `setup`, such as Go from a tarball, must be on `PATH` for the agent's shell. `path` lists directories to add to `PATH` in the image.
5. **Indexing also runs when the map is stale at startup.** It runs not only when the map is missing. Otherwise, after you commit to the branch between runs, orchestrators read an old map until the owner's next message.
6. **A `read` path cannot overlap any system's scope.** A read-only path into another system's code would let a worker read that system, which round 1 question 7 rules out. Shared code must be outside every scope.

## Terms

- **System**: the code that one agent owns. It is a repo plus a scope.
- **Scope**: a directory inside the repo, relative to the repo root. An empty scope means the whole repo.
- **Owner**: the agent that owns a system. The system has the owner's name.
- **System map**: the file `<scope>/.hikari/map.yaml`, which describes one system.
- **Vocabulary**: the file `vocabulary/<hub>.yaml` next to the topology file, which holds the shared names of one hub.

## 1. Topology

```yaml
repos:
  shop:
    path: ~/code/shop               # ~ expands; relative paths start at the topology file's directory
    branch: hikari/checkout-v2

agents:
  lead:
    description: Product lead.
  orders:
    description: Owns order intake.
    system:
      repo: shop
      scope: services/orders        # optional; empty means the whole repo
      read: [proto]                 # optional read-only paths, relative to the repo root
      test: python3 -m unittest     # optional; the first value of the map's test_command
      container_only: [.venv]       # optional paths, relative to the scope, kept in a Docker volume
    packages: [python3]             # optional Debian packages
    setup: |                        # optional; runs as root when the image is built
      echo extra setup
    path: [/opt/tool/bin]           # optional directories added to PATH

hubs:
  product: {orchestrator: lead, members: [orders]}
```

Rules:

- **T1.** Each repo `path` exists and is the top-level directory of a git repo.
- **T2.** Each `branch` is a valid git branch name.
- **T3.** `system.repo` names a declared repo. Every declared repo is used by at least one system.
- **T4.** `scope`, `read`, and `container_only` are relative paths with no `..` and no leading `/`. The `scope` and `read` directories exist.
- **T5.** Two scopes in one repo do not overlap: neither is the other, or a parent of the other. A whole-repo scope allows no other system in that repo.
- **T6.** No `read` path overlaps a scope of any system. This is change 6.
- **T7.** An agent that orchestrates any hub has no `system`.
- **T8.** Each `packages` entry matches `^[a-z0-9][a-z0-9+.-]*$`. Each `path` entry is an absolute path.
- **T9.** The topology loader reports every violation with the agent or repo name.

## 2. Repos, branches, and commits

At `hikari up`, before any container starts, the server does this for each repo:

- **G1.** If `git status --porcelain` shows anything, `hikari up` stops with an error that names the repo.
- **G2.** If the branch does not exist, the server creates it from `HEAD`. Then it checks the branch out.
- **G3.** It records the commit at `HEAD`, for the summary in G10.
- **G4.** For each system without `<scope>/.hikari/map.yaml`, it writes an unbuilt map: `version`, `system.name`, and `test_command` from `system.test`. The first commit in that scope includes this file.
- **G5.** If git ignores `<scope>/.hikari/map.yaml`, the server prints a warning: then the map is not versioned with the code.

Commits:

- **G6.** Only the server runs git commands that write. It runs them on the host, in the repo.
- **G7.** A commit for an owner runs `git add -A -- <scope>`, then `git commit -- <scope>`. A commit contains only files inside the owner's scope.
- **G8.** The commit author is `<agent> (hikari) <<agent>@hikari.invalid>`. The committer comes from the host git config.
- **G9.** Before each commit, the server checks that the configured branch is still checked out. If it is not, the commit fails and the error says why.
- **G10.** The server runs git commands for one repo one at a time.
- **G11.** On shutdown, the server leaves the branch checked out and prints, for each owner, the number of commits it authored since G3.

There are two kinds of commit:

- **G12.** The `commit_changes(message)` tool commits with the owner's message. If the scope has no changes, it commits nothing and says so.
- **G13.** After every turn of an owner, the driver asks the server for an automatic commit, with the message `<agent>: changes left at the end of a turn`. If the scope has no changes, it commits nothing.

## 3. Mounts

The container of an owner sees only:

- **M1.** The scope, read-write, at `/workspace/<scope>`. This is also the working directory. For a whole-repo scope, the path is `/workspace`.
- **M2.** Each `read` path, read-only, at `/workspace/<read>`.
- **M3.** `<scope>/.hikari`, read-only, at the matching path. The directory is mounted, not the file: the server replaces the file with an atomic rename, and a file mount would keep showing the old file.
- **M4.** For each `container_only` path, a named Docker volume at the matching path. The volume name comes from a hash of the repo path, the scope, and the path, so the volume persists between runs.
- **M5.** For a whole-repo scope, an empty read-only directory over `/workspace/.git`. A container never sees `.git`.
- **M6.** An agent without a system gets no mounts.

## 4. Images

- **I1.** An agent with `packages`, `setup`, or `path` runs an image built `FROM hikari-node`. It installs the packages, then runs the setup script as root with `bash -euo pipefail`, then adds the `path` directories to the front of `PATH`. The image runs as the `agent` user.
- **I2.** The image tag is `hikari-node-<hash>`. The hash covers the base image ID, the packages, the setup script, and the path entries. When the tag exists, the image is not built again.
- **I3.** Build output appears in the `hikari up` log with the prefix `[build <agent>]`. If a build fails, `hikari up` stops.

## 5. System maps

### Schema

The approved D5 schema, with changes 1 and 2:

```yaml
version: 1
system:
  name: orders
  summary: Takes orders from checkout and stores them.
  responsibilities: [Validate and store orders., Publish an event for each new order.]
  interfaces:
    provides:
      - {name: POST /orders, kind: http, description: Creates an order.}
    consumes:
      - {name: inventory.reserve, kind: function, description: Reserves stock.}
  layout:
    - {path: orders/, description: Order validation and storage.}
  test_command: python3 -m unittest
  described_state: {commit: 4e8ada5, updated: 2026-10-21T09:30:00Z}
features:
  - id: create-order
    name: Create an order
    description: Validates an order, reserves stock, and stores it.
    interfaces: [POST /orders, inventory.reserve]
    code: [orders/create.py]
    tests:
      refs: [tests/test_create.py]
      passed: 6
      failed: 0
      ran: 2026-10-21T09:28:00Z
      content_hash: 9f2c...
    health: passing
    findings: [A client retry can create a duplicate order.]
    plans: [Add idempotency keys.]
    related: [cancel-order]
```

### Rules

- **S1.** Feature IDs match `^[a-z][a-z0-9-]{0,47}$` and are unique in the map.
- **S2.** Each name in a feature's `interfaces` is in the system's `provides` or `consumes`.
- **S3.** Each `related` entry is the ID of another feature in the same map. Removing a feature also removes the references to it.
- **S4.** Each `code`, `tests.refs`, and `layout` path exists in the scope. The server rejects a change that names a path that does not exist.
- **S5.** The server rejects a change that breaks S1 to S4, and the error names the rule. The file on disk never holds an invalid map.
- **S6.** The server writes the map with an atomic rename. It reloads the map when the file changes on disk, for example after you edit it by hand. If a hand edit makes the map invalid, the server keeps the last valid map, and the UI shows the error.
- **S7.** `interfaces[].kind` is a free lowercase word. The prompt suggests `http`, `grpc`, `event`, `function`, `file`, and `cli`.

### Health

The server computes `health`. Owners cannot set it.

- **S8.** `untested`: the feature has no `tests.refs`, or its tests never ran.
- **S9.** `stale`: the current contents of the feature's `code` and `tests.refs` paths have a different hash than `tests.content_hash`.
- **S10.** `failing`: `tests.failed` is more than 0.
- **S11.** `passing`: none of the above.
- **S12.** The checks run in the order S8, S9, S10, S11. The first match decides.

### Map state

- **S13.** `unbuilt`: `described_state.commit` is empty.
- **S14.** `stale`: files in the scope, other than `.hikari/`, changed after `described_state.commit`. This covers commits, uncommitted changes, and new untracked files.
- **S15.** `current`: none of the above.

### Views

- **S16.** The full view is the whole map plus the computed health and state.
- **S17.** The orchestrator view leaves out `layout`, `code`, `tests.refs`, and `tests.content_hash`. It keeps the test counts and `ran`.

## 6. Vocabularies

- **V1.** Each hub has one vocabulary file, `vocabulary/<hub>.yaml`, in the topology file's directory. If it does not exist, the server creates it the first time the orchestrator writes to it.
- **V2.** The format is the approved round 3 draft: `version`, `hub`, `terms` (`term`, `definition`, `avoid`), and `interfaces` (`name`, `kind`, `description`).
- **V3.** Only the hub's orchestrator can change it. Every participant of the hub can read it.
- **V4.** Defining a term or an interface that exists replaces it.
- **V5.** The server keeps a version number for each vocabulary. The driver uses it in D3.

Flags in the orchestrator view of a system:

- **V6.** An interface that the system provides or consumes, but that no vocabulary of the orchestrator's hubs defines.
- **V7.** An interface that the system consumes, but that no system visible to the orchestrator provides.

The server never rejects a map change because of the vocabulary.

## 7. Access

- **A1.** An owner reads and changes only its own map, through the owner tools.
- **A2.** An orchestrator reads the orchestrator view of each system whose owner is a direct member of a hub it orchestrates. In `team.yaml`, `lead` would not see the systems of `#eng`.
- **A3.** No other agent reads any map.
- **A4.** The server enforces A1 to A3 for each request, from the node token. A tool that a role does not have is also not registered in that agent's MCP server.
- **A5.** The UI shows the full view of every map and every vocabulary. It cannot change them.

## 8. MCP tools

Every agent keeps `post_to_hub`, `send_direct_message`, `list_hubs`, and `check_inbox`, and gets:

| Tool | Purpose |
|---|---|
| `get_vocabulary(hub)` | Read the vocabulary of a hub you are in. |

Owners also get:

| Tool | Purpose |
|---|---|
| `get_my_system()` | Read your full map, with health and state. |
| `set_system_info(summary?, responsibilities?, provides?, consumes?, layout?, test_command?)` | Replace the given fields of `system`. Fields you leave out stay as they are. |
| `upsert_features(features: [{id, name, description, interfaces, code, test_refs, findings, plans, related}])` | Add features, or replace the ones with these IDs, as one change. Test results stay. (Built as a batch, not one feature per call: see `docs/design.md`.) |
| `remove_feature(id)` | Remove a feature and the references to it. |
| `record_test_results(results: [{feature, passed, failed}])` | Store test results, with the time and the content hash. |
| `mark_map_current()` | Commit the pending changes in the scope, then record the commit as the described state. |
| `commit_changes(message)` | Commit the changes in your scope. |

Orchestrators also get:

| Tool | Purpose |
|---|---|
| `list_systems()` | The visible systems, with summary, map state, and a count of features by health. |
| `get_system(name)` | The orchestrator view of one system, with the flags of V6 and V7, and which visible systems provide each interface it consumes. |
| `search_features(query)` | Features of the visible systems whose ID, name, description, or interfaces contain the query, ignoring case. |
| `get_feature(system, id)` | The orchestrator view of one feature. |
| `define_term(hub, term, definition, avoid?)` | Add or replace a term. |
| `define_interface(hub, name, kind, description)` | Add or replace an interface. |
| `remove_entry(hub, name)` | Remove a term or an interface. |

## 9. Driver

- **D1.** At startup, if the owner's map is `unbuilt` or `stale`, the driver runs an indexing turn before it reads messages. During that turn, the agent's state is `indexing`. Messages to the agent wait in its inbox.
- **D2.** The prompt of the indexing turn tells the owner to read its system, to define the system info and 5 to 30 features, to run the tests and record the results, then to call `mark_map_current`. For a stale map, it lists the changed paths and asks the owner to update only the affected entries.
- **D3.** Before each later turn of an owner, the driver gets the map state from the server. If the map is stale, the turn prompt starts with the changed paths and a request to update the map. For every agent, the turn prompt names each hub whose vocabulary changed since the agent's last turn.
- **D4.** The system prompt of an owner states:
  - its scope and its read-only paths,
  - that it must not try to change `.hikari/`,
  - that it uses the map tools and the hub vocabulary's names,
  - that it calls `commit_changes` after each unit of work,
  - that a blocker outside its system goes to its orchestrator.
- **D5.** The system prompt of an orchestrator states that it has no code. It plans from the system tools, keeps each hub's vocabulary, and asks owners about anything that a map does not answer.
- **D6.** After each turn of an owner, the driver requests the automatic commit of G13, including after a failed turn.

## 10. UI

- **U1.** The sidebar gets a Systems group, with one entry for each system. Each entry shows a dot for the map state.
- **U2.** A system view (`#/system/<name>`) shows:
  - the owner, the repo, scope, and branch, and the map state, with the changed paths when the map is stale;
  - the summary, responsibilities, interfaces with the flags of V6 and V7, and the layout;
  - the features, each with its health and test counts. A feature opens to show its description, code and test paths, findings, plans, and related features.
- **U3.** Health colors: green for `passing`, red for `failing`, amber for `stale`, and a gray outline for `untested`. Everything else stays in the gray palette.
- **U4.** A hub view gets a Vocabulary tab, next to the messages, that lists the hub's terms and interfaces.
- **U5.** The `indexing` state appears like `working`, with its own label.
- **U6.** The snapshot holds all maps and vocabularies. A change to a map or a vocabulary reaches the UI as an event, without a reload.

## 11. Out of scope

- Work orders, change logs, and the validation agent (`docs/brainstorm/phase-2.md`).
- Pushing, merging, and pull requests.
- Network allowlists.
- Matching file ownership for bind mounts on Linux. Docker Desktop on macOS handles it.
- Keeping Claude sessions between runs.
- Editing maps or vocabularies in the UI.

## 12. Known limitations

- The first indexing turn reads the whole system, so it costs many tokens.
- Test results are reported on the honor system. The server checks only that the paths exist.
- Native dependencies that an owner installs outside `container_only` paths land in your repo as Linux builds.

## 13. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Topology.** Repos, systems, packages, setup, and path, with rules T1 to T9. Tests cover each rule.
2. **Git package.** Clean check, branch preparation, scoped commits with the agent as author, branch check, changed paths since a commit, and content hashes. Tests use temporary git repos and cover G1 to G13, including two owners in one repo.
3. **Map package.** Schema, rules S1 to S7, health S8 to S12, state S13 to S15, views S16 and S17, and atomic save with reload. Table tests for each rule.
4. **Vocabulary package.** Format, changes, versions, and flags V6 and V7. Tests.
5. **Server.** Node endpoints for the owner tools, commits, turn context, system queries, and vocabularies, with access rules A1 to A4. The UI snapshot and events get maps and vocabularies. Tests for each access rule.
6. **Launcher.** Mounts M1 to M6, image builds I1 to I3, and the working directory. Tests check the generated `docker run` and `docker build` arguments.
7. **Driver and MCP server.** Tools by role, prompts D2 to D5, indexing turns, and automatic commits. Tests cover tool registration by role and the prompt text.
8. **UI.** Systems view, Vocabulary tab, and the `indexing` state. Unit tests for the new state logic, and browser checks.
9. **Demo and live test.** `make demo-repo` creates a small Python repo, `shop`, with two systems, `services/orders` and `services/inventory`, under `.hikari/demo/`. `examples/systems.yaml` has an orchestrator and the two owners. The live run checks that:
   - both owners build valid maps;
   - the orchestrator plans a change from the maps alone, and its container has no mounts;
   - an owner makes the change, records tests, and marks its map current;
   - the commits are on the configured branch, with the owner as author and only files from its scope;
   - the UI shows the maps, the health, and the vocabulary.
10. **Docs.** README and `docs/design.md` describe systems, maps, vocabularies, and the git workflow.
