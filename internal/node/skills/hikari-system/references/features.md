# The feature wiki

One markdown document for each feature, in `/hikari/public/export/features/<id>.md`. You read the files; you change them only with `change_features`.

## The document

```markdown
---
id: reserve-stock
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
| `name` | you | two to five words, as a user would say it |
| `description` | you | one sentence, at most 200 characters |
| `interfaces` | you | optional; names from the system's `provides` or `consumes` |
| `related` | you | optional; neighbors that the body does not link, given as ids in a write |
| `created` | hikari | the create write; a rename keeps it |
| `updated` | hikari | the last write of the document |
| `described_commit`, `described_at` | hikari | set by `mark_map_current` |

A property that no rule names belongs to the user; hikari keeps it and never reads it.

The body has one required section, `## Purpose`: why the feature exists and for whom, what it promises, and what it refuses. Two to six sentences; more when the motivation needs it. Use the vocabulary's names. Name no path, no identifier from the code, and no command: the orchestrator reads it, and a reader who cannot see the code must understand it. Other sections are allowed. `## Notes` belongs to the user: never write it. A modify replaces the body but keeps the user's `## Notes`.

## The rules

hikari refuses a change that breaks one of these and names the write and the rule.

- **W1.** The id matches `^[a-z][a-z0-9-]{0,47}$`. It is unique in your system and in every other system of the network.
- **W2.** `name` is present. `description` is one sentence on one line, at most 200 characters.
- **W3.** Each name in `interfaces` is in your `provides` or `consumes`. Call `set_system_info` first.
- **W4.** The body has a `## Purpose` section with text. A write never gives a `## Notes` section.
- **W5.** `created`, `updated`, `described_commit`, and `described_at` are hikari's. A write that gives one is applied without it, with a warning.
- **W9.** A change has at most 100 writes.
- **K1.** A link is `[[id]]` or `[[id|text]]`. A link to a heading, `[[id#heading]]`, is refused.
- **K2.** Every link resolves to a feature of your system, counting the features that the same change creates. A document does not link itself.

Warnings, which never refuse a change: a feature with no neighbor; a feature with no `code` entry in `layout.yaml`; a Purpose that names a path in your scope; more than 60 features.

## The change

`change_features` takes a list of writes and applies them in order as one change.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `name`, `description`, `body`; optional `interfaces`, `related` | writes a new document |
| `modify` | `id`; `fields` (`name`, `description`, `interfaces`, `related`), `body`, or both | merges the fields and replaces the body, keeping `## Notes` |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in your wiki |
| `remove` | `id`; optional `redirect` | removes the document; links to it point at the redirect, or become plain text |

An empty list in `fields` clears `interfaces` or `related`. After a rename or a remove, the result names the private documents that still use the old id; update them.

## What a feature is

- A feature is one thing the system does that someone outside the code relies on: a user, another system, or the operator. Name who relies on it. If no one outside the code would notice it gone, it is not a feature; it is a part, and it belongs in `layout.yaml`.
- State it in one sentence with one verb and one object: "Reserve stock for an order". If the sentence needs "and", consider two features.
- It can be tested on its own: you can say what proves that it works.
- Not a feature: a file, a package, a class, a helper, a dependency, a refactor, a build step, a test suite, a configuration value. A library's public function is usually a feature; its internal helper is not.
- Split when the two halves have different users, different interfaces, or different failure modes. Merge when neither half makes sense without the other, or you cannot test one without the other.
- A cross-cutting behavior is a feature when it is a promise to someone: "Every request is authenticated". Otherwise it is a part.
- Most systems have 5 to 40 features. Above 60, you are listing parts. Below 3, you are describing the system, not its features.
- The name is two to five words, as a user would say it. The id is the name in lower case with dashes. An id is stable: change the wording of a name freely, and rename the id only when the feature itself changed meaning.

## How to link

Link two features when:

1. one needs the other to work: it calls it, or consumes what it produces;
2. they act on the same thing: the same object of the vocabulary, the same data, the same resource;
3. a user does them together in one flow: create an order, then cancel it.

Do not link because two features live in the same file or package. Do not say what kind of link it is; the prose around a link can say why in a few words, and hikari never reads that. A link has no direction: write it once, on either document, where the prose makes it natural, and hikari reads the graph both ways. Every feature should have a neighbor. One with none is either part of nothing, which means the feature set is wrong, or new. A feature that links most of the others is a hub; that is information about the system, not a mistake. There are no feature groups: the links are the structure.
