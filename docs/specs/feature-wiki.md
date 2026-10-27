# Spec: the feature wiki

Status: built. Checked in a live run of the shop on 2026-10-22: both owners built their wikis from nothing in one indexing turn each, at about $0.12 per owner; every document passed the rules; lead read a document through `get_feature` and as a file under `/hikari/public/readonly/systems/inventory/features/` and found the same text; a host edit of `inventory/stock.py` made the notice name the stale features, and the owner re-ran the tests and marked the map current. The rename and the release copy are covered by the package tests. Approved with the three changes below. One change made during the build: a modify clears `interfaces` or `related` with an empty list, not with `null`, because JSON gives a tool no way to tell `null` from an absent field.

Terms changed in `shared-language-names.md`; see `docs/glossary.md`.

The wikis spec (`docs/specs/wikis.md`) later made the feature wiki the system wiki: the directory is `wiki/`, a document has a `kind`, `change_features` became `change_wiki`, and `get_feature` and `search_features` became `get_document` and `search_wiki`.

The system graphs spec (`docs/specs/system-graphs.md`) later added the code-owned property `created` (rules W10, W11, Q7), between `related` and `updated`.

The tests and health spec (`docs/specs/test-health.md`) later changed S19 and S21 (a feature's test files come from its touching tests), replaced the notes of E4 with rewrites of the private documents, and replaced the `tests` and `health` placeholder of Q1.

Sources: `docs/brainstorm/feature-wiki.md` (round 1) and `feature-wiki-answers.md`. This spec replaces the `features` part of section 5 of `docs/specs/system-maps.md` (the map's feature entries, rules S1 to S3), its owner tools `upsert_features` and `remove_feature`, and rule D7 (the skill as one file). `layout.yaml`, `test.yaml`, their rules P1 to P6, health (S8 to S12), and the map state (S13 to S15) stay as they are until the testing phase (`docs/brainstorm/next-phase.md`).

## Summary

Each feature of a system becomes one markdown document in the store, with a property header that the server checks and a body for people and agents. Documents link each other with `[[id]]` links; a link has no direction and no kind. There are no feature groups: the links are the structure. The owner changes the wiki through one tool, `change_features`, which applies a set of writes as a whole. The map's `features` list is generated from the wiki, so orchestrators, releases, and the UI read what they read today. Staleness becomes per feature: the server names the features whose code changed, and the changed paths that belong to no feature. The `hikari-system` skill becomes a package with references, examples, and two read-only agents, so that two owners map the same way.

## Decisions

From round 1 and its answers:

1. The wiki lives in the store, `store/systems/<system>/features/<id>.md`, beside `map.yaml`. The server writes every document. No new mount.
2. A document is the same for every reader. Nothing in it is private, and the server never rewrites one for a reader.
3. The map is a projection of the wiki. Its `features` list is generated.
4. `layout.yaml` is the owner's code map, unchanged: `parts`, and the code paths of each feature. `test.yaml` is unchanged.
5. The header holds only what describes the feature: `id`, `name`, `description`, `interfaces`, `related`. Code owns `updated`, `described_commit`, and `described_at`. No `code`, `tests`, `health`, `findings`, or `plans`.
6. The body has one required section, `## Purpose`, which cites no code. `## Notes` is the user's.
7. Every `[[link]]` in a document is a neighbor, in either direction, of no kind. Every link resolves inside the system.
8. Ids are unique across every system of the topology.
9. One tool, `change_features`, with create, modify, rename, and remove, applied as one change.
10. The skill is a package: `SKILL.md`, references, examples, and the agents `hikari-feature-survey` and `hikari-feature-review`.
11. `get_feature` returns the document as it is on disk. No UI work in this phase beyond what the schema forces.
12. No backward compatibility. A map of an old version is deleted at startup.
13. `findings` and `plans` leave the map and the tools. `tests` and `health` stay in the map as computed today, as a placeholder.

## Changes from the approved brainstorm

Writing exact requirements changed three details. Please approve or reject each.

1. **Flat code-owned properties.** `described: {commit, at}` becomes `described_commit` and `described_at`. Obsidian shows a nested property as raw text; flat properties render and sort.
2. **The server keeps `## Notes`.** The brainstorm said a modify that changes `## Notes` is refused. Instead, a modify replaces every section except `## Notes`, which the server carries over, and a body given with a `## Notes` section is refused. The owner never has to copy the user's notes to keep them.
3. **The tool takes ids in `related`; the file holds links.** `related: [check-availability]` in a write becomes `related: ["[[check-availability]]"]` in the document, so Obsidian reads the property as links and the owner writes ids as it does for interfaces.

## Terms

- **Feature document**: one markdown file that describes one feature: a property header and a body.
- **Feature wiki**: the feature documents of one system.
- **Neighbor**: a feature that a document links, or that links the document.
- **Change**: one set of writes to a wiki, applied as a whole or not at all.
- **Described commit**: the commit whose code a document describes.
- **Code map**: `layout.yaml`, which says where each feature's code is.

## 1. The store layout

```
store/systems/<system>/
  map.yaml                 the public map; its features list is generated from the wiki
  features/<id>.md         one feature document; the server writes it
  staging/
  releases/<deliverable>/<x.y.z>/
    map.yaml               the public map at release time
    features/              the wiki at release time
```

- **L10.** `Prepare` creates `features/` for each system.
- **L11.** A publish copies `features/` into the release beside `map.yaml` (extends R17).
- **L12.** The store README names `features/` and the copy in a release.

## 2. The feature document

```markdown
---
id: reserve-stock
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
related: ["[[check-availability]]"]
updated: 2026-10-22T05:57:04Z
described_commit: c69dd499311b21c5210c0ae3e984060aa0ba051a
described_at: 2026-10-22T05:57:09Z
---

## Purpose

An order must not promise stock that another order already holds. A reservation moves a quantity from available to held, so that two orders cannot both take the last unit. It refuses a request for more than is available, and it refuses a quantity that is not positive, because a reservation of nothing hides a bug in the caller. A held quantity returns to available through [[release-stock]].

## Notes

(the user's; the owner never writes here)
```

The header:

| Property | Owner | Value |
|---|---|---|
| `id` | owner | the file name; `^[a-z][a-z0-9-]{0,47}$` |
| `name` | owner | two to five words, as a user would say it |
| `description` | owner | one sentence, at most 200 characters |
| `interfaces` | owner | optional; names from the system's `provides` or `consumes` |
| `related` | owner | optional; links as `"[[id]]"`, for neighbors the body does not name |
| `updated` | code | the time of the last write of the document |
| `described_commit`, `described_at` | code | set by `mark_map_current`; empty until the first |

Rules:

- **W1.** The id matches `^[a-z][a-z0-9-]{0,47}$`. It is the file name, `<id>.md`. It is unique in the system and across every system of the topology (decision 8). The refusal for a collision with another system says only that another system uses the id.
- **W2.** `name` is present. `description` is one sentence of at most 200 characters, not empty.
- **W3.** Each name in `interfaces` is in the system's `provides` or `consumes`. `set_system_info` is refused when it would remove an interface that a document names (as S2 does today).
- **W4.** The body holds a `## Purpose` section with text. Other level-two sections are allowed. `## Notes` is the user's: a write that gives a body with `## Notes` is refused; a modify keeps the existing `## Notes` as it is (change 2).
- **W5.** Code owns `updated`, `described_commit`, and `described_at`. A write that gives one is applied without it, and the result warns.
- **W6.** A property that no rule names is the user's. The server keeps it through every write and never reads it.
- **W7.** The server writes a document with an atomic rename, as it writes the map. It writes the header in the order of the table, then the body.
- **W8.** The server watches `features/` as it watches `map.yaml` (S6). When a document changes on disk, it reads the wiki again and checks it. When the wiki is invalid, the server keeps the last valid wiki, and the owner view, `get_my_system`, and the UI show the error. A valid hand edit is accepted as it is; the server does not set `updated` for it.
- **W9.** A change has at most 100 writes.

Warnings, which never refuse a change and which `get_my_system` lists: a document with no neighbor; a feature with no `code` entry in `layout.yaml`; a `## Purpose` that contains a path that exists in the scope; a system with more than 60 features.

## 3. Links

- **K1.** A link is `[[id]]` or `[[id|text]]`, in the body or in `related`. Nothing else is a link: a markdown link, a URL, or a `[[id#heading]]` form is text, and `[[id#heading]]` is refused.
- **K2.** Every link resolves to a feature of the same system, counting the features that the same change creates. A document does not link itself.
- **K3.** The neighbors of a feature are every feature it links and every feature that links it. The map's `related` lists them, sorted (section 5).
- **K4.** A rename rewrites every link to the old id in every document of the system, in the body and in `related`, and keeps the link text after `|`. A remove with `redirect` rewrites the links to the redirect; a remove without one makes each `[[old]]` the plain text of its id, or of its text after `|`, and drops it from `related`.

## 4. The change tool

`change_features(writes)` is an owner tool. It applies the writes as one change: the server applies them to a copy of the wiki, checks the result against sections 2 and 3, and writes every changed document, or writes nothing and reports every problem with the write and the rule.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `name`, `description`, `body`; optional `interfaces`, `related` (ids) | writes a new document |
| `modify` | `id`; optional `fields` (`name`, `description`, `interfaces`, `related`; `null` removes an optional one), optional `body` | merges the fields and replaces the body except `## Notes`; one of the two is required |
| `rename` | `id`, `to` | renames the file and rewrites the links (K4) |
| `remove` | `id`; optional `redirect` | removes the document and rewrites the links (K4) |

- **E1.** Writes apply in order. A later write may name a document that an earlier write created or renamed.
- **E2.** `create` and `modify` set `updated`. `rename` sets `updated` on the renamed document and on each document whose links it rewrote. `remove` touches `updated` only on documents whose links it rewrote.
- **E3.** `related` in a write is a list of ids (change 3). The server writes them as links.
- **E4.** The result lists each write as applied, with its warnings (section 2), and after a `rename` or a `remove`, the private documents that still name the old id, so that the owner updates `layout.yaml` and `test.yaml` (rule P2).
- **E5.** A rename or a remove keeps nothing of the old id in the map: the generated `features` list reflects the wiki after the change. Test results recorded for a feature stay with it across a rename, as `upsert_features` keeps them today, and are dropped on a remove.
- **E6.** `upsert_features` and `remove_feature` are removed, with their endpoints.

## 5. The map as a projection

`map.yaml` goes to version 3. Its `system` section does not change. Its `features` list is generated:

```yaml
features:
  - id: reserve-stock
    name: Reserve stock
    description: Hold a quantity of a SKU for an order when enough is available.
    interfaces: [Stock.reserve]
    related: [check-availability, release-stock]
    tests: {passed: 7, failed: 0, ran: 2026-10-22T05:57:04Z, content_hash: dfa1e19f09fff748}
    health: passing
    state: current
```

- **Q1.** Each entry carries `id`, `name`, `description`, and `interfaces` from the document; `related` from K3; `tests` and `health` as the server computes them today; and `state` from section 6. The server writes the list whenever the wiki, the test results, or the state change, in id order.
- **Q2.** `findings` and `plans` are gone from the map, the tools, and the views.
- **Q3.** `get_feature(system, id)` returns the map entry and `document`, the text of the file as it is on disk. `search_features(query)` also matches the body. `list_systems` and `get_system` do not change.
- **Q4.** A map file with a version other than 3 is deleted at startup, with a log line that names the file, and the system starts unbuilt (decision 12). The private documents in the repo are not touched; the owner's indexing turn lists their problems (P5) and the owner rebuilds.

## 6. State per feature

- **S19.** A feature's code paths are its `code` entry in `layout.yaml`; a directory covers every file under it. Its test files are the `files` of its entries in `test.yaml`.
- **S20.** A feature is `undescribed` while `described_commit` is empty; `stale` when a file under its code paths or among its test files changed since `described_commit`, counting commits, uncommitted changes, and new files (as S14); otherwise `current`. A feature with no code paths and no test files is never stale.
- **S21.** The unmapped paths of a system are the paths that changed since the map's described commit (S14) and that are under no feature's code paths and are not a test file of any feature.
- **S22.** `mark_map_current` sets `described_commit` and `described_at` on every document, and the map's `described_state`, after the commit it makes today. The map's `state` keeps rules S13 to S15.
- **S23.** The owner view adds `stale_features` (each with its changed paths), `undescribed_features`, `unmapped_paths`, and `warnings` (section 2). The public view carries each feature's `state`.

## 7. Driver and prompts

- **D8.** The indexing turn (D1) runs when the map is `unbuilt` or `stale`, as today. Its prompt for an unbuilt map says: load the `hikari-system` skill and follow its Build procedure. For a stale map it lists the stale features with their changed paths, the undescribed features, and the unmapped paths, and says: follow the skill's Keep true procedure.
- **D9.** The turn notices (D3) replace "these paths changed" with the same three lists, in one notice, and keep the private-document and vocabulary notices.
- **P16.** The owner prompt names the wiki: your features are documents in `/hikari/public/export/features/`, which you read as files and change only with `change_features`; `layout.yaml` is your code map; load the `hikari-system` skill before you build or change them.
- **P17.** The orchestrator prompt says that each system's feature documents are in `/hikari/public/readonly/systems/<s>/features/`, that `get_feature` returns a document, and that a document's Purpose says why a feature exists and what it promises.
- **P18.** `TurnContext` carries `stale_features`, `undescribed_features`, and `unmapped_paths` in place of `changed`.

## 8. The skill package

- **H11.** The built-in skill is a directory embedded in the binary. At the start of each turn of an owner, the driver writes it to `~/.claude/skills/hikari-system/` and the agents to `~/.claude/agents/hikari-feature-survey.md` and `~/.claude/agents/hikari-feature-review.md`, replacing what is there. It never writes them into the repo or the store.
- **H12.** A shared skill named `hikari-system`, or a shared agent whose name starts with `hikari-`, is skipped with a warning in the agent's log (as H4).
- **T17.** The topology check refuses a shared agent whose name starts with `hikari-`, as T16 refuses a deliverable named like a built-in skill.

The files:

```
hikari-system/
  SKILL.md                 what you own, and the procedures: Build, Keep true, Test, Release
  references/features.md   the document, its rules, what a feature is, how to link
  references/layout.md     layout.yaml: parts and the code map, how to build, keep, and use it
  references/tests.md      test.yaml, as today
  references/examples/     three feature documents of the shop demo
```

`SKILL.md`:

- **What you own**: the table of documents (the wiki, `layout.yaml`, `test.yaml`, the map as a projection), who reads each, and how each changes.
- **Build**, for a system with no wiki:
  1. Orient: read the instruction files, the manifests, the entry points, and the test directory names. Write `parts` in `layout.yaml`.
  2. Survey: for each part, send `hikari-feature-survey` with the part's paths, or read the part yourself when the scope has about 40 source files or fewer.
  3. Decide the feature set against `references/features.md`: merge, split, name, and give ids.
  4. Link against the link rules.
  5. Write one `change_features` call with every create.
  6. Write the code map in `layout.yaml`. Write `test.yaml`, run the tests, record the results, and call `mark_map_current`.
- **Keep true**, when a notice lists stale features and unmapped paths:
  1. For each stale feature, read its changed paths and decide: the description still holds (no write), the behavior changed (modify), or the feature is gone (remove with a redirect).
  2. For each unmapped path, decide: it belongs to a feature (add it to that feature's `code`), it is a new feature (create, and add its code), or it is a part (add it to `parts`).
  3. When more than about ten paths changed, send `hikari-feature-review` first and work from its findings.
  4. Run the tests of the affected features, record the results, call `commit_changes`, then `mark_map_current`.
- **Test** and **Release**: as today.

`references/features.md` holds the document format and the rules of sections 2 and 3, and these guidelines, approved in round 1:

What a feature is:

- A feature is one thing the system does that someone outside the code relies on: a user, another system, or the operator. Name who relies on it. If no one outside the code would notice it gone, it is not a feature; it is a part, and it belongs in `layout.yaml`.
- State it in one sentence with one verb and one object: "Reserve stock for an order". If the sentence needs "and", consider two features.
- It can be tested on its own: you can say what proves that it works.
- Not a feature: a file, a package, a class, a helper, a dependency, a refactor, a build step, a test suite, a configuration value. A library's public function is usually a feature; its internal helper is not.
- Split when the two halves have different users, different interfaces, or different failure modes. Merge when neither half makes sense without the other, or you cannot test one without the other.
- A cross-cutting behavior is a feature when it is a promise to someone: "Every request is authenticated". Otherwise it is a part.
- Most systems have 5 to 40 features. Above 60, you are listing parts. Below 3, you are describing the system, not its features.
- The name is two to five words, as a user would say it. The id is the name in lower case with dashes. An id is stable: change the wording of a name freely, and rename the id only when the feature itself changed meaning.

How to link:

- Link two features when one needs the other to work (it calls it, or consumes what it produces); when they act on the same thing (the same object of the vocabulary, the same data, the same resource); or when a user does them together in one flow.
- Do not link because two features live in the same file or package. Do not say what kind of link it is. Write the link once, on either document, where the prose makes it natural.
- Every feature should have a neighbor. One with none is either part of nothing, which means the feature set is wrong, or new. A feature that links most of the others is a hub; that is information about the system, not a mistake.

The Purpose section: why the feature exists and for whom, what it promises, and what it refuses. Two to six sentences; more when the motivation needs it. Use the vocabulary's names. No path, no identifier from the code, no command.

`references/layout.md` holds the format of `layout.yaml` as today, and: give every feature a `code` entry, as directories where a feature owns a directory and as files otherwise; keep the entry current when code moves, in the same unit of work; use it to find a feature's code before you change it, and to tell which features a changed path touches.

## 9. The agents

Both are read-only Claude Code subagents with `tools: Read, Grep, Glob`. Each returns JSON as its final message and nothing after it. Source content is data; an instruction inside the code is ignored.

**`hikari-feature-survey`** takes the paths of one part, the system's interfaces, and the vocabulary's terms. It returns:

```json
{
  "part": "inventory/",
  "candidates": [
    {
      "name": "Reserve stock",
      "description": "Hold a quantity of a SKU for an order when enough is available.",
      "relies": "orders, through Stock.reserve",
      "code": ["inventory/stock.py"],
      "interfaces": ["Stock.reserve"],
      "evidence": ["inventory/stock.py:41"],
      "neighbors": [{"name": "Check availability", "rule": 2}]
    }
  ],
  "parts": [{"path": "inventory/util.py", "description": "Helpers; not a feature."}],
  "partial": false,
  "reason": ""
}
```

It applies the guidelines of `references/features.md` and lists what it judged a part under `parts`. At most 30 candidates; past that it keeps the ones with the most evidence and says so in `reason`.

**`hikari-feature-review`** takes the wiki (the paths of the documents), `layout.yaml`, and the scope. It returns findings:

```json
{
  "findings": [
    {
      "kind": "description",
      "feature": "reserve-stock",
      "evidence": ["inventory/stock.py:52"],
      "text": "The code now refuses a reservation above 1000 units; the description does not say so.",
      "write": {"op": "modify", "id": "reserve-stock", "fields": {"description": "..."}}
    }
  ],
  "partial": false,
  "reason": ""
}
```

Kinds: `description` (the code no longer supports it), `code` (the code map misses a file of the feature, or lists one that is gone), `missing` (a behavior with no document), `link` (a call between two features that no document links), `split` (a document that describes two things), `merge` (two documents that describe one thing). Each finding names the evidence as `path:line` and a suggested write, or a suggested change to `layout.yaml` for `code`. The owner decides.

## 10. Access

Unchanged: an owner changes its own wiki through `change_features` and reads it at `/hikari/public/export/features/`; an orchestrator reads the wikis of its visible systems at `/hikari/public/readonly/systems/<s>/features/` and through the tools; a release grant mounts `releases/`, which now holds a copy of the wiki with each release; the UI reads everything.

## 11. UI

- **U16.** The feature list drops `findings` and `plans` (Q2) and shows `state` beside `health`. Nothing else changes in this phase.

## 12. Stores from earlier versions

- **O3.** A `map.yaml` of a version other than 3 is deleted at startup (Q4). The shop seed holds no wiki; owners build it in the live run.

## 13. Out of scope

- A generated index page in the wiki.
- Groups computed from the links.
- A `terms` property that links features to vocabulary terms.
- Links between systems.
- Editing the wiki in the UI, and rendering it as a vault.
- Findings, plans and work, and testing and health: `docs/brainstorm/next-phase.md`.

## 14. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Documents.** A `featurewiki` package: parse and write a document (W1 to W7), links (K1 to K4), the change (E1 to E5), the checks and warnings, with table tests for each rule and for each op.
2. **Map and state.** Map version 3 with the generated features list (Q1, Q2), per-feature state (S19 to S23), the owner view, the deletion of old maps (Q4, O3).
3. **Store.** `features/` in `Prepare`, the copy in a release, the README (L10 to L12).
4. **Server and tools.** The `change_features` endpoint and tool, `get_feature` and `search_features` with the document (Q3), the removal of the old tools (E6), the id check across systems (W1), the watch of `features/` (W8), `TurnContext` (P18), the topology check (T17).
5. **Skill package and agents.** The embedded directory and its install (H11, H12), `SKILL.md`, the references, the examples, the two agents. A test checks that every tool and property the texts name exists.
6. **Driver.** The indexing prompt and the notices (D8, D9), the prompts (P16, P17).
7. **UI.** U16.
8. **Live test.** In the shop: both owners build wikis from nothing; the documents pass the rules; lead reads a document through `get_feature` and as a file; a change to `inventory/stock.py` makes `reserve-stock` stale and the notice names it; a rename rewrites the links; a release holds the wiki.
9. **Docs.** README, `docs/design.md` (why the wiki is in the store, why links have no kind, why the map is a projection), and the pointers in `docs/specs/system-maps.md`.
