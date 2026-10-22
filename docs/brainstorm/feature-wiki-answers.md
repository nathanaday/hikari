# Feature wiki: round 1 answers

Answered in chat on 2026-10-22. The direction: the feature map does one job, the
features and the links between them. Findings, plans, and test health are separate
entities with their own design later; they link to features, and their relation to a
feature is computed, never written into the feature document. Nothing in a document
is hidden from one reader and shown to another: a document is the same for every
reader.

## Answers

| Item | Answer |
|---|---|
| A1 | Leans agree; the host layout was unclear. See the revised recommendation below. Agreed that the owner can read the wiki and changes it only through the tool. |
| A2 | No. No part of a document is private, and the server never rewrites a document for one reader. The code paths of a feature go into a code map that the owner keeps for its own work. |
| A3 | Agree. |
| A4 | Asked whether `layout.yaml` is that code map. Yes: see below. |
| B1 | Simpler. No `code` (A2), no `tests`, no `findings`, no `plans`, no `health`. Each property must be kept fresh, so the header holds only what describes the feature. |
| B2, B3, B4 | Agree. |
| C1, C2 | Agree. |
| D1, D2 | Agree. |
| E1, E2 | Agree. |
| F1 to F6 | Agree. |
| G1 | Agree, without the sanitizing: `get_feature` returns the document as it is on disk. |
| G2 | Hold. No UI work until tests, health, and findings have their own design. |
| H1 | No. No backward compatibility during development. An old map is ignored and deleted when found. |

## What the answers change

**A1, revised.** With A2, a document has no private part, so the reason for the repo is
gone, and the wiki is the public map. I now recommend the store:
`store/systems/<system>/features/<id>.md`, beside `map.yaml`.

- Orchestrators already mount `store/systems/<s>/` read-only, so they read the
  documents as files, as they read `map.yaml`. Owners already mount their own
  directory read-only at `/hikari/public/export/`. No new mount.
- The store opens as one Obsidian vault of every system.
- A release copies the wiki with the map, so a release shows its features in full.
- The documents are not in git with the code. The store is a plain directory, by
  design. `described` on each document says which commit it describes, so staleness
  does not need git history.

The repo (`<repo>/<scope>/.hikari/features/`) stays possible, with one more read-only
mount at `/hikari/features/`. The user decides in round 2.

**B4, revised.** Ids are unique across every system of the topology, not only in one
repo, because the store is one vault. The server knows every system, so the check is
the same.

**A2 and A4: the code map is `layout.yaml`, as it is today.** `layout.yaml` keeps
`parts` and its `features` section, which maps each feature id to its code paths. It
stays private, in `<scope>/.hikari/private/`, edited by the owner, checked by the
server (rules P2 and P3). Its purpose is the owner's own work: find the code of a
feature, and know which features a changed path touches. The skill package gets a
reference, `references/layout.md`, on how to build, keep, and use it. The per-feature
staleness of D2 reads the code paths from it.

**B1: the header.**

```yaml
---
id: reserve-stock
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
related: []                                   # optional: links the body does not make
updated: 2026-10-22T05:57:04Z                 # code
described: {commit: c69dd49, at: 2026-10-22T05:57:09Z}   # code
---
```

The owner gives `id`, `name`, `description`, `interfaces`, and `related`. Code owns
`updated` and `described`. Nothing else.

**A3: the map's feature entries.** `id`, `name`, `description`, `interfaces`, and
`related` come from the documents. `tests` and `health` stay in the map as they are
computed today, from `test.yaml` and `record_test_results`, as a placeholder until the
testing phase replaces that mechanism. `findings` and `plans` leave the map and the
tools.

**E1: the writes.** A create takes `id`, `name`, `description`, `body`, and the optional
`interfaces` and `related`. Nothing else changes.

**F1: the package** adds `references/layout.md`. `references/tests.md` stays as the
placeholder for the testing phase.

**G2:** no UI change in this phase. The UI keeps showing the map as today.

**H1:** at startup, the server deletes a `map.yaml` whose version is not the current
one, logs it, and the owner builds the wiki from nothing. Stores and repos used during
development start empty.

## Planted for later

Four items came out of this round and are recorded in `next-phase.md`: a document of
the hikari files on the host and their mounts, findings, plans and work, and testing
and health.
