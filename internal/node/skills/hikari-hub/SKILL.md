---
name: hikari-hub
description: How to keep the wiki of a hub you orchestrate - the stories, rules, and principles that say what the hub's systems must do together, which you change through change_hub_wiki and link to the features of the hub's systems - and how to read the user's plans and notes. Load it before you write or change a hub wiki, and when you plan work that crosses systems.
---

# The hub wiki

Each hub has a wiki in `/hikari/public/readonly/hubs/<hub>/wiki/`: one markdown document for each story, rule, principle, plan, and note of the hub. You write the stories, rules, and principles of the hubs you orchestrate, only with `change_hub_wiki`. The user writes the plans and notes. Every participant of the hub reads the wiki: the owners learn from it why their features matter and which rules they must keep.

A system wiki holds what one system does: its features, and its own principles and rules. The hub wiki holds what the systems do together, and what the user needs from them. Its documents link the documents of the hub's systems, so the links across the wikis show how the ecosystem fits together.

## The kinds you write

| Kind | What it holds | Example |
|---|---|---|
| `story` | A flow that an actor goes through to reach a goal, across systems, and its outcome. | "A customer orders two items, and the order reserves stock for both." |
| `rule` | A statement that must always hold, in every flow. | "Stock never goes below zero." |
| `principle` | A lasting choice that guides how the systems work together, with its reason. | "The edge computes each alarm; the cloud never recomputes one." |

- A story names its actor (a user, an operator, a device, another system), the goal, the steps in order, and the outcome that proves it worked. Write the steps as prose with a link to the feature of each step: "The customer places the order through [[create-order]], which holds stock with [[reserve-stock]]." A story that a test can check is a good story: say what someone would see when it works.
- A rule is one sentence that is true or false at any moment. Link the stories that must keep it, or let the stories link the rule. A rule that only one system can break belongs in that system's wiki; ask the owner.
- A principle is a choice, not a fact: say what it rules out, and why. Few principles are better than many.
- Do not write features in a hub wiki. A feature belongs to the system that has it. If no system has a step of a story yet, write the story anyway, name the missing step in its Purpose, and ask an owner for the feature.

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

- The id is lowercase letters, digits, and dashes, at most 48 characters, and unique in the whole store: no other wiki, of a hub or of a system, may use it.
- `name` is two to five words. `description` is one sentence of at most 200 characters.
- The body has a `## Purpose` section with text. Name no code. `## Notes` is the user's; never write it.
- A link is `[[id]]`. You can link the documents of your hub's wiki, of each system whose owner is a member of the hub, and of each child or parent hub. Every link must resolve.
- `key` is the user's: a key story is one the user chose as part of the integration test that matters most. You cannot set it, and every write keeps it.

## The change

`change_hub_wiki` takes the hub and a list of writes, and applies them in order as one change: all or none.

| Op | Takes | Does |
|---|---|---|
| `create` | `id`, `kind`, `name`, `description`, `body`; optional `related` | writes a new document |
| `modify` | `id`; `fields` (`name`, `description`, `related`), `body`, or both | merges the fields and replaces the body, keeping `## Notes` |
| `rename` | `id`, `to` | renames the file and rewrites every link to it in every wiki |
| `remove` | `id`; optional `redirect` | removes the document; links to it point at the redirect, or become plain text |

## Plans and notes

The user's plans and notes are in the same folder. A plan is work the user intends; its `status` says how far it is. Read them for context: they say where the user wants the ecosystem to go. Act on a plan only when the user asks you to. Never write, rename, or remove one.

## Carry out a plan

When the user asks you to carry out a plan, you track it; the actors do the work. A plan's steps each name an actor, what changes, what the step needs first, and when it is done.

1. Read the plan, and each system plan that it names.
2. Post the steps in the hub as a list, each with its state: waiting, sent, done, or blocked.
3. When the needs of a step are met, send the step to its actor by direct message: the step's id, what changes, what it needs, and when it is done. Name the system plan in the actor's wiki, where the owner reads its part. Send a step only after the releases that it needs are announced.
4. A step whose actor is `session` or the user is the user's: ask the user, and wait for their answer.
5. When an actor reports a step done, check what you can see: the release in the announcement, the health in the map, the stories in `get_test_plan`. Then post the new state of the steps.
6. When a step cannot be done as written, tell the user, with the reason and the actor's words. Never change the plan; it is the user's.

## Keep the wiki true

- When a feature that a story links changes or is removed, check the story. hikari rewrites the links of a rename for you.
- When a story's support is failing, a feature it links is failing in its system's tests. Ask the owner.
- `get_document` returns a document with its backlinks: the documents in any wiki that link it. `search_wiki` searches every wiki you can read.
