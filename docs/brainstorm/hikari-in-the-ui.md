# Hikari in the UI: Round 1

Source: your message of 2026-10-28, from a run of hikari on a large project.

## What I Take from Your Proposal

You run hikari in three windows, and you want one. Each window does one job today:

- **A Claude Code terminal with the Hikari plugin.** High-level planning, questions about the service, status, and the decisions that the workflow needs from you. The orchestration agent could take some of this, but it is busy with the run, and you do not want to interrupt it. That separation holds: the orchestration agent works with the partition agents, and the meta-level work stays with you.
- **The `hikari up` terminal.** Its one-line summaries are what you read most, since a line per event skims better than the UI's turn cards.
- **The web UI.** Which agents work, which wait, and whether the run still moves.

Two changes bring the first two windows into the third:

- **All activity becomes a feed**, close to what the terminal prints: one line per event, with the agents' states pinned, instead of the second copy of the room that it is today.
- **A hikari agent lives in the UI**, outside the room: a Claude Code session on your machine, with the Hikari plugin and everything else your sessions have, for planning and for questions about the project. It opens as a chat anchored to the right side of the page, with Claude Code's own controls where they fit: slash commands, model, effort, and permission mode. Its toggle is an animated white pixel-art cat on the toolbar, Hikari herself, whose expression follows the run: asleep when nothing happens, content when the work goes well, alarmed when something is wrong.

The `hikari up` terminal keeps printing what it prints today, since a run still starts from a terminal. The Claude Code terminal becomes optional.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

- **Feed**: the run's events as one line each, in order: a message, an agent's state change, the end of a turn, a module's notice. The terminal prints it; the UI shows it under All activity.
- **Strip**: the agents' states, each with its current step, and the cost of the run, pinned above the feed. It is the terminal's footer.
- **Hikari agent**: the glossary's term, widened: a Claude Code session on the host with the Hikari plugin, in your terminal or in the UI's panel. It does not work on the code.
- **Panel**: the chat with the hikari agent, anchored to the right of the page.
- **Hikari**: the cat on the toolbar, which toggles the panel and shows the mood of the run.
- **Mood**: what the cat shows: asleep, content, busy, alarmed, or thinking.

## What Exists Today

- `hikari up` prints one line per message (`cmd/hikari/dashboard.go`, `messageLine`), errors as they happen, and a footer with the web UI's address, each agent's state and current step, and the cost. The footer is redrawn in place, and a pipe gets plain lines.
- The server publishes one event stream: messages, each agent's status, each step of each turn, and the modules' views. The UI's All activity renders the same messages and turn cards as the room, with a channel label on each message.
- Each container's driver runs `claude -p --output-format stream-json --resume <session>` and turns the stream into activity entries, which the UI shows live as turn cards (`internal/driver`). The driver keeps the session id across turns.
- The Hikari plugin's `room` skill posts to the room as you, with `hikari send`, and follows the run through `messages.jsonl` and `hikari status`. The session-start hook tells a session which project it is in and whether hikari runs for it.
- The glossary already names the hikari agent: a session on the host with the plugin, which does not work on the code.

---

## A. The Feed

**A1. The server builds the feed, and the terminal and the UI both show it.** Today the terminal derives its lines from the event stream in `dashboard.go`, and the UI would have to derive the same lines a second time, in TypeScript, from the same events, and the two would drift. I recommend one producer in the server: a `feed` entry is an event of its own, with a time, a kind, the agent or sender it concerns, one line of text, and a link to what it summarizes (a message id, a turn, a task). The terminal prints each entry's text, and the UI lists them. The server keeps the last few hundred entries, as it keeps activity, and the snapshot carries them, so a page that opens mid-run sees the recent past.

**A2. What makes a line.** The terminal's lines today, plus the ones you read the footer for:

| Event | Line |
|---|---|
| a message | `14:02  orders → room  Done: the coupon field is in [[create-order]]…`, as today |
| a turn ends | `14:05  orders  turn done · 3m 12s · edited 4 files`, from the turn's summary; a failed turn in red |
| an agent's state changes | `14:05  orders  idle`, `starting`, `error: …`, `exited: …` |
| a module's notice | `14:06  hikari  [[coupons]] orders: testing → done`, from the tasks module's log |
| a start-up step | `Read shop · 3 agents`, `Connected 3 agents`, as the terminal prints them |

A tool call is not a line: the strip shows the current step, and the turn card keeps the rest. Each line is one line; the UI truncates as the terminal does, and the whole text is one click away.

**A3. A click opens what the line summarizes.** A message line opens its conversation at that message; a turn line opens the agent's direct messages at that turn; a task line opens the task in Work. All activity becomes the way in, and the room and the direct messages keep the full view.

**A4. The strip sits above the feed, as the terminal's footer sits below it.** One row per agent, or one line with all of them when they fit: the state glyph, the name, the current step in a few words, and the run's cost at the end. The UI already has the state dot and the step labels of the turn card; the strip reuses them. I recommend the strip only on All activity, since the sidebar already shows each agent's state everywhere else. A second option is a strip on every page; say so if you want it.

**A5. Unread counts and the sidebar.** All activity keeps its place at the top of the sidebar and its unread count. A message in the room or in a direct message counts for its own conversation, as today, and the feed counts nothing of its own, so that the count on All activity stays the count of messages you have not seen.

**A6. The feed is a record too.** The server appends each entry to `.hikari/runs/<run>/feed.jsonl`, next to `messages.jsonl`, so that a session on the host, or the hikari agent of section B, can read what happened without the UI. The run's log keeps everything else.

## B. The Hikari Agent

**B1. It is your own Claude Code, on the host, in the project root.** `hikari up` runs `claude -p` with the stream-json output, as the driver does in a container, but on your machine, as you, with your `~/.claude`: your plugins, your settings, your memory, and your login. It needs `claude` on the PATH, which `hikari doctor` checks. It has what a terminal session has, including the Hikari plugin and its skills, since the plugin reaches every session on the machine. It takes no agent token, and it is not a participant of the room: it posts there as you, with `hikari send`, through the `room` skill, as a terminal session does.

**B2. What it knows at the start of a turn.** The session-start hook already tells a session the project, the version, the stage, and whether hikari runs. `hikari up` adds, through `--append-system-prompt`, that the session runs inside hikari's UI for this run, the path of the run's folder, and that the feed of A6 and `messages.jsonl` are where the run's events are. Nothing else changes in how it works: the skills decide the rest, as in a terminal.

**B3. One session per project, resumed across runs.** The agent keeps its Claude session across turns, as the driver does, and `hikari up` keeps the session id in `.hikari/assistant.json`, so that the next run resumes it. A New chat button in the panel starts a fresh session. The panel's transcript is the activity stream of the session, kept in `.hikari/assistant/<session>.jsonl`, so that a page that reloads, or a run that restarts, shows the past turns.

**B4. The panel is a chat, with Claude Code's controls in its header.** Your messages, and the agent's turns as the turn cards of today, with its closing text as the reply. In the header, as menus: the model, the effort, the permission mode (B5), New chat, and Stop, which ends the running turn and leaves the session to resume. The composer completes `/` with the skills and commands that Claude Code lists, since a typed `/skill` runs in a headless session, while Claude Code's built-in commands such as `/compact` do not; the spec checks which flags Claude Code offers for the model, the effort, and the permission mode, and the header shows only what the flags support.

**B5. Permissions: ask by default, and the panel asks.** A headless session cannot prompt, so the containers run with every permission skipped. On your own machine that is the wrong default: a page in a browser would run any command as you. Claude Code lets a headless session route its permission prompts to an MCP tool (`--permission-prompt-tool`). I recommend hikari serve that tool: a prompt becomes a card in the panel, with the tool and its input and Allow and Deny, and the agent waits for your click. The permission mode menu then offers what Claude Code offers: ask, accept edits, plan, and bypass, with ask as the default and bypass marked as such.

**B6. Only a loopback listener gets the agent.** The panel runs commands on your machine as you. The UI's token guards every endpoint, and `hikari up` listens on localhost by default. When `-listen` names another address, the agent is off and the cat says why, unless a flag turns it on. The containers are unaffected either way.

**B7. The cat is Hikari, a white pixel-art cat, drawn as an SVG sprite with a few frames per mood.** No image files, so the binary gains nothing to download, and the frames follow the page's theme. Her moods, from the run's state:

| Mood | When |
|---|---|
| asleep | every agent idle, no queue running, and the hikari agent idle |
| content | an agent works, no error, no block, no failing health that a running scope names |
| alarmed | an agent in error or exited with a reason, a scope blocked, or the hikari agent's turn failed |
| thinking | the hikari agent works on your message |
| busy | the hikari agent works while agents work, drawn as thinking |

A dot on the cat says that the hikari agent answered since you last opened the panel. She sits at the right end of the page header, on every page, and a click toggles the panel; Esc closes it.

**B8. The panel is a sheet anchored to the right, open across views.** It keeps its width when you move between pages, and you can drag its edge. The main page shrinks rather than hides, so the feed and the panel fit side by side on a wide screen; on a narrow one the panel covers the page.

**B9. The containers' agents never see the hikari agent.** It is not in the room, not in the topology, and not in the feed, except through the messages it posts as you with `hikari send`, which are yours. Its cost is shown in the panel, apart from the run's cost in the strip, since your login pays for it and the project's credential pays for the run.

## C. What Stays as It Is

**C1. The terminal.** `hikari up` prints the same lines, from the feed of A1, and keeps its footer.

**C2. The orchestration agent.** Nothing here changes what it does or what it sees.

**C3. A terminal session with the plugin** keeps working as today. The panel is a second home for the same agent, not a replacement.

## D. Docs and Order

**D1. The glossary** widens "Hikari agent" to the terminal or the panel, names the cat, and adds feed, strip, and mood. The design notes get a section on why the feed has one producer, and one on why the hikari agent runs as you with your permissions asked, where the containers' agents run with every permission skipped. The README's tour mentions the cat.

**D2. Two specs, the feed first.** The feed is a day's work and gives you the window you look at most; the hikari agent is the larger one, and its spec can take what the feed's build learned. Each spec gets a live check on the shop: the feed against the terminal's lines, and the panel with a real session through a permission prompt.

**D3. Out of this round, planted:** the panel as a place to approve the agents' questions from the room (the orchestration agent's questions to you still arrive as messages); voice input, which the inspiration shows; a cat for each partition agent; and the panel outside a run, in editing mode, where the hikari agent could help with the topology.
