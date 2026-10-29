# Spec: The Hikari Agent in the UI

Status: draft.

Sources: `docs/brainstorm/hikari-in-the-ui.md` (sections B to D) and `hikari-in-the-ui-answers.md`, and a probe of Claude Code 2.1.312 on 2026-10-28: with `--input-format stream-json`, `--output-format stream-json`, and `--permission-prompt-tool stdio`, a headless session writes a `control_request` of subtype `can_use_tool` (the tool's name, its input, a description, and suggestions) to stdout, waits for a `control_response` on stdin (`allow` with the input, or `deny` with a message), and its `init` message lists the session's slash commands, model, and permission mode. The names are those of `docs/glossary.md`, with the terms of the round: hikari agent, panel, Hikari, and mood.

## Summary

`hikari up` runs the hikari agent for you: your own Claude Code, on your machine, in the project root, with your plugins, your settings, and your login, so the session has everything a terminal session has, the Hikari plugin among them. It is not in the room. A panel anchored to the right of the page holds your chat with it: your messages, its turns as the turn cards of today, its closing text as the reply, and a card for each permission it asks for, with Allow and Deny. The panel's header sets the model, the effort, and the permission mode, starts a new chat, and stops a turn. One session per project resumes across runs. Hikari, a white pixel-art cat at the top right of the page, toggles the panel and shows the mood of the run.

## Decisions

From the round and its answers:

1. The hikari agent is your own Claude Code on the host, in the project root, with your `~/.claude`; it is not a participant of the room (B1).
2. Its turn starts with the session-start hook's context and a few lines from `hikari up` (B2).
3. One session per project, resumed across runs, with New chat; the transcript is kept (B3).
4. The panel is a chat with Claude Code's controls in its header, and the composer completes `/` with the session's slash commands (B4).
5. Permissions ask by default, and the prompts are cards in the panel (B5).
6. The agent is on only for a loopback listener, unless a flag turns it on (B6).
7. Hikari is an SVG sprite with the moods of B7, and a dot when the agent answered while the panel was closed.
8. The panel is a sheet anchored to the right, open across views, with a draggable edge (B8).
9. The containers' agents never see the hikari agent, and its cost is shown apart from the run's (B9).
10. The terminal, the orchestration agent, and a terminal session with the plugin are unchanged (C1 to C3).
11. The glossary, the design notes, and the README describe it (D1); the panel in editing mode stays planted (D3).

## Decisions Made in This Spec

1. **The code calls it the assistant.** The package is `internal/assistant`, the snapshot's field `assistant`, and its routes `/api/ui/assistant/…`, since "agent" names the containers' agents everywhere in the code. The prose keeps the glossary's name, the hikari agent.
2. **One process per turn, with stream-json input,** as the driver runs a container's turn, so that a permission prompt can be answered on stdin during the turn, and a change of model, effort, or permission mode takes effect at the next turn with no process to restart. Stop kills the process, and the session resumes at the next turn, since Claude Code writes the session as it goes.
3. **The driver's tap moves to a package of its own,** `internal/claude`, with the stream's types, so that the assistant and the driver turn Claude Code's stream into the same activity entries, and the panel shows the same turn cards as the room.
4. **A message during a turn is refused.** The panel's composer is disabled while the agent works; Stop ends the turn first. Queued messages would need the rules of a second conversation.
5. **The transcript is a file per session,** `.hikari/assistant/<session>.jsonl`, with the user's messages, the activity entries, and the permission prompts with their answers, and `.hikari/assistant/session.json` names the session with its model, effort, permission mode, and cost. `.hikari/` is already ignored.
6. **The permission mode menu offers ask, accept edits, plan, and bypass,** which are Claude Code's `default`, `acceptEdits`, `plan`, and `bypassPermissions`; bypass also passes `--dangerously-skip-permissions`, as the containers do, and the menu marks it. `auto`, `dontAsk`, and `manual` are left out.
7. **The model menu offers Claude Code's default and its aliases,** `fable`, `opus`, `sonnet`, and `haiku`, and the effort menu Claude Code's default and `low` to `max`. The default passes no flag, so your settings decide.
8. **The slash commands come from the session's `init` message,** so the composer completes nothing before the first turn of a run. A probe turn at start would cost tokens for a list.
9. **A pending prompt shows on the cat as a question mark,** and an answer that ended while the panel was closed as a dot, so that neither waits unseen.
10. **The cat is a sprite of pixel rows,** each frame a few strings of one character per pixel, drawn as SVG rectangles. The moods are asleep, content, alarmed, and thinking; an agent working while the hikari agent works draws thinking.
11. **The flag is `-agent auto|on|off`,** with `auto` on for a loopback listener.

## Requirements

### The claude package

- **HA1.** `internal/claude` holds the stream's types (`Event`, with the fields of `control_request`: `request_id`, and `request` with `subtype`, `tool_name`, `input`, `description`, and `permission_suggestions`), `Tap` (the driver's tap, with a `Post func(api.Activity)` in place of the client), and the clipping helpers. The driver uses them, and its activity tests move with the tap.

### The assistant

- **HA2.** `assistant.New(dir, root, context, listener)` opens the assistant of a project: `dir` is `.hikari/assistant/`, `root` the project root, `context` the lines of HA4, and `listener` receives its events. It reads `session.json` and the session's transcript when they exist.
- **HA3.** `Send(text)` runs one turn: `claude -p --input-format stream-json --output-format stream-json --verbose --include-partial-messages --permission-prompt-tool stdio --permission-mode <mode> [--dangerously-skip-permissions] [--model <m>] [--effort <e>] [--resume <session>] --append-system-prompt <context>`, in `root`, with the process's environment, the text as one user message on stdin, and stdin kept open. It records the message, opens a turn in the tap, feeds the stream to the tap, answers each `can_use_tool` request from the panel (HA5), keeps the session id and the slash commands of `init`, and ends the turn with the result's text and cost, or with the error. While a turn runs, `Send` fails with "the hikari agent is working; stop it or wait".
- **HA4.** The context tells the session: that it runs as the hikari agent inside hikari's web UI, for the project named and the run's folder; that hikari runs, with its UI's URL; where the run's feed and messages are (`feed.jsonl` and `messages.jsonl` in the run's folder); and that its closing text is the reply that the person reads in the panel.
- **HA5.** A `can_use_tool` request becomes a prompt: its id, turn, time, tool, input, and description. `Answer(id, allow, message)` writes the `control_response`: `allow` with the input, or `deny` with the message or "The person denied it in hikari's panel". An answered prompt keeps its answer. A prompt of a turn that stops is answered `deny`.
- **HA6.** `Stop()` kills the process; the turn ends with the error "stopped". `NewChat()` forgets the session id, so the next turn starts a session, and empties the transcript shown. `SetOptions(model, effort, mode)` keeps the three for the next turn.
- **HA7.** The `View` is: `enabled` (with `reason` when off), `session`, `model`, `effort`, `permission_mode`, `state` (`idle` or `working`), `cost` (the session's, summed from each turn's), `commands` (the slash commands), `messages` (the user's, each with an id, a time, and its text), `activity` (the entries, the last 400 with old turns dropped whole, as the server keeps an agent's), and `prompts` (each with its answer when it has one). The listener gets an event for each change: a message, an activity entry (sent as the tap sends it), a prompt, an answer, and the state with its options.
- **HA8.** The transcript and `session.json` are written as the view changes: each message, entry, prompt, and answer appended to `<session>.jsonl`, and `session.json` written at the end of each turn and on `SetOptions` and `NewChat`.

### The server and hikari up

- **HA9.** The run's server carries the assistant: `SetAssistant` gives it one, the snapshot gains `assistant` (its `View`, or `{enabled: false, reason}` without one), and each of its events is published as an event of type `assistant` with the change. Its routes, for the UI's cookie only: `POST /api/ui/assistant/send {text}`, `POST /api/ui/assistant/answer {id, allow, message}`, `POST /api/ui/assistant/stop`, `POST /api/ui/assistant/new`, and `PUT /api/ui/assistant/options {model, effort, permission_mode}`. A refused send answers 409 with its reason.
- **HA10.** `hikari up -agent auto|on|off` (default `auto`) runs the assistant for a run when `on`, or when `auto` and the listener's host is a loopback address, and `claude` is on the PATH. Otherwise the snapshot says why: "the hikari agent is off for a listener outside this machine; pass -agent on to run it" or "claude is not on the PATH". The assistant stops with the run, and its files stay for the next.
- **HA11.** `hikari doctor` warns when `claude` is not on the PATH, since the hikari agent in the UI needs it.

### The UI

- **HA12.** Hikari, the cat, sits at the top right of the page on every page, including the topology editor. A click toggles the panel; Esc closes it. Her mood follows the state (decision 10): `thinking` while the hikari agent works; else `alarmed` when an agent is in `error`, an agent exited with a detail, a scope of a running queue is `blocked`, or the hikari agent's last turn failed; else `content` when an agent is `working` or `indexing`; else `asleep`. A pending prompt shows a question mark on her, and a turn that ended while the panel was closed a dot, until the panel opens.
- **HA13.** The cat is an inline SVG sprite: pixel rows per frame, at least two frames per mood, cycling with CSS or a timer, white with an outline that follows the theme, with `aria-label` naming the mood.
- **HA14.** The panel is anchored to the right of the page beside the main view, which shrinks; on a narrow screen it covers the page. Its width is draggable at its left edge and kept in `localStorage`. It stays open across views, and its open state is kept too. When the assistant is off, the panel says why.
- **HA15.** The panel's header shows "Hikari", the state, and the session's cost, and holds menus for the model, the effort, and the permission mode (decisions 6 and 7), a New chat button, and a Stop button while a turn runs. A change of a menu calls the options route.
- **HA16.** The panel's body is the timeline of the session: your messages, the turns as `TurnCard`s with the closing text as the reply, and each prompt as a card at its place in the turn: the tool, its description, its input (the command, the file, or the input as JSON), and Allow and Deny while it waits, or its answer once given. It follows new entries while the reader is at the bottom.
- **HA17.** The composer sends on Enter, with Shift-Enter for a line, is disabled while a turn runs, and completes `/` with the session's slash commands: a list filtered by what is typed, chosen with the arrows and Enter or Tab, or a click.
- **HA18.** The store keeps `assistant` from the snapshot and applies each `assistant` event; it counts an ended turn as unread while the panel is closed, and clears it when the panel opens.

### Docs

- **HA19.** The glossary widens "Hikari agent" to the terminal or the panel, and gains panel, Hikari (the cat), and mood. `docs/design.md` gets a section on why the hikari agent runs as you with your permissions asked, where the containers' agents run with every permission skipped, and why it is not in the room. The README's tour of the UI names the cat and the panel.

### Checks

- **HA20.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover: the tap in its package (HA1); a turn against a fake `claude` on the PATH that writes `init`, a `control_request`, and a `result`, with the prompt answered, the session kept, the commands kept, and the transcript and `session.json` written (HA3, HA5, HA7, HA8); a refused send during a turn, Stop, and New chat (HA3, HA6); the context (HA4); the routes and the snapshot (HA9); the `-agent` rule (HA10); the mood of each state (HA12); the panel's header, timeline, prompt card, and composer completion (HA15 to HA17); and the store's events and unread (HA18).
- **HA21.** A live check on the shop with headless Chrome, with a real `claude`: the cat shows, the panel opens, a message gets a reply through a permission prompt answered Allow, a `/` completes a Hikari skill, Stop ends a turn, and the session resumes after a restart of `hikari up`.

## Build Plan

1. The claude package and the driver (HA1).
2. The assistant (HA2 to HA8), with its fake-claude tests.
3. The server, hikari up, and doctor (HA9 to HA11).
4. The UI: the cat, the panel, the composer, and the store (HA12 to HA18).
5. The docs (HA19) and the checks (HA20, HA21).
