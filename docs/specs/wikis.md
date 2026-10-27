# Spec: the hub and system wikis

Status: built. Checked in a live run on 2026-10-22 of a project from `hikari init` with an orchestrator, `lead`, and two owners, `tally` and `ballots`, in the project repository. The owners built their wikis. A plan written by hand in `#main`'s folder was committed as the user (`user: wiki edits before lead's change`) just before the lead's change. The lead loaded `hikari-hub` and wrote a story that links four features of both systems and a principle, and named the plan without linking it. The story's support was `passing`. Through the UI endpoints, a note was written and the story marked key, each committed as the user, and a story with a dead link was refused with K2. The tally owner read the hub wiki from its mount and named the story and the principle that concern it. Its turn context listed the plan the user then added to its wiki. At stop the repository was clean, and an edit made while hikari was stopped was committed as the user at the next start. The Chrome extension was not connected, so the views were checked by render tests, not in a browser.

The tester, environments, and image releases were removed in `remove-e2e.md`.

Changes made during the build:

1. **A user document is lighter, not dashed** (U3). A dashed outline already means a stale feature, so a plan or a note is drawn smaller and paler under the Kind lens, and neutral under Tests.
2. **The selection of a wiki map is in the URL** (U5): `#/map/<system>/<doc>` and `#/hub/<hub>/map/<doc>`. A move between maps adds a history entry; a selection replaces it.
3. **A change fans out to the wiki itself.** The change fan-out dropped the wiki of a change when it notified the peers of a system, so a system wiki sent no event. Found by the server test.
4. **The start check of the project repository commits only wiki edits** (C5), and only when every changed path is in a wiki folder.

Sources: `docs/archive/e2e.md` (round 1, section B) and `e2e-answers.md`, and the user's note of 2026-10-22: "the hub-level and system-level feature wikis ... contain principles, features, business rules, that connect the fabric of the entire ecosystem together, and also serve as a place for the user to store future plans, roadmaps, and notes." The note adds a step to the E2E work, before the tester: this is now the second step. The tester, the plan, and the lock follow in the third step; environments in the fourth; bench results in the fifth.

A review of the draft against the code found that hand edits, renames across wikis, concurrent changes, and authorship needed exact rules. Sections 3, 4, and 9 hold them, and decision 8 onward.

## Summary

Each system and each hub has a wiki: one markdown document per entry, with a property header and links. A document has a kind. A system wiki holds features, principles, and rules; a hub wiki holds stories, rules, and principles. Agents write these through one change tool for each wiki, as owners write features today. Plans and notes are your kinds: you write them in Obsidian or in the UI, and every agent that reads the wiki reads them. Ids are unique across the store, so `[[reserve-stock]]` means one document wherever it appears, and a hub's story links the features of its systems with the same links. The links across wikis are the fabric of the ecosystem: the Map view draws each hub's wiki with the documents it links in its systems, and each document lists the documents that link it.

## Decisions

From round 1 and its answers:

1. The orchestrator writes the hub's stories (A1, B1).
2. Stories use the format of the feature wiki (B1).
3. Two kinds for hub knowledge: story and rule (B2). Your note adds principles, and plans and notes for you.
4. A story links features across systems (B3).
5. You mark the key stories; only you can set `key` (B4).
6. Story health comes from E2E results only; beside it, the UI shows the health of the features it links (B5). E2E results come in step 3.
7. Owners read the stories that link their features (B6).
8. One level of hubs for E2E (B7).

## Decisions made in this spec

1. **One format and one package for both wikis, with a kind.** The feature wiki's document, rules, and change model serve every kind. The package `featurewiki` becomes `wiki`, and the directory `features/` becomes `wiki/`.
2. **Plans and notes are yours.** Agents never write them. A file in a wiki with no `kind` is a note, so a page you drop into the folder in Obsidian is valid as it is.
3. **No `system/id` link form.** Ids are already unique across every system (W1). This spec extends the rule to hub documents, so `[[id]]` resolves everywhere, in hikari and in Obsidian, which resolves a link by file name.
4. **An owner reads the whole wiki of each hub it is in,** not only the stories that link its features (decision 7 asked for less). A hub's principles and rules apply to every member, so a member must read them, and a hub document holds no code. The stories that link a feature are listed on the feature, so the owner finds them at once.
5. **A hand edit is checked more loosely than a tool write, and file by file.** A tool write must resolve every link and keep every id unique, but only for the documents it writes. A hand edit that breaks a link or repeats an id is kept, with a warning. A hand edit that breaks a document's header or a rule that every write keeps affects only that document: hikari keeps its last valid version and shows the problem. A wiki that refuses your note is a worse place to write than one that warns.
6. **hikari commits your wiki edits with your git identity.** Plans and notes are knowledge, not configuration, so they need no deliberate review commit as a policy does. Your files in `store/shared/` stay yours to commit. Every tool change is committed when it is made, so an uncommitted change in a wiki is always yours.
7. **The map's feature list holds features only.** Principles, rules, and stories have no code and no unit tests in this step. Their health is planted for the E2E step.
8. **You can write any document by hand,** in Obsidian or in the UI, as B4 agreed for stories. An agent kind that you write must keep the rules of its kind. A user document stays yours: agents never change one.
9. **One lock for every wiki.** A change to one wiki reads the ids of the others and can rewrite their links, so hikari applies one wiki change at a time.
10. **A link names a document by its last path segment.** Obsidian writes `[[systems/orders/wiki/reserve-stock]]` when two files have one name, as a release copy does. hikari reads that link as `[[reserve-stock]]`, so a link that Obsidian completed still works.

## Terms

- **Wiki**: the documents of one system or one hub, in one directory.
- **Document**: one markdown file of a wiki.
- **Kind**: what a document holds: `feature`, `principle`, `rule`, `story`, `plan`, or `note`.
- **Agent document**: a document of a kind that an agent writes: feature, principle, rule, or story.
- **User document**: a plan or a note. You write it.
- **Readable wikis**: the wikis that an agent can read (section 5).
- **Backlinks**: the documents that link a document, in any wiki.
- **Support**: the health of the features that a story or a rule links (section 6).

## 1. The store layout

```
store/
  hubs/<hub>/wiki/<id>.md          the hub wiki; its orchestrator writes the agent documents
  systems/<system>/wiki/<id>.md    the system wiki; its owner writes the agent documents
  systems/<system>/releases/<deliverable>/<x.y.z>/wiki/   the agent documents at release time
```

- **L1.** The store's `Prepare` creates `hubs/<hub>/wiki/` for each hub, and `systems/<system>/wiki/` for each system. `features/` is no longer used, and hikari does not move or detect it (rule O1 of the deliverables spec).
- **L2.** A publish copies the feature, principle, and rule documents of the system wiki into the release's `wiki/`. Plans and notes stay out of releases.
- **L3.** The store README describes `hubs/` and `wiki/`.

## 2. Kinds

| Kind | In | Author | Required section | Health |
|---|---|---|---|---|
| `feature` | system | owner | `## Purpose` | from unit tests, as today |
| `principle` | system, hub | owner, orchestrator | `## Purpose` | none |
| `rule` | system, hub | owner, orchestrator | `## Purpose` | support (hub only) |
| `story` | hub | orchestrator | `## Purpose` | support; E2E results in step 3 |
| `plan` | system, hub | you | none | none |
| `note` | system, hub | you | none | none |

- **K1.** A feature is what the feature wiki spec says it is. A principle is a lasting choice that guides the work, with its reason: "The library stays general: a capability belongs in it when a second, different plugin would want it." A rule is a statement that must always hold: "Stock never goes below zero." A story is a flow that an actor goes through to reach a goal, across systems: "A customer orders two items, and the order reserves stock for both." A plan is work you intend, such as a roadmap item. A note is anything else you want to keep.
- **K2.** `kind` is a header property. A document with no `kind` is a note.
- **K3.** A story may set `key: true`. Only you set it: a tool write that gives `key` is refused, and every write keeps the value that is there.
- **K4.** A plan may set `status`, a free word such as `idea`, `planned`, `doing`, or `done`. hikari shows it and never reads it otherwise.

## 3. The document

An agent document has the header of a feature document today, with `kind` after `id`:

```markdown
---
id: hotspot-opens-incident
kind: story
name: A hotspot opens an incident
description: A hotspot that grows in front of a camera opens an incident in the cloud, and a notification goes out.
related: ["[[alarm-episodes]]", "[[incident-view]]"]
key: true
created: 2026-10-23T05:35:10Z
updated: 2026-10-23T05:35:10Z
---

## Purpose

An operator must learn of a growing hotspot before it becomes a fire. ...
```

- **D1.** The rules W1 to W9 of the feature wiki spec apply to agent documents, with these changes:
  - W1: the id is unique across every wiki of the store, hubs included. A user document's name counts too, compared without case.
  - W3: only a feature may name `interfaces`.
  - W4: every agent kind needs a `## Purpose` with text.
  - `described_commit` and `described_at` exist only in a system wiki. `mark_map_current` sets them on every agent document of the system.
- **D2.** A user document needs only `kind` (or none). `name`, `description`, and every other property are optional; an `id` property is ignored. Its name is its file name without `.md`. A name holds at most 200 bytes, does not start or end with a space or start with a dot, and holds none of the characters that Obsidian refuses: `* " \ / < > : | ? # ^ [ ]`. A name that another document of the store has, compared without case, is a warning.
- **D4.** A file with no property header is a note. A file whose header does not parse, and an agent document that breaks one of W1 to W4, is a problem of that file only: hikari keeps the file's last valid version when it has one, leaves the file out when it does not, and lists the problem in the wiki's warnings. A subfolder of a wiki is not part of it.
- **D5.** A hand edit can change a document's kind. A feature that becomes a note leaves the map; its owner removes its entry from `layout.yaml`. A note that gets an agent kind becomes an agent document of its wiki and must keep the rules of D1.
- **D3.** The order of the known properties is `id`, `kind`, `name`, `description`, `interfaces`, `related`, `key`, `status`, `created`, `updated`, `described_commit`, `described_at`. hikari writes them in that order and keeps your other properties after them.

## 4. Links

- **N1.** A link is `[[id]]` or `[[id|text]]`, as today. A target with slashes names the document of its last segment, and a target that ends in `.md` names the document without it (decision 10). `[[id#heading]]` is refused in an agent document and a warning in a user document.
- **N1a.** The wikis of the store are the wikis of the topology's systems and hubs. Release copies and the folders of systems or hubs that are no longer in the topology are not.
- **N1b.** A link resolves by exact name. When two documents have the name, which only user documents can (D2), it resolves to the one in the linking document's wiki, else in the hub wikis, else in the system wikis, in name order.
- **N2.** A tool write may link only agent documents that its wiki can link:
  - a system wiki: its own documents, and the documents of each hub wiki whose hub the system's owner is a member of;
  - a hub wiki: its own documents, the documents of each system whose owner is a direct member of the hub, and the documents of each hub wiki whose orchestrator is a member of the hub (a child hub) or whose members include the hub's orchestrator (a parent hub).
- **N3.** A user document may link any document of the store. A link that resolves to nothing is a warning.
- **N4.** A hand edit that leaves an agent document with a link that N2 does not allow is kept, with a warning on the wiki. A tool write checks N2 and W1's uniqueness only for the documents it writes, so a warning that a hand edit left does not block the next change.
- **N5.** A rename of an agent document rewrites every link to it in every wiki of the store, user documents included. A remove rewrites them to the redirect, or to plain text, in every wiki. hikari rewrites the file as it is on disk: an agent document that parses is written again from its parsed form; a user document, and a file that does not parse, get the link text replaced and nothing else.
- **N5a.** A change that would write an agent document whose file on disk does not parse is refused, and names the document: the user's edit of it comes first.
- **N6.** Backlinks are computed for every document from the links of every wiki.
- **N7.** The neighbors of a feature in the map's `related` stay features of the same system. The warning "no neighbor" counts the agent documents of the same wiki.

## 5. Who reads what

- **A1.** Every agent reads the wiki of each hub it is in, at `/hikari/public/readonly/hubs/<hub>/wiki/`, read-only.
- **A2.** An orchestrator also reads the wiki of each child hub: a hub whose orchestrator is a member of a hub that it orchestrates.
- **A3.** An owner reads its own wiki at `/hikari/public/export/wiki/`, as it reads `features/` today. An orchestrator reads its visible systems' wikis at `/hikari/public/readonly/systems/<system>/wiki/`.
- **A4.** Readable wikis, for the read tools, are the wikis of A1 to A3.

## 6. Health and support

- **H1.** A feature's health is computed as today. The map's `features` list holds only features.
- **H2.** The support of a story or a rule in a hub wiki is the health of the features it links, in any system, summed as the topology map sums a system: `failing` when one is failing, else `pending` when one is pending, else `passing` when one is passing, else `untested`. A story that links no feature has no support.
- **H3.** In this step, a story and a rule have no health of their own. Step 3 adds it from E2E results.

## 7. Tools

- **T1.** `change_wiki(writes)` replaces `change_features` for owners. A create gives `kind`: `feature`, `principle`, or `rule`. The other ops are as today. hikari writes the map from the wiki after each change.
- **T2.** `change_hub_wiki(hub, writes)` is for the orchestrator of the hub. A create gives `kind`: `story`, `rule`, or `principle`.
- **T3.** A tool write that gives `kind` `plan` or `note`, or that modifies, renames, or removes a user document, is refused: user documents are yours. The tools have no `key` field, so no tool can set it.
- **T4.** `get_document(id)` replaces `get_feature` and is open to every agent. It returns the document as it is on disk, its wiki, its kind, its backlinks in the agent's readable wikis, and, for a feature, its health and state, or, for a story or a rule of a hub, its support. For a document outside the agent's readable wikis, it says which wiki holds it and that the agent should ask its orchestrator.
- **T5.** `search_wiki(query)` replaces `search_features` and is open to every agent. It searches the readable wikis and returns each hit's wiki, id, kind, name, and description.
- **T6.** `get_my_system` lists the warnings of the system wiki, as it lists the feature warnings today.

## 8. Notices

- **E1.** Before each turn, an agent learns which documents of its readable wikis changed since its last turn, by someone other than itself. hikari keeps, for each agent and each readable wiki, the text of each document as of the agent's last turn, and updates it for the documents that the agent's own changes write: `<notice>The wiki of #shop changed: hotspot-opens-incident (story), roadmap (plan).</notice>`. Changes to the wikis of an orchestrator's visible systems are left out, because owners change them often and the map shows them.
- **E2.** The prompts say that plans and notes are the user's: read them for context, and act on a plan only when the user asks for it.

## 9. The project repository

This section changes the rules PR14 to PR17 of `docs/specs/project-repo.md`.

- **C1.** Before a tool change of a wiki, hikari commits your uncommitted edits of that wiki with your git identity: `user: wiki edits before <agent>'s change`. So an agent's commit never holds your edit.
- **C2.** After a change, hikari commits the files that the change wrote or deleted, with the map for an owner, and the files of other wikis that a rename or a remove rewrote. The author is the agent. The message is `<agent>: change the wiki: <writes>` or `<orchestrator>: change the #<hub> wiki: <writes>`.
- **C3.** At stop, hikari commits each owner's `map.yaml` as the owner, and every other change in the wikis with your git identity: `user: wiki edits left when hikari stopped`. Every tool change is already committed, so what is left is yours.
- **C4.** A change that you make in the UI (section 10) is committed at once with your git identity: `user: <write, remove, mark key, unmark key> <id> in <wiki>`.
- **C5.** When `hikari up` finds the project branch checked out and the only uncommitted changes in the wiki folders, it commits them with your git identity before it checks that the repository is clean. On another branch, it refuses as before.
- **C6.** When git has no identity, a commit with your identity fails. The server logs it, and the change stays uncommitted.

## 10. The UI

The snapshot carries every wiki: for each document its id, kind, name, description, links, `key`, `status`, and, for a feature, its health, for a story or a rule, its support. A change to a wiki is sent as an event.

- **U1.** The topology map draws a node for each hub, with the shape of a hexagon, joined to each member that owns a system and to each child hub. A hub node opens into the hub's wiki map, as a system opens into its feature map. The lenses of the topology map leave a hub node neutral.
- **U2.** A system's map becomes its wiki map: one node per document of every kind. A hub's wiki map has one node per document of the hub. Both maps add one smaller node for each document of another wiki that links a document of the map or that a document of the map links, labeled with that wiki. An edge joins two documents when either links the other.
- **U3.** Each kind has a shape: a feature and a story a circle, a principle a diamond, a rule a square, a plan and a note a small circle with a dashed outline.
- **U4.** The wiki maps have three lenses: Kind (a color for each kind), Tests (a feature's health, a story's or a rule's support), and Age. A document with no `created`, such as a user document, has the oldest age.
- **U5.** The document panel shows any document: its kind, its wiki, its key or status, its health or support, the body, and its backlinks. A link in the body or in the backlinks selects that document, and opens its wiki when it is in another.
- **U6.** On a wiki map, you can create a document of any kind that the wiki holds, edit any document, remove a user document, and mark or unmark a story key. The editor is a text area with the document's markdown.
- **U7.** A hub's page gets a Wiki tab beside Messages and Vocabulary, which lists its documents by kind, key stories first, and opens each one on the map.
- **U8.** The server checks a UI write as a hand edit that must be valid: an agent document must keep the rules of D1, including N2 and uniqueness, and a user document's name must keep D2. It refuses a write that does not, with the problems. You cannot remove an agent document in the UI; ask its author.

## 11. Skills and prompts

- **S1.** The `hikari-system` skill teaches the kinds of a system wiki: when a behavior is a feature, when a choice is a principle, when a statement is a rule. `references/features.md` becomes `references/wiki.md`.
- **S2.** A new built-in skill, `hikari-hub`, is installed for orchestrators. It teaches how to write stories, rules, and principles, how to link them to the features of the hub's systems, and what a key story is.
- **S3.** The owner prompt names the hub wikis it reads. The orchestrator prompt names the hub wiki, its tool, and the child hub wikis it reads.
- **S4.** `hikari-hub` is a reserved name, like `hikari-system`: a shared skill of that name is skipped, and a deliverable of that name is refused.

## 11a. Limits

- The store README is written only when it is missing, so an existing store keeps its old README.
- hikari compares names as they are on disk. macOS stores a name in one Unicode form, and a link typed in another form does not resolve.

## 12. Out of scope

- E2E health of stories and rules (step 3).
- Unit tests that link rules or principles.
- Code paths for principles and rules.
- A rich editor in the UI. You can always edit the files in Obsidian.

## 13. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **The package.** Rename `featurewiki` to `wiki`; kinds, the header, the rules of D1 to D3 and K1 to K4; loose loads (decision 5); links that resolve through a check from outside (N2, N3).
2. **The store.** `wiki/` for systems, `hubs/<hub>/wiki/`, the release copy (L1 to L3).
3. **The manager.** Hub wikis, the tools (T1 to T6), links and renames across wikis (N2 to N6), support (H2), notices (E1), commits (C1 to C3).
4. **Mounts, tools, prompts, and skills.** Sections 5, 7, 8, and 11 in the launcher, the MCP server, and the driver.
5. **UI.** The snapshot, the events, the write endpoints (C4, U8), and the views (U1 to U7).
6. **yami and the shop.** Move yami's `features/` to `wiki/` with `kind: feature` in each document.
7. **Live check and docs.** In a project repository, the orchestrator writes stories and a principle that link the features of two owners; you add a plan in Obsidian and a note in the UI; an owner reads the hub wiki and is told of the changes; the commits carry the right authors. Then the README, `docs/design.md`, and the feature wiki spec's pointer to this spec.
