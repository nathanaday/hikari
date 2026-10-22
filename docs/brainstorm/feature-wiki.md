# Feature wiki: round 1

## What I take from your proposal

Today a feature is one entry in `map.yaml`, with its code paths in `layout.yaml` and its tests in `test.yaml`. The map picks up most features, but it has no room for the reasons behind a feature, its links are a bare list of ids, and nothing says when one feature's description was last true.

You want each feature to be its own markdown document, in the style of the Obsidian wikis that Almagest builds:

- One document per feature, with a property header that code can check: a feature id, the code paths, and when and at which commit it was recorded.
- A body for people and for other agents: purpose, motivation, what the feature promises. The body does not cite code.
- Links between neighboring features, with no label on the link and no feature groups. The groups come out of the links.
- The owner proposes a change set, and a tool applies it, so that every document always has the schema.
- A skill package in each owner's container that makes the result consistent: the skill, references, examples, and read-only mapping agents.
- Documents that Obsidian can open, so that the UI can render the wiki later.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

- **Feature document**: one markdown file that describes one feature: a property header and a body.
- **Feature wiki**: the feature documents of one system, in one directory.
- **Neighbor**: a feature that a document links. A link has no direction and no kind.
- **Change**: one set of writes to the wiki that the server applies as a whole, or not at all.
- **Described commit**: the commit whose code a document describes.

## What changes in the current design

1. The feature wiki replaces the `features` list as the thing the owner writes. `map.yaml` keeps its `features` list, but the server now generates it from the wiki, so that orchestrators, releases, and the UI keep working as they are.
2. `layout.yaml` keeps `parts` and loses `features`. The code paths of a feature move into its document.
3. `test.yaml` does not change.
4. The tools `upsert_features` and `remove_feature` go away. One tool, `change_features`, applies a change to the wiki.
5. The `hikari-system` skill grows from one file into a package: the skill, references, examples, and two agents.
6. Staleness becomes per feature. The driver names the features that a change in the code affects, not only the paths.

---

## A. Where the wiki lives and who reads it

**A1. In the scope, written by the server.** I recommend `<scope>/.hikari/features/<id>.md` on the host, mounted read-only at `/hikari/features/` in the owner's container. The server writes every document, through `change_features`, as it writes the map today.

- The documents describe the code at a commit, so they belong in the repo and change in the same commits as the code, as the private documents do today (`docs/design.md`, "One store for everything that is not code").
- You review them with the code, in the branch that hikari commits to.
- The server writes them, so the schema always holds and the server knows at once when the wiki changed. A rename rewrites every link, which an agent editing files by hand would get wrong.
- Obsidian opens the repo, or the scope, as a vault. Links resolve by file name, so `[[reserve-stock]]` works from any folder.

The alternative is the store, `store/systems/<system>/features/`, which would make the whole store one vault of every system. But the documents hold code paths, and the store is mounted read-only into every orchestrator's container, so paths would leak into the one place that must hold none.

Do you agree?

**A2. The code paths are the only private part.** A document is public except for its `code` property. The server projects each document into the public map (A3) without `code`, and `get_feature` returns the body to the orchestrator. The rule that the body cites no code (B2) keeps the body public-safe.

Do you agree?

**A3. The map is a projection.** `map.yaml` goes to version 3. Its `system` section stays as it is, written by `set_system_info`. Its `features` list is generated from the wiki: `id`, `name`, `description`, `interfaces`, `findings`, `plans`, `related` (the neighbors, C2), `tests`, and `health`. The owner cannot write the list directly. Releases copy the map as today, so a release still shows what it provides.

Do you agree?

**A4. layout.yaml keeps the parts.** `parts` stays as the owner's orientation in the code and the place for things that are not features: a package, a helper, a test fixture. `layout.yaml` goes to version 2 with `parts` only; a file with `features` is refused with a message that names the wiki. `test.yaml` does not change: it is the owner's working notes, edited by hand, and a feature's tests are a plan, not a description.

Do you agree?

## B. The feature document

**B1. The header.** The file name is the id. The properties:

```yaml
---
id: reserve-stock                       # the file name; ^[a-z][a-z0-9-]{0,47}$
name: Reserve stock                     # two to five words, as a user would say it
description: Hold a quantity of a SKU for an order when enough is available.   # one sentence, at most 200 characters
interfaces: [Stock.reserve]             # names from the system's provides or consumes
code: [inventory/stock.py]              # private: paths relative to the scope; a directory covers its files
related: []                             # optional: links that the body does not make, as "[[id]]"
findings: [State is in memory only; nothing is persisted.]
plans: []
updated: 2026-10-22T05:57:04Z           # code: the last write of this document
described: {commit: c69dd49, at: 2026-10-22T05:57:09Z}   # code: set by mark_map_current
tests: {passed: 7, failed: 0, ran: 2026-10-22T05:57:04Z, content_hash: dfa1e19f09fff748}   # code
health: passing                         # code: the health at the last write
---
```

The owner gives `id`, `name`, `description`, `interfaces`, `code`, `related`, `findings`, and `plans`. Code owns `updated`, `described`, `tests`, and `health`, drops them from any write, and warns. A property that no schema names is yours: code keeps it and never reads it, so you can add your own in Obsidian.

Do you agree?

**B2. The body.** One required section, `## Purpose`: why the feature exists and for whom, what it promises, and what it refuses. Two to six sentences; more when the motivation needs it. It uses the vocabulary's names. It names no path, no identifier from the code, and no command: the orchestrator reads it, and a reader who cannot see the code must understand it. Other sections are allowed. `## Notes` is yours: the owner never writes it, and a modify that changes it is refused.

Two examples are at the end of this document.

Do you agree?

**B3. Links.** A link is `[[id]]` or `[[id|Name]]`, in the body or in `related`. Every link in a document, wherever it is, is a neighbor. A link has no direction: the owner writes it once, on either document, and the server reads the graph both ways. A link has no kind: the prose around it can say why, in a few words, but code never reads that. Every link must resolve to a feature of the same system, counting the features that the same change creates.

The alternative is one place only, a `related` list in the header. It is easier to check, but then prose that names a neighbor would have to repeat it in the header, and the two drift apart.

Do you agree?

**B4. Ids are unique in the repo.** Two systems in one repo cannot use the same id. The server knows every system of a repo, so it refuses a create whose id another system uses, with a message that says only that. Without this rule, Obsidian opened on the repo would resolve `[[create]]` to whichever file it finds first.

Do you agree?

## C. The rules

**C1. The checks.** The server refuses a change that breaks a rule and names the write and the rule, as it does for the map today.

- **F1.** The id matches `^[a-z][a-z0-9-]{0,47}$`, is unique in the system and in the repo (B4), and is the file name.
- **F2.** `name` is present. `description` is one sentence of at most 200 characters.
- **F3.** Each name in `interfaces` is in the system's `provides` or `consumes`.
- **F4.** Every link resolves to a feature of the system, and no document links itself.
- **F5.** Each path in `code` is relative to the scope, stays inside it, and exists.
- **F6.** The body has `## Purpose`. A modify keeps `## Notes` as it was.
- **F7.** Code-owned properties are dropped from a write, with a warning.
- **F8.** A change has at most 100 writes.

Warnings, which never refuse: a document with no link, a document with no `code`, a `## Purpose` that contains a path that exists in the scope, and a system with more than 60 features. `get_my_system` lists them, as it lists the problems of the private documents today.

Do you agree?

**C2. Neighbors in the map.** The map's `related` of a feature is every feature that it links or that links it, sorted. This is the only place code uses the graph for now.

Do you agree?

## D. Staleness per feature

**D1. `described` is per document, set by `mark_map_current`.** `mark_map_current` commits the pending changes in the scope, then records the commit and the time in every document and in the map's `described_state`, as today. A write of a document sets `updated` only. So a header always says when its text last changed and which commit it describes.

**D2. A feature is stale when its code changed.** The server compares the current contents of a feature's `code` paths with their contents at `described.commit`, counting uncommitted changes and new files, as the map state does today (rule S14). The map stays `stale` as a whole when any path in the scope changed, and the owner view and the driver's notice now say which features the changed paths belong to, and which changed paths belong to no feature. A feature's state is separate from its health: health comes from tests, state from the description.

This replaces "these paths changed, update the affected entries" with "these features are stale: reserve-stock (inventory/stock.py), and these paths belong to no feature: inventory/report.py". The second half is how a missing feature gets found.

Do you agree with D1 and D2?

## E. The change tool

**E1. One tool, `change_features(writes)`.** It takes a list of writes and applies them as one change, or none when any write breaks a rule. The ops, after Almagest's change contract:

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `name`, `description`, `body`, and the optional properties | a new document |
| `modify` | `id`, `fields` (merged over the header; `null` removes one), `body` (replaces the body; leave it out to keep it) | changes a document |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in the system |
| `remove` | `id`, optional `redirect` | removes the document; links to it go to the redirect, or become plain text |

The result lists each write as applied, with the warnings. The tool has no approval step: the owner is the author, it runs headless, and the server's checks are the gate. Test results stay on a modify and a rename, as `upsert_features` keeps them today.

Do you agree?

**E2. The other tools stay.** `set_system_info`, `record_test_results`, `mark_map_current`, `commit_changes`, `get_my_system`, and the orchestrator tools keep their names. `get_feature` adds the body. `search_features` also searches the body.

Do you agree?

## F. The skill package

**F1. Files.** The built-in skill becomes a directory, embedded in the binary and written into the container at start as today, with two agents beside it:

```
~/.claude/skills/hikari-system/
  SKILL.md                       the procedures: build, keep true, test, release
  references/features.md         the document, the rules, what a feature is, how to link
  references/tests.md            test.yaml, as it is in the skill today
  references/examples/           three feature documents from the shop demo
~/.claude/agents/
  hikari-feature-survey.md       read-only: candidate features in a part of the scope
  hikari-feature-review.md       read-only: the wiki checked against the code
```

The agent names carry the `hikari-` prefix, and the topology check refuses a shared agent with that prefix, as it refuses a shared skill named `hikari-system`.

Do you agree?

**F2. What a feature is.** This is the heart of the package, and the part that must make two owners map the same way. My draft of the guidelines in `references/features.md`:

- A feature is one thing the system does that someone outside the code relies on: a user, another system, or the operator. Name who relies on it. If no one outside the code would notice it gone, it is not a feature; it is a part, and it belongs in `layout.yaml`.
- State it in one sentence with one verb and one object: "Reserve stock for an order". If the sentence needs "and", consider two features.
- It can be tested on its own: you can say what proves that it works.
- Not a feature: a file, a package, a class, a helper, a dependency, a refactor, a build step, a test suite, a configuration value. A library's public function is usually a feature; its internal helper is not.
- Split when the two halves have different users, different interfaces, or different failure modes. Merge when neither half makes sense without the other, or you cannot test one without the other.
- A cross-cutting behavior is a feature when it is a promise to someone: "Every request is authenticated". Otherwise it is a part.
- Most systems have 5 to 40 features. Above 60, you are listing parts. Below 3, you are describing the system, not its features.
- The name is two to five words, as a user would say it. The id is the name in lower case with dashes. An id is stable: change the wording of a name freely, and rename the id only when the feature itself changed meaning.

Do you agree with this draft, and is there a rule you would add?

**F3. How to link.** My draft:

Link two features when:

1. one needs the other to work: it calls it, or consumes what it produces;
2. they act on the same thing: the same object of the vocabulary, the same data, the same resource;
3. a user does them together in one flow: create an order, then cancel it.

Do not link because two features live in the same file or package. Do not say what kind of link it is. Write the link once, on either document, where the prose makes it natural. Every feature should have a neighbor; one with none is either part of nothing, which means the feature set is wrong, or new. A feature that links most of the others is a hub; that is information about the system, not a mistake.

Do you agree?

**F4. Build.** The procedure in `SKILL.md` for a system with no wiki:

1. Orient: read the instruction files, the manifests, the entry points, and the test directory names. Write `parts` in `layout.yaml`.
2. Survey: for each part, send `hikari-feature-survey` with the part's paths, or read the part yourself when the scope is small (about 40 source files or fewer). Each survey returns candidate features: name, one sentence, who relies on it, code paths, interfaces, evidence as `path:line`, and candidate neighbors with the number of the link rule.
3. Decide the feature set against F2: merge, split, name, and give ids.
4. Link against F3.
5. Write one change with every create.
6. Write `test.yaml`, run the tests, record the results, and call `mark_map_current`.

Do you agree?

**F5. Keep true.** When the driver reports stale features and unmapped paths:

1. For each stale feature, read the changed paths and decide: the description still holds (no write), the behavior changed (modify), or the feature is gone (remove with a redirect).
2. For each unmapped path, decide: it belongs to a feature (modify its `code`), it is a new feature (create), or it is a part (add it to `parts`).
3. When the change is large, more than about ten changed paths, send `hikari-feature-review` first and work from its findings.
4. Run the tests of the affected features, record the results, call `commit_changes`, then `mark_map_current`.

Do you agree?

**F6. The agents.** Both are read-only and return JSON in their final message, after the Almagest workers.

- `hikari-feature-survey` takes the paths of one part, the system's interfaces, and the vocabulary. It returns the candidate features of F4 step 2. It never writes. The owner merges the surveys, so that links cross parts.
- `hikari-feature-review` takes the wiki and the scope. It checks each document against the code and returns findings: a description that the code no longer supports, a `code` list that misses a file of the feature, a behavior in the code with no document, a link that rule 1 of F3 requires and that no document makes, and a document that describes two things. Each finding names the document, the evidence as `path:line`, and the suggested write. The owner decides.

Do you agree?

## G. Orchestrators and the UI

**G1. Orchestrators see the body.** `get_feature` returns the document without `code`: the header and the body. `get_system` and `list_systems` do not change. The orchestrator prompt says that a feature's purpose is in `get_feature`.

Do you agree?

**G2. The UI shows the document.** A feature opens to show the body rendered as markdown, with each link opening that feature, and its state (current or stale) beside its health. The system view lists the unmapped paths with the changed paths. That is the whole UI change for now; rendering the wiki as a vault, with a graph, waits.

Do you agree?

## H. Existing maps

**H1. Convert at startup.** When a system has a version 2 map with features and no `features/` directory, the server writes one document per feature from the map and from `layout.yaml`: the properties from the map's entry, `code` from the layout's entry, `related` from the map's `related`, and a body whose `## Purpose` is the description. It sets `described` from the map, removes `features` from `layout.yaml`, writes the version 3 map, and logs what it did. So the shop demo and your yami maps keep their features, and the owner's next turn improves the bodies instead of starting over.

Do you agree?

## I. Out of scope

I keep these out. Say so if one must be in.

- A generated index page, `index.md`, in the wiki. Obsidian's file list and graph serve for now, and the UI will render the wiki.
- Groups computed from the links: connected components or communities, shown in the UI. The graph is in the map's `related`, so this can come later without a change to the documents.
- A `terms` property that links a feature to the vocabulary's terms, so that an orchestrator can find the features of every system that touch an order.
- A public copy of the wiki in the store, without `code`.
- Editing the wiki in the UI.
- Links between systems.

---

## Examples

Two documents of the shop demo, as the owner of `inventory` and the owner of `orders` would write them.

`services/inventory/.hikari/features/reserve-stock.md`:

```markdown
---
id: reserve-stock
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
code: [inventory/stock.py]
findings: [State is in memory only; nothing is persisted.]
updated: 2026-10-22T05:57:04Z
described: {commit: c69dd49, at: 2026-10-22T05:57:09Z}
tests: {passed: 7, failed: 0, ran: 2026-10-22T05:57:04Z, content_hash: dfa1e19f09fff748}
health: passing
---

## Purpose

An order must not promise stock that another order already holds. A reservation moves a quantity from available to held, so that two orders cannot both take the last unit. It refuses a request for more than is available, and it refuses a quantity that is not positive, because a reservation of nothing hides a bug in the caller. A held quantity returns to available through [[release-stock]], and [[check-availability]] reports what is left.
```

`services/orders/.hikari/features/cancel-order.md`:

```markdown
---
id: cancel-order
name: Cancel an order
description: Cancel a placed order and release the stock it holds.
interfaces: [cancel_order, Stock.release]
code: [orders/cancel.py]
related: ["[[validate-order]]"]
plans: [Refuse a cancellation after the order shipped.]
updated: 2026-10-22T05:57:47Z
described: {commit: 18fb34a, at: 2026-10-22T05:57:49Z}
tests: {passed: 3, failed: 0, ran: 2026-10-22T05:57:47Z, content_hash: 4330ee154f75885f}
health: passing
---

## Purpose

A customer can change their mind after [[create-order]] placed an order, and the stock that the order holds must go back on sale. Cancelling sets the order to cancelled and releases each item's reserved quantity through the inventory. An order can be cancelled once: a second cancellation is refused, so that stock is never released twice.

## Notes

Shipping does not exist yet; see plans.
```

In the first document, the body makes both links, so `related` is left out. In the second, `related` adds a link that the prose does not make.
