# The system wiki

One markdown document for each feature, principle, and rule of your system, in `/hikari/public/export/wiki/<id>.md`. You read the files; you change your documents only with `change_wiki`. The user's plans and notes are in the same folder.

## The kinds

| Kind | Who writes it | What it holds |
|---|---|---|
| `feature` | you | One thing the system does that someone outside the code relies on. Most documents are features. |
| `principle` | you | A lasting choice that guides how the system is built, with its reason: what it rules out, and why. |
| `rule` | you | A statement about the system's data or behavior that must always hold: "A reservation never exceeds the available stock." |
| `plan` | the user | Work the user intends, with a `status`. Read it for context; act on it only when the user asks. |
| `note` | the user | Anything else the user keeps. A file with no `kind` is a note. |

A behavior that someone relies on is a feature. A promise that every feature must keep is a rule. A reason for how you build is a principle. Most systems have few principles and few rules; a system with none is normal.

## The document

```markdown
---
id: reserve-stock
kind: feature
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
related: ["[[check-availability]]"]
created: 2026-10-22T05:57:04Z
updated: 2026-10-22T05:57:04Z
described_commit: c69dd499311b21c5210c0ae3e984060aa0ba051a
described_at: 2026-10-22T05:57:09Z
---

## Purpose

An order must not promise stock that another order already holds. A reservation moves a quantity from available to held, so that two orders cannot both take the last unit. It refuses a request for more than is available, and it refuses a quantity that is not positive, because a reservation of nothing hides a bug in the caller. A held quantity returns to available through [[release-stock]].
```

| Property | Who sets it | Value |
|---|---|---|
| `id` | you | the file name; lowercase letters, digits, and dashes, at most 48 characters |
| `kind` | you | `feature`, `principle`, or `rule` |
| `name` | you | two to five words, as a user would say it |
| `description` | you | one sentence, at most 200 characters |
| `interfaces` | you | a feature only; names from the system's `provides` or `consumes` |
| `related` | you | optional; neighbors that the body does not link, given as ids in a write |
| `created` | hikari | the create write; a rename keeps it |
| `updated` | hikari | the last write of the document |
| `described_commit`, `described_at` | hikari | set by `mark_map_current` |

A property that no rule names belongs to the user; hikari keeps it and never reads it.

The body has one required section, `## Purpose`. For a feature: why it exists and for whom, what it promises, and what it refuses. For a principle: the choice, what it rules out, and why. For a rule: why it must hold, and what breaks if it does not. Two to six sentences; more when the motivation needs it. Use the vocabulary's names. Name no path, no identifier from the code, and no command: other agents read it, and a reader who cannot see the code must understand it. Other sections are allowed. `## Notes` belongs to the user: never write it. A modify replaces the body but keeps the user's `## Notes`.

## The rules

hikari refuses a change that breaks one of these and names the write and the rule.

- **W1.** The id matches `^[a-z][a-z0-9-]{0,47}$`. It is unique in every wiki of the store: your system, every other system, and the public wiki.
- **W2.** `name` is present. `description` is one sentence on one line, at most 200 characters.
- **W3.** Only a feature names `interfaces`, and each is in your `provides` or `consumes`. Call `set_system_info` first.
- **W4.** The body has a `## Purpose` section with text. A write never gives a `## Notes` section.
- **W5.** `created`, `updated`, `described_commit`, and `described_at` are hikari's. A write that gives one is applied without it, with a warning.
- **W9.** A change has at most 100 writes.
- **K1.** A link is `[[id]]` or `[[id|text]]`. A link to a heading, `[[id#heading]]`, is refused.
- **K2.** Every link resolves to a feature, principle, or rule of your wiki, counting the documents that the same change creates, or to a story, rule, or principle of the public wiki. A document does not link itself.

Warnings, which never refuse a change: a feature with no neighbor; a feature with no `code` entry in `layout.yaml`; a Purpose that names a path in your scope; more than 60 features.

## The public wiki

The public wiki is in `/hikari/public/readonly/shared/wiki/`. Its stories say which flows use your features, its rules say what must always hold across the systems, and its principles guide how the systems work together. Keep them: a rule of the public wiki is a rule of your system. When a story needs a feature you do not have, or your feature cannot keep a rule of the public wiki, tell the orchestrator. Only the orchestrator changes the public wiki.

## The change

`change_wiki` takes a list of writes and applies them in order as one change.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `kind`, `name`, `description`, `body`; optional `interfaces` (a feature), `related` | writes a new document |
| `modify` | `id`; `fields` (`name`, `description`, `interfaces`, `related`), `body`, or both | merges the fields and replaces the body, keeping `## Notes` |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in every wiki |
| `remove` | `id`; optional `redirect` | removes the document; links to it point at the redirect, or become plain text |

An empty list in `fields` clears `interfaces` or `related`. After a rename or a remove of a feature, the result names the private documents that still use the old id; update them. You cannot write, rename, or remove the user's plans and notes.

## What a feature is

- A feature is one thing the system does that someone outside the code relies on: a user, another system, or the operator. Name who relies on it. If no one outside the code would notice it gone, it is not a feature; it is a part, and it belongs in `layout.yaml`.
- State it in one sentence with one verb and one object: "Reserve stock for an order". If the sentence needs "and", consider two features.
- It can be tested on its own: you can say what proves that it works.
- Not a feature: a file, a package, a class, a helper, a dependency, a refactor, a build step, a test suite, a configuration value. A library's public function is usually a feature; its internal helper is not.
- Split when the two halves have different users, different interfaces, or different failure modes. Merge when neither half makes sense without the other, or you cannot test one without the other.
- A cross-cutting behavior is a feature when it is a promise to someone: "Every request is authenticated". A promise about the data that every feature keeps is a rule. Otherwise it is a part.
- Most systems have 5 to 40 features. Above 60, you are listing parts. Below 3, you are describing the system, not its features.
- The name is two to five words, as a user would say it. The id is the name in lower case with dashes. An id is stable: change the wording of a name freely, and rename the id only when the feature itself changed meaning.

## How to link

Link two documents when:

1. one needs the other to work: it calls it, or consumes what it produces;
2. they act on the same thing: the same object of the vocabulary, the same data, the same resource;
3. a user does them together in one flow: create an order, then cancel it;
4. a feature keeps a rule or follows a principle: link the rule or the principle.

Do not link because two features live in the same file or package. Do not say what kind of link it is; the prose around a link can say why in a few words, and hikari never reads that. A link has no direction: write it once, on either document, where the prose makes it natural, and hikari reads the graph both ways. Every feature should have a neighbor. One with none is either part of nothing, which means the feature set is wrong, or new. A feature that links most of the others is a center of the system; that is information about the system, not a mistake. There are no feature groups: the links are the structure.
