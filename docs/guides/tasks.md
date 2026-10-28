# The Task Pipeline

This guide follows one idea from a stub to a done scope. The pipeline is the `tasks` module: a project turns it on with `modules: [tasks]` under `x-hikari`, which `hikari init` writes for you. The shop example has it on.

Every task is a markdown file in `vault/tasks/`, and every queue a file in `vault/tasks/queues/`. Obsidian opens them, and you can edit them there; hikari commits your edits as yours. The Work tab of the web UI, `hikari task` and `hikari queue` in a terminal, and the `tasks` skill of the Hikari plugin in your own Claude Code session all change the same files.

## 1. Plant a stub

A stub is an idea in a few words. Plant one in the Work tab's field, in a terminal, or by asking your Claude Code session to "stub this":

```sh
hikari task stub "Cancel an order before it ships
A customer can cancel until the warehouse picks the order."
```

The first line is its name. hikari writes `vault/tasks/cancel-an-order-before-it-ships.md`:

```markdown
---
version: 1
id: cancel-an-order-before-it-ships
kind: task
stage: stub
name: Cancel an order before it ships
author: user
created: "2026-10-28T09:00:00Z"
---

Cancel an order before it ships
A customer can cancel until the warehouse picks the order.
```

A note that you make in Obsidian with `stage: stub` in its properties is a stub too; hikari fills in the rest at its next read. An agent plants a stub with `plant_stub` when it finds work outside what it was asked to do, and the stub waits for you like any other.

## 2. Scope it with the orchestration agent

Press Scope on the stub. It sends the orchestration agent, in your direct messages, "Please scope [[cancel-an-order-before-it-ships]]". The agent reads the stub and the feature maps, may ask partition agents what would change, and asks you its questions in that conversation. When you have answered, it writes the scope with `scope_task`. The same file now has `stage: scope`, the partitions it touches, and the features whose health decides that it is done:

```markdown
partitions: [orders, inventory]
features: ["[[cancel-order]]", "[[release-stock]]", "[[customer-cancels]]"]
```

Its body has your words under `## Request`, which the agent never changes, then `## Goal`, `## Parts` with what each partition changes, `## Done when`, and `## Answers`. A scope outside any queue is a draft: ask the agent to revise it as often as you like.

## 3. Queue it

A queue orders scopes, and putting a scope in one approves it. Make a queue and add the scope, in the Work tab or a terminal:

```sh
hikari queue new API work
hikari queue add api-work cancel-an-order-before-it-ships
```

A new queue is paused. Move a scope up or down, or to another queue, with the buttons of its card or with `hikari queue move`; take it out with `hikari queue out`, which makes it a draft again. A scope that has started stays where it is. Each scope's `queue` property follows the queue documents.

## 4. Start the queue

Press Start, or run `hikari queue start api-work` while `hikari up` runs. The queue starts its first scope that is not done. hikari sends the orchestration agent, as `hikari`, "The queue api-work started [[cancel-an-order-before-it-ships]]", and the agent sends each partition its part.

A partition works on one scope at a time, across all queues. When a second running queue reaches a scope that needs a busy partition, that scope is `waiting`, and its `wait` property says why: "inventory is busy with [[cancel-an-order-before-it-ships]]". A scope with `needs: ["[[other-scope]]"]` waits until that scope is done. Pause finishes the scope in progress and starts no other.

## 5. Follow the statuses

Each partition agent reports its part: `started` when it begins, `testing` when its suites run, `blocked` with a reason the moment it cannot go on, and `done` when its suites pass. hikari grants `done` only when the health of every feature of the scope in that partition is passing; until then, the part stays `testing`, and the agent learns which features fail.

The scope's status follows its parts: `blocked` when one is blocked, `testing` when every part is testing or done, and `validating` when every part is done. Then the orchestration agent checks the flows across the partitions, and asks for `done`, which hikari grants only when the scope's features in the orchestration feature map pass. When a scope is done, its partitions are free, and each running queue that waited for them starts its next scope.

Every change is a line under `## Log`, with the time, who changed it, and why. You are the authority on the project: Set status in the task's panel, or `hikari task status <scope> <status>`, sets any status, with no health check, such as to clear a block that the agents cannot.

## Where things are

| Path | What it holds |
|---|---|
| `vault/tasks/<id>.md` | a task: a stub, or a scope with its parts, statuses, and log |
| `vault/tasks/queues/<id>.md` | a queue: its name, its state, and its scopes in order |
| `/hikari/tasks/` | the folder as the orchestration agent reads it, read-only |

A partition agent mounts none of it: it reads the scopes that name it with `get_task`. A file that hikari cannot read, or that names an unknown partition, feature, or scope, is a problem, which the Work tab and `hikari task list` show; hikari leaves that file alone until you fix it.
