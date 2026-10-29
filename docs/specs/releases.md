# Spec: releases that travel

Status: built. Checked in a live run of the shop on 2026-10-29: inventory published 0.1.0 with the tag `dev`, hikari pushed the product to the `local-dir` remote, a tag outside `grants.tags` and a pin outside `grants.pins` were refused, orders pinned `staging`, the product was removed and `hikari release pull` and `hikari sync` brought it back and checked it, `hikari release list` and the UI showed the tags, the remote, and the pin, and the room carried the announcements.

## Changes made during the build

1. The user's `hikari release pin` and `pull` pull in the CLI process, not through the server, because a call through the running hikari times out after 30 seconds and a pull can take longer. The server's pin for the user records the pin only; the agents' `pin_release` pulls in the server.
2. `tags.yaml` and `pins.yaml` written by `hikari release` while hikari is stopped count as your edits, so `hikari up` commits them as yours, like your wiki edits. That commit's message is now "user: vault edits ...".
3. A pin by a version is refused when `grants.pins` names the partition, as PN4 says; the shop's `orders` therefore pins `staging` or `production` only.
4. The UI's partition page shows the pins only once the partition is described, like the rest of that page.

Source: `docs/brainstorm/releases.md`. This spec extends `docs/specs/deliverables.md`: the record of a release, tags, remotes, pins, and the listing. It replaces rule SY11 of `docs/specs/linked-repos.md`, and the ignore lines of rule PR12 of `docs/specs/project-repo.md` where they concern releases.

## Summary

A release becomes a record and a product. The record is text that hikari writes, and it is committed to the vault on your branch, so that a clone of the project holds the complete catalog of releases. The product is the bytes, which stay out of git and are pulled through a remote: a named pair of commands in the vault, written by the project, which hikari runs on the host with your credentials. Tags are mutable names for versions, declared by the deliverable and granted to agents. A consumer pins a release of a partition it may read, and `hikari sync` pulls the products of the pins. Every agent lists the catalog with one tool.

## Decisions made in this spec

1. **The tag links move under `tags/`.** `releases/<deliverable>/latest` becomes `releases/<deliverable>/tags/latest`, beside the other tags, so that one ignore line covers every link and `tags.yaml` is the one source of truth. hikari removes the old link at start.
2. **hikari runs the remote's commands on the host, in the server and in `hikari sync`.** The deliverables spec said that hikari never uploads. It still never calls a service itself: it runs the command the project wrote, with the environment the host already has for `hikari sync` and `hikari push`. A container's credentials are not needed for a remote.
3. **Grants stay lists of names.** The restrictions on tags are two optional maps beside them, `grants.tags` and `grants.pins`, so that the topology editor, the UI, and every reader of a grant list keep working.
4. **A push that fails leaves no release.** `publish_release` pushes before it moves the product, and removes the record when the push fails, so that a retry is the same call again.
5. **`record_release` pulls into the staging area and then publishes.** A release built elsewhere goes through the same checks as one built in the container, and gets the same record.
6. **A pin by tag follows the tag.** The pin stores the tag, and the consumer's container sees the tag link, which hikari keeps current.

## Terms

- **Record**: the committed part of a release: `release.yaml`, `SHA256SUMS`, and `features/`.
- **Product**: the files of a release, in `product/`. A product can be absent on a machine.
- **Catalog**: every record in the vault, on this branch.
- **Remote**: a named definition in the vault of how a product is pushed to and pulled from a home outside the vault.
- **Tag**: a mutable name for one version of a deliverable of a partition. `latest` is the tag hikari keeps.
- **Pin**: a consumer partition's declaration of the release it uses: a version, or a tag.

## 1. The record and the product

```
partitions/<id>/read-only/releases/<deliverable>/
  tags.yaml                     the tags, by hikari; committed
  tags/<tag> -> ../<x.y.z>      one link for each tag; ignored
  <x.y.z>/
    release.yaml                the manifest; committed
    SHA256SUMS                  the digest of every product file; committed
    features/                   the feature map at release time; committed
    product/                    the files; ignored
```

- **RL1.** A release is a record and a product. The record is `release.yaml`, `SHA256SUMS`, and `features/`. The product is `product/`.
- **RL2.** The vault's ignore file holds `partitions/*/staging/`, `partitions/*/read-only/releases/*/*/product/`, and `partitions/*/read-only/releases/*/tags/`. hikari writes and commits it as rule PR18 says.
- **RL3.** `publish_release` commits the record and `tags.yaml` on the project branch as the partition agent, with the message `<agent>: release <deliverable> <version>`. A tag change commits `tags.yaml` with `<agent>: tag <deliverable> <version> as <tag>`. A pin change commits `pins.yaml` with `<agent>: pin <partition> <deliverable> <version or tag>` or `<agent>: unpin <partition> <deliverable>`. The user's changes through the CLI and the UI are committed as the user.
- **RL4.** At start, hikari commits every record and `tags.yaml` that git does not track, as `hikari: add the records of releases`, so that releases published by an earlier version enter the catalog. It writes `tags.yaml` from the versions on disk where the file is missing, with `latest` at the newest.
- **RL5.** At start and after each change of `tags.yaml`, hikari writes the links under `tags/` from `tags.yaml`: one relative link for each tag, and no other entry. It removes a `latest` link beside `tags/` from an earlier version.
- **RL6.** The manifest gains `remote`, the name of the remote the product was pushed to or pulled from, when there is one. Its other fields do not change.
- **RL7.** A record whose product is absent is listed with `product: absent`. Reading `product/` of such a release in a container finds no directory.

## 2. Tags

```yaml
# tags.yaml
version: 1
tags:
  latest: 1.2.0
  production: 1.1.0
```

- **TG1.** A tag name matches `^[a-z][a-z0-9._-]{0,31}$`. `latest` is reserved: hikari points it at the newest version by semver at each publish, and no tool or command sets it.
- **TG2.** A deliverable declares its tags as `tags`, a list of RE2 patterns, each anchored at both ends. A plain name is a pattern that matches itself. A deliverable without `tags` has only `latest`.
- **TG3.** `tag_release(deliverable, version, tag)` is a partition agent's tool for a deliverable it holds. It refuses a version that is not released, a tag that matches no pattern of the deliverable, `latest`, and a tag outside the agent's `grants.tags` for that deliverable. It writes `tags.yaml`, the links, and the commit (RL3), and posts `Tagged <partition> <deliverable> <version> as <tag>` in the room from the agent.
- **TG4.** `grants.tags` is an optional map from a held deliverable to a list of patterns (TG2). When the map names the deliverable, the agent may publish with and set only the tags that match one of its patterns. When it does not, the agent may set any tag of the deliverable. The topology check refuses a key that is not a held deliverable, and an invalid pattern.
- **TG5.** `publish_release` takes an optional `tags`, a list of tag names set on the new version in the same change. Each is checked as TG3 checks a tag.
- **TG6.** `hikari release tag <partition> <deliverable> <version> <tag>` and `hikari release untag <partition> <deliverable> <tag>` do the same as you, with no grant check, through the running hikari, or on the files when it is stopped. `untag` cannot remove `latest`.

## 3. Remotes

```
public/remotes/<name>/remote.yaml
```

```yaml
version: 1
description: GitHub releases of acme/inventory.
push: |
  tar -C "$HIKARI_PRODUCT" -czf "/tmp/$HIKARI_DIGEST.tgz" . &&
  gh release create "$HIKARI_DELIVERABLE-$HIKARI_VERSION" --repo acme/inventory \
    --notes-file "$HIKARI_RELEASE/release.yaml" "/tmp/$HIKARI_DIGEST.tgz" "$HIKARI_RELEASE/SHA256SUMS"
pull: |
  gh release download "$HIKARI_DELIVERABLE-$HIKARI_VERSION" --repo acme/inventory --pattern '*.tgz' -O - |
  tar -C "$HIKARI_PRODUCT" -xzf -
timeout: 10m
```

- **RM1.** A remote is a directory of `public/remotes/` with `remote.yaml`: `version: 1`, a `description` that is not empty, `push` and `pull`, each an optional shell command, at least one of them set, and an optional `timeout`, a duration, 10 minutes by default. Its name matches the name rule of deliverables. Unknown fields are refused.
- **RM2.** A deliverable names its remote with `remote: <name>`, optional. Loading a deliverable loads its remote, and a missing or invalid remote is a problem of the deliverable.
- **RM3.** hikari runs a command with `sh -c` on the host, in the vault's directory, with the host's environment and these variables: `HIKARI_REMOTE`, `HIKARI_PARTITION`, `HIKARI_DELIVERABLE`, `HIKARI_VERSION`, `HIKARI_DIGEST` (the product digest, `sha256:` and 64 hex, empty on the pull of `record_release`), `HIKARI_PRODUCT` (the directory to upload, or the empty directory to fill), `HIKARI_RELEASE` (the record's directory, which holds `release.yaml` and `SHA256SUMS`), and `HIKARI_VAULT` (the vault's directory). It never parses the command.
- **RM4.** A command that exits non-zero, or runs past the timeout, fails with its last lines of output in the error. On success, the output goes to the log.
- **RM5.** `publish_release` of a deliverable with a remote that has `push` runs the push after every check passes and the record is written, with `HIKARI_PRODUCT` the staging directory. When the push fails, hikari removes the record and reports the error; the staging area is untouched. When it succeeds, the product moves into the release as before, and the manifest records the remote (RL6).
- **RM6.** `pull(partition, deliverable, version)` makes an empty directory inside the release, runs the remote's `pull` with `HIKARI_PRODUCT` set to it, checks that its files are exactly the files of `SHA256SUMS` with the same digests, and renames it to `product/`. On a failure or a mismatch, it removes the directory and reports which files differ. A release whose product exists is not pulled again. A deliverable without a remote, or whose remote has no `pull`, cannot be pulled, and the error says that its owner must publish it again on this machine.
- **RM7.** `pull_release(partition, deliverable, version or tag)` is a tool of every agent for a partition whose releases it reads (its own, its visible partitions, and its release grants). `hikari release pull <partition> <deliverable> <version or tag>` does the same for you.
- **RM8.** `record_release(deliverable, version, notes, tags)` is a partition agent's tool for a deliverable it holds whose remote has `pull`. It refuses when the staging directory of the deliverable is not empty. It pulls the version into the staging directory with `HIKARI_DIGEST` empty, then publishes as `publish_release` does, without the push, and the manifest records the remote.
- **RM9.** Targets and certificates do not change. A deliverable may have a remote, targets, both, or neither.

## 4. Pins

```yaml
# partitions/<id>/read-only/pins.yaml
version: 1
pins:
  - partition: inventory
    deliverable: python-library
    version: 1.2.0
  - partition: schemas
    deliverable: json-schemas
    tag: production
```

- **PN1.** A pin names a partition, a deliverable, and either a version or a tag. A partition has at most one pin for each partition and deliverable. hikari writes the file; agents change it only through tools.
- **PN2.** `pin_release(partition, deliverable, version or tag)` is a partition agent's tool for a partition in its `grants.releases`. It refuses a partition it cannot read, a version that is not released, a tag that `tags.yaml` does not hold, and a pin outside `grants.pins`. It writes the pin and the commit (RL3), and pulls the product when it is absent and the deliverable can be pulled (RM6); a pull that fails keeps the pin and reports the error.
- **PN3.** `unpin_release(partition, deliverable)` removes the pin.
- **PN4.** `grants.pins` is an optional map from a partition in `grants.releases` to a list of patterns (TG2). When the map names the partition, the agent may pin only a tag that matches one of its patterns, and no version. When it does not, the agent may pin any version or tag. The topology check refuses a key that is not in `grants.releases`, and an invalid pattern.
- **PN5.** `hikari sync` pulls the product of every pin of every partition that is absent on this machine, after the repositories, and prints one line for each: pulled, present, or why not. A pin whose deliverable cannot be pulled is a warning, not a failure. A pull that fails is a failure (rule SY8).
- **PN6.** `hikari check` and `hikari up` warn, and do not stop, about a pin whose product is absent, and about a pin whose version or tag is not in the catalog.
- **PN7.** The partition agent's prompt lists its pins with the path of each in its container, and tells it to pin what it integrates. `get_my_partition` and the orchestration agent's view of a partition carry the pins.
- **PN8.** `hikari release pin <consumer> <partition> <deliverable> <version or tag>` and `hikari release unpin <consumer> <partition> <deliverable>` do the same for you, as the user, with the grant checks of PN2.

## 5. The listing

- **LS1.** `list_releases` is a tool of every agent. For each partition whose releases the agent reads (RM7), it lists each release, newest first within each deliverable: the partition, the deliverable, the version, the tags that point at it, when it was released, the first line of its notes, the remote, whether the product is present on this machine, and the partitions that pin it. An argument `partition` narrows it to one.
- **LS2.** `hikari release list [topology.yaml]` prints the same for every partition, from the files, with hikari running or stopped.
- **LS3.** The UI's partition view shows, for each release, its tags and whether its product is present, and a Pins section with the partition's pins. The release view shows the tags and the remote. The vault listing endpoint returns the remotes.
- **LS4.** The orchestration agent's prompt says that `list_releases` is the catalog, and that a release may lack its product on this machine until it is pulled.

## 6. Prompts and skills

- **PS1.** The deliverables section of the partition agent's prompt (P12) names, for each held deliverable, its remote, its tags, and the tags the agent may set, and says: tag a release with `tag_release` or the `tags` of `publish_release`; record a release built elsewhere with `record_release`.
- **PS2.** The grants section (P10, P15) names `tags/<tag>` in place of `latest`, and the pins.
- **PS3.** The `hikari-do-work` skill's "Publish a release" section covers tags, remotes, and `record_release`, and the "Integrate a release" guidance tells a consumer to pin.
- **PS4.** The restore prompt does not change: there is no restore of releases.

## 7. The example

- **EX1.** The shop gets a remote `local-dir`, whose push copies the product into `${SHOP_RELEASES:-$HOME/.hikari/shop-releases}/<partition>/<deliverable>/<version>/` and whose pull copies it back. It shows that a remote is the project's command and reads the host's environment.
- **EX2.** `python-library` names the remote and declares the tags `dev`, `staging`, and `production`. `inventory` may set `dev` and `staging`; `orders` may pin only `staging` and `production`.
- **EX3.** `make demo-repo` resets the remote's directory.

## 8. Out of scope

- A remote that hikari ships with credentials or a client. The example and the docs show GitHub releases and S3 as commands.
- A tag with more than one version, or a version with a lifecycle hikari enforces.
- Pins of the orchestration agent.
- Garbage collection of products.
- Verifying a certificate against its service.

## 9. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Vault: records and tags.** The ignore file (RL2), `tags.yaml` and the links (TG1, TG2, RL5), the manifest's `remote` (RL6), the deliverable's `remote` and `tags` (RM2, TG2), and the catalog with absent products (RL7).
2. **Vault: remotes.** The format (RM1), running a command (RM3, RM4), the push in publish (RM5), and the pull with its verification (RM6).
3. **Vault: pins.** The file and its checks (PN1).
4. **Topology.** `grants.tags` and `grants.pins` (TG4, PN4).
5. **Manager and server.** The commits (RL3, RL4), the tools and routes: `tag_release`, `pull_release`, `record_release`, `pin_release`, `unpin_release`, `list_releases` (TG3, TG5, RM7, RM8, PN2, PN3, LS1), the announcements, and the warnings (PN6).
6. **CLI and sync.** `hikari release list|tag|untag|pull|pin|unpin` (TG6, RM7, LS2, PN8), and the pulls of `hikari sync` (PN5).
7. **Prompts and skills.** PS1 to PS3.
8. **UI.** LS3.
9. **Example, live check, and docs.** EX1 to EX3; a live run where inventory publishes with a tag, orders pins, and a fresh clone syncs and gets the product; the README, `docs/design.md`, the vault README text, the guides, the glossary, and the pointers in the deliverables, linked-repos, and project-repo specs.
