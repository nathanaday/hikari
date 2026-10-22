# Deliverables: round 1

## What I take from your proposal

Today an owner can publish one kind of release, and the rules of that release live in three places: the `release.yaml` format in the store code, the "Publish a release" section of the `hikari-system` skill, and the `release-docs` skill. You cannot see from the store what a system will deliver, and you cannot change the rules without changing hikari.

You want those rules in the store, as a named package of files:

- **A layout schema** that says what the product must contain, at the level of detail you choose: a list of allowed file types, a block list, or an exact map of every file.
- **Cloud promises**, optional, that say where the product must also be published: GitHub releases, Hugging Face, or Docker Hub. When the agent publishes, each promise becomes a certificate with the details.
- **A skill** that tells the agent how to produce the release.

Binding a package to a container makes its agent able to publish it, and the agent knows the package by name. One agent can hold several packages, and you can tell it which one to publish. hikari keeps the metadata: who published, when, from which commit, with which hikari version, and what the release holds.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

I use these words below. A1 asks whether you want them.

- **Deliverable**: a named package of instructions in the store: what to produce, where it must go, and how to make it.
- **Product**: the files that an agent makes for one release of a deliverable.
- **Release**: a published product with its manifest. It never changes.
- **Target**: a promise in a deliverable that the product is also published to a cloud service.
- **Certificate**: the record of one target met, which the agent gives when it publishes.
- **Publish**: turn a staged product into a release.

## What changes in the current design

1. The owner no longer writes `release.yaml`. Its fields go away: `product.kind`, `product.path`, `runtime`, `docs.index`, and `interfaces`. The deliverable says what the product must contain, and hikari writes the manifest.
2. The built-in `release-docs` skill goes away. The deliverable's skill says what docs the product needs and how to write them.
3. A remote product (`kind: remote`) becomes a target with a certificate.
4. `releases/<version>/` becomes `releases/<deliverable>/<version>/`, and `staging/` holds one directory for each deliverable.
5. An owner publishes only the deliverables that the topology grants it. There is no default.

---

## A. The package

**A1. The name.** I recommend **deliverable** for the package, and the terms above. "Release" stays the published thing, and "publish" stays the verb, so the current tool, prompts, and UI keep their words. The alternatives are "release type" (`releases/python-library/1.2.0` reads the same) and "publication", which would rename everything.

Do you agree?

**A2. Where the packages live.** I recommend `store/shared/deliverables/<name>/`, in the shared area:

- Every container already mounts the shared area, so an orchestrator can check a release against its deliverable, and a consumer can read what it gets.
- The driver refreshes the shared area before each turn, so an edit to a deliverable applies from the agent's next turn, without a restart.
- The grant (section B) decides who publishes a deliverable and whose container gets its skill. Reading a definition gives no power.

The alternative is a new area, `store/deliverables/`, mounted into each container only when granted. It hides definitions from agents that do not hold them, but needs a mount for each grant, which takes a restart.

Do you agree?

**A3. The files of a package.** The directory name is the deliverable's name, so the name has one source. The package holds:

```
store/shared/deliverables/<name>/
  deliverable.yaml      the description, the layout rules, and the targets
  SKILL.md              how to produce the product; a Claude Code skill
  ...                   any other files the skill refers to, such as templates
```

```yaml
version: 1
description: A Python library that other systems install from the release.   # required
layout:                        # optional; rules on the files of the product (A4)
  require: [README.md, pyproject.toml, "src/**/*.py"]
  allow: [README.md, pyproject.toml, "src/**/*.py", "docs/**/*.md"]
  forbid: ["**/__pycache__/**", "**/*.pyc"]
targets:                       # optional; where the product must also go (A5)
  - type: github-release
    repo: acme/orders
```

hikari installs `SKILL.md` as the skill `<name>` in the container of each agent that holds the deliverable. hikari sets no frontmatter: the author of the skill decides whether it forks its context, as `release-docs` did. One package has one skill.

Do you agree?

**A4. The layout rules.** Three lists of glob patterns, each optional, matched against the paths of the product:

- `require`: each pattern must match at least one file.
- `allow`: when set, every file must match at least one pattern.
- `forbid`: no file may match any pattern.

They give your three levels: a list of file types is `allow: ["**/*.py", "**/*.md"]`; a block list is `forbid`; an exact map is `require` and `allow` with the same list of paths. Patterns use `**` for any directories, and a pattern without `/` matches at the top only. A product must hold at least one file, unless the deliverable has targets.

Do you agree?

**A5. Targets and certificates.** A target promises that the product is also published to a cloud service. Three types for now: `github-release`, `huggingface`, and `dockerhub`. A target names its type and may pin where it goes. When the promise pins a field, the certificate must match it; when it does not, the certificate says where the product went, so one deliverable can serve several systems.

```yaml
targets:
  - type: github-release
    repo: acme/orders              # optional pin
  - type: dockerhub
    repository: acme/orders        # optional pin
  - type: huggingface
    repo: acme/orders-model        # optional pin
    kind: model                    # optional pin: model, dataset, or space
```

The agent uploads with the tools and credentials that compose gives its container; hikari never uploads (rule R8 stays). When it publishes, the agent gives one certificate for each target, in the `publish_release` call. hikari checks each certificate against its promise and writes them into the manifest:

```yaml
certificates:
  - type: github-release
    repo: acme/orders
    tag: v1.2.0
    url: https://github.com/acme/orders/releases/tag/v1.2.0
    assets: [{name: orders-1.2.0.tar.gz, sha256: "..."}]   # optional
  - type: dockerhub
    repository: acme/orders
    tag: "1.2.0"
    digest: sha256:...
  - type: huggingface
    repo: acme/orders-model
    kind: model
    revision: 8d6c4f2...
    url: https://huggingface.co/acme/orders-model
```

hikari checks the shape of each certificate: the required fields of its type, a URL that parses, a digest that looks like one. It does not call the service to verify, because that needs network access and credentials on the server. The orchestrator can open the URL. Remote verification can come later.

I chose the tool call over certificate files in the staging area so that the product stays exactly what the agent made, and the layout rules never have to make room for hikari's files.

Do you agree with the three types, the pins, and the tool call?

## B. Binding a deliverable to an agent

**B1. The grant.** A deliverable binds to an agent in the topology:

```yaml
services:
  orders:
    x-hikari:
      system: {repo: shop, scope: services/orders}
      grants:
        deliverables: [python-library, docker-image]
        releases: [inventory]
```

I recommend that only owners can hold deliverables for now. An owner has code to build from, a workspace, a map and a commit that give the release its provenance, and a place in the store and the UI. A member without a system has none of these. The topology check refuses a deliverable grant on an agent without a system. Lifting that later is a small change in the same places.

Do you agree? Or do you want any agent to publish, for example a writer that delivers a set of files? Then a granted agent without a system would get a staging area and releases under `store/systems/<agent>/`, a manifest without a map or a commit, and a place in the UI.

**B2. No deliverable, no publishing.** An owner with no deliverable grant cannot publish, and the tools say so. hikari ships no built-in deliverable. The examples ship deliverables in their stores, which also show how to write one: `python-library` for the shop, where `inventory` publishes its stock library. Today's `release-docs` guidance moves into that example skill.

Do you agree?

**B3. Skill names.** The skill of a deliverable is installed under the deliverable's name. The topology check refuses a deliverable named like a shared skill or a built-in skill (`hikari-system`), so an agent always knows which skill a name means.

Do you agree?

## C. Publishing

**C1. Staging.** The owner builds each product in its own directory: `/hikari/public/export/staging/<deliverable>/`. The whole `staging/` mount stays read-write, as today. `publish_release` moves the entries of `staging/<deliverable>/`, so the other deliverables' drafts stay.

Do you agree?

**C2. A directory, not an archive.** You proposed that the tool packs the product into a `.tar.gz`. I recommend that the release keeps the product as a directory, with a `SHA256SUMS` file beside it:

- Consumers install and read with normal tools, as decision 7 of the store spec set: no tools for reading releases. An archive would make every consumer extract first.
- The UI renders the docs and lists the files of a release from the directory.
- The read-only mounts already make a release immutable. `SHA256SUMS` lets anyone check it, with `sha256sum -c`, and the manifest records the digest of that file as the digest of the product.
- When a target needs an archive, for example a GitHub release asset, the skill tells the agent to build one as part of the product, and the certificate records its digest.

Do you agree?

**C3. The manifest.** hikari writes `release.yaml`; the owner writes nothing in it.

```yaml
deliverable: python-library
version: 1.2.0
system: orders
released: 2026-10-22T10:00:00Z
notes: Adds cancel_order.
hikari: 0.0.1
source_commit: 8d6c4f2
hubs: [shop]                   # where the release was announced
product:
  path: product/
  files: 12
  bytes: 48213
  digest: sha256:...           # of SHA256SUMS
map: map.yaml
certificates: []               # section A5
```

The release directory:

```
releases/<deliverable>/<version>/
  release.yaml
  SHA256SUMS
  product/
  map.yaml
releases/<deliverable>/latest -> <version>
```

Do you agree?

**C4. The tools.** Two owner tools:

- `check_release(deliverable)`: checks the staged product against the layout rules and reports every problem, without publishing. The skill tells the agent to run it before it publishes.
- `publish_release(deliverable, version, notes, certificates)`: runs the same checks plus the map and certificate checks, then publishes. It rejects with every problem listed, as today.

The announcement in the owner's hubs becomes `Released <system> <deliverable> <version>: <notes>`. The wake rules stay: it wakes each hub's orchestrator.

Do you agree?

**C5. Versions.** Semver, greater than the last version of the same deliverable. Each deliverable has its own `latest`.

Do you agree?

## D. Who sees what

**D1. Consumers.** `grants.releases: [inventory]` stays as it is and mounts `releases/` of that system, which now holds every deliverable of the system. A grant on one deliverable of a system is not needed yet.

Do you agree?

**D2. The orchestrator.** Its prompt lists the deliverables of each member with their descriptions. When a release is announced, it checks the release against the deliverable's definition in the shared area and against what the hub agreed. It cannot change deliverables; it asks you.

Do you agree?

**D3. The UI.** The Shared view gets a Deliverables section: each deliverable with its description, its layout rules, its targets, its skill text, and the agents that hold it. A system view groups its releases by deliverable. A release view shows the manifest, the files with their digests, and the certificates with links.

Do you agree?

## E. Stores in the old layout

**E1.** Releases published before this change are in `releases/<version>/`. At startup, hikari warns about each one, names the new layout, and ignores it, as it did for old maps and vocabularies (rule L6). The examples' stores move to the new layout in the same commit.

Do you agree?

## F. Out of scope

I keep these out of this spec. Say so if one must be in.

- A check command that hikari runs in the container before it publishes, such as `pip install product/`. The skill tells the agent what to run.
- Verifying a certificate against the cloud service.
- Grants on one deliverable of a system.
- Publishers without a system (B1).
- hikari uploading anything.
