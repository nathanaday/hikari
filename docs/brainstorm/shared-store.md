# Shared store and releases: round 1

## What I take from your proposal

Isolation works. The next step gives isolated systems a controlled way to share knowledge, so that one system can integrate what another system builds.

- **The store** is one directory on the host that holds every hikari document that is not code. You can open it and see your whole ecosystem: its shared rules, its contracts, and every released version of every system.
- **The shared area** (`store/shared`) reaches every container: the vocabulary, global policies, skills, and agent definitions.
- **Schema grants** (`store/schema/<interface>`) hold contract documents, such as proto files, for one interface. A container gets one when the topology grants it.
- **Releases** (`store/systems/<system>/releases/<x.y.z>`) hold a released deliverable and its user-facing docs. A release can also point to a deliverable that lives elsewhere, such as Docker Hub.
- **Access to releases.** The orchestrator reads the releases of its systems, so it can check that a system released a working deliverable with good docs. Another container can get a read-only grant to a system's releases, for example to integrate a library. Grants are set in the topology, with the other container settings.
- **A doc builder** in each container writes the release docs, in the style that a global skill defines.
- **The future E2E test agent** will test across systems using these releases. It is out of scope here, but this spec must not block it.

Each question below has my recommendation. A short answer like "agree" or "no, because…" is enough.

---

## A. The store

**A1. What is in this spec?** I recommend:

- **In:** the store layout, the shared area, publishing releases, release grants, the doc builder, and a Releases view in the UI.
- **Next spec:** schema grants (`store/schema`). See E5.
- **Later spec:** the E2E test agent.

Do you agree?

**A2. Where the store lives, and how it is versioned.** I recommend:

- A `store:` field in the topology, which defaults to `store/` next to the topology file.
- The store is a plain directory, not a git repo that hikari commits to. Releases are versioned by their immutable version directories, and they can hold large binaries, which git handles badly.
- You can still put `shared/` under git yourself.

Do you want hikari to version the store with git instead?

**A3. "Branch" means a directory.** When you write "the system releases branch", I read it as the directory tree `store/systems/<system>/releases`, not a git branch. Is that right?

**A4. Which documents move into the store.** I recommend:

- **`map.yaml` moves** to `store/systems/<system>/map.yaml`. It is the public surface of the system, so it belongs with the system's releases in the public store, not in the repo.
- **Vocabularies move** to `store/shared/vocabulary/<hub>.yaml`.
- **The private documents stay** in the repo, at `<scope>/.hikari/private`. They describe the code at a commit, so they must change with the code in the same commits. A store outside git would lose that history.

The repo then holds only code and the private documents. Do you agree, or do you want the private documents in the store too?

## B. What a container sees

**B1. One absolute path, `/hikari`, instead of `.hikari` in the scope.** I recommend that every container sees its hikari directories at `/hikari`, not in its working directory:

```
/hikari/private/              read-write   <repo>/<scope>/.hikari/private   (owners only)
/hikari/public/export/        mixed        store/systems/<own system>/      (owners only; see B2)
/hikari/public/readonly/      read-only    store/shared/ and the grants
```

The reasons:

- Orchestrators have no scope, so a `.hikari` directory inside a scope cannot work for them. With `/hikari`, every agent finds its documents at the same path.
- Docker creates an empty directory on the host for every mount point. Mount points inside the scope would put `public/export` and `public/readonly` into your repo.
- The code workspace and the hikari surfaces stay apart.

The `.hikari` directory inside the scope is hidden in the container, so the private documents appear at one path only. Do you agree?

**B2. What the owner can write in `export`.** If the owner could write all of `export`, it could change `map.yaml` without the checks of the map tools, and it could change a release after publishing it. I recommend:

```
/hikari/public/export/map.yaml          read-only   changed only through the map tools
/hikari/public/export/staging/          read-write  the owner prepares its next release here
/hikari/public/export/releases/         read-only   published releases, which never change
```

Do you agree?

## C. The shared area

**C1. What goes into `store/shared`, and who writes it.** I recommend:

```
store/shared/
  vocabulary/<hub>.yaml     written by each hub's orchestrator, through the vocabulary tools
  policies/<name>.md        written by you; rules for every agent
  skills/<name>/SKILL.md    written by you; Claude Code skills that every agent can load
  agents/<name>.md          written by you; Claude Code subagent definitions
```

At startup, the driver connects these to Claude Code in each container:

- Each skill becomes available as a skill, through a link in `~/.claude/skills/`.
- Each agent definition becomes available as a subagent, through a link in `~/.claude/agents/`.
- Every policy goes into the system prompt. Policies are rules, so they must always apply; a skill loads only when the agent chooses to load it.

hikari's own skill, `hikari-system`, stays built into the binary, so it always matches the binary's version.

Do you agree with the four kinds, and with who writes each one?

**C2. Who can read which vocabulary.** Today, only the participants of a hub can read its vocabulary. If `store/shared` reaches every container, every agent can read every hub's vocabulary. Two ways out:

- **(a)** Accept it. Vocabularies hold only names and definitions, never code or system details.
- **(b)** Mount only the vocabularies of an agent's own hubs. That keeps today's rule, at the cost of one mount for each hub.

I recommend (a), because it keeps `shared` one simple mount. You also wrote "the system vocabulary", singular. Do you want one global vocabulary instead of one per hub? If so, who owns it when there are several top-level orchestrators?

## D. Releases

**D1. How a release is published.** I recommend:

1. The owner builds the deliverable and its docs in `/hikari/public/export/staging/`.
2. The owner calls a new tool, `publish_release(version, notes)`.
3. The server checks the staging contents (D3), then moves them to `store/systems/<system>/releases/<version>/`. A release never changes after this.
4. The server writes the manifest fields that it knows: the time, the source commit, and a copy of the public map at that commit.
5. The server points `releases/latest` at the new version.
6. The server posts a message from the owner in the owner's hubs: "Released orders 1.2.0: \<notes\>". That message wakes the orchestrator under the current wake rules.

Do you agree?

**D2. The release manifest.** I recommend one file, `release.yaml`, in each release:

```yaml
version: 1.2.0
system: orders
released: 2026-10-22T10:00:00Z     # written by the server
source_commit: 8d6c4f2              # written by the server
notes: Adds cancel_order.
product:
  kind: files                       # files or remote
  path: product/                    # for files: the deliverable, inside the release
  # for remote:
  # location: {type: docker, ref: docker.io/acme/orders:1.2.0}
  # other types: github-release, huggingface, url
runtime:
  requires: [python >= 3.11]
  install: pip install product/orders-1.2.0.tar.gz
docs:
  index: docs/README.md             # the entry point of the user-facing docs
interfaces: [create_order, cancel_order]   # what this release provides, by vocabulary name
map: map.yaml                       # the copy of the public map, written by the server
```

Please review the fields. In particular: are `runtime` and `interfaces` what you want a consumer to see first?

**D3. What publishing checks.** I recommend these checks, and that a failed check rejects the release:

- The version is valid semver, and greater than every published version.
- The map is current, and the private documents are valid. The release then matches what the map describes.
- `release.yaml` is valid. For `kind: files`, the product path exists. For `kind: remote`, the location is set.
- The docs index exists and is not empty.

Feature health does not block a release. The copy of the map in the release shows it, so a release with a known failing feature stays possible and visible. Do you agree?

**D4. Remote releases are pointers only.** A container has no credentials, so it cannot push an image to Docker Hub or upload to GitHub. I recommend that a remote release is only a manifest that points to an artifact that you or your CI published, and that hikari never uploads anything. Do you agree?

**D5. The doc builder.** You described it as a read-only agent. Claude Code can restrict a subagent's tools, but it cannot restrict which paths a writing tool may change. So I recommend:

- hikari ships a default `doc-builder` subagent with the tools Read, Grep, and Glob only. It reads the code and the private documents, and it returns the docs as text. The owner writes the files into `staging/docs/`. The doc builder then cannot change any file.
- The doc builder follows the skill `store/shared/skills/release-docs/`, if you write one. That skill is where your preferences for the docs' look, tone, and structure go.
- You can replace the default by writing `store/shared/agents/doc-builder.md`.
- The owner prompt tells the owner to run the doc builder before it publishes. The server checks only that the docs exist (D3).

Do you agree?

## E. Grants

**E1. Grants are fixed when a container starts.** A bind mount cannot be added to a running container. I recommend:

- Grants are set in the topology, and change only when the container restarts.
- When the orchestrator sees that a system needs another system's release, it asks you in chat, and you change the topology.
- Grants that change at run time are a later spec.

```yaml
agents:
  orders:
    system: {repo: shop, scope: services/orders}
    grants:
      releases: [inventory]        # read-only: store/systems/inventory/releases
```

Do you agree?

**E2. A grant covers every version of a system.** I recommend that a release grant mounts the whole `releases/` directory of that system, read-only, including `latest`. The consumer chooses a version, and records the version its integration tests use in its own test plan. Pinning a version in the topology is a later spec. Do you agree?

**E3. Grants are not limited to hubs.** A system can integrate a library from a system in another hub, such as a platform team's library. I recommend that the topology can grant any system's releases, and that hikari checks only that the system exists and is not the agent's own. Do you agree?

**E4. What an orchestrator sees by default.** You wrote that the lead agent has access to the entire releases tree. I recommend limiting this to the releases and maps of the systems it already sees, the direct members of its hubs. That matches the current access rules. A top orchestrator then sees what its sub-orchestrators publish only if you grant it. Do you agree?

**E5. Schema grants: next spec, or this one?** Contracts such as proto files raise their own questions:

- Who writes `store/schema/<interface>`: the provider, or the orchestrator as the owner of the contract?
- Are schemas versioned with the provider's releases, or on their own?
- How does an interface name such as `POST /orders` become a directory name?

A release can already carry the contract documents that it implements in its docs. I recommend leaving `store/schema` for the next spec. Do you agree, or do you need schemas now?

## F. UI

**F1. A Releases view.** I recommend:

- A Releases section in each system view: the versions, newest first, with each one's notes, time, source commit, product kind, and a link to its docs.
- A docs viewer that renders the release's Markdown docs.
- A Shared view that lists the policies, skills, and agents in `store/shared`.

You can always open the store directly in your editor. Is this enough, or should the UI show more of the store?

---

## What these recommendations add up to

```
store/
  shared/
    vocabulary/<hub>.yaml
    policies/<name>.md
    skills/<name>/SKILL.md
    agents/<name>.md
  systems/
    orders/
      map.yaml
      staging/                       owner's next release
      releases/
        1.0.0/
          release.yaml
          product/
          docs/README.md
          map.yaml                   the map at release time
        1.1.0/
        latest -> 1.1.0
```

The container of `orders`, which has a grant for `inventory`:

```
/workspace/services/orders/          the scope (code)
/hikari/private/                     layout.yaml, test.yaml (read-write)
/hikari/public/export/map.yaml       (read-only)
/hikari/public/export/staging/       (read-write)
/hikari/public/export/releases/      (read-only)
/hikari/public/readonly/shared/      (read-only)
/hikari/public/readonly/systems/inventory/releases/   (read-only, from the grant)
```

The container of `lead`:

```
/hikari/public/readonly/shared/
/hikari/public/readonly/systems/orders/      map.yaml and releases/ (read-only)
/hikari/public/readonly/systems/inventory/
```
