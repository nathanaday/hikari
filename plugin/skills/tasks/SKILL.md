---
name: tasks
description: Work hikari's task pipeline from this session - open an issue for an idea as the user talks, list the issues of the project's GitHub repositories and the local ones, hand an issue to the agents, arrange queues, start or pause a queue, and set a status. Use for keep this for later, open an issue for this, add this to the backlog, what is in the pipeline, work on issue 12, have the agents do this, make a queue, put this issue in the queue, reorder the queue, start the queue, pause the queue, what is the queue working on, in a hikari project with the tasks module.
---

# Work the task pipeline

You work for the user in their own Claude Code session, in a hikari project whose `topology.yaml` turns on the tasks module (`x-hikari.modules: [tasks]`). The unit of work is an issue. When a repository of the project has its origin on GitHub, its issues are the project's, and hikari reads and writes them through GitHub; a project with no such repository keeps its issues in `vault/tasks/` as markdown. A queue orders issues, in `vault/tasks/queues/`. A running queue starts its issues one after another: the orchestration agent scopes each one when it starts, the partition agents do their parts, and hikari posts the scope, each block, and the finish back to the issue. You change the pipeline only through `hikari issue`, `hikari task`, and `hikari queue`, which go through the running hikari, or write the files when it is stopped.

The project root is the folder of `topology.yaml`. Run `cd <root>` once, as a command of its own; when the root lies outside the session's folders, pass `-file <root>/topology.yaml` to each command instead.

An issue is named `<repo>#<number>` for a tracker issue, such as `shop#12`, or by its id for a local one.

## Open an issue

When the user says "keep this for later", "open an issue", or names an idea to keep, open it in their words, without asking for details:

```sh
hikari issue new "<a short title>" "<what and why, in the user's words>"
```

It goes to the tracker of the project's first repository with one, or to the vault with `-local`, or to a named repository with `-repo <repo>`. Tell the user its ref and its link. An issue starts no work.

## See the pipeline

`hikari issue list` shows each tracker and its issues, with the queue and status of each issue that the agents hold. `hikari task list` shows the tasks and each queue with its state and its tasks in order, each task's status and its parts' statuses, why a waiting task waits, and the documents that have problems. `hikari issue sync` reads the trackers again.

## Hand an issue to the agents

When the user asks the agents to work on an issue:

```sh
hikari issue work <issue>
```

It puts the issue at the front of the work queue, makes the queue when it is missing, and starts it; hikari must be running (`hikari up` in the user's own terminal). The orchestration agent scopes the issue from its body, its comments, and the feature maps, and comments on the issue with the scope. Do this only when the user asks: it starts the agents' work.

## Arrange queues

Putting an issue in a queue approves it. Only do so when the user asks.

```sh
hikari queue new <name>                 # a new queue, paused
hikari queue add <queue> <issue> [n]    # at place n, or at the end
hikari queue move <queue> <issue> [n]   # within its queue, or to another
hikari queue out <issue>                # back out of every queue
hikari queue rename <queue> <name>
hikari queue remove <queue>             # only an empty queue
```

A task that has started stays where it is.

## Start and pause

`hikari queue start <queue>` starts a queue; hikari must be running. `hikari queue pause <queue>` starts no next task; the one in progress goes on. Start a queue only when the user asks: it starts the agents' work.

## Statuses

The agents report their statuses, and hikari grants `done` only when the health of the scope's features passes. The user is the authority, and may set any status with `hikari task status <task> <status> [note]`, or one part's with `-part <partition>`, for example to clear a block that the agents cannot. Set one only when the user asks.

## Where the work landed

When the user asks what is merged or what comes next, point them to the Repositories page of the web UI, which shows each repository's branch, its pull requests with their checks, and lets them push, open a pull request, and merge. The orchestration agent answers the same question from `list_repositories` when asked in the room.
