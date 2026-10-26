# Topology editor: round 1

Source: your message of 2026-10-25.

## What I take from your proposal

- **You manage the topology from the UI.** Every part of it: the project's name, repos and their remote URLs, hubs, agents and their systems, renames of any of them.
- **The editing is well tested and clear.** Each endpoint has unit tests, and the UI says what each change does before it does it.
- **The editor looks like the rest of the UI.** It uses the same sidebar, panels, type, and colors as the system and hub views.
- **You can see and edit the YAML itself.** A YAML view shows the compose file, lints it, and saves it. Seeing the file teaches where the topology lives and what it looks like, so the form never hides the file.
- **Tasks:** the transport architecture, a map of every topology feature, endpoint tests, the frontend design and wiring, and frontend validation.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Decided

- **The compose file stays the single source of truth.** The editor writes the file. It keeps no second copy of the topology, in the store or anywhere else.
- **The server stays the authority on validity.** The UI checks names and references as you type, but only the server's checks decide whether a file is valid, as `hikari check` does today.

## Terms

- **Structure view**: the form editor of the topology: project, repos, hubs, agents.
- **YAML view**: the compose file as text, with lint findings at their lines.
- **Operation**: one named edit of the topology, such as `rename_agent` or `add_member`.
- **Plan**: what a list of operations would do: the new file text, its diff, its findings, and its effects outside the file.
- **Revision**: the SHA-256 of the compose file's bytes. Every write names the revision it started from.
- **Finding**: one lint result, with a severity, a message, a path in the YAML such as `x-hikari.hubs.eng.members[1]`, and a line.
- **Running topology**: the topology that `hikari up` loaded at start. The containers run it until the next start.
- **Store effect**: a change that an operation makes outside the compose file, such as moving `store/hubs/eng` to `store/hubs/platform`.

## What exists today

- The UI talks to the server over HTTP with JSON. It reads with `GET`, writes with `PUT`, `POST`, and `DELETE` under `/api/ui/`, and receives every change through one server-sent event stream, `/api/ui/events`, which starts with a snapshot (`internal/server/http.go`). A cookie from the printed token guards every UI endpoint.
- Writes from the UI are small and targeted: a wiki document, a key story, a gap approval, a bench result. Agents change their maps and wikis through operation-style endpoints: `POST /api/node/wiki` applies a list of writes as a whole, and `POST /api/node/vocabulary/{hub}` takes an `op`.
- The Go types in `internal/api` and the TypeScript types in `web/src/lib/types.ts` are written by hand. `web/src/lib/rules.ts` repeats the server's wake rules, and its tests repeat the Go test cases.
- The server exists only while `hikari up` runs. `up` refuses an invalid topology, so a broken compose file never reaches the UI.
- `topology.Load` runs `docker compose config`, then parses and validates the resolved output. Its errors are joined strings with no position. `hikari check` adds the start checks of `linked.Check`: clean repos, missing clones, ignore rules.
- The containers read the topology once, at start: mounts, networks, and the system prompt. The driver keeps the Claude session in memory, so a restart starts every agent fresh.
- Names are keys outside the file. A system's store directory is `store/systems/<owner>`, a hub's is `store/hubs/<hub>`, and a vocabulary is `store/shared/vocabulary/<hub>.yaml` with the hub's name inside it. A release manifest and a tester's `releases.lock` record the system's name. The compose project name prefixes every container, network, and named volume.
- In a project repository, `hikari up` refuses uncommitted changes outside the wikis. An edited compose file that nobody committed blocks the next start.
- `internal/sysmap/rewrite.go` already edits YAML files through `yaml.v3` nodes.

I tried the `yaml.v3` round trip on the four example compose files: decode to a node tree, encode with an indent of two. None came back identical. The encoder dropped every blank line, wrote `<<: *agent` as `!!merge <<: *agent`, and moved aligned line comments next to their values. This matters for C1.

---

## A. Transport

**A1. Keep HTTP with JSON for requests and the event stream for changes.** I compared six options for a local UI that one person uses, served by the binary.

- **REST over HTTP with JSON, and the existing event stream** (recommended). It is what the UI and the node API already use. The Go standard library serves it, `httptest` tests it with no network, and `curl` debugs it. Its weakness is that the Go and TypeScript types are written twice. A2 and A5 answer the other weakness: a rename touches many resources, which plain CRUD models badly.
- **JSON-RPC over HTTP.** One endpoint with method names. It fits operations well, but it gives up status codes, the router, and the conventions of every other endpoint, for no gain that operations on REST paths (A2) do not give.
- **gRPC, or Connect with protobuf.** Typed clients for Go and TypeScript from one schema, and streaming. The browser cannot speak gRPC without a proxy, so it would be Connect or gRPC-Web. Either one adds `buf`, a code generator, and a generated client to the build, and a second style of API beside the existing one. The typing is the only gain, and A5 gets most of it with no new tools.
- **GraphQL.** Flexible queries for many clients. hikari has one client, and the topology is one small document that the UI reads whole.
- **A WebSocket for both directions.** It would replace the event stream and the `POST` endpoints. Two people editing at once would justify it, but hikari has one user, and the event stream already pushes every change. It adds reconnection and message framing for nothing.
- **The file itself, edited in the browser.** The UI would parse and change the YAML with the `yaml` npm package, which keeps comments and blank lines well, and send whole files. One endpoint would suffice. But renames move store directories, which only the server can do, and the edit logic would then live in TypeScript, away from the Go checks and the Go tests.

Do you agree?

**A2. The topology API takes operations, not resources.** A rename of a hub changes the hub's key, its tester's `tests:` key, and its store directory. A CRUD API (`PATCH /hubs/eng {name: platform}`) hides that inside a field update. An operation names the intent, and the server applies a list of them as a whole, as `change_wiki` does:

```
GET  /api/ui/topology               the source text, the model, the findings, the revision, and whether a run is live
POST /api/ui/topology/plan          {revision, operations} → the plan; writes nothing
POST /api/ui/topology/apply         {revision, operations} → writes the file and the store effects; returns the new revision
POST /api/ui/topology/lint          {text} → the findings of a text; writes nothing
PUT  /api/ui/topology/source        {revision, text} → writes the text as typed, after lint
GET  /api/ui/topology/options       the deliverables of the store, the models, and the repos' directories, for pickers
```

Plan and apply run the same code, so the plan you saw is what apply does. A list of operations applies as a whole or not at all: a rename of an agent and its new description save together. Do you agree?

**A3. The server applies the operations, in Go.** One package, `internal/topoedit`, turns operations into text edits and store effects, and runs the checks of `hikari check` on the result. The UI sends operations and shows plans; it never edits YAML text itself, except in the YAML view, where you type. The edit logic then has one implementation, beside the checks, with Go tests. Do you agree?

**A4. Every write names the revision it started from.** Others edit the compose file too: you in an editor, the hikify skill in a host session, `git checkout`. A write whose revision does not match the file fails with `409 Conflict` and the current text, and the UI offers to reload or to apply your operations again to the new text. The server watches the file and sends a `topology` event with the new revision when it changes on disk, so an open editor learns of a change at once. Do you agree?

**A5. The wire types stay hand-written, with shared fixtures.** Generating TypeScript from Go would need a generator in the build, for one package of types. Instead, the Go tests write each response of the topology API into `web/src/lib/fixtures/topology/*.json`, and a Go test fails when a fixture is out of date. The TypeScript tests read the same fixtures, so a field that changes in Go breaks the UI's tests. Do you agree?

## B. When an Edit Takes Effect

**B1. An edit changes the file; it reaches the agents at the next start.** A container cannot get a new mount while it runs, and each agent reads its prompt once. So the editor never changes a running agent. While a run is live, the Structure view shows the changes between the running topology and the file, as a list such as "renamed hub eng to platform; added dev to #platform", with "Applies at the next start". Do you agree?

**B2. A new command, `hikari edit <compose.yaml>`, serves the editor with no containers.** Today the UI exists only during a run, and a run refuses an invalid file, so the editor could never open a broken file to fix it, nor shape a topology before its first start. `hikari edit` starts the server and the web UI with only the Topology view and the store views, and prints the URL as `up` does. It loads a file that fails its checks, and shows the findings. Do you agree?

**B3. Renames that move store state are refused during a run.** Containers mount `store/systems/<owner>` and `store/hubs/<hub>`, so a move under a running container breaks its mounts. During a run, the editor allows every operation that only changes the file, and it disables renames and removals of owners and hubs with "Stop hikari, or use `hikari edit`". The alternative is to record the rename and move the store at the next start. That keeps a queue of pending moves outside the file, which breaks the decision that the file is the only truth. Do you agree?

**B4. In a project repository, each save is a commit as you.** An uncommitted compose file blocks the next `hikari up` (rule PR8 of the project repository spec), and a teammate learns of the change only from a commit. So `apply` and `PUT source` commit the compose file and the store effects on the project branch, with your git identity and a message from the operations, such as `user: rename hub eng to platform`. The topology's history is then `git log compose.yaml`. The design notes say that configuration in the shared area stays yours to commit, because it changes how agents behave; a save in the editor is your explicit act, so it fits that reason. Without a project repository, hikari writes the file and commits nothing. Do you agree?

**B5. Restart from the UI is planted, not built.** A "Restart with these changes" button would stop the containers, commit what the owners left, move the store, load the file again, and start. The server's state (the log, the inboxes, the systems manager) is built from one topology today, so a restart in place is its own piece of work. Do you agree to plant it?

## C. Writing the File

**C1. An operation changes only the bytes of the YAML nodes it touches.** The round trip above showed that `yaml.v3` cannot write a whole file back unchanged. So `topoedit` parses the source with `yaml.v3` for its node positions, finds the smallest node that an operation changes, and splices new text into that span only: a rename replaces one scalar, a new member is one more item in a flow list, a new agent is a block inserted after the last agent with its siblings' indentation. Comments, blank lines, anchors, and key order outside the span stay byte for byte. Every operation's test checks this: the diff of the old and new text touches only the lines of the changed nodes. Do you agree?

**C2. The Structure view edits `x-hikari` and the identity of each service; the YAML view edits everything.** Compose has hundreds of keys, and hikari's checks of them are guards, not a schema. So the Structure view edits the name, every `x-hikari` key, the service names, and the sidecars' `serves`. It shows each service's image or build, and its other compose keys, read-only, with "Edit in YAML" that opens the YAML view at that service. A new agent copies the compose keys of an agent you choose, such as its `<<: *agent` line, so it starts with an image and credentials. Do you agree?

**C3. The editor shows and writes the source, never the resolved file.** `docker compose config` substitutes `.env`, so its output can hold your token, and it expands anchors and `${HIKARI_BRANCH:-hikari}`. The editor never sends the resolved text to the browser. A field whose source is an expression shows the expression, with the value it resolves to beside it in muted text, and a form edit of that field replaces the expression only when you confirm it. Do you agree?

## D. Feature Map

**D1. The operations.** This list covers every key of the topology today. Each row is one operation with its own tests.

| Area | Operation | Store effects and cascades |
|---|---|---|
| Project | `rename_project` | none in the store; warns that containers, networks, and named volumes take the new prefix, and old volumes stay behind |
| Project | `set_project_branch` | none |
| Project | `set_store` | refused while the store directory is not empty; warns |
| Project | `enable_project_repo`, `disable_project_repo` | disable is refused while a system uses `repo: project` or a repo has a url |
| Repos | `add_repo` (url or path, branch) | none; a url needs the project repo |
| Repos | `set_repo_url`, `set_repo_path`, `set_repo_branch` | none; a url change warns that `hikari sync` is needed |
| Repos | `rename_repo` | rewrites every `system.repo`; moves the clone `repos/<old>` of a linked repo |
| Repos | `remove_repo` | refused while a system uses it |
| Hubs | `add_hub` (name, orchestrator, description) | none until start, which prepares the store |
| Hubs | `rename_hub` | rewrites the tester's `tests:`; moves `store/hubs/<hub>`; renames the vocabulary file and its `hub:` field |
| Hubs | `set_hub_description`, `set_orchestrator` | none |
| Hubs | `add_member`, `remove_member` | removing the tester's membership is refused while it tests the hub |
| Hubs | `remove_hub` | refused while an agent would be left in no hub; keeps the store directory and says so |
| Agents | `add_agent` (name, template agent, description, hubs) | none |
| Agents | `rename_agent` | rewrites hub roles, other agents' `grants.releases`, and sidecars' `serves`; for an owner, moves `store/systems/<owner>` (D2) |
| Agents | `set_description`, `set_instructions`, `set_model` | none |
| Agents | `set_system` (repo, scope, read, test), `clear_system` | clear keeps the store directory and says so |
| Agents | `set_release_grants`, `set_deliverable_grants` | none |
| Agents | `set_tests` (hub), `set_e2e_limits` | none |
| Agents | `remove_agent` | removes it from every hub; refused while it orchestrates a hub, or another agent's grants or a sidecar name it |
| Sidecars | `add_sidecar` (name, serves, template), `set_serves`, `remove_sidecar` | none |
| Services | `rename_service` for a sidecar | none |

Is anything missing? The obvious candidates are the store's deliverables and shared skills, which are files in the store, not the topology; I would leave them to the Shared view.

**D2. A rename of an owner with releases is refused.** A release is immutable, and its manifest and every tester's `releases.lock` record the system's name, with a digest. A rename would either rewrite immutable files or break every pin. A rename of an owner with no releases moves its store directory and rewrites nothing else. The longer answer is an id for each system that a rename does not change, which I would plant. Do you agree?

**D3. A removal keeps the store, and says so.** Removing a hub or an owner leaves `store/hubs/<hub>` or `store/systems/<owner>` in place, and the plan names the directory. A wiki is knowledge, and the store is versioned in a project repository, so you can delete the directory in a commit of your own. Do you agree?

## E. Lint and Validation

**E1. Findings carry a path and a line.** Today the checks return joined strings. I would change `topology` to return findings with a severity, a message, and the YAML path of the key at fault, and `topoedit` maps each path to a line through the source's node tree. A path that exists only in the resolved file, such as a key from an anchor, maps to the line of the merge key. `hikari check` prints the line too, as `compose.yaml:14: hub "eng" references unknown agent "dve"`. Do you agree?

**E2. Lint runs every check of `hikari check`, in stages, and reports the first stage that fails.** First the YAML syntax, with `yaml.v3`'s line; then the shape of `x-hikari`, with unknown keys; then the topology rules; then `docker compose config` on a temporary copy beside the file, so that `.env` and relative paths resolve; then the start checks: clean repos, clones, ignore rules. The start checks are warnings in the editor, because a dirty repo is not a fault of the file. Lint is fast enough to run as you type, after a pause of half a second; the compose stage took 0.25 s on the team example here, so the YAML view shows the first stages at once and the compose findings when they arrive. Do you agree?

**E3. The UI checks fields as you type, and repeats the server's rules with shared cases.** The rules that a form can check alone repeat in `web/src/lib/topology-rules.ts`: the name pattern and the reserved `user`, unique names across agents and hubs, references to agents and repos that exist, an orchestrator that owns no system, one tester per hub, url or path but not both, branch names. As with `rules.ts`, one JSON file of cases, `internal/topology/testdata/rules.json`, drives both the Go tests and the TypeScript tests. A field shows its error under it, and the save button stays enabled: the plan shows the server's findings, which decide. Do you agree?

## F. The UI

**F1. A Topology item in the sidebar, under Shared.** It opens a view with two tabs, Structure and YAML, in the header style of the hub tabs. The view is the same in `hikari up` and `hikari edit`. In `edit`, it is the first view, and the sidebar shows only it and Shared. Do you agree?

**F2. The Structure view is a list on the left and an inspector on the right.** The list has four groups, Project, Repos, Hubs, and Agents, with the agents nested under the hubs they orchestrate, as the Map draws them. The inspector shows the selected item's fields, its references ("in #eng and #product; its releases are granted to inventory"), and its actions: rename, remove, add a member. A small hub diagram at the top of a hub's inspector shows the orchestrator and members with the existing node avatars. Do you agree, or would you rather edit on the Map, by selecting a node of the topology graph?

**F3. Every save shows its plan first.** Edits in the inspector collect as pending operations, listed at the bottom of the view with a count, "3 changes". Review opens the plan: the diff of the compose file, colored as in a code review, the store effects ("moves store/hubs/eng to store/hubs/platform"), the findings, and, during a run, "Applies at the next start". Save applies it. The diff is the point you asked for: every form edit shows the YAML it writes. Do you agree?

**F4. The YAML view uses CodeMirror 6.** It gives YAML highlighting, a lint gutter and underlines from the findings, and search, from a few small packages. Monaco would give more and weigh several times as much, and a plain textarea gives no line markers. The view shows the file's path above the text, and saves through `PUT source` with the same plan and review as F3. Do you agree?

**F5. A broken file still opens.** When the text fails a stage of E2, the Structure view shows the findings and "Fix it in YAML", and stays read-only until the file parses. The YAML view always works. Do you agree?

## G. Tests

**G1. Every endpoint has `httptest` tests for its success and its refusals.** Each test builds a temporary project: a compose file, a store, and git repos where the case needs them. For each endpoint, the tests check the response, the file's bytes afterward, the store effects, the commit (B4), the revision conflict (A4), the refusal during a run (B3), and the UI cookie. The `docker compose config` stage runs only where Docker is present, as the topology tests already do.

**G2. Every operation has table tests in `topoedit`, on the example files.** Each case is an operation, a starting file, the expected text, and the expected findings. Every case also checks the splice rule of C1: no byte outside the changed nodes changes. A last test applies a random sequence of operations to each example and checks that the file still loads.

**G3. The UI has vitest tests for the rules, the plan review, and the pending operations.** The rules run the shared cases (E3). The components render from the Go fixtures (A5): a broken file, a plan with store effects, a conflict.

Do you agree with G1 to G3?

## H. Steps

**H1. Three steps, each with its own spec, built in order.**

1. The API: findings with paths, `internal/topoedit` with every operation, the endpoints, `hikari edit`, and their tests.
2. The YAML view: CodeMirror, lint as you type, save with review.
3. The Structure view: the list, the inspector, pending operations, the plan review, and the field checks.

The YAML view comes before the Structure view because it needs only lint and save, and it gives a working editor early. Do you agree?
