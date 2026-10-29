# Spec: deliverables

Status: built. Checked in a live run of the shop: inventory loaded the `python-library` skill, staged its package and README, ran `check_release`, published 0.1.0, and lead checked the release against the deliverable's definition. `sha256sum -c SHA256SUMS` passes in the release directory.

The tester, environments, and image releases were removed in `remove-e2e.md`.

Hubs were removed in `remove-hubs.md`.

Terms changed in `shared-language-names.md`; see `docs/glossary.md`.

`releases.md` makes the record of a release travel with the vault, adds tags, remotes, and pins, and moves the `latest` link under `tags/`.

Sources: `docs/brainstorm/deliverables.md` and `deliverables-answers.md`. This spec replaces section 5 (Releases) of `docs/specs/shared-store.md`, its rules R1 to R11, its decisions 1 and 7 where they concern releases, and the `release-docs` skill (H5).

## Summary

The rules of a release move out of hikari and into the store, as a named package called a deliverable. A deliverable says what a product must contain (layout rules), where it must also be published (targets), and how the owner makes it (a skill). The topology grants deliverables to owners. An owner can hold several deliverables and publishes each one by name. hikari checks the product against the deliverable, writes the manifest with the metadata it knows, and announces the release. You can read every deliverable in the store and in the UI, change it, and see which agents hold it.

## Decisions

All from round 1, all agreed:

1. The package is a **deliverable**. "Release" stays the published thing; "publish" stays the verb.
2. Deliverables live in the shared area, `store/shared/deliverables/<name>/`. The grant decides who publishes one and whose container gets its skill.
3. A package is `deliverable.yaml`, `SKILL.md`, and any files the skill refers to. The directory name is the name.
4. Layout rules are three glob lists: `require`, `allow`, `forbid`.
5. Targets are `github-release`, `huggingface`, and `dockerhub`. A target may pin where the product goes. The agent uploads; hikari never uploads and does not call the service. Certificates come in the publish call.
6. Only owners hold deliverables, through `grants.deliverables`. No deliverable, no publishing; hikari ships no default.
7. A release keeps the product as a directory with a `SHA256SUMS` file, not an archive.
8. hikari writes the whole manifest. The owner-written `release.yaml` and the built-in `release-docs` skill go away.
9. Two tools: `check_release` and `publish_release`.
10. Releases nest by deliverable, each with its own `latest`. Old releases in `releases/<x.y.z>/` get a startup warning and are ignored.

## Terms

- **Deliverable**: a named package of instructions in the store: what to produce, where it must go, and how to make it.
- **Product**: the files that an owner makes for one release of a deliverable.
- **Release**: a published product with its manifest. It never changes.
- **Target**: a promise in a deliverable that the product is also published to a cloud service.
- **Certificate**: the record of one target met, which the owner gives when it publishes.
- **Holder**: an agent that the topology grants a deliverable.
- **Publish**: turn a staged product into a release.

## 1. Topology

```yaml
services:
  inventory:
    x-hikari:
      system: {repo: shop, scope: services/inventory}
      grants:
        deliverables: [python-library]   # the deliverables this owner publishes
  orders:
    x-hikari:
      system: {repo: shop, scope: services/orders}
      grants:
        deliverables: [python-library]
        releases: [inventory]            # unchanged: read-only access to releases
```

- **T13.** `grants.deliverables` is a list of deliverable names. Each name matches `^[a-z][a-z0-9-]{0,31}$` and appears once.
- **T14.** Only an agent with a system can hold deliverables. The topology check refuses a deliverable grant on any other agent.
- **T15.** At startup, the server checks that each granted deliverable exists in the store and is valid (section 3). A missing or invalid deliverable stops the start, with every problem listed. A deliverable in the store that no agent holds is allowed.
- **T16.** A deliverable's name must differ from every shared skill name and from `hikari-system`. The server checks this at startup.

## 2. The store layout

```
store/
  shared/
    deliverables/<name>/
      deliverable.yaml             you write it
      SKILL.md                     you write it; installed in each holder's container
      ...                          other files the skill refers to
  systems/<system>/
    map.yaml
    staging/<deliverable>/         the owner builds the product here
    releases/<deliverable>/
      <x.y.z>/
        release.yaml               written by hikari
        SHA256SUMS                 written by hikari
        product/                   the files the owner staged
        map.yaml                   the public map at release time
      latest -> <x.y.z>
```

- **L7.** `Prepare` creates `shared/deliverables/`, and `staging/` and `releases/` for each system, as before. It does not create `staging/<deliverable>/`; the owner does, by building there.
- **L8.** `releases/<deliverable>/` is created on the first publish of that deliverable.
- **L9.** The store README describes this layout.

## 3. The deliverable format

```yaml
version: 1
description: A Python library that other systems install from the release.
layout:
  require: [README.md, pyproject.toml, "src/**/*.py"]
  allow: [README.md, pyproject.toml, "src/**/*.py", "docs/**/*.md"]
  forbid: ["**/__pycache__/**", "**/*.pyc"]
targets:
  - type: github-release
    repo: acme/inventory
```

- **D1.** `deliverable.yaml` is valid YAML with only the fields above, and `version: 1`.
- **D2.** `description` is required and not empty. It tells the holder and the orchestrator what the product is.
- **D3.** `layout` is optional. Each of its three lists is optional. Every pattern is a valid glob (section 4).
- **D4.** `targets` is optional. Each target has a `type` from section 5 and may set the pin fields of that type. At most one target of each type.
- **D5.** `SKILL.md` is required and not empty. It is a Claude Code skill: hikari installs it as it is and sets no frontmatter. The author decides whether it forks its context.
- **D6.** The server reads a deliverable from the store each time it needs it: at startup (T15), on `check_release`, on `publish_release`, and for the UI. A change applies at the next read. A deliverable that became invalid after startup makes `check_release` and `publish_release` fail with its problems.

## 4. Layout rules

The rules apply to the paths of the product's files, relative to `staging/<deliverable>/`, with `/` as the separator.

- **G1.** `require`: each pattern must match at least one file.
- **G2.** `allow`: when the list is set and not empty, every file must match at least one pattern.
- **G3.** `forbid`: no file may match any pattern.
- **G4.** Glob syntax: `*` matches any run of characters inside one path segment, `?` one character, `[...]` a character class, and `**` any number of segments, including none. A pattern without `/` matches at the top level only. Matching is case-sensitive.
- **G5.** Only regular files count. Directories are not matched, and empty directories are allowed and kept. A link in the product is a problem: the product must not contain links.
- **G6.** The product must hold at least one file, unless the deliverable has targets.
- **G7.** Each failed rule is one problem, named with the pattern and, for `forbid`, the first files that matched. The problems are listed together with the other publish checks.

The three levels: a list of file types is `allow: ["**/*.py", "**/*.md"]`; a block list is `forbid`; an exact map is `require` and `allow` with the same list of paths.

## 5. Targets and certificates

A target promises that the product is also published to a cloud service. The owner uploads with the tools and credentials that compose gives its container. hikari never uploads and does not call the service.

| Type | Pin fields | Certificate fields |
|---|---|---|
| `github-release` | `repo` | `repo` (`owner/name`), `tag`, `url` (https), `assets` (optional: `name`, `sha256`) |
| `dockerhub` | `repository` | `repository`, `tag`, `digest` (`sha256:` + 64 hex) |
| `huggingface` | `repo`, `kind` | `repo`, `kind` (`model`, `dataset`, or `space`), `revision` (40 hex), `url` (https) |

- **C1.** A certificate has a `type` from the table and the fields of its type. `publish_release` takes certificates as a list of flat objects; a field of another type, or an unknown field, is a problem.
- **C2.** Each target of the deliverable needs exactly one certificate of its type. A certificate whose type is not a target of the deliverable is a problem.
- **C3.** Where the target pins a field, the certificate's field must equal it.
- **C4.** hikari checks shapes only: required fields present and not empty, `url` an absolute https URL, `digest` and `sha256` values well-formed, `revision` 40 hex characters, `kind` one of the three.
- **C5.** hikari writes the certificates into the manifest as given, after the checks.

## 6. Publishing

### The manifest

hikari writes `release.yaml`. The owner writes nothing in it.

```yaml
deliverable: python-library
version: 1.2.0
system: inventory
released: 2026-10-22T10:00:00Z
notes: Adds Stock.release.
hikari: 0.0.1
source_commit: 8d6c4f2a...
hubs: [shop]
product:
  path: product/
  files: 12
  bytes: 48213
  digest: sha256:...
map: map.yaml
certificates:
  - type: github-release
    repo: acme/inventory
    tag: v1.2.0
    url: https://github.com/acme/inventory/releases/tag/v1.2.0
```

- **R12.** `SHA256SUMS` lists every file of the product, one line per file in `sha256sum` format (`<hex>  product/<path>`), sorted by path. `sha256sum -c SHA256SUMS` passes from the release directory.
- **R13.** `product.digest` is the SHA-256 of `SHA256SUMS`, written as `sha256:<hex>`. `files` and `bytes` count the product's files.
- **R14.** `hikari` is the version of the server. `hubs` lists the hubs where the release was announced. `source_commit` is the commit that the map describes.

### The tools

Both are owner tools. They refuse, with a message that says so, when the owner holds no deliverable or names one that it does not hold.

- **R15.** `check_release(deliverable)` runs every check of a publish except the version and the certificates: the deliverable is valid (section 3), `staging/<deliverable>/` exists, the layout rules (section 4), the map is current, and the private documents are valid. It reports every problem, or a summary of the product: the number of files and bytes. It changes nothing.
- **R16.** `publish_release(deliverable, version, notes, certificates)` runs the checks of R15, plus: `version` is semver (`x.y.z`, optional pre-release) and greater than every published version of the same deliverable; `notes` is not empty; the certificates meet section 5. When any check fails, it reports every problem and changes nothing.
- **R17.** On success, the server creates `releases/<deliverable>/<version>/`, moves each entry of `staging/<deliverable>/` into its `product/`, writes `SHA256SUMS` and `release.yaml`, copies the public map to `map.yaml`, and points `releases/<deliverable>/latest` at the version. `staging/<deliverable>/` stays, empty.
- **R18.** The server posts a message from the owner in each hub that the owner is a member of: `Released <system> <deliverable> <version>: <notes>`. The wake rules do not change.
- **R19.** A release never changes: the server refuses a version that exists, and every container sees releases read-only.
- **R20.** Feature health does not block a release. The copy of the map records it.

## 7. Containers and the driver

- **H7.** The built-in `release-docs` skill is removed. A shared skill of that name is an ordinary shared skill.
- **H8.** Before each turn, the driver links each deliverable that the agent holds at `~/.claude/skills/<name>`, pointing at `/hikari/public/readonly/shared/deliverables/<name>/`, so that `SKILL.md` and the files beside it resolve. `Self` carries the names in a new field, `deliverables`.
- **H9.** A shared skill with the name of a held deliverable is skipped with a warning in the agent's log, as `hikari-system` is today (H4). Startup refuses the collision (T16); this rule covers a change made while hikari runs.
- **H10.** The dangling-link removal (H6) covers deliverable links, since they point into the shared area.

Prompts:

- **P12.** The owner prompt gets a section "Your deliverables". For each held deliverable: its name, its description, where its definition is, and its staging path. It says: load the skill `<name>` to make the product, build it in `/hikari/public/export/staging/<name>/`, run `check_release`, then `publish_release` with a version and notes, and certificates when the deliverable has targets. An owner with no deliverable reads instead that it cannot publish and must ask the user for a deliverable when a release is needed.
- **P13.** The `hikari-system` skill's "Publish a release" section shrinks to a pointer: releases follow the deliverables in your prompt; keep tests, build caches, and private documents out of a product unless the deliverable asks for them.
- **P14.** The orchestrator prompt lists each member's deliverables with their descriptions. When a release is announced, the orchestrator reads its `release.yaml`, the product's docs, and the deliverable's definition in `/hikari/public/readonly/shared/deliverables/<name>/`, and checks that the product meets what the deliverable promises and what the hub agreed. It cannot change deliverables; it asks the user. `Self` carries the members' deliverables in a new field, `member_deliverables`.
- **P15.** The grants prompt (P10) names the new paths: `releases/<deliverable>/<version>/` with `release.yaml`, `SHA256SUMS`, `product/`, and `map.yaml`, and `releases/<deliverable>/latest`.

## 8. Access

Unchanged from the store spec: an owner writes only its staging area; an orchestrator reads the maps and releases of its visible systems; a release grant mounts `releases/` of a system, which now holds every deliverable of that system; every agent reads the shared area, deliverables included.

## 9. UI

- **U12.** The Shared view gets a Deliverables section. Each entry shows the name, the description, the holders, the targets, the layout rules, and opens to the skill text. The listing endpoint of the shared area returns them.
- **U13.** A system view groups its releases by deliverable, newest first within each, each line with the version, the file count, the source commit, and the time.
- **U14.** The release view is at `#/system/<owner>/release/<deliverable>/<version>`. It shows the manifest, the certificates with their URLs as links, and the files with their digests. It renders `product/README.md` as the main page when it exists.
- **U15.** The release detail endpoint becomes `/api/ui/store/releases/{system}/{deliverable}/{version}`.

## 10. Stores from earlier versions

- **O1.** hikari does not detect or convert stores, maps, vocabularies, or releases in a layout from an earlier version. During development each version starts fresh. A directory under `releases/` that is not a deliverable name is not listed. Remove such files by hand.
- **O2.** The shop example's seed gets `shared/deliverables/python-library/`, and its compose grants it to `inventory` and `orders`. `make demo-repo` resets the store.

## 11. Out of scope

- A check command that hikari runs in the container before it publishes. The skill tells the owner what to run.
- Verifying a certificate against the cloud service.
- Grants on one deliverable of a system.
- Holders without a system.
- hikari uploading anything.
- Other target types. Add one by adding a row to the table in section 5.

## 12. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Topology.** `grants.deliverables`, rules T13 and T14.
2. **Store: deliverables.** The format and its checks (D1 to D5), the glob matcher and layout rules (G1 to G7), targets and certificates (C1 to C4), with tests on temporary stores.
3. **Store: releases.** The new layout (L7 to L9), the manifest (R12 to R14), `SHA256SUMS`, and the checks and the publish (R15 to R17, R19).
4. **Server.** Startup checks (T15, T16), the two node endpoints, the announcement (R18), and `Self` with `deliverables` and `member_deliverables`.
5. **Driver and tools.** Skill links (H7 to H10), the prompts (P12 to P15), `check_release` and `publish_release` in the MCP server.
6. **UI.** Deliverables in the Shared view, releases by deliverable, the release view (U12 to U15).
7. **Example and live test.** The shop's `python-library` deliverable (O2). In a live run, inventory publishes its stock library through the deliverable, lead checks it against the definition, and orders tests against it.
8. **Docs.** README, `docs/design.md`, the store README text, and the store spec's pointer to this spec.
