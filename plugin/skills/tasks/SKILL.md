---
name: tasks
description: Work hikari's task pipeline from this session - plant a stub of an idea as the user talks, list the stubs, draft scopes, and queues, arrange scopes into queues, start or pause a queue, and set a status. Use for stub this, plant a stub, add this to the backlog, what is in the pipeline, make a queue, put this scope in the queue, reorder the queue, start the queue, pause the queue, what is the queue working on, in a hikari project with the tasks module. Scoping a stub is a conversation with the orchestration agent, through room.
---

# Work the task pipeline

You work for the user in their own Claude Code session, in a hikari project whose `topology.yaml` turns on the tasks module (`x-hikari.modules: [tasks]`). The pipeline is markdown in `vault/tasks/`: stubs, which are the user's ideas; scopes, which the orchestration agent writes from a stub with the user; and queues of scopes in `vault/tasks/queues/`. A running queue starts its scopes one after another, one scope at a time in each partition, and hikari's agents report the status of their parts. You change the pipeline only through `hikari task` and `hikari queue`, which go through the running hikari, or write the files when it is stopped.

The project root is the folder of `topology.yaml`. Run `cd <root>` once, as a command of its own; when the root lies outside the session's folders, pass `-file <root>/topology.yaml` to each command instead.

## Plant a stub

When the user says "stub this" or names an idea to keep, plant it in their words, without asking for details:

```sh
hikari task stub "<a short name on the first line>
<what and why, in the user's words>"
```

A stub needs no scope yet. Tell the user its id.

## See the pipeline

`hikari task list` shows the stubs, the draft scopes, each queue with its state and its scopes in order, each scope's status and its parts' statuses, why a waiting scope waits, and the documents that have problems. Read a task's whole document at `vault/tasks/<id>.md`.

## Scope a stub

Scoping is a conversation between the user and the orchestration agent: it asks its questions, and writes the scope. Hand the stub over with the room skill, in a direct message to the orchestration agent:

```sh
hikari send -to <orchestration agent> "Please scope [[<id>]]. Follow the Scope a stub procedure of the hikari-scope-work skill."
```

Then relay its questions and the user's answers. The Scope button of the Work tab in the web UI does the same.

## Arrange queues

A scope in a queue is approved. Only put a scope in a queue when the user asks.

```sh
hikari queue new <name>                 # a new queue, paused
hikari queue add <queue> <scope> [n]    # at place n, or at the end
hikari queue move <queue> <scope> [n]   # within its queue, or to another
hikari queue out <scope>                # back to a draft
hikari queue rename <queue> <name>
hikari queue remove <queue>             # only an empty queue
```

A scope that has started stays where it is.

## Start and pause

`hikari queue start <queue>` starts a queue; hikari must be running (`hikari up` in the user's own terminal). `hikari queue pause <queue>` starts no next scope; the scope in progress goes on. Start a queue only when the user asks: it starts the agents' work.

## Statuses

The agents report their statuses, and hikari grants `done` only when the health of the scope's features passes. The user is the authority, and may set any status with `hikari task status <scope> <status> [note]`, or one part's with `-part <partition>`, for example to clear a block that the agents cannot. Set one only when the user asks.
