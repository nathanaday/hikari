# Design notes

This document gives the reasons for the main decisions in hikari.

## Hubs, not a free graph

The topology is a set of hubs. Each hub has one orchestrator and zero or more
members, and the user is in every hub. Everyone in a hub can post to it, and
any two nodes that share a hub can send direct messages to each other.

A hub matches how a team works: one agent coordinates, the others do focused
work, and everyone can read the room. An agent that is a member of one hub and
the orchestrator of another connects the two, so a tree of hubs gives a
hierarchy in which each agent sees only its own rooms. Direct messages keep a
side conversation out of the room, but they never reach outside the hubs that
two nodes share.

## Not every post starts a turn

Every participant of a hub receives every post, but a post starts a turn at
once only for the orchestrator, for each participant that the post mentions as
`@name`, and for all members when the orchestrator posts without mentions.
Other participants read the post at the start of their next turn.

If every post started a turn for every member, one message in a room of N
agents would start N turns, and each reply would start N more. With these
rules, the orchestrator decides who works, a member wakes another member only
by name, and nothing is lost: every post still reaches every participant. The
server records in each message which participants it woke, and the UI shows
this under each post.

## Owners, maps, and orchestrators that never see code

An agent can own a system: a scope inside a git repo. The owner is the only
agent that reads or changes that code. Orchestrators own no code. They plan
from system maps, which describe what each system does, which interfaces it
provides and consumes, and how healthy its features are. A team works the
same way: a lead knows which features exist and who owns them, and asks the
owner whether a change is feasible. The owner decides how to make it.

The full spec is `docs/specs/system-maps.md`. The rest of this section gives
the reasons for its main decisions.

## The repo stays on the host

The scope is a bind mount of your repo, not a clone. You keep using the repo
as usual, and the change in the scope is the work itself; there is no second
copy to sync. The container sees only the scope, its read-only paths, and
its map directory, at the same relative paths as in the repo. It never sees
`.git`, because `git status` would show every file outside the scope as
deleted, and because git access would let an agent rewrite history or
switch branches.

## Only the server writes to git

Local commits need no credentials, so the server commits on the host:
`git add -A -- <scope>`, then `git commit -- <scope>`. A commit holds only
the owner's files, even when two owners share a repo and one has staged
files. The agent is the author and you are the committer, so `git log`
shows who wrote each change and who accepted it into the repo.

One working tree has one branch, so agents that share a repo share its
branch, and the topology declares the branch once for each repo. `hikari up`
refuses a repo with uncommitted changes, so that your unfinished work never
enters an agent commit. To keep the next start possible, hikari commits what
each owner left when a turn ends and when hikari stops.

## A public map and private documents

The map is the boundary of a container. Other agents never see the code, so
the map holds only what they need: what the system does, its interfaces, and
whether each feature exists and is tested. A file path or a test command in
the map would invite them to reason about code that they cannot see. The
owner still needs that knowledge, so it lives in two private documents in
the same directory: `layout.yaml`, which says where each feature lives in the
code, and `test.yaml`, a test plan. One test command for a whole system
limits the tests an owner can describe; the plan names suites, such as unit
and integration tests, and says for each feature what its tests cover and
what they leave out.

Only the owner can describe its system accurately, so it writes all three
documents. It changes the map through tools, not by editing the file: the
server applies each change to a copy, checks it, and writes the file with an
atomic rename. A typo cannot corrupt the public surface, and the server knows
at once when it changed. The private documents are the owner's working
notes, so the owner edits them with its normal file tools, and the container
mounts them read-write while the map stays read-only. The server reads them
when it computes health, and it refuses to record test results or to mark
the map current while they are invalid, so that they stay true. A skill in
the container, not a file in your repo, teaches the owner their formats.

All three documents live in the scope, so they are versioned and reviewed
with the code that they describe.

Owners add features in batches. A feature can name related features, and
the server checks that they exist; one feature at a time, a feature could not
name one that the owner had not added yet.

Health is computed, never set. Test results are reported on the honor
system, but the server records a hash of the feature's code and test files,
as the private documents list them, with each result. When the files change, the feature is `stale` until its
tests run again. A map records the commit that it describes; when files in
the scope change after that commit, the map is `stale`, and the driver lists
the changed paths in the owner's next prompt.

## Orchestrators see what they orchestrate

An orchestrator reads only the systems of the direct members of its hubs. In
a tree of hubs, a parent orchestrator does not see the systems of a child
hub; the child hub's orchestrator speaks for them, as a manager speaks for a
team. Workers read no other system's map. When an owner is blocked by
another system, it asks its orchestrator.

## A vocabulary for each hub

Owners cannot read each other's maps, so they cannot agree on names by
themselves. The orchestrator of each hub keeps a vocabulary of terms and
interface names. Matching interface names is how the orchestrator connects
systems: "orders consumes `Stock.reserve`, which inventory provides". The
server never rejects a map change because of the vocabulary, because a map
must describe what the code does. It flags names that the vocabulary lacks,
and consumed interfaces that no system provides, and the orchestrator decides
whether to add the name or to ask the owner to rename.

## One store for everything that is not code

Maps, vocabularies, shared rules, and releases live in one directory, the
store, so that you can open it and see your whole ecosystem. The private
documents are the exception: they describe the code at a commit, so they
stay in the repo and change in the same commits as the code. The store is
a plain directory, not a git repo. Releases are versioned by their
immutable version directories, and they can hold large binaries, which git
handles badly.

## /hikari: the same paths in every container

Every container sees its hikari documents under `/hikari`, not in its
working directory. Orchestrators have no scope, so a `.hikari` directory
inside a scope could not work for them. Docker also creates an empty
directory on the host for every mount point, so mount points inside the
scope would leave directories in your repo. The scope's own `.hikari`
directory is hidden, so the private documents appear at one path only.

An owner can write only its staging area. Its map stays read-only, so it
keeps the checks of the map tools, and its published releases stay
read-only, so they never change.

## Releases connect isolated systems

A system that another system needs, such as a library, publishes it as a
release: the deliverable and the docs that someone who cannot see the code
needs to use it. Publishing checks that the map is current, so that a
release matches what the map describes, and records the source commit and a
copy of the public map. Feature health does not block a release; the copy
of the map shows it.

Publishing moves the staging area's contents into the release one entry at
a time. It does not rename the staging directory, because the owner's
container mounts that directory: after a rename, the mount would follow the
directory into the release, and the owner could still write to it.

A container cannot get a new bind mount while it runs, so release grants
are part of the topology and take effect when the container starts. A grant
covers every version of a system, and the consumer chooses a version.

## Shared knowledge reaches agents as Claude Code expects it

Shared skills and agent definitions are linked into `~/.claude` in each
container, where Claude Code finds them. Policies are rules, so they go into
every system prompt; a skill applies only when an agent loads it. The
driver connects the shared area again before each turn, so a change applies
from the next turn without a restart. hikari's own `hikari-system` skill
always comes from the binary, so that it matches the binary's version.

The `release-docs` skill runs in its own context (`context: fork`), so that
writing docs does not fill the owner's context, and it waits for the result
(`background: false`), because a headless turn ends when its prompt is
answered.

## One server routes all messages

Every message goes through the server. The server knows the topology, checks
each message against the hubs, and records each message in the log.

The other option was to connect agents directly, with Docker networks that
follow the topology. That design spreads the topology across Docker networks,
makes the log a copy of traffic, and lets an agent with a shell reach any
service on a shared network. With one server, there is one place to check
access and one log that is always complete.

## Identity comes from a token

The server gives each container a random token at start. The server finds the
sender from the token, never from the request body. An agent with a shell can
read its token and call the server with `curl`, but it can act only as itself.
That is the same reach that its MCP tools give it.

## Containers cannot reach each other

Each agent has its own bridge network. Containers can reach the internet,
because Claude Code needs the Anthropic API, and the server on the host. They
cannot reach each other. A sidecar joins only the network of the agent it
serves. hikari gives each agent its networks with compose's `!override` tag,
so the list replaces the user's list instead of merging with it.

## The UI needs its own secret

Containers can reach the server, so the server cannot trust a request because
it arrives at the server address. The UI endpoints need a secret that the
server prints at start, as Jupyter does. Without the secret, an agent could
read the full log or send messages as the user.

## Agents run headless, one turn per batch of messages

The driver in each container waits for a message that wakes its agent. Then it
takes every pending message and runs `claude -p --resume <session>` with the
messages as the prompt. The agent keeps its context across turns because every
turn resumes the same session. Messages that arrive during a turn wait for the
next turn, or the agent reads them with `check_inbox`.

Two other options exist:

- An interactive session that calls a blocking `wait_for_message` tool in a
  loop. The model can stop the loop, and a long tool call can time out.
- Claude Code channels, which push MCP notifications into a live session.
  This feature is a research preview. It is a good candidate when it becomes
  stable, because it removes the per-turn process start.

The headless loop is predictable, and its output (stream-json) also gives
the driver tool calls and cost for each turn.

## The server runs on the host

The server must start containers. A server in a container needs the Docker
socket, which gives full control of the host. On the host, the server is a
plain process that is easy to run and to debug.

On Docker Desktop, containers reach a host service on 127.0.0.1 through
`host.docker.internal`. On Linux, `host.docker.internal` maps to the Docker
bridge address. There, start the server with `-listen 0.0.0.0:7420`, or listen
on the bridge address, so that containers can reach it.

## The binary embeds the UI

The UI is a Vite and React app in `web/`. The `hikari` binary embeds its build
output, so a run needs no Node.js and no second process. The `web/dist/.keep`
file lets the Go code compile before the first UI build; the server then
serves a page that asks for `make ui`. The Linux builds that hikari mounts
into containers embed the UI too; they never serve it, and one build is
simpler than a second variant without it.

The UI signs in by posting the token from the printed URL to the server, which
answers with a cookie. A sign-in through an API call, not a page redirect,
works the same behind the Vite dev server, which owns the page in development.

The composer shows who a draft will wake. To do this, `web/src/lib/rules.ts`
repeats the wake rules of the server. The tests of both use the same cases.

## Compose owns the environment; hikari owns the boundaries

An owner must run its tests, so its container needs the system's toolchain,
and some systems need more: secrets, a database, a GPU. The topology once grew
its own fields for this, such as Debian packages and a setup script, and each
new need would have copied another part of Docker compose. So the topology is
now a compose file, and every compose feature works.

hikari keeps the parts that must not be written by hand: the mounts of each
scope and of the store, the networks, the tokens, the entrypoint, and the
hikari binary. It writes them into an override file for each run, which
compose merges with the user's file. Before it runs anything, hikari reads the
file as compose resolves it (`docker compose config`) and refuses a file that
would cross a boundary: a bind mount of a repo or the store, the Docker
socket, host networking, privileges, or a mount over `/hikari` or over
another agent's code. So the guarantees come from checks, with one test for
each rule, instead of from hikari building every container itself.

hikari reads compose's YAML output, because the JSON output drops `x-` keys
on services. It requires a project name, because the name keeps named volumes
between runs. It mounts its own Linux binary into each container instead of
asking images to include it, so an image never holds an old hikari after an
upgrade. The image needs only Claude Code and a user that is not root.

A new named volume belongs to root, so the agent's user cannot write it. For
each agent with named volumes, the override file adds a prepare service that
runs once as root, gives the volumes to the agent's user, and exits; the agent
starts after it. Each token reaches compose as a variable that the override
file names, so no token is written to disk.

## The terminal is for people; the log is for the record

`hikari up` sends everything that compose and the containers print, such as
each tool call of each agent, to the run's log file. The terminal shows the
start-up steps, one line for each message, and a footer pinned to the bottom
with the web UI address, each agent's state, and the cost so far. The footer
is redrawn in place with plain ANSI codes, so hikari needs no full-screen
TUI: the terminal keeps its scrollback, and output to a pipe stays plain
text. Each run keeps its records in one folder, `.hikari/runs/<run-id>/`, and
writes `messages.jsonl` only when the run has a message.

## Known limitations

- Agents can still send messages to each other without end, for example by
  mentioning each other. The wake rules and the system prompt make this less
  likely, but no hard limit exists.
- If a turn fails, the messages of that turn are not given again.
- The log and the inboxes are in memory. The run's `messages.jsonl` is a record only.
  The server does not read it back at start.
- Claude Code loads MCP tools on demand, so the first turn of each agent
  spends one tool call to load the hikari tools.
- The first indexing turn of an owner reads its whole system.
- No agent sees another system's code. A system can test against another
  system's published releases when the topology grants them, but an E2E
  test agent that runs tests across systems is open work.
- Release grants change only when a container starts, and there are no
  schema grants yet: a system that owns a contract publishes it in its
  releases.
- Files that an owner creates in its scope belong to the container user. On
  Docker Desktop for macOS this is your user; on Linux, it is the user ID of
  the agent's image, 1000 in `hikari-agent`.
- A sidecar serves one agent. Agents cannot share a sidecar yet.
