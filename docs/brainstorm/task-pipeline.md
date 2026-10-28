# Task Pipeline: Round 1

## What I Take from Your Proposal

Hikari can run coordinated work today, but only one way: you write a plan, post "carry out the plan" in the room, and the orchestration agent tracks its steps in chat. Nothing records an idea before it is a plan, nothing orders several pieces of work, and the state of a step lives only in the room's messages.

You want a pipeline of tasks:

- **A task is a markdown document** with an Obsidian property header, so it opens in Obsidian, diffs in git, and grows new properties without a migration.
- **A stub** plants an idea with no friction: a title and a few words.
- **A scope** comes out of a conversation between you and the orchestration agent about a stub. The agent asks its questions, then writes the scope, which names every partition that the work touches.
- **A queue** is an ordered list of scopes. Queues can run in parallel. You or the agent make queues, move scopes into them, and reorder them.
- **Running a queue** works through its scopes in order, and shows the scope in progress with its status: started, blocked, testing, validating, or done.
- **The agents do the work as they do today.** The pipeline does not rebuild what Claude Code already does well. Its skills tell each agent to report the state of its part through a tool.
- **The pipeline is a module** that later work can extend, or that a project can leave out.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

- **Task**: one markdown document of the pipeline. Its stage says what it is.
- **Stub**: a task in its first stage: your words, not yet scoped.
- **Scope**: a task that the orchestration agent has scoped: what changes, in which partitions, and what shows it is done.
- **Part**: the share of a scope that one partition does. A scope has one part for each partition it names.
- **Queue**: an ordered list of scopes that run one after another.
- **Status**: where a scope, or one of its parts, stands while it runs.
- **Module**: a piece of hikari that owns a directory of the vault, a few tools, a skill for each kind of agent, and a tab of the UI, behind one interface.

---

## A. The Module

**A1. A module is a Go package behind one interface, compiled into the binary.** The interface lists what a module adds: the vault directory it owns and the schema of its documents, its MCP tools and the kinds of agent that get each one, its routes in the UI's API, a skill for each pack, the notices it adds to an agent's turn, and its tab in the UI. The server, the MCP server, the driver, and the UI each ask the enabled modules for their share, and know nothing else of them. The tasks module is the first. Findings and change logs, planted in `phase-2.md`, would follow the same interface. I recommend against modules loaded at run time, such as external processes or Go plugins: one binary keeps one version, which is what the plugin's launcher relies on.

**A2. Build the interface now, but only as wide as tasks needs.** An interface with one implementation tends to guess wrong about the second. I recommend we write it now, since you asked for a module, and widen it only when a second module needs more.

**A3. A project turns a module on in the topology.** `x-hikari.modules: [tasks]` turns on the tasks module, and `hikari init` writes it into the starter topology. A project without the key has no task tools, no task skills, and no Work tab, so a module costs nothing where it is not wanted.

## B. The Documents

**B1. Every task lives in one folder, `vault/tasks/<id>.md`, whatever its stage.** The stage is a property, not a folder. A stub that becomes a scope keeps its file, its git history, and every `[[id]]` link to it. In Obsidian, a filter on `stage` gives the view of each stage.

**B2. A stub needs only a title.** Its properties are `id`, `kind: task`, `stage: stub`, `name`, `created`, and `author`. Its body is your words, as you wrote them. hikari fills in the `id` (from the name) and `created` when they are missing, so a note with one line and `stage: stub` is a stub.

**B3. A scope adds the partitions and what done means.** Its added properties:

- `partitions`: the partition agents that the work touches. hikari checks each one against the topology.
- `features`: the features it changes or creates, as `[[id]]` links into the feature maps.
- `status`, `parts`, and `queue`, which hikari writes (section E).

Its body keeps your words under `## Request`, and the orchestration agent writes `## Goal`, a `## Parts` section with one subsection for each partition (what that partition changes, in terms of features and interfaces, not code), `## Done when`, and `## Answers` (its questions and your answers). The orchestration agent never changes `## Request`.

**B4. Unknown properties stay.** You can add your own properties, such as `priority` or `area`, and hikari keeps them through every write. This is what keeps the documents open to later needs without a migration: hikari owns the properties it names, and leaves the rest to you.

**B5. Each document carries a schema version,** `version: 1`, so that a later round can change the format and say which files it upgrades.

## C. From Stub to Scope

**C1. Anyone can plant a stub.** You plant one from the UI, from Obsidian, or from your Claude Code session (section F). An agent plants one with a tool, `plant_stub`, when it finds work outside what it was asked to do, such as a partition agent that finds a bug in a consumer it cannot see. A stub that an agent plants names the agent as its `author`, and waits for you like any other.

**C2. You hand a stub to the orchestration agent, and the conversation happens in your direct messages with it.** A Scope button on the stub sends "Please scope [[id]]" to the orchestration agent, so the room stays free for work. The agent reads the stub and the feature maps, asks its questions in that conversation, and writes the scope with a tool, `scope_task`, once it has your answers. The tool changes the stage to `scope` and writes the properties and the sections of B3.

**C3. While it scopes, the orchestration agent may ask partition agents questions, but not start their work.** For example, "@orders, what would change in [[create-order]] to accept a coupon?" A partition agent answers from its map and its code. It starts work only when its part arrives through a running queue.

**C4. A scope outside any queue is a draft; putting it in a queue approves it.** The orchestration agent can revise a draft when you ask. Once a scope is in a queue, only you change its sections, and the agent can only propose a change in your conversation. One gesture approves the scope, with no separate approval step to remember.

**C5. A scope names its partitions, and hikari computes the rest.** The page of each partition lists the scopes that name it, from `partitions`, as a backlink. The scope never lists what follows from other documents, as a feature document never lists its tests.

## D. Queues

**D1. A queue is its own document, `vault/tasks/queues/<id>.md`, with an ordered list of its scopes.** Its property `scopes` holds `[[id]]` links in order. A linked list kept as a `next` property on each scope would need several files rewritten to move one scope, and two edits could break the chain. One list in one file reorders in one write, and Obsidian shows it as links. The queue document is the only record of membership and order: hikari computes each scope's `queue` from it.

**D2. A scope is in at most one queue.** Moving it to another queue takes it out of the first, in one write.

**D3. You and the orchestration agent both edit queues.** You make, rename, reorder, and remove queues in the Work tab or in Obsidian. The orchestration agent does the same with a tool, `change_queue`, when you ask it, such as "put these five scopes into two queues: one for the API, one for the UI." Neither of you can reorder or remove a scope that has started.

**D4. A partition works on one scope at a time, across all queues.** When two running queues each reach a scope that needs `orders`, the second scope waits, with the status `waiting` and the reason "orders is busy with [[x]]", until the first is done. A partition agent is one container, so two scopes at once would interleave its context and its commits. Parallel queues then run in parallel where their partitions differ, and in turn where they share one.

**D5. A scope can wait for a scope in another queue.** A property `needs: [[id]]` keeps a scope `waiting` until each scope it names is done. Order within a queue covers most cases; `needs` covers the one where an API queue must release before a UI queue's scope uses it.

## E. Running a Queue

**E1. You start, pause, and stop a queue; the server moves it forward.** Start marks the queue running and its first scope `started`, and sends the orchestration agent "Carry out [[id]]". When a scope is done, the server starts the next one by itself. A blocked scope pauses its queue until you, or the agent, clear the block.

**E2. Each part has a status, and the scope's status follows from its parts.**

| Status | Of a part | Of a scope |
|---|---|---|
| queued | waits for its turn | in a queue, not started |
| waiting | | waits for a busy partition or a `needs` |
| started | the partition agent works on it | the orchestration agent has sent the parts |
| blocked | cannot go on, with a reason | a part, or the scope itself, is blocked |
| testing | the code is changed, and its suites run | every part is testing or done |
| validating | | every part is done, and the orchestration agent checks the flows |
| done | the partition's suites pass for the features of the scope | the orchestration agent's flows pass, and it marks the scope done |

**E3. An agent never declares its own work done.** A partition agent asks for `done` with the tool `set_task_status`, and the server accepts it only when the health of each feature that the scope names, in that partition, is passing; otherwise the part stays `testing`, with the failing features as the reason. The orchestration agent's `done` for the scope likewise needs the health of the orchestration features it names. This is the README's rule that hikari, not the agent, computes health, applied to the pipeline.

**E4. The status lives in the task's own properties, with a log in its body.** hikari writes `status`, each part's status in `parts`, and one line under `## Log` for each change: the time, who, the old and new status, and the note. Obsidian then shows the state of every task without the UI, and git keeps its history. hikari writes only the properties it owns and the log, so your edits of the other sections are never lost.

**E5. How the server wakes the orchestration agent.** Start, the next scope, and a cleared block each need a message to the agent. I recommend the server send it as `hikari`, a new sender that only the server uses, so that the room and the log show what you did and what hikari did. Sending it as you would say that you wrote messages you did not write.

## F. Where You Work with Tasks

**F1. A Work tab in the UI.** On the left, your stubs, each with a Scope button. Then the draft scopes. Then the queues, as columns, each scope with its status and its parts as small chips, with the running scope at the top of its column. You drag a scope into a queue or along it, and start or pause a queue from its header. A click on a task opens its document, its parts, and its log.

**F2. A `hikari task` command, and a `tasks` skill in the Hikari plugin.** `hikari task stub "<words>"`, `hikari task list`, and `hikari queue add|move|start|pause`. Your Claude Code session can then plant a stub as you talk ("stub this"), and arrange queues for you. When hikari is stopped, the command writes the file in the vault, and hikari commits it as your edit at its next start, as it does for wiki edits today. Starting a queue needs a running hikari.

**F3. Obsidian works with no extra step.** A note in `vault/tasks/` with `stage: stub` is a stub. hikari checks every task document and lists the problems of one it cannot read, as it does for the wikis.

## G. Who Sees What

**G1. The orchestration agent reads every task, read-only, at `/hikari/tasks/`.** It changes tasks only through its tools.

**G2. A partition agent sees only the scopes that name it, through a tool, `get_task`.** Each container mounts whole directories, so a mount would show it every task, including stubs and scopes of other partitions. A tool returns the scopes that name the caller, whole, since a scope speaks of features and interfaces and holds no code. A partition agent sees no stubs and no queues.

**G3. The documents live in the project repository, and the server commits each write as its writer:** you, an agent, or `hikari` for a status that it computes.

## H. The Skills

**H1. One task skill for each pack, and no new way of working.** The partition agent's `hikari-task-work` says: when a part arrives, read the scope with `get_task`, report `started`, do the work with `hikari-plan-work`, `hikari-do-work`, and `hikari-validate-work` as today, report `testing` when the suites run and `done` when they pass, and report `blocked` with the reason the moment it cannot go on. The orchestration agent's `hikari-scope-work` says how to scope a stub (the questions, the sections, `scope_task`), how to carry out a scope (send each part when its needs are met, and follow their statuses), and how to validate it.

**H2. The task notice in each turn.** The driver tells an agent at the start of each turn which of its parts are active and their status, so that a turn that starts for another reason does not forget a part in progress.

## I. What Stays as It Is, and What Waits

**I1. Plans stay as they are.** A plan is your document of intent, and hikify's plan is a document of steps with actors. I recommend we leave both, and plant one change: hikify's execution would write its steps as scopes into a queue, so that onboarding uses the same pipeline.

**I2. The glossary gets the terms of this round:** task, stub, scope, part, queue, status, and module. "Work order", the planted name in `phase-2.md`, retires in favor of "task".

**I3. Out of this round, planted:** a pipeline whose stages and statuses you configure, estimates, assigning a task to a person, notifications outside the UI, a queue that starts by itself on a schedule, and metrics such as how long each status lasts.
