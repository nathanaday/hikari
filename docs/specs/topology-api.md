# Spec: the topology API and the `hikari up` loop

Status: built. Checked in a live run on 2026-10-25. `hikari up` in an empty directory printed the address and waited: "compose.yaml does not exist; create it in the web UI". `create` made a project repository with its first commit. One `apply` of six operations named the project `Editor Live`, added a path repo and the owner `app` from the template `orchestrator` in `#main`, and committed `user: name the project "Editor Live"; add repo app; add agent app in #main; give app a system; 2 more`; its diff changed only the lines of the touched nodes. Start from the API started both agents. During the run, a plan of `rename hub main to core` said "applies at start" and listed the `#main` of a hand-written note; the apply committed the compose file and the note as you, through the run's lock. Stop returned to editing, the UI's `send` answered 503, and Start began a new run whose snapshot showed `#core`, while the store kept `hubs/main`. Ctrl-C stopped the containers and exited.

Changes made during the build:

1. **The run's server takes the node API before the agents connect.** The host passed requests to the run only once it was running, so the agents' first call got 503, and both containers exited. The host now attaches the run's server when it exists, and still shows `starting`. Found in the live run.
2. **A directory that holds only `.hikari` counts as empty** (TA6). `hikari up` writes its log in `.hikari/` before the project exists, so `create` found the directory not empty and wrote a compose file without a project repository. Found in the live run.
3. **A new `repos` key goes before `hubs`**, where the README writes it. Found in the live run.
4. **Start answers `202`, and a failure later comes in the snapshot** (UP4). A start takes up to a minute, so the request ends when lint passes, and a failure of the start sequence reaches the UI as the snapshot's `start_findings` when it returns to editing.
5. **Every load reads the file through stdin**, not only lint (decision 2), so that `hikari check`, `hikari up`, and the editor run one code path.
6. **The model shows a text field as compose reads it.** A description or instructions show `$` for `$$`, and a form edit writes `$` as `$$`, so that compose keeps a literal dollar. A text that holds an expression shows as written. `IsExpression` marks interpolated values (TM2); the model holds each value as the source writes it, and the UI asks.
7. **`hikari sync` and the project scaffold take the host skills writer from the command.** The server now reaches the editor, the scaffold, and `linked`, and their import of the host skills closed an import cycle with the MCP server's tests. `internal/scaffold` holds the files of `hikari init`.
8. **A start in the same second as the last one gets a suffix**, so each run keeps its own folder (UP7). Found in the tests.
9. **A project name that starts with a digit gets `project-`.** YAML reads `name: 002` as a number, which compose refuses. Found in the tests.
10. **`add_agent` with hubs is one line of the commit message**, as `add agent dev in #main`.

Sources: `docs/brainstorm/topology-editor.md` (round 1), `topology-editor-round-2.md`, and `topology-editor-answers.md`. This is the second of four steps of the topology editor. It builds on `entity-ids.md`.

## Summary

The UI gets an API that reads, checks, and changes the compose file. A change is a list of operations, such as `rename_hub` or `add_member`. The server turns operations into text edits that touch only the YAML nodes they change, shows the result as a plan, and applies the plan as a whole. In a project repository, each saved change is a commit as you. A rename can carry the new name into the wikis and the vocabularies, wherever you choose.

`hikari up` serves the UI before it has a topology. It starts at once when the file is valid, as today. When the file is missing or broken, or a start check fails, it waits in an editing state, with the editor open in the UI. You start and stop the agents from the UI, and Ctrl-C still stops everything.

No new screen is built in this step. Steps 3 and 4 build the YAML view and the Structure view on this API.

## Decisions

From the rounds and their answers:

1. HTTP with JSON, and the existing event stream (A1).
2. The API takes operations, with plan and apply (A2).
3. The server applies the operations, in Go (A3).
4. Every write names the revision it started from (A4).
5. The wire types stay hand-written, with shared fixtures (A5).
6. An edit changes the file, and reaches the agents at the next start (B1).
7. In a project repository, each save is a commit as you (B4).
8. An operation changes only the bytes of the YAML nodes it touches (C1).
9. The editor shows and writes the source, never the resolved file (C3).
10. Findings carry a path and a line (E1).
11. `hikari up` serves the UI without a topology (B2 answer), opens `compose.yaml` by default (R1), creates a project or a compose file (R2), returns to the editor on a failed start (R3), and has Start and Stop (R4).
12. An id can change only while nothing is keyed by it (I4).
13. Propagation rewrites `@name` and `#name` only, and you choose each place (I5).

## Decisions made in this spec

1. **The model comes from the source, not from the checked topology.** The Structure view must open a file with a broken reference, because that is where a form helps most: you remove the unknown member. So the API reads its model from the YAML itself, with anchors and merge keys applied, and without the checks. Only a YAML syntax error leaves no model.
2. **Lint runs the whole load, with the text on stdin.** The topology checks need compose's resolved output, because interpolation such as `${HIKARI_BRANCH:-hikari}` and merge keys decide the values. `docker compose -f - --project-directory <dir> config` reads the text from stdin, with `.env` and relative paths resolved from the file's directory, so lint writes no temporary file. It took 0.25 s on the team example.
3. **Five primitive edits under the operations.** Every operation is made of: set a scalar, set or delete a key of a mapping, append to a list, remove from a list, and rename a key. Each primitive splices the text of one node; a flow node, such as `{orchestrator: lead, members: [dev]}`, is written again whole, on one line. Comments, blank lines, and anchors outside the node keep their bytes.
4. **A field set to the empty string removes its key.** The operations take each optional field as a pointer: absent leaves the key, `""` removes it, and any other value sets it.
5. **A new agent copies its template's text.** The new service is the template service's text, under the new id, with the template's `x-hikari` replaced and its anchors removed. So `<<: *agent`, an image, or a build is kept as the template writes it.
6. **The editing state serves only the editor.** The store views need a topology that loaded, so the editing state serves the topology API, sign-in, and the event stream. The Shared view and the maps come back when the agents run.
7. **A transition closes the event streams.** The browser opens the stream again by itself, and the first event of every stream is a snapshot that carries the mode. So the UI learns of a start or a stop without a second kind of message.
8. **During a run, the systems manager makes the commits.** Agents commit to the project repository at any time, so a commit of the editor during a run goes through the manager's lock. In the editing state, the editor commits by itself.
9. **The commit goes on the checked-out branch.** In the editing state, the project repository can be on any branch. A save commits there, as you would. During a run, that is the project branch.

## Terms

- **Mode**: `editing` or `running`. `starting` and `stopping` are the moments between them.
- **Source model**: the topology as the YAML writes it, with ids, names, references, and each entity's line, read without the checks.
- **Primitive edit**: one of the five edits of decision 3.
- **Occurrence**: one place where `@old` or `#old` appears in a wiki body or a vocabulary description.

## 1. Findings

- **TF1.** A finding has a severity (`error` or `warning`), a message, a stage (`yaml`, `compose`, `topology`, or `start`), and, when hikari knows it, a YAML path such as `x-hikari.hubs.eng.members[1]` and a line.
- **TF2.** The topology checks return their errors with the path of the key at fault. The path is in ids, as the file writes it.
- **TF3.** A path maps to the line of its node in the source. When a key comes from a merge key, the path maps to the line of the deepest node that the source writes.
- **TF4.** A YAML syntax error carries the line that the YAML parser reports. A compose error carries no path.
- **TF5.** `hikari check` prints the line before each error that has one, as `compose.yaml:14: hub "eng" references unknown agent "dve"`.

## 2. Lint

- **TL1.** Lint runs these stages on a text, and stops at the first stage with an error: the YAML syntax, then compose and the topology checks of `hikari check`, through `docker compose config` on stdin (decision 2).
- **TL2.** When the text loads, lint adds the start checks of `hikari check` as warnings: uncommitted changes, missing clones, ignore rules, credentials. A dirty repo is not a fault of the file.
- **TL3.** Lint writes nothing.

## 3. The Source Model

- **TM1.** The model holds the project (id, name, store, project branch), each repo (id, name, url, path, branch), each hub (id, name, description, orchestrator, members), each agent (id, name, description, instructions, model, system, grants, tests, e2e limits, image or build, its other compose keys by name), and each sidecar (id, serves). Each entity has its line.
- **TM2.** A value is shown as the source writes it. A value that holds `${`, which compose interpolates, is marked as an expression.
- **TM3.** Each agent, hub, and repo has `id_locked`, true when something outside the file is keyed by its id: a directory `store/systems/<id>` or `store/hubs/<id>`, a vocabulary file, or a clone `repos/<id>`.
- **TM4.** A text with a YAML syntax error has no model.

## 4. Operations

Each operation is a JSON object with an `op` and its fields. Ids name the entities, as in the file.

| Op | Fields | Refused when |
|---|---|---|
| `set_project` | `name`, `branch`, `store` | `branch` is set without a project repository |
| `enable_project` | `branch` | the project repository is already on |
| `disable_project` | | a system uses `repo: project`, or a repo has a `url` |
| `add_repo` | `id`, `url` or `path`, `branch` | the id is taken |
| `set_repo` | `id`, `url`, `path`, `branch` | |
| `remove_repo` | `id` | a system uses it |
| `add_hub` | `id`, `orchestrator`, `description`, `members` | the id is taken |
| `set_hub` | `id`, `description`, `orchestrator` | |
| `add_member`, `remove_member` | `hub`, `agent` | the agent is already a member, or is not |
| `remove_hub` | `id` | an agent would be in no hub, or an agent tests it |
| `add_agent` | `id`, `template`, `description`, `hubs` | the id is taken |
| `set_agent` | `id`, `description`, `instructions`, `model`, `tests` | |
| `set_system` | `id`, `repo`, `scope`, `read`, `test` | |
| `clear_system` | `id` | |
| `set_grants` | `id`, `releases`, `deliverables` | |
| `set_e2e` | `id`, `cpus`, `memory` | |
| `remove_agent` | `id` | it orchestrates a hub, or a grant, a sidecar, or no other agent would name it |
| `add_sidecar` | `id`, `serves`, `image` | the id is taken |
| `set_serves` | `id`, `agent` | |
| `remove_sidecar` | `id` | |
| `rename` | `kind` (`agent`, `hub`, `repo`), `id`, `name` | |
| `change_id` | `kind`, `id`, `new_id` | `id_locked` (TM3) |

- **TE1.** An operation edits the source text with primitive edits only (decision 3). No byte outside the nodes it changes changes.
- **TE2.** Optional fields follow decision 4.
- **TE3.** `rename` sets the entity's `name` field, or removes it when the new name is the id. It changes no reference.
- **TE4.** `change_id` renames the key, and every reference to it: `orchestrator`, `members`, `grants.releases`, `tests`, `serves`, and `system.repo`.
- **TE5.** `add_agent` copies its template's text (decision 5), or writes `image: hikari-agent` and the credential variables when it names no template. It adds the agent to each hub of `hubs` as a member.
- **TE6.** `remove_agent` removes the agent from the members of every hub. `remove_hub` and `clear_system` leave the store directory in place, and the plan says so.
- **TE7.** A list of operations applies in order, as a whole. When one is refused, the response names its index and the reason, and nothing is written.
- **TE8.** An operation that leaves the file failing its checks is not refused: the plan shows the findings, which decide whether you save. The findings of the plan are lint's (TL1 to TL3) on the new text.

## 5. Propagation

- **TP1.** For each `rename` of an agent or a hub in a plan, hikari lists each occurrence of `@<old name>` or `#<old name>` in the bodies of the documents of every wiki, and in the definitions and descriptions of every vocabulary. A match is the whole name: `@orders` does not match `@orders-db`.
- **TP2.** Each occurrence has a key, `<store path>:<line>:<column>`, the line's text, and the replacement.
- **TP3.** `apply` takes the keys you chose, and rewrites only those. A key whose text no longer matches is skipped, and the response names it.
- **TP4.** A system map's summary is listed without a key: the owner writes it.

## 6. The API

Every endpoint needs the UI cookie.

| Endpoint | Body | Response |
|---|---|---|
| `GET /api/ui/topology` | | `file`, `exists`, `revision`, `text`, `model`, `findings`, `mode`, `commits` (whether a save makes a commit) |
| `POST /api/ui/topology/lint` | `text` | `findings` |
| `POST /api/ui/topology/plan` | `revision`, `operations` | `text`, `diff`, `findings`, `effects`, `occurrences`, `applies_at_start` |
| `POST /api/ui/topology/apply` | `revision`, `operations`, `propagate` | `revision`, `commit`, `skipped` |
| `PUT /api/ui/topology/source` | `revision`, `text` | `revision`, `commit` |
| `POST /api/ui/topology/create` | | `revision` |
| `GET /api/ui/topology/options` | | `deliverables`, `models`, `templates` |
| `POST /api/ui/run/start` | | `204`, or the findings that stop the start |
| `POST /api/ui/run/stop` | | `204` |

- **TA1.** `revision` is the SHA-256 of the file's bytes, in hex, or `""` for a missing file.
- **TA2.** A write whose `revision` is not the file's fails with `409`, the file's revision, and its text.
- **TA3.** `diff` lists the lines of a line diff of the old and new text, each with `op` (` `, `+`, `-`) and its line numbers.
- **TA4.** `effects` lists, in words, what a plan does outside the file: the store directories it leaves, and the clone that `hikari sync` needs.
- **TA5.** `PUT source` accepts a text that fails its checks, and returns its findings with the new revision. A text with a YAML syntax error is accepted too: it is your file.
- **TA6.** `create` writes, in the directory of `file`, a new project repository when the directory is empty (the files of `hikari init`), and else `compose.yaml` from the same starter without `x-hikari.project`. It fails when the file exists.
- **TA7.** `options` lists the store's deliverables when the store exists, the models `sonnet`, `opus`, and `haiku`, and the agents that a new agent can copy.
- **TA8.** The server watches the file, and sends a `topology` event with the new revision within a second of a change on disk.
- **TA9.** A failed operation, a refusal, and a bad body answer `400` with `error`; a missing entity answers `404`.

## 7. Commits

- **TC1.** When the topology sets `x-hikari.project` and the file's directory is the top of a git repository, `apply` and `PUT source` commit the compose file, and every wiki and vocabulary file that propagation changed, with your git identity, on the checked-out branch.
- **TC2.** The message is `user: ` and a summary of the operations, such as `user: rename hub eng to platform; add dev to #eng`, or `user: edit compose.yaml` for `PUT source`.
- **TC3.** Without a git identity, or outside a project, the save writes the files and commits nothing, and the response says why.
- **TC4.** During a run, the systems manager makes the commit (decision 8).

## 8. `hikari up`

```
hikari up [-listen addr] [-no-launch] [compose.yaml]
```

- **UP1.** Without an argument, `up` opens `compose.yaml` in the current directory.
- **UP2.** `up` listens and prints the web UI address first. Then it runs the start sequence of today. When the file is missing, fails to load, or a start check fails, `up` prints the reason and `Editing <file>: open the web UI`, and waits in the editing state.
- **UP3.** In the editing state, the server serves sign-in, the topology API, the run endpoints, and an event stream whose snapshot has `mode: editing`. The other UI endpoints answer `503` with "hikari is not running agents".
- **UP4.** `POST /api/ui/run/start` lints the file first, and refuses with the findings when one is an error. Otherwise it runs the start sequence. A failure returns to the editing state, and the start's answer is the findings.
- **UP5.** `up` never starts by itself after a save.
- **UP6.** `POST /api/ui/run/stop` does what Ctrl-C does today without exiting: it stops the containers, commits what the owners left, and returns to the editing state.
- **UP7.** Each start is a new run, with its own folder in `.hikari/runs/`. The UI's messages start empty.
- **UP8.** The address and its token stay valid for the life of the process.
- **UP9.** A change of mode closes every event stream (decision 7). The snapshot of a running mode carries `mode: running`, and every snapshot carries the file and its revision.
- **UP10.** Ctrl-C stops a run when there is one, and exits.
- **UP11.** With `-no-launch`, `up` behaves as today.

## 9. Out of Scope

- The YAML view (step 3) and the Structure view (step 4).
- Restart in one step. Stop, then Start.
- Editing the store's deliverables, skills, and policies.

## 10. Tests

1. **Findings.** The path of each topology check that has one; path to line, through a merge key; a YAML syntax error's line; `hikari check`'s line prefix.
2. **Edits.** Each primitive on block and flow nodes, with the byte rule of TE1 checked by a diff that touches only the node's lines; each operation of section 4 on the example files, with its refusals; a random sequence of operations on each example that still parses.
3. **Propagation.** Matches and non-matches of TP1; apply with chosen keys; a stale key.
4. **API.** Each endpoint with `httptest`: its answer, the file's bytes, the commit, the conflict, the cookie, and `503` in the editing state. The responses are written to `web/src/lib/fixtures/topology/` and a test fails when a fixture is out of date (A5).
5. **`up`.** The editing state with a missing file and with a broken file; create; start refused by lint; start and stop with `-no-launch`, which starts no container.

## 11. Build Plan

1. **Findings.** TF1 to TF5, with tests.
2. **Edits.** `internal/topoedit`: the primitives, the source model, the operations, and propagation, with tests.
3. **API.** Lint, the endpoints, commits, the file watch, and the fixtures, with tests.
4. **`hikari up`.** The editing state, create, Start, and Stop, with tests.
5. **Live check and docs.** Start `hikari up` in an empty directory, create the project from the API, add an owner of a demo repo and a hub by operations, start, stop, rename the hub with propagation, and start again. Then update the README, `docs/design.md`, and this spec's status.
