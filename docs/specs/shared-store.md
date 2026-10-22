# Spec: the shared store and releases

Status: built. Section 5 (Releases), rules R1 to R11, and the `release-docs` skill are replaced by `docs/specs/deliverables.md`.

Changes made during the build:

- Orchestrators also learn which release grants their members have, so that they can plan tests against releases. They cannot change grants; they ask the user.
- In the release view, a relative link between docs opens the linked file in the viewer.
- The `hikari-system` skill tells owners to keep tests, build caches, and private documents out of the product. A live run showed `__pycache__` files in a release.

Sources: `docs/brainstorm/shared-store.md` and `shared-store-answers.md`.

## Summary

All hikari documents that are not code move into one directory, the store. Its shared area reaches every container: the hub vocabularies, and your policies, skills, and agent definitions. Each system gets a directory with its public map and its releases. An owner publishes a release, a deliverable and its user-facing docs, which then never changes. The orchestrator reads the releases of its systems, and the topology can grant any agent read-only access to another system's releases, for example so that it can integrate a library. Every container sees its hikari documents at one path, `/hikari`.

## Decisions to approve

The brainstorm left these points open, or a decision there needed more detail. Please approve or change each one.

1. **The doc builder is a built-in skill, `release-docs`.** It runs in its own context (`context: fork`) and writes the docs into `staging/docs/`. It sets `background: false`, so that the turn waits for the docs before the owner continues. If you write `store/shared/skills/release-docs/`, it replaces the built-in skill.
2. **Old maps and vocabularies are not moved or detected.** A map left in a repo at `<scope>/.hikari/map.yaml`, or a vocabulary left in `vocabulary/` next to the topology, is ignored without a warning. hikari does not track layouts of earlier versions.
3. **The orchestrator does not see an owner's staging area.** An empty, read-only directory hides `staging/` in the orchestrator's container. The orchestrator checks releases, not drafts.
4. **A consumer sees releases only, not maps.** A release grant mounts only `releases/` of the other system. Each release carries a copy of its system's public map, so a consumer still sees what the release provides.
5. **The driver refreshes shared knowledge before each turn.** It reads the policies again and links any new skills and agent definitions. A change in `store/shared` therefore applies from the next turn, without a restart.
6. **hikari writes `store/README.md` once,** when the file does not exist. It explains the store layout, for you and for anyone else who opens the store.
7. **No new tools for reading releases.** Releases are files in the mounted directories, so agents read them with their normal file tools. The only new tool is `publish_release`.

## Terms

- **Store**: the directory that holds every hikari document that is not code.
- **Shared area**: `store/shared`, which every container can read.
- **Release**: a published version of a system: a manifest, a deliverable or a pointer to one, and user-facing docs. A release never changes after it is published.
- **Staging area**: `store/systems/<system>/staging`, where an owner prepares its next release.
- **Release grant**: read-only access, set in the topology, to another system's releases.

## 1. Topology

```yaml
store: store                # optional; the default is store/ next to the topology file

agents:
  orders:
    system: {repo: shop, scope: services/orders}
    grants:
      releases: [inventory] # read-only access to store/systems/inventory/releases
```

- **T10.** `store` is a path. `~` expands, and a relative path starts at the topology file's directory. The default is `store`.
- **T11.** Each name in `grants.releases` is a system (an owner) in the topology, and not the agent's own system.
- **T12.** Any agent can have release grants, including members without a system and orchestrators.

## 2. The store layout

```
store/
  README.md                          written once by hikari (decision 6)
  shared/
    vocabulary/<hub>.yaml            written by each hub's orchestrator, through the vocabulary tools
    policies/<name>.md               written by you
    skills/<name>/SKILL.md           written by you
    agents/<name>.md                 written by you
  systems/
    <system>/
      map.yaml                       written by the server, through the owner's map tools
      staging/                       written by the owner
      releases/
        <x.y.z>/
          release.yaml
          product/                   when the product kind is files
          docs/
          map.yaml                   the public map at release time
        latest -> <x.y.z>
```

- **L1.** At startup, the server creates every directory above that does not exist: `shared/` with its four subdirectories, and `staging/` and `releases/` for each system in the topology.
- **L2.** The store is a plain directory. hikari does not commit to it.
- **L3.** The map of each system moves from `<scope>/.hikari/map.yaml` to `store/systems/<system>/map.yaml`. Its format and its rules do not change.
- **L4.** The vocabulary of each hub moves from `vocabulary/<hub>.yaml` to `store/shared/vocabulary/<hub>.yaml`. Its format and its rules do not change.
- **L5.** The private documents stay in the repo, at `<scope>/.hikari/private`.
- **L6.** hikari does not look for files in the places of L3 and L4. It ignores them without a warning (decision 2).

## 3. What a container sees

Every agent sees its hikari documents under `/hikari`.

An owner:

```
/workspace/<scope>/                   read-write   the code
/hikari/private/                      read-write   <repo>/<scope>/.hikari/private
/hikari/public/export/                read-only    store/systems/<own system>/ (map.yaml, releases/)
/hikari/public/export/staging/        read-write   store/systems/<own system>/staging/
/hikari/public/readonly/shared/       read-only    store/shared/
/hikari/public/readonly/systems/<s>/releases/   read-only, for each release grant
```

An orchestrator:

```
/hikari/public/readonly/shared/       read-only
/hikari/public/readonly/systems/<s>/  read-only    store/systems/<s>/ for each visible system
/hikari/public/readonly/systems/<s>/staging/    an empty read-only directory (decision 3)
/hikari/public/readonly/systems/<s>/releases/   read-only, for each release grant outside its visible systems
```

A member without a system sees `shared/` and its release grants.

- **M7.** The `.hikari` directory inside an owner's scope is hidden by an empty read-only directory, so that the private documents appear only at `/hikari/private`.
- **M8.** No container sees another system's staging area, private documents, or code.
- **M9.** The map file is not mounted by itself. Its directory is mounted, because the server replaces the file with an atomic rename.

## 4. The shared area in Claude Code

Before each turn, the driver connects the shared area to Claude Code (decision 5):

- **H1.** Each directory `shared/skills/<name>/` that holds a `SKILL.md` gets a link at `~/.claude/skills/<name>`.
- **H2.** Each file `shared/agents/<name>.md` gets a link at `~/.claude/agents/<name>.md`.
- **H3.** Every file `shared/policies/*.md`, in name order, goes into the system prompt under a `# Policies` heading, with each file's name.
- **H4.** The built-in skill `hikari-system` is always the version in the binary. A shared skill with that name is ignored, with a warning in the agent's log.
- **H5.** The built-in skill `release-docs` is installed for owners, unless `shared/skills/release-docs/` exists. Then the shared skill takes its place (decision 1).
- **H6.** The driver removes the links of skills and agents that no longer exist in the shared area.

## 5. Releases

### The manifest

The owner writes `staging/release.yaml`. The server adds the fields that it knows when it publishes.

```yaml
version: 1.2.0                      # written by the server, from publish_release
system: orders                      # written by the server
released: 2026-10-22T10:00:00Z      # written by the server
source_commit: 8d6c4f2              # written by the server: the map's described commit
notes: Adds cancel_order.           # written by the server, from publish_release
product:
  kind: files                       # files or remote
  path: product/                    # for files: the deliverable, inside the release
  # for remote:
  # location: {type: docker, ref: docker.io/acme/orders:1.2.0}
runtime:
  requires: [python >= 3.11]
  install: pip install product/orders-1.2.0.tar.gz
  run: python -m orders
docs:
  index: docs/README.md
interfaces: [create_order, cancel_order]
map: map.yaml                       # written by the server
```

### Publishing

An owner calls `publish_release(version, notes)`.

- **R1.** The server checks, and rejects the release with every problem listed when a check fails:
  - `version` is valid semver (`x.y.z`, with an optional pre-release suffix) and greater than every published version.
  - The map is `current`, and the private documents are valid.
  - `staging/release.yaml` is valid YAML with only the fields above.
  - For `kind: files`, `product.path` exists in the staging area. For `kind: remote`, `location.type` is one of `docker`, `github-release`, `huggingface`, or `url`, and `location.ref` is set.
  - `docs.index` exists in the staging area and is not empty.
  - Each name in `interfaces` is an interface that the map provides.
- **R2.** Feature health does not block a release. The copy of the map in the release records it.
- **R3.** The server moves the contents of `staging/` into `releases/<version>/`, entry by entry. It does not rename `staging/` itself, because the owner's container mounts that directory. `staging/` is empty afterwards.
- **R4.** The server writes the final `release.yaml` with its fields filled in, and copies the current public map to `releases/<version>/map.yaml`.
- **R5.** The server points the relative link `releases/latest` at the newest version.
- **R6.** The server posts a message from the owner in each hub that the owner is a member of: `Released <system> <version>: <notes>`. Under the current wake rules, this message wakes each hub's orchestrator.
- **R7.** A release never changes after it is published: the server refuses a version that exists, and every container that can see a release sees it read-only.
- **R8.** hikari never uploads anything. A remote release only points to an artifact that you or your CI published.

### The doc builder

- **R9.** The built-in skill `release-docs` has `context: fork` and `background: false`. It writes user-facing docs for the next release into `staging/docs/`, with `README.md` as the index. Its sources are the code, the private documents, and the public map.
- **R10.** The docs cover what the product is, how to install and run it, how to use each interface it provides, with examples, its configuration, its errors and limits, and what changed in this version.
- **R11.** The skill tells the doc builder to change only files under `staging/docs/`.

## 6. Access

- **A6.** An owner reads and writes its own staging area, and reads its own map and releases.
- **A7.** An orchestrator reads the maps and releases of its visible systems, the direct members of the hubs it orchestrates.
- **A8.** An agent reads the releases of each system in its `grants.releases`.
- **A9.** Every agent reads the shared area. Only each hub's orchestrator changes its hub's vocabulary, as today.
- **A10.** The UI reads the whole store.

## 7. Prompts and tools

- **P7.** New owner tool: `publish_release(version, notes)`.
- **P8.** The owner prompt and the `hikari-system` skill name the private documents at `/hikari/private/`, and describe the release flow: build the deliverable in `staging/`, run the `release-docs` skill, write `release.yaml`, then call `publish_release`.
- **P9.** The orchestrator prompt says where the releases of its systems are, and that it must check a new release: that its docs explain how to use what it provides, and that its interfaces match what was agreed.
- **P10.** The prompt of an agent with release grants lists each granted system and the path of its releases. It tells the agent to choose a version, and to record that version in its test plan when its tests use it.
- **P11.** Every prompt says that `/hikari/public/readonly/shared/` holds the hub vocabularies, policies, skills, and agents.

## 8. UI

- **U7.** A system view gets a Releases section: the versions, newest first, each with its notes, time, source commit, and product kind, and a link to its docs.
- **U8.** A docs view (`#/system/<name>/release/<version>`) renders the release's docs index, and lists the other files of the release. A Markdown file renders as Markdown; another text file shows as text.
- **U9.** A Shared view (`#/shared`) lists the policies, skills, and agents in `store/shared`. A policy or skill opens to show its text.
- **U10.** The server serves release files and shared files to the UI as text only, never outside the store, and up to a size limit.
- **U11.** A new release reaches the UI as an event, without a reload.

## 9. Out of scope

- Schema grants (`store/schema`). A system that owns a contract publishes it in its releases.
- Pinning a version of a release in the topology.
- Grants that change while a container runs.
- Uploading releases, and building them in CI or in the cloud.
- The E2E test agent.

## 10. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Topology.** `store` and `grants.releases`, rules T10 to T12.
2. **Store.** Layout and creation (L1 to L6). The map and the vocabularies move into the store.
3. **Releases.** Manifest format, the checks of R1, publishing (R3 to R5, R7), and the announcement (R6), with tests on temporary stores.
4. **Mounts.** The container layout of section 3 for owners, orchestrators, and members (M7 to M9).
5. **Driver.** Links and policies from the shared area (H1 to H6), the `release-docs` skill, and the prompts (P8 to P11). `publish_release` in the MCP server.
6. **UI.** Releases section, docs view, and Shared view (U7 to U11).
7. **Demo and live test.** The demo grants `orders` the releases of `inventory`. In a live run, inventory publishes its stock library with docs, lead checks the release, and orders tests its cancellation against the released library, which is the cross-system test that the last run could not do.
8. **Docs.** README, `docs/design.md`, and the system maps spec where the map and the vocabularies moved.
