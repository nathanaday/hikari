# Spec: the vault layout and its mounts

Status: built. Docker was not running, so the live check of 2026-10-26 ran without containers: `hikari check` passed on the shop and failed on a copy with `store/`, `shared/`, `systems/`, and `layout.yaml`, naming each move; with `hikari up -no-launch`, the vault took the layout of V1, the orchestration agent's `define_term` through `hikari mcp` landed in `vault/public/vocabulary/terms.yaml`, the shop's override file (written by a throwaway test, because `-no-launch` writes none) gave each agent the mounts of V3, and headless Chrome showed the same mounts in each inspector and the policies, the deliverable, and the granted skill on the Vault page, with no console error. No container ran, so `docker inspect`, the driver's skill links, and a turn's commit of `can-edit/` were checked only by tests.

Changes made during the build:

1. **A release copies the feature map as `features/`.** It holds `map.yaml` and the agent documents, and the manifest says `map: features/map.yaml`. An older release keeps `map.yaml` and `wiki/`, and stays readable, because its manifest names its map.
2. **The orchestration agent's `can-edit/` is committed at the end of each of its turns.** It has no `apply`, so the driver now asks the server to commit after its turns too, and the server commits only its `can-edit/`.
3. **At stop, what is left in a `can-edit/` is yours.** Every turn ends with an apply and the containers stop before the final commits, so `CommitAll` commits every `can-edit/` and `granted/` with your identity.
4. **Your edits are found by `partitions.IsUserPath`,** which replaces `IsWikiPath`: a document of a wiki, or a file of a `can-edit/` or a `granted/`. `map.yaml` now sits in `features/`, so the user's commit of a feature map leaves it out with an `:(exclude)` pathspec, which `gitrepo.CommitPaths` now takes.
5. **The checks of V7 and V11 are start checks** in `linked.Check`, which `hikari check`, `hikari up`, and the topology editor's lint share. A skill's name, a skill granted twice, and the `hikari-` prefix are topology errors.
6. **The topology API gives `mounts` in the state and in the plan,** keyed by service id, from `launcher.AgentMounts`. A host path is relative to the topology file's directory when it is inside it, and absolute otherwise, as for the shop's repository; the empty directory that hides a scope's `.hikari` has an empty host path. `/api/ui/topology/options` lists the vault's skills, and the agent inspector edits `grants.skills` for every agent.
7. **The UI's listing is `/api/ui/vault/public`:** the policies, the skills with the agents they are granted to, and the deliverables. The sidebar's Shared is Vault. A release's detail gives its directory, so the UI no longer builds the path from the partition's name.
8. **The shop's seed grants a skill.** `skills/python-style` is granted to both partition agents, so a run of the shop shows the mount and the link. The old `examples/shop/store` of earlier runs is in `.hikari/demo/shop-store-before-vault`.
9. **`hikari status` and `hikari sync` read a map in the project repository by the partition's id,** through the vault's own path; before, they used the name.

Sources: `docs/brainstorm/shared-language.md` (questions D1 to D5, F2, F6, and G4) and `shared-language-answers.md`. This is step 4 of the seven steps of the shared language. The names are those of `docs/glossary.md`.

## Summary

The vault takes the layout of the Phase 2 canvas. Public files sit in `public/`. Each partition has a `read-only/` part, which its partition agent and the orchestration agent read, a `can-edit/` part, which its partition agent writes, and a staging area. The orchestration agent has the same two parts in `orchestration/`. The directory a file is in decides who mounts it and how, so each container mounts a few fixed directories at fixed paths under `/hikari`. Your own skills move out of the shared area into `vault/skills/`, and the topology grants them to agents, as it grants deliverables. A guide shows where every file of the shop lives on the host and where it appears in each container, and the Structure tab shows each agent's mounts.

## Decisions

From the round and its answers:

1. The vault follows the canvas: `public/`, `partitions/<partition>/{read-only,can-edit,staging}`, and `orchestration/{read-only,can-edit}` (D1).
2. Each container mounts the vault in a few fixed places (D2).
3. `granted/` and `can-edit/` hold files that you put there; Hikari commits an agent's changes in `can-edit/` at each apply (D3).
4. Another partition's releases are granted by path in the topology (D4).
5. Policies and deliverables stay yours, in `public/`. The tools of the orchestration agent write only the vocabulary and the public wiki (D5).
6. Your skills live in `vault/skills/<name>/`, granted with `grants.skills`. Shared agent definitions go (F2).
7. `docs/guides/vault.md` is the worked example of the shop (F6).
8. Each agent's inspector shows its mounts, from the function that writes the override file (G4).
9. The vocabulary is `public/vocabulary/terms.yaml` (B6 and its answer).

## Decisions made in this spec

1. **The default vault is `vault/` next to the topology file.**
2. **`map.yaml` sits beside the documents, in `features/`.** The feature map is one directory, so a release copies it whole and Obsidian shows it as one folder.
3. **Hikari moves nothing from an old layout, and refuses to start on one.** A vault that holds `shared/` or `systems/`, a project with `store/` and no `vault/`, and a private directory that holds `layout.yaml` and no `code-map.yaml` each fail `hikari check` with the move that fixes it, which `docs/guides/migrating.md` (step 7) lists. A silent start on an old layout would show empty maps.
4. **`layout.yaml` is `code-map.yaml` on disk.** Step 3 renamed the concept; this step moves the files, with the rest of the layout.
5. **A granted skill mounts at `/hikari/skills/<name>`, and the driver links it into `~/.claude/skills/`.** A skill's grant then also bounds what the container can read.

## Requirements

### The layout

- **V1.** A vault is laid out as follows. Hikari prepares every directory that the topology needs at start, and writes `README.md` once.

```
vault/
  README.md
  .gitignore                    partitions/*/staging/ and partitions/*/read-only/releases/
  public/
    vocabulary/terms.yaml       the orchestration agent, through tools
    wiki/                       the orchestration agent, through tools; your plans and notes
    policies/<name>.md          yours
    deliverables/<name>/        yours: deliverable.yaml and SKILL.md
  skills/<name>/                yours: SKILL.md and its files
  partitions/<id>/
    read-only/
      features/                 map.yaml and the documents of the feature map
      releases/<deliverable>/<version>/
      granted/                  yours
    can-edit/                   yours, and the partition agent's
    staging/<deliverable>/      the partition agent's
  orchestration/
    read-only/
      features/                 the orchestration feature map (step 6)
      granted/                  yours
    can-edit/                   yours, and the orchestration agent's
```

- **V2.** `orchestration/` exists only when the topology names an orchestration agent. A partition's directory is keyed by its partition agent's id, as `systems/<id>` was.

### Mounts

- **V3.** Each container mounts the vault as follows, and nothing else of it.

| Container path | Partition agent | Orchestration agent | Mode |
|---|---|---|---|
| `/hikari/public` | `public/` | `public/` | read-only |
| `/hikari/self` | `partitions/<id>/read-only/` | `orchestration/read-only/` | read-only |
| `/hikari/can-edit` | `partitions/<id>/can-edit/` | `orchestration/can-edit/` | writable |
| `/hikari/staging` | `partitions/<id>/staging/` | none | writable |
| `/hikari/private` | its private files | none | writable |
| `/hikari/partitions/<name>` | none | each partition's `read-only/` | read-only |
| `/hikari/grants/<name>/releases` | each granted partition's `read-only/releases/` | none | read-only |
| `/hikari/skills/<name>` | each granted skill | each granted skill | read-only |

- **V4.** No container mounts another partition's `can-edit/`, `staging/`, or private files. The empty directories that hid `staging/` inside a mounted system directory are gone, because no mount contains a staging area.
- **V5.** A user's bind mount cannot overlap the vault, as it could not overlap the store.

### Grants

- **V6.** `grants.releases` keeps its meaning and mounts as in V3.
- **V7.** `grants.skills` lists skills of `vault/skills/`. Any agent can hold a skill. `hikari check` refuses a skill that does not exist, that has no `SKILL.md`, or whose name is a deliverable's or starts with `hikari-`. The driver links each granted skill into `~/.claude/skills/` before each turn, as it links a deliverable's skill.
- **V8.** Shared agent definitions are gone: no directory, no link into `~/.claude/agents/`, and no check. Hikari's own `hikari-` agents still come from the binary.

### Writes and commits

- **V9.** The orchestration agent's tools write `public/vocabulary/terms.yaml` and `public/wiki/`, and no other public file. Hikari never writes `policies/`, `deliverables/`, `skills/`, or a `granted/` directory.
- **V10.** In a project repository, an apply also commits the agent's changes in its `can-edit/`, with the agent as author. Your own changes in `can-edit/` and `granted/` are committed with your identity at start and at stop, as your wiki edits are.

### Old layouts

- **V11.** `hikari check` and `hikari up` refuse, with the move that fixes each one: a vault that holds `shared/` or `systems/`; a topology without `x-hikari.vault` whose directory holds `store/` and no `vault/`; and a partition's private directory that holds `layout.yaml` and no `code-map.yaml`.

### Agents and UI

- **V12.** The prompts, the skills, and the README name the paths of V3.
- **V13.** The UI's vault views read the new layout: the public area (vocabulary, wiki, policies, deliverables), your skills, and each partition's releases.
- **V14.** The topology API's plan gives each agent's mounts (container path, host path relative to the project, and mode) when the planned text passes the checks, from the function that writes the override file. The agent's inspector shows them as a table, and says why when the text does not pass.

### Docs

- **V15.** `docs/guides/vault.md` lays out the shop's vault and lists, for each file, its host path and its container path in the partition agent `orders`, the partition agent `inventory`, and the orchestration agent. The README links it.
- **V16.** `docs/design.md`'s sections on the store and on `/hikari` describe the vault and its mounts, with the reason for D2: the directory decides the mode, so one mount per directory needs no rule per file.

### Examples

- **V17.** `examples/shop/vault` and `examples/shop/seed` have the new layout, and `make demo-repo` resets the shop's vault from the seed.

### Checks

- **V18.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. A test of the override file checks the mounts of V3 for a partition agent with a release grant and a skill grant, and for the orchestration agent.
- **V19.** A live check of the shop: `hikari check` passes; `hikari check` on a copy with an old `store/` layout fails with the move; with `hikari up`, each container's mounts match V3 (`docker inspect`, or the override file when Docker is not running); the orchestration agent defines a term and it lands in `public/vocabulary/terms.yaml`; and the inspector shows the mounts.
