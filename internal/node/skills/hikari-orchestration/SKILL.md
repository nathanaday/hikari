---
name: hikari-orchestration
description: How the orchestrator keeps the public wiki - the stories, rules, and principles that say what the systems must do together, which you change through change_public_wiki and link to the features of the systems - and the vocabulary, and how to read the user's plans and carry one out. Load it before you write or change the public wiki, and when you plan work that crosses systems.
---

# The public wiki

The public wiki is in `/hikari/public/readonly/shared/wiki/`: one markdown document for each story, rule, principle, plan, and note of the project. You write its stories, rules, and principles, only with `change_public_wiki`. The user writes the plans and notes. Every agent reads the public wiki: the owners learn from it why their features matter and which rules they must keep.

A system wiki holds what one system does: its features, and its own principles and rules. The public wiki holds what the systems do together, and what the user needs from them. Its documents link the documents of the systems, so the links across the wikis show how the project fits together.

## The kinds you write

| Kind | What it holds | Example |
|---|---|---|
| `story` | A flow that an actor goes through to reach a goal, across systems, and its outcome. | "A customer orders two items, and the order reserves stock for both." |
| `rule` | A statement that must always hold, in every flow. | "Stock never goes below zero." |
| `principle` | A lasting choice that guides how the systems work together, with its reason. | "The edge computes each alarm; the cloud never recomputes one." |

- A story names its actor (a user, an operator, a device, another system), the goal, the steps in order, and the outcome that proves it worked. Write the steps as prose with a link to the feature of each step: "The customer places the order through [[create-order]], which holds stock with [[reserve-stock]]." A story that a test can check is a good story: say what someone would see when it works.
- A rule is one sentence that is true or false at any moment. Link the stories that must keep it, or let the stories link the rule. A rule that only one system can break belongs in that system's wiki; ask the owner.
- A principle is a choice, not a fact: say what it rules out, and why. Few principles are better than many.
- Do not write features in the public wiki. A feature belongs to the system that has it. If no system has a step of a story yet, write the story anyway, name the missing step in its Purpose, and ask an owner for the feature.

## The document

```markdown
---
id: customer-orders-items
kind: story
name: A customer orders items
description: A customer orders two items, and the order reserves stock for both before it is placed.
related: ["[[stock-never-negative]]"]
created: 2026-10-23T05:35:10Z
updated: 2026-10-23T05:35:10Z
---

## Purpose

A customer must not be told that an order is placed when the stock is not there. The customer sends the items through [[create-order]]. Orders checks the items with [[validate-order-items]] and holds the stock of each item with [[reserve-stock]] in inventory. When every item is held, the order is placed; when one is not, no stock stays held and the customer learns which item is short.
```

The rules are those of the system wikis, with the kind added:

- The id is lowercase letters, digits, and dashes, at most 48 characters, and unique in the whole store: no other wiki may use it.
- `name` is two to five words. `description` is one sentence of at most 200 characters.
- The body has a `## Purpose` section with text. Name no code. `## Notes` is the user's; never write it.
- A link is `[[id]]`. You can link the documents of the public wiki and of every system wiki. Every link must resolve.

## The change

`change_public_wiki` takes a list of writes, and applies them in order as one change: all or none.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `kind`, `name`, `description`, `body`; optional `related` | writes a new document |
| `modify` | `id`; `fields` (`name`, `description`, `related`), `body`, or both | merges the fields and replaces the body, keeping `## Notes` |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in every wiki |
| `remove` | `id`; optional `redirect` | removes the document; links to it point at the redirect, or become plain text |

## The vocabulary

The vocabulary, `/hikari/public/readonly/shared/vocabulary/terms.yaml`, holds the names that everyone in the project uses: each domain term with its definition and the names to avoid, and each interface between systems with its kind. You are its only author: `define_term`, `define_interface`, and `remove_entry` change it, and every agent reads it with `get_vocabulary`. `get_system` flags each interface of a system that the vocabulary lacks. When an owner uses another name for something that the vocabulary defines, ask the owner to rename it.

## Plans and notes

The user's plans and notes are in the same folder. A plan is work the user intends; its `status` says how far it is. Read them for context: they say where the user wants the project to go. Act on a plan only when the user asks you to. Never write, rename, or remove one.

## Carry out a plan

When the user asks you to carry out a plan, you track it; the actors do the work. A plan's steps each name an actor, what changes, what the step needs first, and when it is done.

1. Read the plan, and each system plan that it names.
2. Post the steps in the room as a list, each with its state: waiting, sent, done, or blocked.
3. When the needs of a step are met, send the step to its actor by direct message: the step's id, what changes, what it needs, and when it is done. Name the system plan in the actor's wiki, where the owner reads its part. Send a step only after the releases that it needs are announced.
4. A step whose actor is `session` or the user is the user's: ask the user, and wait for their answer.
5. When an actor reports a step done, check what you can see: the release in the announcement, the health in the map, the support of the stories in the public wiki. Then post the new state of the steps.
6. When a step cannot be done as written, tell the user, with the reason and the actor's words. Never change the plan; it is the user's.

## Keep the wiki true

- When a feature that a story links changes or is removed, check the story. hikari rewrites the links of a rename for you.
- When a story's support is failing, a feature it links is failing in its system's tests. Ask the owner.
- `get_document` returns a document with its backlinks: the documents in any wiki that link it. `search_wiki` searches every wiki you can read.
