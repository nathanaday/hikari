# Shared language: round 1

Source: your message of 2026-10-26, with `Shared Language.md` and `Phase 2.canvas` in the Hikari journal.

## What I take from your proposal

- **One vocabulary, used everywhere.** Each concept has one name in the code, the docs, the UI, the prompts, and the files on disk. A blurry boundary between two concepts is a bug.
- **Fewer concepts, done well.** Hubs leave: a project is the one room. The E2E work (the tester, environments, bench results) moves out of the way until the integration agent has a design.
- **The store becomes the vault, and its layout says who sees what.** Public files for everyone, and for each partition a read-only part and a writable part. The canvas draws this layout.
- **A system becomes a partition: a flat list of folders, possibly from several repositories.** The agent in it sees one monorepo. An apply commits its changes to each source repository on the host.
- **Agents are named by where they run and what they see.** Partition agent, orchestration agent, Hikari agent, and later the integration agent. Each gets a pack of skills.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Decided

These come straight from your glossary and need no answer.

- **Hubs are removed.** Each project is one hub. Trees of hubs, members lists, and hub ids go.
- **"Lead" stays removed.** One stale mention is left in `docs/specs/containerize.md` (CZ6); the docs pass of this work drops it.
- **The integration agent is planted, not built.** It needs its own design round once the home of the E2E suites is settled.
- **The topology file is a compose file.** Every compose feature keeps working; only the name changes.
- **Private knowledge stays in the partition.** The code map and the test map stay with the code, and no other agent reads them.
- **Only the orchestration agent changes public files, and only through tools.** Every agent mounts them read-only.

## Terms

| Term | Replaces | Meaning |
|---|---|---|
| Hikari | | The binary: its CLI, server, API, and UI. |
| Project | topology, network | One topology file and everything it names. One project has one room. |
| Repository | repo | A git repository on the host, linked by path or cloned from a URL. |
| Topology file | compose file | The compose file with `x-hikari` keys that defines a project. |
| Vault | store | The directory of every Hikari file that is not code. |
| Partition | system, scope, node | The folders that one container mounts as its workspace. |
| Partition agent | owner | The agent in a partition's container. |
| Orchestration agent | orchestrator | The agent that reads every partition's public files and no code. |
| Hikari agent | host session | Your Claude Code session on the host, with the host skills. |
| Integration agent | tester | Planted. |
| Feature map | system map, system wiki | A partition's public description: its feature documents and `map.yaml`. |
| Code map | `layout.yaml` | Private: where each feature lives in the code. |
| Test map | `tests.yaml` | Private: which features each test touches. Unchanged. |
| Apply | `commit_changes` | Commit a partition's changes to each source repository. |

## What Exists Today

The three inventories behind this round found the following.

- **Hubs reach almost everything.** An agent is an orchestrator only because a hub names it. The wake rules, direct messages, which systems an orchestrator sees, the vocabularies (one per hub), the hub wikis and their `story` kind, the tester, release announcements, and the release manifest's `hubs:` field all derive from the hub list. About 1,000 lines of Go and 800 lines of TypeScript are hub logic, and the `hikari-hub` skill is hub-only.
- **The E2E work is about 3,800 lines in its own files, but it reaches into shared code in a few places.** `internal/e2e`, `internal/environments`, `systems/tester.go`, `systems/environment.go`, the `hikari-tester` skill, and `hub-tests.tsx` move out whole. The surgical parts are: `codeHash` in `testReport` and `RecordRun` (a story's lock hash joins a test result's hash), the `extra` ids of `sysmap.Validate`, the pin notices of the linked repositories spec (SY11), story health in `wikiView`, and the `tests:` and `e2e:` topology fields with their editor operations.
- **The E2E commits are spread across about 25 commits**, and later work, such as ids and the topology editor, edits the same files. A `git revert` would conflict at every step.
- **Image releases (`internal/images`) came with environments.** `publish_release` builds an image for any deliverable with an `image:` section, but only environments run the images.
- **Each owner has one scope in one repo.** `System{Repo, Scope, Read, Test}`, `launcher.SystemMounts`, the commit path, the private documents' root, and the UI's `TopologySystem` all assume it. Read paths must be in the same repo.
- **An owner mounts its scope at `/workspace/<scope>`, its private documents at `/hikari/private`, its own system directory read-only at `/hikari/public/export`, and its staging area writable.** Every agent mounts `store/shared` read-only. An orchestrator mounts the whole system directory of each member owner, read-only, and has no writable mount.
- **There is no concept of secrets.** Compose's `environment` and `env_file` pass through untouched.
- **Shared skills and agent definitions in `store/shared` reach every container.** Deliverables reach only the owners the topology grants them to.

---

## A. Moving the Removed Work Out of the Way

**A1. Tag the current `main` as `archive/hubs-and-e2e`, then delete the code on `main`.** A tag costs nothing, never drifts, and `git show archive/hubs-and-e2e:internal/e2e/e2e.go` reads any file back. A long-lived branch would fall further behind with every rename in this round, and nobody could merge it. Because a revert conflicts, the removal is a forward change: one commit for E2E and one for hubs. Do you agree?

**A2. The specs and brainstorms of removed features move to `docs/archive/`.** `docs/specs/` then describes Hikari as it is. The moves: `tester.md`, `environments.md`, `bench.md`, `e2e.md`, and `e2e-answers.md`. Specs that only mention hubs or the tester, such as `wikis.md` and `entity-ids.md`, stay where they are, with a line at the top that names what this round removed. `docs/archive/README.md` names the tag. Do you agree?

**A3. Everything below leaves with E2E.**

- The tester: `tests:`, `e2e:` limits, rules TS1 to TS4, the plan, gaps, approvals, and the release lock.
- Environments, bench procedures and results, and the `/api/ui/e2e/*` and `/api/node/e2e/*` routes.
- The tools `pin_releases`, `start_environment`, `stop_environment`, `environment_status`, and `get_test_plan`.
- The `hikari-tester` skill, the Tests tab, and the tester fields in the Structure tab.
- The `sensors` example and `make example-sensors`.

A test result's hash goes back to the code of the features it touches, as it was before the tester (`sysmap.ContentHash`), and `sysmap.Validate` loses its `extra` ids. Do you agree?

**A4. Image releases leave too.** Without environments, nothing runs an image release, and the integration agent's round will decide how products reach a test. The `image:` section of a deliverable then becomes an error that names this round. Do you agree, or do you want to keep image releases for a deliverable of your own?

**A5. A suite that needs the whole application stays in its repository, outside every test command.** The principle "each partition tests only its own code" stays. Today hikify moves such a suite to the tester. Instead, hikify will record it in `onboarding/` as left for integration and change nothing. The `onboard` example's browser test becomes the worked case. Do you agree?

## B. One Project, One Room

**B1. The topology names its orchestration agent at the top: `x-hikari.orchestration: <service id>`.**

```yaml
name: shop
x-hikari:
  orchestration: conductor
  repos:
    shop: {path: ../shop, branch: hikari/demo}
```

`hubs` goes. A key at the top allows one orchestration agent and makes two impossible to write. A role field on each service would allow two and need a check against it. Do you agree?

**B2. A project has zero or one orchestration agent.** One partition needs nobody to coordinate it, and a required orchestration agent would cost a container and its turns for nothing. `hikari init` and hikify still write one. Do you agree, or must every project have one?

**B3. Every agent is a partition agent or the orchestration agent.** Your glossary names no other kind, so an agent with no folders, such as `writer` and `critic` in the `studio` example, can no longer be written. The `studio` and `team` examples go, and `shop` becomes the only small example. Do you agree?

**B4. The room keeps today's wake rules, without the hub.** Every agent receives every post. A post starts a turn at once for the orchestration agent, for each agent that it mentions as `@name`, and for every partition agent when the orchestration agent posts without mentions. With no orchestration agent, a post without mentions starts a turn for every agent. Do you agree?

**B5. Any agent can send a direct message to any agent.** Today two agents need a shared hub, and with one room every pair shares it. The tools become `post` (was `post_to_hub`), `send_direct_message`, `list_agents` (was `list_hubs`), and `check_inbox`. Do you agree, or should partition agents reach each other only through the room?

**B6. The project has one vocabulary, at `vault/public/vocabulary.yaml`.** The file loses its `hub:` field, and `get_vocabulary`, `define_term`, `define_interface`, and `remove_entry` lose their `hub` argument. Your canvas draws `vocabulary/` as a folder. Did you mean a folder of term documents in the wiki format, or is one file enough for now? I recommend one file.

**B7. A hub's wiki splits in two.** Its rules and principles apply to every agent, so they become the project's public wiki at `vault/public/wiki/`, which every agent reads and only the orchestration agent writes, through `change_public_wiki`. Its stories link features across partitions, so they move to the orchestration feature map (F4). The key-story flag existed for E2E, so it goes. Do you agree?

**B8. A release is announced in the room, and new manifests have no `hubs:` field.** A release is immutable, so old manifests keep the key, and Hikari ignores it when it reads them. Do you agree?

## C. Names

**C1. The rename goes all the way down.** It covers Go packages and identifiers, wire types, MCP tool names, topology keys, directories on disk, UI labels, prompts, skills, and docs. A rename that stops at the UI would leave two words for each concept in the code, which is the blur your glossary removes. Some examples:

| Today | After |
|---|---|
| `internal/store`, `store.Store` | `internal/vault`, `vault.Vault` |
| `internal/systems`, `systems.Manager` | `internal/partitions`, `partitions.Manager` |
| `internal/sysmap` | `internal/featuremap` |
| `x-hikari.store` | `x-hikari.vault` |
| `x-hikari: {system: {...}}` | `x-hikari: {partition: {...}}` |
| `get_my_system`, `set_system_info`, `list_systems`, `get_system` | `get_my_partition`, `set_partition_info`, `list_partitions`, `get_partition` |
| `commit_changes` | `apply` |
| `layout.yaml` | `code-map.yaml` |

`sysmap.Store`, `wiki.Store`, and `vocab.Store` are stores of documents, a different use of the word. They become `featuremap.Files`, `wiki.Files`, and `vocab.File`, so that "store" leaves the code. `web/src/lib/store.ts`, the UI's reducer, keeps its name, because "store" is the standard name of that pattern in React. Do you agree?

**C2. "Node" leaves the vocabulary.** Your glossary says partition replaces node. In the code, though, node is the driver that runs Claude Code in every container, the orchestration agent's included. I recommend that `internal/node` become `internal/driver` and `hikari node` become `hikari driver`, so that "partition" means only the folders and their container. Do you agree?

**C3. The default topology file is `topology.yaml`.** `hikari up`, `check`, `sync`, `push`, and `status` open it when no file is named, and `hikari init` writes it. Hikari passes the file to compose with `-f`, so compose does not care about the name. An old `compose.yaml` is not opened by default, and `up` names the new file when it finds only the old one. Do you agree?

**C4. Hikari ships no migrator.** Hikari has no users other than you, and a migrator would be code that runs once. The build migrates `myprojects/yami` and `myprojects/cs566-project` by hand, with `git mv` in their project repositories, and `docs/guides/migrating.md` lists the moves for anyone else. Do you agree?

**C5. Your glossary becomes `docs/glossary.md`, and the docs link to it.** The README's first section, `design.md`, the system prompts, and the skills use only its terms. A lint test fails when a retired word, such as `hub`, `store`, or `owner`, appears in a prompt, a skill, or a UI string, with an allow list for the places that name the history. Do you agree?

## D. The Vault

**D1. The vault follows the canvas.**

```
vault/
  README.md                  written once by Hikari
  public/                    every agent, read-only
    vocabulary.yaml          the orchestration agent, through tools
    wiki/                    the project's rules and principles; your plans and notes
    policies/                yours; they go into every system prompt
    deliverables/<name>/     yours; deliverable.yaml and SKILL.md
  partitions/<partition>/
    read-only/               the partition agent reads it; the orchestration agent reads it
      features/              the feature map: map.yaml and the documents
      releases/              published releases
      granted/               files you grant this partition
    can-edit/                files you grant this partition, writable by its agent
    staging/                 the partition agent's next release
  orchestration/
    read-only/
      features/              the orchestration feature map (F4)
      granted/
    can-edit/
```

`features/` holds the whole partition wiki: features, principles, rules, plans, and notes. The canvas calls it the feature map, so the folder takes that name. `orchestration/` sits beside `partitions/` instead of inside it, because the orchestration agent has no folders and no releases. Do you agree?

**D2. Each container mounts the vault in a few fixed places.**

| Container path | Partition agent | Orchestration agent |
|---|---|---|
| `/hikari/public` | `public/`, read-only | `public/`, read-only |
| `/hikari/self` | its `read-only/`, read-only | `orchestration/read-only/`, read-only |
| `/hikari/can-edit` | its `can-edit/` | `orchestration/can-edit/` |
| `/hikari/staging` | its `staging/` | none |
| `/hikari/private` | its private files | none |
| `/hikari/partitions/<name>` | none | each partition's `read-only/`, read-only |
| `/workspace/...` | its folders (E2) | none |

The layout of the vault decides the mode, so each mount is one directory, and Hikari needs no rule per file. The orchestration agent never mounts another partition's `can-edit/`, `staging/`, or private files. Do you agree?

**D3. `granted/` and `can-edit/` hold files that you put there.** Hikari writes into neither, and it shows both in the UI. In a project repository, both are versioned. Hikari commits a partition agent's changes in `can-edit/` with the agent as author at each apply, and commits your own changes in either folder as it commits your wiki edits today. Do you agree?

**D4. Another partition's releases are granted by path in the topology, not by copying.** A library consumer needs every new version, and a copy in `granted/` would go stale. So `grants.releases: [inventory]` stays, and it mounts `partitions/inventory/read-only/releases/` at `/hikari/grants/inventory/releases`. Any other file you want to share across partitions goes in `public/` or in each partition's `granted/`. Do you agree?

**D5. Policies and deliverables stay yours, in `public/`.** The orchestration agent's tools write only `vocabulary.yaml` and `wiki/`. A policy changes every agent's system prompt, so an agent that changed one would rewrite its own rules. Shared skills and agent definitions leave `public/`: F2 replaces them with skill packs and grants. Do you agree?

## E. Partitions

**E1. A partition is a list of folders, each from any repository.**

```yaml
services:
  orders:
    x-hikari:
      partition:
        folders:
          - {repo: shop, path: services/orders}
          - {repo: shop, path: libs/money, read_only: true}
          - {repo: protos, path: orders}
        test: python3 -m pytest -q ... --junitxml=/hikari/private/reports/all.xml
```

`read:` paths become folders with `read_only: true`, and they can now come from another repository. A read-only folder is never committed. Do you agree?

**E2. Each folder mounts at `/workspace/<repo>/<path>`, unless the folder sets `at:`.** The working directory is `/workspace`. One rule for every partition means a folder from a second repository never moves the paths of the first, so a code map and a test command never break when the partition grows. `at:` covers code that expects a fixed path, such as a Go module. This changes today's `/workspace/<scope>`, so every test command and code map moves once, in the migration (C4). Do you agree?

**E3. Folders keep today's isolation rules.** No two partitions write the same folder or folders that nest, and no folder of one partition, read-only or not, lies inside another partition's writable folder. Reading another partition's code would break the promise that an agent cannot see the code it does not own. Do you agree?

**E4. A partition's private files live in its first writable folder,** at `<folder>/.hikari/private/`, as they do in the scope today. They then change in the same commits as the code they describe. Paths in the code map are workspace paths, such as `shop/services/orders/app.py`, as the agent sees them. Do you agree?

**E5. `apply` commits every writable folder, one commit per repository, on each repository's branch, with one message.** Hikari still applies at the end of each turn and at stop, because `hikari up` refuses a dirty repository, and a change left uncommitted would block your next start. An explicit `apply` then only names the commit. Do you agree, or should only the agent's `apply` commit?

**E6. Branches stay one per repository.** Two partitions that write the same repository share its branch, as today. Do you agree?

**E7. Secrets stay compose's `env_file` and `environment` on the partition's service, and the docs name that as the partition's secrets.** The canvas draws Secrets as a private partition file. A file-based secret would need its own rules: where it lives, how it stays out of git, how the UI hides it. I recommend planting that, and documenting the compose way now. Do you agree, or do you have a case where `env_file` is not enough?

**E8. A repository linked by URL is still cloned into `repos/<name>` in the project repository.** Your glossary says "an internal Hikari folder". `repos/` is ignored by git, and it is the same on every teammate's machine. A project without a project repository can still link only by path. Do you agree?

## F. Agents and Skill Packs

**F1. The built-in skill packs come from the binary, and the UI lists them as granted.** They must match the binary that checks their tools, as `hikari-system` does today, so a copy in the vault would drift. The driver installs them in `~/.claude/skills/` before each turn.

| Pack | Skills |
|---|---|
| Partition agent | `plan-work`, `do-work`, `validate-work`, `map-features` |
| Orchestration agent | `plan-work`, `health-check`, `map-features` |

Today's `hikari-system` skill splits across the partition pack, and `hikari-hub` becomes the orchestration pack, without its hub parts. Do you agree?

**F2. Your own skills live in `vault/skills/<name>/`, and the topology grants them: `grants.skills: [name]`.** Deliverables already work this way. A copy in each partition's `granted/` would drift, and the grant shows in the topology who has which skill. Shared agent definitions, `store/shared/agents/`, go: no example and neither of your projects defines one. Do you agree?

**F3. This round sets the packs' shape, not their content.** `validate-work` is the validation agent planted in `phase-2.md`, now a skill. `plan-work` meets the work orders planted there. Each needs its own round. Until then, `do-work` and `map-features` carry today's text, `validate-work` runs the suites and reads the health, and `plan-work` asks the partition agent to state its plan in the room before a change of more than one feature. Do you agree?

**F4. The orchestration feature map links partition features into larger features.** Its documents have the kinds `feature`, `principle`, and `rule`, as a partition's do. An orchestration feature links the partition features that make it up with `[[id]]`, as a hub story does today. Its health is computed from theirs: failing when one is failing, pending when one is pending, untested when none is tested, and otherwise passing. This is how a hub story shows support today, so no new rule is needed. Do you agree?

**F5. The Hikari agent's skills learn the new model.** `containerize-repo` wraps each repository as one partition with one folder, and writes one orchestration agent. `hikify` designs partitions of folders instead of systems in hubs, drafts the orchestration feature map instead of hub stories, and records E2E suites as left for integration (A5). Its drafts move from `onboarding/wiki/hubs/<hub>/` to `onboarding/vault/`. Do you agree?

**F6. The guide planted as "The hikari files on the host" in `next-phase.md` is written in this work, as `docs/guides/vault.md`.** It is D1 and D2 with a worked example, the shop: the host path of every file and the container path it mounts to. Do you agree?

## G. The UI

**G1. The sidebar has the room, the agents, and the partitions.** Hub pages, the hub diagram, the network map, and the Tests tab go. The room has the Messages, Vocabulary, and Wiki tabs that a hub had. The orchestration agent's page adds its feature map. Do you agree?

**G2. The Structure tab lists the project, its repositories, its agents, and its sidecars.** The project's inspector picks the orchestration agent. A partition agent's inspector edits the folder list: repository, path, read-only, and `at:`. The operations `add_hub`, `set_hub`, `remove_hub`, `add_member`, `remove_member`, and `set_e2e` go, and `set_orchestration` and `set_folders` are new. Do you agree?

**G3. The Map draws the orchestration agent in the middle and each partition around it.** Hub hexagons and hub edges go. An edge from one partition to another means a release grant. Do you agree?

**G4. Each agent's inspector shows its mounts: host path, container path, and mode.** The table comes from the same function that writes the override file, so it cannot disagree with the container. It makes D2 visible where you edit the topology. Do you agree?

---

## Proposed Steps

Each step gets its own spec when the step before it is built, as the topology editor's steps did.

1. **Archive and remove E2E.** The tag, then A1 to A5. Health goes back to plain unit tests. The tests pass, and hubs still exist.
2. **Remove hubs.** B1 to B8: one room, the orchestration agent named at the top, one vocabulary, the public wiki, and no plain agents. The UI's hub surfaces go (G1 and G2, for hubs only).
3. **Rename.** C1 to C3 and C5. A mechanical change with no change of behavior, done in one pass so that the tree is never half renamed. The lint test of C5 lands here.
4. **The vault layout and its mounts.** D1 to D5, F6, and G4.
5. **Partitions of many folders.** E1 to E8, and the folder editor of G2.
6. **Skill packs and the orchestration feature map.** F1 to F4, and G3.
7. **The Hikari agent and the docs.** F5, the README, `design.md`, the guides, and the migration of your two projects (C4).

Steps 1 and 2 shrink the code before step 3 renames it, so the rename touches less. The glossary words reach the code in step 3. Until then, the specs of steps 1 and 2 use today's words.
