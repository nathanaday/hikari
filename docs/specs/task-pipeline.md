# Spec: The Task Pipeline, and Modules

Status: specified.

Sources: `docs/brainstorm/task-pipeline.md` and `task-pipeline-answers.md`. The names are those of `docs/glossary.md`, with the terms of the round: task, stub, scope, part, queue, status, and module.

## Summary

Hikari gets modules: a module owns a directory of the vault, its tools, a skill for each kind of agent, its routes, its CLI commands, and a tab of the UI, behind one interface, and a project turns it on in `x-hikari.modules`. The first module is `tasks`, a pipeline of work. A stub is your idea in a few words. The orchestration agent scopes it with you in your direct messages, into a scope that names its partitions and says what done means. A queue is a document that orders scopes; putting a scope in a queue approves it. A running queue starts its scopes in order, one scope at a time in each partition, and each partition agent reports the status of its part through a tool. The server, not the agent, grants `done`, from the health of the scope's features.

## Decisions

From the round and its answers:

1. A module is a Go package behind one interface, compiled into the binary, and only as wide as tasks needs (A1, A2).
2. A project turns a module on with `x-hikari.modules`, and `hikari init` turns `tasks` on (A3).
3. Every task is `vault/tasks/<id>.md`; its stage is a property (B1). A stub needs only a name (B2). A scope names its partitions and features, and has the sections Request, Goal, Parts, Done when, and Answers (B3). Unknown properties stay (B4), and each document has `version: 1` (B5).
4. You and the agents plant stubs (C1). You hand a stub to the orchestration agent in your direct messages, and it writes the scope with `scope_task` (C2). It may ask partition agents questions while it scopes (C3). A scope in a queue is approved (C4). A partition's scopes are computed from `partitions` (C5).
5. A queue is `vault/tasks/queues/<id>.md`, with its scopes in order (D1). A scope is in at most one queue (D2). You and the orchestration agent edit queues, and a started scope stays where it is (D3). A partition works on one scope at a time (D4). `needs` makes a scope wait for scopes in other queues (D5).
6. You start and pause a queue, and the server moves it forward (E1). Parts and scopes have the statuses of E2. The server grants `done` (E3). The status lives in the task's properties, with a log in its body (E4). The server writes its own messages as `hikari` (E5).
7. A Work tab (F1), `hikari task` and `hikari queue` with a `tasks` skill in the Hikari plugin (F2), and Obsidian with no extra step (F3).
8. The orchestration agent reads every task at `/hikari/tasks/` (G1). A partition agent reads only the scopes that name it, through `get_task` (G2). Each write is committed as its writer (G3).
9. `hikari-task-work` for partition agents and `hikari-scope-work` for the orchestration agent, and a task notice in each turn (H1, H2).
10. Plans stay as they are (I1). The glossary gains the round's terms, and "work order" retires (I2). Configurable pipelines, estimates, people as assignees, outside notifications, schedules, and metrics wait (I3).

## Decisions Made in This Spec

1. **Two halves of a module.** A module runs on the host in `hikari up`, and in each container in `hikari mcp` and `hikari driver`. The interface has an agent half (tools, skills, mounts, commands) that needs only the topology or the agent's `Self`, and a host half that the server opens with a `Host`: what a module may ask of the running hikari.
2. **The registry is one package, `internal/modules`,** that lists the modules. The interface is `internal/module`, and the tasks module is `internal/modules/tasks`, so that the server, the MCP server, the driver, the launcher, and the commands import the registry, and the module imports neither the server nor the partitions.
3. **The `hikari` sender is a participant with the role `system`,** listed after the agents and before you. It never has a turn, and no agent can send to it. An agent may not be named `hikari`.
4. **A scope's own status changes only in these ways:** the server sets `queued`, `waiting`, `started`, and `validating`, and follows the parts to `testing` and `blocked`; the orchestration agent sets `blocked` with a reason, clears a block, and asks for `done`; you may set any status, since you are the authority on the project, and your `done` needs no health.
5. **Pause finishes the current scope.** A paused queue starts no next scope; the scope in progress goes on, since its agents are already at work.
6. **The server wakes the orchestration agent only when it must act:** a queue's scope starts, a part is done, a part is blocked, a block is cleared, or every part is done. The other changes reach it in the notice of its next turn.
7. **The queue's `state` is `running` or `paused`.** A new queue is paused.
8. **The CLI of a module is a command of its own.** `hikari task` and `hikari queue` dispatch to the tasks module. With hikari running, they call the UI's API through `.hikari/host.json`. With hikari stopped, `hikari task stub` and the queue edits write the files, and the next start commits them as yours; `hikari queue start` and `pause` need a running hikari.
9. **Reordering in the Work tab uses buttons and a menu,** "Move up", "Move down", and "Move to queue", which a test can drive and which keyboards reach, instead of drag and drop.

## Requirements

### Modules

- **MO1.** `internal/module` defines `Module`: its `Name`; its `Skills(role)`, each a skill folder of an embedded file system; its `Tools(server, client, self)` for the MCP server in a container; its `Mounts(topology, agent)`; its `Commands`; and `Open(host)`, which returns the host half, `Instance`: its agent routes and UI routes, its `Notices(agent)`, its `View()` for the UI, `UserPath(path)`, and `Watch(ctx)`.
- **MO2.** `Host` gives a module: the topology; the health of a feature in a partition's feature map or the orchestration feature map; a commit of vault paths as an author (you, an agent, or `hikari`); a message from `hikari` to an agent; and a call that tells the UI that the module's view changed.
- **MO3.** `x-hikari.modules` is a list of module names. An unknown name is an error that lists the known ones. `hikari init`'s starter topology sets `modules: [tasks]`.
- **MO4.** For each enabled module, `Self` names it in `modules`; the MCP server adds its tools; the driver installs its skills into the agent's pack and lists them with the pack; the launcher adds its mounts; the server mounts its routes under `/api/agent/<module>/` and `/api/ui/<module>/`, adds its notices to the turn context's `notices`, puts its view in the snapshot's `modules`, and publishes an event of type `module` when it changes; and the start of `hikari up` commits your edits of its user paths, as it does for the wikis.
- **MO5.** A project without a module has none of its tools, skills, mounts, routes, notices, or view.

### Tasks: the documents

- **TK1.** A task is `vault/tasks/<id>.md`, a markdown document with a property header. Its properties: `version`, `id`, `kind: task`, `stage` (`stub` or `scope`), `name`, `author`, `created`, `updated`; for a scope, `partitions`, `features`, and `needs`; and `status`, `queue`, and `parts`, which hikari writes. A property that hikari does not name stays, in its place, through every write.
- **TK2.** A document with `stage: stub` and a name is a stub. hikari fills in a missing `id` from the file's name, and a missing `created`, `version`, and `kind`, at its next read, as a write by `hikari`.
- **TK3.** A scope's body has, in order, `## Request`, `## Goal`, `## Parts` with a `### <partition>` for each partition, `## Done when`, `## Answers`, and `## Log`. `scope_task` keeps `## Request` as the stub had it: the stub's whole body.
- **TK4.** Each entry of `partitions` is a partition agent of the topology, each `features` entry is a `[[id]]` link to a feature of a partition the scope names or of the orchestration feature map, and each `needs` entry is a `[[id]]` link to another scope. A broken entry is a problem of the document.
- **TK5.** A queue is `vault/tasks/queues/<id>.md`, with `version`, `id`, `kind: queue`, `name`, `state`, `created`, `updated`, and `scopes`, an ordered list of `[[id]]` links. A scope is in at most one queue, and a stub in none. The queue document alone records membership and order; hikari writes each scope's `queue` from it.
- **TK6.** A document that hikari cannot read, or that breaks TK4 or TK5, is a problem, named in the module's view with the reason, and the scheduler leaves it alone. hikari never rewrites a document with a problem.
- **TK7.** hikari writes only the properties it owns (`id` and `created` when missing, `updated`, `status`, `queue`, `parts`) and the `## Log`. It reloads the folder each second, so that your edits in Obsidian take effect, and commits them as yours.

### Tasks: stubs and scopes

- **TK8.** `plant_stub(name, body)` writes a stub with the calling agent as `author`. You plant one in the Work tab, with `hikari task stub`, or as a file.
- **TK9.** The Work tab's Scope button sends the orchestration agent the direct message "Please scope [[<id>]]. Follow the Scope a stub procedure of the hikari-scope-work skill." from you.
- **TK10.** `scope_task(id, partitions, features, needs, goal, parts, done_when, answers)` is the orchestration agent's. It turns a stub or a draft scope into a scope, with the sections of TK3. It is refused for a scope in a queue.

### Tasks: queues

- **TK11.** `change_queue` is the orchestration agent's, and the Work tab and `hikari queue` give you the same operations: create a queue, rename it, remove an empty one, add a scope at a position, move a scope within its queue or to another, and take a scope out. A stub cannot enter a queue. A scope whose status is past `queued` or `waiting` cannot move or leave.
- **TK12.** Start and Pause are yours, in the Work tab and with `hikari queue start` and `pause`. A queue's state is `running` or `paused`, and a new queue is paused (decision 7).

### Tasks: running

- **TK13.** The scheduler runs after every change. For each running queue, in the order of their ids, it takes the first scope that is not done. When its `needs` are done and none of its partitions has a part in a started scope that is not done, it sets the scope `started` and its parts `queued`, and sends the orchestration agent, as `hikari`, "The queue <queue> started [[<id>]]. Follow the Carry out a scope procedure of the hikari-scope-work skill." Otherwise it sets the scope `waiting`, with the reason in its log.
- **TK14.** `set_task_status(task, status, note)` sets the caller's part of a scope for a partition agent: `started`, `testing`, `blocked` (a note is required), or `done`. The server grants `done` only when the health of every feature of the scope that is in the caller's partition is `passing`; otherwise the part stays `testing`, and the tool's answer names each feature that is not passing, with its health.
- **TK15.** A scope's status follows its parts: `blocked` when a part is blocked; `testing` when every part is `testing` or `done` and one is `testing`; `validating` when every part is done. When every part is done, the server sends the orchestration agent "Every part of [[<id>]] is done. Validate it: follow the Validate a scope procedure of the hikari-scope-work skill."
- **TK16.** For the orchestration agent, `set_task_status` on a scope sets `blocked` (with a note), clears a block (`started`), or asks for `done`, which the server grants only when the scope is `validating` and every orchestration feature that the scope names has the health `passing`. You may set any status of a scope or a part, with no health check (decision 4).
- **TK17.** A done scope frees its partitions, and the scheduler starts the next scope of each running queue that waited for them.
- **TK18.** Each change of a status adds a line to `## Log`: the time, who, the old status, the new status, and the note. The server commits it as the writer: the agent, you, or `hikari`.
- **TK19.** The server wakes the orchestration agent with a message from `hikari` when a part is done, blocked, or no longer blocked (decision 6).

### Tasks: who sees what

- **TK20.** The orchestration agent mounts `vault/tasks/` read-only at `/hikari/tasks/`. A partition agent mounts nothing of it.
- **TK21.** `get_task(id)` returns a scope to a partition agent that the scope names, and nothing else to it. `list_tasks` returns, to a partition agent, the scopes that name it, with their statuses; to the orchestration agent, the stubs, the drafts, and each queue with its scopes and statuses.

### Tasks: skills, notices, CLI

- **TK22.** The partition pack gets `hikari-task-work`: when a part arrives, read the scope with `get_task`, set `started`, do the work with `hikari-plan-work`, `hikari-do-work`, and `hikari-validate-work`, set `testing` when the suites run, ask for `done` when they pass, set `blocked` with the reason the moment it cannot go on, and plant a stub for work outside its part.
- **TK23.** The orchestration pack gets `hikari-scope-work`, with three procedures: Scope a stub (read it, read the feature maps, ask the user its questions in their direct messages, ask partition agents only questions, and write the scope with `scope_task`); Carry out a scope (send each part to its partition agent by direct message when its needs within the scope are met, and follow the statuses); and Validate a scope (read the health of the features it names and the flows, then ask for `done`, or set `blocked` with what failed and send the part back).
- **TK24.** Each turn's notice names the agent's parts of started scopes, with their statuses, and, for the orchestration agent, each started or validating scope.
- **TK25.** `hikari task stub <words>`, `hikari task list`, `hikari task status <id> <status> [note]`, and `hikari queue new|rename|remove|add|move|out|start|pause|list` work as decision 8 says.
- **TK26.** The Hikari plugin gets a `tasks` skill: plant a stub from the conversation, list the pipeline, arrange queues, and start or pause one, through `hikari task` and `hikari queue`.

### Tasks: the UI

- **TK27.** The Work tab appears when the module is on. It shows your stubs, each with Scope; a field that plants a stub; the draft scopes; and each queue as a column with its state and Start or Pause, its scopes in order with their statuses and their parts' statuses, Move up, Move down, Move to queue, and Take out; and a New queue button.
- **TK28.** A click on a task opens its document: its properties, its sections as markdown, its parts with their statuses, and its log, with Set status for you.
- **TK29.** A partition's page lists the scopes that name it, with their statuses. The UI shows a message from `hikari` with its own sender.

### Docs

- **TK30.** `docs/glossary.md` defines module, task, stub, scope, part, queue, and status, and retires "work order". `docs/design.md` gives the reasons for modules, for queues as documents, for one scope per partition, and for the server's `done`. A guide, `docs/guides/tasks.md`, walks through a stub to a done scope. The README names the pipeline in How It Works.

### Checks

- **TK31.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover a round trip of a task with your own properties; problems; queue operations and their refusals; the scheduler with two queues that share a partition, and `needs`; part and scope statuses; `done` refused while a feature fails, and granted when it passes; the log and the commits' authors; what a partition agent can and cannot read; the module's absence from a project without it; the CLI with hikari stopped; and the Work tab.
- **TK32.** A live check of the shop with the module on, started with `-no-launch`: through the agents' API, a stub becomes a scope of `orders` and `inventory`, enters a queue, starts, waits on a busy partition in a second queue, moves through the statuses, is refused `done` while a feature fails, and finishes; and headless Chrome shows the Work tab at each step.

## Build Plan

1. Modules: the interface, the registry, `x-hikari.modules`, `Self.modules`, the turn context's notices, mounts, skills, routes, the snapshot and events, user paths, commands, and the `hikari` sender (MO1 to MO5, decision 3).
2. Tasks: the documents and the queues, with their problems and the file operations (TK1 to TK7, TK11).
3. Tasks: the scheduler and the statuses, the gate of `done`, the log, and the commits (TK12 to TK19).
4. Tasks: the agent API and the tools, the UI's API, what each agent sees, the skills, the notices, and the CLI (TK8 to TK10, TK20 to TK26).
5. The Work tab and the partition page (TK27 to TK29).
6. Docs (TK30), and the checks (TK31, TK32).
