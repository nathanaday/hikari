# Releases that travel: round 1

Agreed end to end on 2026-10-29. The spec is `docs/specs/releases.md`.

## The finding

In a dry run of a large project, a fresh clone had no releases: hikari keeps each partition's `releases/` out of git, so every consumer of a library failed to configure, and the comparisons that needed an SDK release skipped. The code, the topology, the images, and the pins were reproducible; the released products were not.

## The strategy

hikari stays flexible and high level: it containerizes agents and skills, and it does not enforce one way to store releases or one release policy. A release workflow belongs to the project. What hikari gives is a contract that any workflow can fill:

- The local `releases/` directory stays the default, with its limitation stated: a clone must get the products again.
- Remote homes are a must, and they differ: an OS image goes to large storage, a small library to GitHub releases. hikari assumes no single remote interface.
- Some teams build and upload in CI, and some forbid local builds. A release can be made elsewhere and recorded.
- A release is not automatically ready for production. Tags such as dev, staging, and production, or any system the project chooses, say what a release is for. hikari assigns no meaning to a tag.
- Each repository and partition can be configured: what it may release (which tags), how (build and push locally, or record a CI build), and where (a remote, or local only).
- Partitions, hikari, and every agent can pull the release they want and list what exists for the topology.

## What I take from it

Split a release into a record and a product. The record is small text: the manifest, the checksums, and the feature map copy. Commit it to the vault and keep only the bytes out of git. A fresh clone then holds the complete catalog, and the local directory becomes a cache keyed by digest.

Keep the remotes out of hikari. A remote is a named definition in the vault with a `push` command and a `pull` command, the way git and Docker use helpers. hikari supplies the environment, runs the command, and verifies pulled bytes against the committed checksums. The project supplies the implementation, and the credentials are the host's, where `hikari sync` and `hikari push` already use your git.

Versions stay immutable and tags are mutable pointers to versions, which `latest` already is. The deliverable declares which tags exist, and the grant says which tags each holder may set and which tags each consumer may pin. Promotion is a tag move, with no rebuild and no second upload.

A release built elsewhere is a release whose product hikari never staged: an agent, or you, records it, and hikari pulls the bytes through the remote and checks the digest.

## Decisions

1. A pin lives in a file of the consumer partition that hikari writes through a tool, so that an agent can bump a dependency without a topology change, and hikari checks it against the release grants.
2. The catalog travels on each person's branch, like the feature maps, and merges the same way.
3. One remote for each deliverable, to start.
4. hikari does not verify a certificate against its service. The digest check on pull is the verification that matters.
5. The restore rule for releases (SY11 of the linked repositories spec), where an owner publishes again on its first turn, goes away: the record travels, and the product is pulled.
