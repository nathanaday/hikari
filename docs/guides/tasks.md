# The Task Pipeline

This guide follows one issue from GitHub to done. The pipeline is the `tasks` module: a project turns it on with `modules: [tasks]` under `x-hikari`, which `hikari init` writes for you. The shop example has it on.

The unit of work is an issue. When a repository of the project has its `origin` on GitHub, its issues are the project's: the Work tab of the web UI reads them, lets you open, edit, close, and comment on them, and keeps them in step with GitHub every minute. A project whose repositories are not on GitHub keeps its issues in the vault, as markdown in `vault/tasks/`, and the Work tab draws one list of both.

Each issue that enters a queue gets a task, a markdown document in `vault/tasks/`, which records the scope, the statuses, and the log. Every queue is a file in `vault/tasks/queues/`. Obsidian opens them, and you can edit them there; hikari commits your edits as yours. The Work tab, `hikari issue`, `hikari task`, and `hikari queue` in a terminal, and the `tasks` skill of the Hikari plugin in your own Claude Code session all work the same pipeline.

## 1. Open an issue

Open it on GitHub as you always do, or in the Work tab's form, or in a terminal:

```sh
hikari issue new "Cancel an order before it ships" "A customer can cancel until the warehouse picks the order."
```

It goes to the tracker of the project's first repository with one; `-repo <name>` picks another, and `-local` keeps it in the vault. An agent opens one with `create_issue` when it finds work outside its part, such as a fault in code it cannot see; the body says which agent opened it. An issue starts no work.

A local issue is a file:

```markdown
---
version: 1
id: cancel-an-order-before-it-ships
kind: task
stage: stub
name: Cancel an order before it ships
author: user
created: "2026-11-04T09:00:00Z"
---

A customer can cancel until the warehouse picks the order.
```

A note that you make in Obsidian with `stage: stub` in its properties is a local issue too; hikari fills in the rest at its next read.

## 2. Hand it to the agents

Press **Work on this** on the issue, or run `hikari issue work shop#12`. The issue goes to the front of the `work` queue, which hikari makes when it is missing, and the queue starts. Or plan first: drag issues into a queue of your own, in the order you want, and press Start when you are ready. Putting an issue in a queue approves it.

When the queue reaches the issue, hikari makes its task, `vault/tasks/shop-12.md`, with the issue's title as its name, its body as the request, and `issue: shop#12`. The task is `started`, hikari comments on the issue that the work has begun, sets the label `hikari:working`, and tells the orchestration agent, as `hikari`:

> The queue work started [[shop-12]], the issue shop#12 "Cancel an order before it ships", which is not scoped yet. Scope it now: ...

While a started task is not scoped, no other queue starts its next task; the Work tab shows why it waits.

## 3. The orchestration agent scopes it

The agent reads the issue and its comments with `get_issue`, reads the feature maps, decides what the issue leaves open, and writes the scope with `scope_task`. The same file now has `stage: scope`, the partitions it touches, and the features whose health decides that it is done:

```markdown
partitions: [orders, inventory]
features: ["[[cancel-order]]", "[[release-stock]]", "[[customer-cancels]]"]
```

Its body has the issue's words under `## Request`, which the agent never changes, then `## Goal`, `## Parts` with what each partition changes, `## Done when`, and `## Answers` with the assumptions the agent made. hikari posts the scope to the issue as a comment, so you read the plan where the request is. If the agent read the issue wrong, correct it there and set the status back with Set status; the agent asks you in your direct messages only when a choice would waste the partitions' work either way.

You can also ask for a plan before any work: press **Plan** on an issue, and the agent comments on it with the partitions, the features, and the open questions, and starts nothing.

## 4. Follow the statuses

Each partition agent reports its part: `started` when it begins, `testing` when its suites run, `blocked` with a reason the moment it cannot go on, and `done` when its suites pass. hikari grants `done` only when the health of every feature of the scope in that partition is passing; until then, the part stays `testing`, and the agent learns which features fail.

When the agent's topology or profile says `review: required`, done also needs a review: the agent reads its whole change with `get_changes`, sends the fresh `hikari-reviewer` subagent with the diff, the part, and the policies and styles it was given, fixes the blockers, and passes the verdict as `review`. hikari refuses done without it, and the verdict is a line of the task's log, so you read who reviewed and what was found. The [workflows guide](workflows.md) says how to require a review.

The scope's status follows its parts: `blocked` when one is blocked, `testing` when every part is testing or done, and `validating` when every part is done. Then the orchestration agent checks the flows across the partitions, and asks for `done`, which hikari grants only when the scope's features in the orchestration feature map pass. A block reaches the issue as a comment and the label `hikari:blocked`; done reaches it as a comment that names the features and the branch that holds the work, and the label `hikari:done`.

Every change is a line under `## Log`, with the time, who changed it, and why. The issue's panel sums the log into a scorecard: how long the scope took, how many turns its partition agents spent inside their parts and what they cost, how often it was blocked, how often done was refused, and how many reviews it carried; and it lists the timeline, the log with the decisions hikari asked and your answers in order. The cost is the sum of each partition agent's turns from when its part started to when it was done, so a part that was blocked for an hour while its agent did other work shows that hour's cost too; it is a measure, not a bill. You are the authority on the project: Set status in the issue's panel, or `hikari task status <task> <status>`, sets any status, with no health check, such as to clear a block that the agents cannot.

## 5. Merge

The work is on your branch of each repository the scope touched. Open the repository's page in the Repositories section, push the branch, open a pull request with `Closes #12` in its body, watch its checks, and merge; the [repositories guide](repositories.md) has the details. GitHub closes the issue when the pull request merges.

## Queues

A queue orders issues. Make one in the Work tab or a terminal:

```sh
hikari queue new API work
hikari queue add api-work shop#12
```

A new queue is paused. Drag a card to move it, or use the buttons of its card, or `hikari queue move`; take it out with `hikari queue out`. A task that has started stays where it is. A partition works on one scope at a time, across all queues: when a second running queue reaches an issue that needs a busy partition, its task is `waiting`, and its `wait` property says why. A scope with `needs: ["[[other-task]]"]` waits until that task is done. Pause finishes the scope in progress and starts no other.

## Where things are

| Path | What it holds |
|---|---|
| `vault/tasks/<id>.md` | a task: a local issue, or the record of a tracker issue, with its scope, parts, statuses, and log |
| `vault/tasks/queues/<id>.md` | a queue: its name, its state, and its tasks in order |
| `/hikari/tasks/` | the folder as the orchestration agent reads it, read-only |

A partition agent mounts none of it: it reads the scopes that name it with `get_task`. A file that hikari cannot read, or that names an unknown partition, feature, or task, is a problem, which the Work tab and `hikari task list` show; hikari leaves that file alone until you fix it.

## Commands

```
hikari issue list                     the issues of every tracker, and the local ones
hikari issue new [-repo r] [-local] <title> [body]
hikari issue work <issue>             front of the work queue, which starts
hikari issue sync                     read the trackers again
hikari task list                      the tasks and the queues
hikari task status <task> <status> [note]
hikari queue new|rename|remove|add|move|out|start|pause|list
```

An issue is `<repo>#<number>`, such as `shop#12`, or the id of a local issue.
