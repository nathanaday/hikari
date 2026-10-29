# Spec: The Feed

Status: built. Checked on 2026-10-28 on the shop, started with `-no-launch`, through the agents' API, the UI's API, and headless Chrome:

- **The terminal and the feed agree.** A post from `hikari send`, a direct message from the orchestrator to `orders`, the orchestrator's ended turn (`turn done · 4.2s · 1 tool call · $0.04`), a private message between the partitions, hikari's message and two log lines when the queue `api` started `[[coupons]]`, and `inventory`'s error printed as the same lines in the terminal and in `feed.jsonl`, with the two start-up steps in the journal only.
- **All activity** listed the same entries under the strip, with `inventory · error` and `$0.04 spent`. A click on the post showed it whole, as markdown, and a second click folded it. The names opened the room for the post and for the turn woken in the room, `#/work/coupons` for a log line, and `inventory`'s direct messages for its error; the private message between the partitions had no link.

No container ran, so a turn from a real driver was not checked; the server's turn entry is covered by its test.

Changes made during the build:

1. **An agent that exits with a reason after exiting without one makes an entry,** since the driver may report the exit before it knows the reason (FD3).
2. **A message of a module** (hikari's direct message to the orchestrator) is a message entry, as every message is, beside the module's log lines.
3. **The strip shows an agent's step only while it works,** and `· error` for an agent in error; an idle agent shows its dot and its name (FD9).
4. **The room and the direct messages scroll to the message or the turn** that the feed opened, through an id on each message row and turn card (FD10).

Sources: `docs/brainstorm/hikari-in-the-ui.md` (section A) and `hikari-in-the-ui-answers.md`. The names are those of `docs/glossary.md`, with the terms of the round: feed, strip, and entry.

## Summary

The run's events become a feed: one entry per message, per ended turn, per agent that connects, fails, or exits, per change of a task's status, and per start-up step. The server produces the entries, keeps the recent ones, publishes each as an event, and appends each to `feed.jsonl` in the run's folder. The terminal prints the entries it printed as lines before, from the feed, and the UI's All activity lists them under a strip of the agents' states and the run's cost, in place of the second copy of the room it showed. A click on an entry shows its whole text, and its name opens the conversation, the turn, or the task it summarizes.

## Decisions

From the round and its answers:

1. The server builds the feed, and the terminal and the UI both show it (A1).
2. An entry for a message, an ended turn, an agent's state, a module's notice, and a start-up step; a tool call is not an entry (A2).
3. A click opens what the entry summarizes (A3).
4. The strip sits above the feed, on All activity only (A4).
5. All activity keeps its unread count of messages, and the feed counts nothing of its own (A5).
6. The feed is appended to `.hikari/runs/<run>/feed.jsonl` (A6).

## Decisions Made in This Spec

1. **An agent's state makes an entry three times: when it connects, when it fails, and when it exits with a reason.** The round's table listed `idle` too, but every ended turn already makes an entry, so an `idle` after each turn would say each thing twice, and `working` is what the strip shows. The terminal prints the same three today.
2. **The terminal prints every entry except a start-up step,** which it printed already as the step ended, with its spinner. The UI lists the steps, so that a page opened mid-run sees how the run started.
3. **An entry carries its links as ids, and the UI resolves them:** the message's id, the turn's id with its agent, or the task's id. The UI finds the conversation of a message as it does today, and the view of a turn from the turn's trigger, so that the server repeats none of the UI's rules.
4. **A private message between two agents stays readable.** All activity was the only view that showed one whole. A click on any entry shows its whole text in place, so the feed keeps that.
5. **The run's cost is a field of the snapshot,** summed by the server from the agents' status reports as the terminal sums it today, and the UI adds each later report, so that the strip and the footer agree.
6. **The module interface widens by one call,** `Feed(text, task)`, which the tasks module calls for each line it writes to a task's log. The entry is hikari's, as the task's log line is.

## Requirements

### The entries

- **FD1.** `api.Entry` is one entry of the feed: `id`, `time`, `kind` (`message`, `turn`, `status`, `module`, or `step`), `who` (the sender, the agent, `hikari` for a module, or empty for a step), `to` (a message's recipient, or empty for the room), `text` (one line, or the whole body of a message), `error` (a failed turn, an error, or an exit with a reason), and its links: `message` (the message's id), `turn` (the turn's id of `who`), and `task` (a task's id).
- **FD2.** The server keeps the last 500 entries, carries them in the snapshot as `feed` and publishes each new one as an event of type `feed`, and appends each as one line of JSON to the feed journal when it has one. `hikari up` gives it `.hikari/runs/<run>/feed.jsonl`, written on the first entry.
- **FD3.** The server makes an entry for: each message delivered, with its sender, recipient, and body; each ended turn, with `turn done` or `turn failed` and the turn's summary (its duration, its tool calls, the files it changed, and its cost, as the UI's turn card writes them); an agent's first `idle` (`connected`), its `error` (`error: <detail>`), and its `exited` with a detail (`stopped: <detail>`); and each line that a module feeds.
- **FD4.** `hikari up` feeds each start-up step as it ends, from the step's result (`Read shop · 3 agents`, `Prepared …`, `Connected 3 agents`), and a failed step with its error.
- **FD5.** `module.Host` gains `Feed(text, task string)`. The tasks module feeds each line it appends to a task's log, as `[[<id>]] <line without its time>`, with the task's id.
- **FD6.** The snapshot carries `cost`, the sum of the costs that the agents' status reports carried so far.

### The terminal

- **FD7.** The terminal prints each entry of the feed except a step, in the form `HH:MM  who → to  text` for a message and `HH:MM  who  text` otherwise, with the time dim, the name bold, and the text of an error red, truncated to the terminal's width as today. The footer is unchanged.

### The UI

- **FD8.** All activity shows the feed: the strip, then the entries in order, each as one line of `time`, `who` (with `→ to` for a message), and `text` truncated to the line, newest at the bottom, following new entries while the reader is at the bottom. A click on a line shows its whole text in place, as markdown for a message; a second click folds it.
- **FD9.** The strip shows each agent with its state dot, its name, and the label of its current step while it works, and the run's cost at the end. It sits above the entries and stays while they scroll.
- **FD10.** The name of an entry opens what it summarizes: a message, the conversation that shows it (the room, or the direct messages with the agent), or nothing for a private message between agents; a turn, the conversation that shows the turn, or the agent's direct messages when the turn is no longer kept; a task, the task in Work.
- **FD11.** The room and the direct messages are unchanged, except that a message no longer shows its channel label, which only All activity used. `turnInView` and `inView` keep their answer for the `all` view.
- **FD12.** The unread counts are unchanged: an entry of the feed counts nothing.

### Docs

- **FD13.** The glossary gains feed, entry, and strip. `docs/design.md` gets a section on why the feed has one producer and why the terminal and the UI show the same entries, after "The terminal is for people; the log is for the record".

### Checks

- **FD14.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover: an entry for a message, an ended turn with its summary, an error, an exit, and a module's line, with the journal's lines (FD2, FD3, FD5); the cost of the snapshot (FD6); the terminal's line of each kind (FD7); the feed view's lines, strip, and expansion (FD8, FD9); the link of each kind (FD10); and the store's handling of the event and of the cost.
- **FD15.** A live check on the shop with headless Chrome: the terminal and All activity show the same entries for a message, a turn, and a task's status change, and a click on each opens its target.

## Build Plan

1. The entries, the server, the journal, and the cost (FD1 to FD3, FD6), with the module call (FD5).
2. The terminal and the start-up steps (FD4, FD7).
3. The UI (FD8 to FD12).
4. The docs (FD13) and the checks (FD14, FD15).
