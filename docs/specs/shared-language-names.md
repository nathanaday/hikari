# Spec: the names of the shared language

Status: built. Docker was not running, so the live check of 2026-10-26 ran without containers: `hikari check examples/shop/topology.yaml` passed, and with `hikari up -no-launch`, `hikari mcp` with each agent's token stood in for the agents. orders called `get_my_partition` and `apply`, which committed as `orders (hikari)`; the orchestration agent called `list_partitions` and `get_partition`; a `map.yaml` with `system:` was rewritten at the next start. Headless Chrome showed Partitions in the sidebar, the Structure tab's Project, Repositories, Agents, Sidecars, and Vault, and "partition agent" under orders, with no console error. No agent turn ran, and no image was built.

Changes made during the build:

1. **The wire fields take the table's words.** `Self` has `partition` and `orchestration`, and `partition_agent_grants` and `partition_agent_deliverables`; a partition's view names its agent in `partition_agent`; the private code map is `code_map`; a status or activity event names its `agent`; the event of a partition is `partition`, and a wiki's scope is `partition` or `public`.
2. **The routes follow.** `set_partition_info` is `PATCH /api/agent/map/partition`, the orchestration agent's reads are `/api/agent/partitions`, a release is `/api/ui/vault/releases/{partition}/...`, and the UI's partition page is `#/partition/<name>`.
3. **`sync`, `push`, `check`, and `status` take the file as optional** (N5). Before, they required it. `scaffold.TopologyFile` names `topology.yaml` for them, for `hikari init`, and for the editor's new project.
4. **A map from a teammate's branch is read loosely.** `featuremap.Decode` reads the old `system:` key too, for `hikari status` and the commit check of `hikari sync`, which read maps that `hikari up` has not rewritten.
5. **Names that the spec did not list follow the table.** The skill reference `layout.md` is `code-map.md`, hikify's draft is `topology.target.yaml`, `NodeAvatar` is `ParticipantAvatar`, the stop commit says `vault changes left when hikari stopped`, and the wake fixture's keys are `orchestration` and `partition_agents`.
6. **The UI shows roles in words.** `roleLabel` maps `orchestration` and `partition` to "orchestration agent" and "partition agent". The Structure tab's group is Repositories, and a partition's private section is Code map.
7. **The lint test is `internal/driver/lint_test.go`** (N11). It builds the system prompt of each kind of agent, and reads the embedded skills and agents, the host skills, and the string literals and JSX text of the components; tests and the vendored `ui/` components are left out. Its allow list keeps paths under `store/`, `store.ts`, "Docker Compose file", a verb, and the graph's nodes.
8. **Some old words stay where they mean something else.** YAML and graph nodes (`yaml.Node`, `nodeTaken`), Docker Compose (`ComposeError`, `composeKeys`, the `compose` method), the deliverable's `layout` rules, the system prompt, and the abbreviations `sys` and `orch`. The service id `orchestrator` and the agent name `lead` in tests are names.
9. **The first commit carries the UI's file moves.** `git mv` staged them with the Go change, so the UI's tests pass from the next commit.

Sources: `docs/brainstorm/shared-language.md` (questions C1 to C3 and C5) and `shared-language-answers.md`. This is step 3 of the seven steps of the shared language.

## Summary

Hikari takes the words of the glossary everywhere a person or an agent reads them, and everywhere in the code: vault, partition, partition agent, orchestration agent, feature map, code map, topology file, driver, and apply. The behavior does not change. The files on disk keep their places and names until step 4, so that this step only renames, and a test of today proves the same behavior under its new names. The glossary moves into the repository as `docs/glossary.md`, and a lint test keeps the retired words out of prompts, skills, and the UI.

## Decisions

From the round and its answers:

1. The rename goes all the way down: Go packages and identifiers, wire types, MCP tools, topology keys, UI labels, prompts, skills, and docs (C1).
2. "Node" leaves the vocabulary. The package `internal/node` becomes `internal/driver`, and `hikari node` becomes `hikari driver` (C2).
3. The default topology file is `topology.yaml` (C3).
4. The glossary is `docs/glossary.md`, and a lint test keeps the retired words out of prompts, skills, and UI strings (C5).

## Decisions made in this spec

1. **Directories and file names on disk move in step 4.** `store/`, `systems/<id>/`, `shared/`, `layout.yaml`, and the container paths under `/hikari` keep their names here. Step 4 moves them in one change with the new layout, so that each directory moves once. The README and the prompts name the paths as they are.
2. **The topology's keys are renamed here.** They are words that you write and read: `x-hikari.vault` replaces `x-hikari.store`, and `x-hikari.partition` replaces `x-hikari.system` on a service. The partition keeps today's fields (`repo`, `scope`, `read`, `test`) until step 5 replaces them with folders. The old keys fail with the strict decoder's error.
3. **The agent API moves to `/api/agent/`, and the vault's UI routes to `/api/ui/vault/`.** `/api/node/` named the old word, and the driver's client is the only caller.
4. **"Orchestrator" leaves the prose, and the code says `orchestration`.** Prompts, skills, the UI, and the docs say "orchestration agent". The code's identifiers say `Orchestration` (for example `Topology.Orchestration`, the agent's name). A service id that you choose, such as `orchestrator` in the examples, is a name, and the lint does not read the topology file.
5. **A participant's role is `orchestration`, `partition`, or `operator`.** `list_agents` and the system prompt show it.
6. **`store.ts` keeps its name.** It is the UI's reducer, and "store" is the standard name of that pattern.
7. **Hikari writes and reads `map.yaml` and the release manifest with `partition:` in place of `system:`.** A release is immutable, so an old manifest with `system:` still loads. An old `map.yaml` is rewritten at start, as a renamed system's map is rewritten today (ID12).

## The Names

| Today | After |
|---|---|
| store, `internal/store`, `store.Store` | vault, `internal/vault`, `vault.Vault` |
| `sysmap.Store`, `wiki.Store`, `vocab.Store` | `featuremap.Files`, `wiki.Files`, `vocab.File` |
| system, `internal/systems`, `systems.Manager` | partition, `internal/partitions`, `partitions.Manager` |
| system map, `internal/sysmap` | feature map, `internal/featuremap` |
| system wiki, ref `system:<name>` | feature map, ref `partition:<name>` |
| owner, `Owner`, `owners` | partition agent, `PartitionAgent`, `partitionAgents` |
| orchestrator (prose) | orchestration agent |
| compose file, `compose.yaml` (hikari's topology) | topology file, `topology.yaml` |
| node, `internal/node`, `hikari node`, `NodeView` | driver, `internal/driver`, `hikari driver`, `AgentView` |
| layout, code layout, `sysmap.Layout` | code map, `featuremap.CodeMap` |
| `/api/node/*` | `/api/agent/*` |
| `/api/ui/store/*` | `/api/ui/vault/*` |
| `get_my_system`, `set_system_info`, `list_systems`, `get_system` | `get_my_partition`, `set_partition_info`, `list_partitions`, `get_partition` |
| `commit_changes` | `apply` |
| `hikari-system` skill | `hikari-partition` skill |
| `SystemView`, `SelfSystem`, `SystemPatch`, `SystemSummary` | `PartitionView`, `SelfPartition`, `PartitionPatch`, `PartitionSummary` |

The table names the concepts; every identifier, type, field, function, file, test, and fixture that carries one of the old words follows it. Compose stays the name of Docker Compose and of its keys and commands, such as `docker compose config` and the compose override file.

## Requirements

### Code

- **N1.** The packages `internal/store`, `internal/systems`, `internal/sysmap`, and `internal/node` are `internal/vault`, `internal/partitions`, `internal/featuremap`, and `internal/driver`, moved with `git mv` so that their history follows.
- **N2.** No Go identifier, wire field, JSON key, or TypeScript identifier in `internal/`, `cmd/`, or `web/src/` contains `Store` or `store` for the vault, `System` or `system` for a partition, `Owner` or `owner`, `Layout` for the code map, or `Node` or `node` for the driver. `store.ts` and React's own words are the exception.
- **N3.** The CLI's subcommand is `hikari driver`, and the override file's entrypoint calls it. `hikari help` lists no `node`.

### Topology

- **N4.** `x-hikari.vault` and a service's `x-hikari.partition` replace `x-hikari.store` and `x-hikari.system`. The editor's operations and fields follow: `set_project {vault}`, `set_partition`, and the error paths `x-hikari.vault` and `services.<id>.x-hikari.partition`.
- **N5.** `hikari up`, `check`, `sync`, `push`, and `status` open `topology.yaml` when no file is named. When it is missing and `compose.yaml` exists, the message says to rename it to `topology.yaml`. `hikari init` writes `topology.yaml`. The examples' files are `topology.yaml`, and the Makefile's `COMPOSE` variable is `TOPOLOGY`.

### Agents

- **N6.** The MCP tools and the agent API take the names of the table. Every prompt, skill, reference, and agent definition in `internal/driver` and every host skill in `internal/onboard` uses the glossary's words: partition, partition agent, orchestration agent, feature map, code map, test map, vault, topology file, apply.
- **N7.** The skill `hikari-system` is `hikari-partition`. `hikari-orchestration` keeps its name.

### UI

- **N8.** Every label, hint, title, and empty state uses the glossary's words. The Structure tab's sections are Project, Repositories, Agents, and Sidecars, and the vault's section is Vault. A partition agent's page is titled by its name with "partition agent" under it.

### Docs

- **N9.** `docs/glossary.md` holds the glossary: each term with its meaning as your journal writes it, updated for the answers of round 1 (zero or one orchestration agent, the vocabulary directory, secrets in compose), and a section of retired words with what replaced them: hub, lead, store, system, owner, node, compose file, layout.
- **N10.** `README.md`, `docs/design.md`, and `docs/guides/` use the glossary's words, and the README's first section links the glossary. The specs under `docs/specs/` and the files under `docs/brainstorm/` and `docs/archive/` keep the words of their time; each built spec gets one line under its status: "Terms changed in `shared-language-names.md`; see `docs/glossary.md`."

### Lint

- **N11.** A Go test fails when a retired word appears in an embedded prompt or skill of `internal/driver`, a host skill of `internal/onboard`, or a string literal or JSX text in `web/src/components`. The retired words are `hub`, `lead` (as a role), `store` (for the vault), `system map`, `owner`, `node` (for the driver), and `compose file`. An allow list in the test names each exception with its reason, such as `store.ts` and `GitHub`.

### Checks

- **N12.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass.
- **N13.** A live run of the shop with `topology.yaml`: `hikari check` passes, and with `hikari up`, a partition agent calls `get_my_partition` and `apply`, the orchestration agent calls `list_partitions`, and the UI shows the new labels.
