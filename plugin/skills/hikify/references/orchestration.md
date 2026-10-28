---
name: hikari-map-features
description: How the orchestration agent keeps its documents - its own feature map of features that link the features of the partitions, which you change through change_wiki; the public wiki of rules and principles for every agent, which you change through change_public_wiki; and the vocabulary. Load it before you write or change your feature map, the public wiki, or the vocabulary.
---

# Your feature map

Your feature map is in `/hikari/self/features/`: one markdown document for each feature, principle, and rule of yours, and the user's plans and notes. You change your documents only with `change_wiki`; the files are read-only to you.

A partition's feature map holds what one partition does. Yours holds what the partitions do together: each of your features is a flow across partitions, made of the partition features that it links. It has no code and no tests of its own. Its health is computed from the features it links: failing when one of them is failing, pending when one is pending, untested when none is tested, and otherwise passing. A principle or a rule shows its support the same way.

Only you read your feature map. The partition agents never see it, and its links to their features do not show among their backlinks. A principle or a rule in your feature map binds only you; a principle or a rule for every agent belongs in the public wiki.

## The kinds you write

| Kind | Where | What it holds | Example |
|---|---|---|---|
| `feature` | your feature map | A flow that an actor goes through to reach a goal, across partitions, and its outcome. | "A customer orders two items, and the order reserves stock for both." |
| `rule` | your feature map or the public wiki | A statement that must always hold, in every flow. | "Stock never goes below zero." |
| `principle` | your feature map or the public wiki | A lasting choice that guides how the partitions work together, with its reason. | "The edge computes each alarm; the cloud never recomputes one." |

- A feature names its actor (a user, an operator, a device, another partition), the goal, the steps in order, and the outcome that proves it worked. Write the steps as prose with a link to the partition feature of each step: "The customer places the order through [[create-order]], which holds stock with [[reserve-stock]]." A feature that a test can check is a good feature: say what someone would see when it works.
- A rule is one sentence that is true or false at any moment. Link the features that must keep it, or let the features link the rule. A rule that only one partition can break belongs in that partition's wiki; ask the partition agent.
- A principle is a choice, not a fact: say what it rules out, and why. Few principles are better than many.
- Do not write a partition's feature in your feature map. A feature that one partition carries out alone belongs to that partition. If no partition has a step of a feature yet, write the feature anyway, name the missing step in its Purpose, and ask a partition agent for the step.

## The document

```markdown
---
id: customer-orders-items
kind: feature
name: A customer orders items
description: A customer orders two items, and the order reserves stock for both before it is placed.
related: ["[[stock-never-negative]]"]
created: 2026-10-23T05:35:10Z
updated: 2026-10-23T05:35:10Z
---

## Purpose

A customer must not be told that an order is placed when the stock is not there. The customer sends the items through [[create-order]]. Orders checks the items with [[validate-order-items]] and holds the stock of each item with [[reserve-stock]] in inventory. When every item is held, the order is placed; when one is not, no stock stays held and the customer learns which item is short.
```

The rules are those of the partitions' feature maps:

- The id is lowercase letters, digits, and dashes, at most 48 characters, and unique in the whole vault: no other wiki may use it.
- `name` is two to five words. `description` is one sentence of at most 200 characters.
- The body has a `## Purpose` section with text. Name no code. `## Notes` is the user's; never write it.
- A link is `[[id]]`. Your feature map can link its own documents, the documents of every partition's feature map, and the rules and principles of the public wiki. The public wiki can link the documents of every partition's feature map, but not yours. Every link must resolve.

## The public wiki

The public wiki is in `/hikari/public/wiki/`: one markdown document for each rule, principle, plan, and note of the project. You write its rules and principles, only with `change_public_wiki`. The user writes the plans and notes. Every agent reads the public wiki: the partition agents learn from it which rules they must keep, and a rule of the public wiki is a rule of every partition. A public wiki has no features; a flow across partitions is a feature of your feature map.

## The change

`change_wiki` changes your feature map, and `change_public_wiki` the public wiki. Each takes a list of writes, and applies them in order as one change: all or none.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `kind`, `name`, `description`, `body`; optional `related` | writes a new document |
| `modify` | `id`; `fields` (`name`, `description`, `related`), `body`, or both | merges the fields and replaces the body, keeping `## Notes` |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in every wiki |
| `remove` | `id`; optional `redirect` | removes the document; links to it point at the redirect, or become plain text |

A create in your feature map is a `feature` when it names no kind.

## The vocabulary

The vocabulary, `/hikari/public/vocabulary/terms.yaml`, holds the names that everyone in the project uses: each domain term with its definition and the names to avoid, and each interface between partitions with its kind. You are its only author: `define_term`, `define_interface`, and `remove_entry` change it, and every agent reads it with `get_vocabulary`. `get_partition` flags each interface of a partition that the vocabulary lacks. When a partition agent uses another name for something that the vocabulary defines, ask the partition agent to rename it.

## Keep the documents true

- When a partition feature that one of your features links changes or is removed, check your feature. hikari rewrites the links of a rename for you.
- When a feature you link is failing, the `hikari-health-check` skill says whom to ask.
- `get_document` returns a document with its backlinks: the documents in any wiki you can read that link it. `search_wiki` searches every wiki you can read, your feature map included.
