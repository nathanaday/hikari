# Design notes

This document gives the reasons for the main decisions in hikari.

## Hikari's principles

Two principles shape every topology, and the hikify skill checks a design
against both.

Each system owns one concern. hikari isolates a library from its
consumers, and a client from its server: each agent owns one concern
behind clean interfaces, in a container that sees only its own code, and
the orchestrator links the systems. An agent that owns one concern holds
all of it in context and cannot break code it cannot see. Code copied
between two systems drifts, and the drift is a bug that no unit test
finds, so a library serves both instead.

Each system tests only its own code, with the unit tests of its language.
A test that needs a display, a browser, the whole application, or another
system is an E2E test, and it belongs to the hub's tester, which runs it
against releases. Inside one system, such a test depends on code the
system does not own, so its failure names the wrong owner, and its tools
weigh down the system's container.

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
code, and `tests.yaml`, a test map that says which features each test
touches.

Only the owner can describe its system accurately, so it writes all three
documents. It changes the map through tools, not by editing the file: the
server applies each change to a copy, checks it, and writes the file with an
atomic rename. A typo cannot corrupt the public surface, and the server knows
at once when it changed. The private documents are the owner's working
notes, so the owner edits them with its normal file tools, and the container
mounts them read-write while the map stays read-only. The server reads them
when it computes health, and it refuses to record a test run or to mark
the map current while they are invalid, so that they stay true. A skill in
the container, not a file in your repo, teaches the owner their formats.

All three documents live in the scope, so they are versioned and reviewed
with the code that they describe.

## The feature wiki

Each feature is one markdown document in the store, beside the map, and the
map's feature list is written from those documents. A map entry of a few
fields had no room for the reasons behind a feature, and nothing said when
one feature's description was last true. A document has a header that the
server checks and a body for people and agents: why the feature exists, what
it promises, and what it refuses. The body cites no code, so the document is
the same for every reader; nothing in it is hidden from one reader and shown
to another. The code paths of a feature stay in the owner's code map,
`layout.yaml`, which only the owner needs.

The documents live in the store, not in the repo, because they are the public
surface: orchestrators already mount each system's store directory, a release
copies them with the map, and the store opens as one Obsidian vault of every
system. Each document records the commit it describes, so staleness needs no
git history.

Links are the only structure. A document links the features it works with as
`[[id]]`, anywhere in its text. A link has no direction, so the owner writes
it once and the server reads the graph both ways. A link has no kind: a
labeled relation is a judgment that goes stale, and nothing would read it.
There are no feature groups, because how broad a group should be is always an
opinion; the clusters of linked features show the groups by themselves.

The owner changes the wiki through one tool that applies a set of writes as a
whole, so that the files always have the schema and features in one change
can link each other. A rename rewrites every link, which an agent editing
files by hand would get wrong. The owner reads the files directly, and a
user edits a document's `## Hub and system wikis

The feature wiki became one of many wikis: every system and every hub has one,
and a document has a kind. A system's features, principles, and rules, and a
hub's stories, rules, and principles, share one format and one change model,
because the reasons to check a header, to keep links as the only structure,
and to apply a change as a whole are the same for every kind.

Ids are unique across the store, which the feature wiki already required
across systems. So `[[reserve-stock]]` names one document in any wiki, in
hikari and in Obsidian, and a story links the features of its systems with
no other syntax. A link names a document by its last path segment, because
Obsidian writes a path when two files share a name, as a release copy does.

The user's plans and notes live beside the agents' documents, so that the
wiki of a system or a hub is also where the user keeps what they intend for
it. Agents read them and never change them. A file with no kind is a note,
so that a page dropped into the folder in Obsidian is valid as it is. A
hand edit is checked file by file: a broken document keeps its last valid
version, and a broken link is a warning, because a wiki that refuses a note
is a worse place to write than one that warns. A tool write checks links
and uniqueness only for the documents it writes, so a warning that a hand
edit left never blocks an agent.

A change reads the ids of every wiki and can rewrite the links of all of
them, so hikari applies one wiki change at a time. Every tool change is
committed when it is made, so an uncommitted change in a wiki is always the
user's, and hikari commits it with the user's identity: before an agent's
next change of that wiki, at stop, and at start. Configuration in the shared
area stays the user's to commit, because it changes how agents behave.

An owner reads the wikis of its hubs, because their rules and principles
apply to it, and an orchestrator reads the wikis of its child hubs, which
speak for their systems as the child orchestrator does. Before each turn,
an agent learns which documents of those wikis someone else changed.

## Notes` section in Obsidian; the server reloads
hand edits and keeps the last valid wiki when one breaks a rule. Findings,
plans, and test health are not in the documents: they will be entities of
their own that link features, so that each relation is computed instead of
kept fresh by hand.

Staleness is per feature. The server knows each feature's code paths from the
code map, so a change in the code names the features it touches, and a
changed path under no feature is how a missing feature gets found.

A map records the commit that it describes; when files in the scope change
after that commit, the map is `stale`, and the driver lists the changed paths
in the owner's next prompt.

## Tests and health

Health is computed, never set, and no agent types a number into it. Each
suite's command makes the test runner write a JUnit XML report, the one
format that almost every runner writes, and the owner only tells the server
which suite ran. The server reads the report, keeps the last result of each
test, and deletes the report. An agent that summed results by hand could
miscount or credit a failure to the wrong feature; a runner's report cannot.
The owner still runs the suites. The server could run them in the owner's
container, which would take the agent out of the path entirely, but that
needs long blocking calls and timeouts, and it belongs with the validation
agent planted for later.

A test is one test case, as the runner names it, and it links the features it
touches with the same `[[id]]` links as the wiki. One test can touch many
features, so the links go from tests to features, and a feature's tests are
computed. The links live in one private file, not a document per test: a
library with five hundred tests would need five hundred documents with
nothing to say beyond their names. One key can cover a test, a describe
block, a file, or a directory, and the most specific key decides, so that a
large system needs a few dozen lines. The links are applied when health is
computed, not when a run is recorded, so an owner can run the suites first
and map the tests afterward.

A result counts only while the test's file and the code of every feature it
touches are unchanged since the run; the server stores their content hashes
with each run. A feature is `failing` when a current result of a touching
test failed, `pending` when a touching test has no current result, `untested`
when no test touches it, and otherwise `passing`. A failure shows first, even
while other tests wait for a run. `pending` is not `stale`: a stale document
is behind the code and `mark_map_current` clears it; pending results are
behind the code and only a run clears them.

Test names, files, and failure messages stay private. The public map shows
each feature's health and counts of its touching tests. Results change with
every run, so the server keeps them out of commits with an ignore file; they
stay on the host between runs, like the map in the store. A rename in the
wiki follows into the test map, the code map, and the recorded hashes, so
that a rename never makes results pending or leaves a private file invalid.

## A tester proves the stories

The orchestrator decides what a hub's systems must do together, in the
hub's stories, rules, and principles. Another agent, the tester, proves it,
because the agent that sets the bar should not also write the check: when
one agent writes both and a test fails, it can change either one. The tester
is an ordinary owner whose scope is the E2E scaffold, so it keeps the
boundary that every owner keeps: it never sees another system's code, and
tests only what the systems release.

Its test map links the hub's documents as an owner's links its features, so
a story's health comes from one test map, one results file, and one health
computation. The "code" of a story is the release lock: a result counts
while the test and the lock are unchanged, so a new pin makes every E2E
result pending through the hashing that already decides when a unit test
result counts. The tester pins with a tool, so that it never types a digest.

The tester's plan lives in its scope, because it describes the scaffold, and
hikari serves it to the orchestrator and the user, who cannot read the
scope. A plan names what it cannot test. A gap counts only when the user
approves it, and an approval holds a digest of the gap's text, so the
tester cannot close a hard story by calling it a gap, nor change a gap after
the user approved it.

## Bench results

Not every story can be checked by a machine: a model on an NPU, or a relay
on a real output, needs hardware that the test system does not have. A gap
says so; a bench procedure does better when the user has the hardware. The
tester writes the steps, the user runs them and records the result, and the
result counts like a test result: the same digest of the lock that makes a
test result pending makes a bench result pending, and so does a change of
the procedure, because a result of other steps says nothing of these.

## Environments run on the host

A tester's container cannot start containers: the Docker socket would give
it the host. So the tester describes its services in a compose file, and
hikari runs it, as hikari runs the topology: it reads the file, refuses what
would cross a boundary, writes an override with the network and the mounts,
and starts the project. The tester's container then joins the environment's
network, so the tests reach each service by name and nothing else.

A release is either mounted into a container or is an image; hikari knows no
other kinds of product. An owner cannot run Docker either, so hikari builds
an image release on the host, from the commit that the map describes, and
the build context never enters the release. The image stays in the host's
Docker: an archive of every version in the store would cost more than a
rebuild from the commit.

`start_environment` waits until the services are healthy, instead of
returning at once and waking the tester later: hikari has no sender for such
a message, and a start takes seconds to minutes. No variable of the host
reaches an environment, so an environment never holds a credential; a test
against a real cloud service is planted.

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
stay in the repo and change in the same commits as the code. Releases are
versioned by their immutable version directories, and they can hold large
binaries, which git handles badly, so git never holds them.

## The project repository

A store outside git loses its history: no one can say who changed a
feature or when. So a project can be a git repository of its own, which
holds the topology and the store, and hikari commits each change that an
agent makes through a tool, with the agent as author. The knowledge is
versioned; products and results are not.

A project opts in with `x-hikari.project`. hikari could find the
repository that holds the compose file by itself, but the examples sit
inside the hikari repo, and hikari would then check out a branch of its
own repo. An explicit key costs one line.

The project repository follows the branch rules of every repo: clean at
start, and agent commits only on a branch that you merge. The checkout of
that branch can change the compose file, so hikari reads it again after
the checkout, and the branch's file decides.

A test run changes the health in a map, but it makes no commit. A commit
for each run would bury the changes of the wiki in noise, so the health
goes into the system's next commit. For the same reason, a time in a
document keeps whole seconds: a finer time in memory would rewrite the map
on each start.

In a project repository, a compose service may bind mount a config
directory next to the compose file. The guard covers only what hikari
mounts itself: the store, `.git`, and the scopes of the systems in it.

## Linked repositories make a project portable

A repo named by a local path means nothing on a teammate's machine. So a
repo can be named by its remote URL, and its clone always sits in
`repos/<name>` in the project repository, which git ignores. One layout
for every clone means one checkout procedure for every teammate, and no
nested git history.

`hikari sync` does the network work, and `hikari up` does none: `up`
refuses a missing clone instead of cloning, so a start never surprises
you. `sync` clones, fetches, and makes a local branch only where none
exists. It never merges, rebases, or resets, because those decisions are
yours. `hikari push` never forces, and it pushes the project repository
last: a map records the commit that it describes, and a teammate must
never get a map whose commit is not on the remote.

Each person's agents commit to their own branch. The branch comes from
compose interpolation, `${HIKARI_BRANCH:-hikari}`, and `.env` sets it for
each machine. hikari reads no `.env` of its own; compose already reads it.

A fresh clone lacks what git does not version: test results and releases.
A missing results file is no evidence, so the map keeps the health that
the teammate's machine recorded, instead of dropping every feature to
pending and committing the drop. The owner's first turn runs its suites
again, and publishes again each release that a tester pins. Then the
owner tells the tester, which pins again. A rebuild can change a
release's digest, so a pin is never carried across machines silently.

## Onboarding runs in your own session

Turning repositories into a hikari project starts before any topology
exists, and every hikari agent needs one. It also needs a view across all
repositories, which no hikari agent has by design. So the onboarding
skills run in your own Claude Code session on the host, where you decide
as the work goes. Hikari's own agents make the code changes later, each in
its own scope.

The binary carries these host skills and writes them into the project's
`.claude/skills/`. A skill versioned in the project would drift from the
binary that checks the topology it writes. Each skill's folder holds a
`.gitignore` of `*`, so git ignores it without a line in the project's own
ignore file, and a new host skill never breaks an older project.

Containerize changes no code. It wraps each repository as one system, and
the owners' first turns map the code and record the tests. That gives the
refactor of hikify three things it would otherwise rebuild: every
repository runs in hikari, every test has a recorded outcome, and each
repository has a map written by the agent that owns it. A suite that
cannot run in a container is a finding, not a change: hikify moves it to
the tester. The one exception is an ignore line for build products, with
your approval. Without it, every machine commits the products again.

`hikari status` reads only files, so the session can watch the owners'
progress without a server API, and the baseline is the same text that a
person reads.

Hikify designs before it changes anything. The design stays in
`onboarding/` and `compose.target.yaml` until you approve it, because the
target's new hubs and systems have no wiki in the store yet. A difference
in behavior between two copies of code is always your question: the skill
never picks one. Fresh reviewers check the plan in rounds, and a reviewer
told to find faults always finds a minor one, so a round of minor findings
ends the review, and four rounds of serious ones stop for your judgment.

The restructure commits go on each repository's hikari branch, as yours.
That branch is the one the agents work on and the one you review and
merge, so a separate branch would only delay the agents. A new repository
reaches `repos/` only by a clone from its remote, so that every clone,
yours or a teammate's, has the same origin.

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
release: the product and the docs that someone who cannot see the code
needs to use it. Publishing checks that the map is current, so that a
release matches what the map describes, and records the source commit and a
copy of the public map. Feature health does not block a release; the copy
of the map shows it.

## Deliverables put the rules of a release in the store

Every system delivers something different: a Python package, a container
image, a set of documents. The rules of a release therefore do not belong
in hikari. They live in the store as a deliverable, a named package of
`deliverable.yaml` and `SKILL.md`, which you can read, change, and bind to
owners in the topology. The layout rules are three glob lists, so that you
can be as loose as a list of file types or as strict as an exact map of
files. The skill tells the owner how to make the product, and hikari sets
none of its frontmatter, so its author decides whether it forks its
context.

Deliverables live in the shared area, which every container already
mounts, so an orchestrator can check a release against its deliverable and
a change applies from the next turn. The grant, not the mount, decides who
publishes a deliverable: only a holder gets its skill and can call the
publish tools, and only an owner can hold one, because an owner has code
to build from, a map and a commit for provenance, and a place in the store.

hikari writes the whole manifest. The owner writes nothing in it, so the
manifest is a record that hikari vouches for: who published, when, from
which commit, with which hikari version, and the digest of every file. A
release keeps its product as a directory, not an archive, so that
consumers and the UI read it with normal tools; the read-only mounts make
it immutable and `SHA256SUMS` makes it checkable.

A target is a promise that the product also goes to a cloud service. The
owner uploads, with the credentials that compose gives its container, and
gives a certificate in the publish call. hikari checks the certificate's
shape against the promise and records it; it never uploads and does not
call the service, which would need credentials and network access on the
server.

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
and its `hikari-` agents always come from the binary, so that they match the
binary's version; a shared agent cannot take their names.

The skill of a deliverable can run in its own context (`context: fork`), so
that building a product does not fill the owner's context; it must then
wait for the result (`background: false`), because a headless turn ends
when its prompt is answered.

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

## The live view shows steps, not a transcript

Each container's driver turns Claude Code's stream into activity entries: a
turn started, thinking, text, a tool call, the turn ended. It posts each
entry as its step starts and again as it changes, with the same ID, and the
server replaces the kept entry and publishes it. So the UI shows a step the
moment it starts, with its result when it ends, without the server ever
reading a transcript. Streamed text is sent at most every 250 ms, so a
narrating agent does not flood the server. The server keeps the last few
hundred entries of each agent and drops old turns whole, so a new UI session
sees recent turns and not a growing log; the run's log file keeps all of it.

In headless mode Claude Code has no way to ask a question: `AskUserQuestion`
is not available, and the text that ends a turn goes to nobody. hikari does
not forward that text as a message, because most of it is a status line and
forwarding it would flood the hubs. Instead the system prompt says that the
closing text reaches no one and that questions go by message, and the UI
shows the closing text in the turn's card, marked when the agent sent no
message during the turn, and highlighted when it asks something.

## The terminal is for people; the log is for the record

`hikari up` sends everything that compose and the containers print, such as
each tool call of each agent, to the run's log file. The terminal shows the
start-up steps, one line for each message, and a footer pinned to the bottom
with the web UI address, each agent's state, and the cost so far. The footer
is redrawn in place with plain ANSI codes, so hikari needs no full-screen
TUI: the terminal keeps its scrollback, and output to a pipe stays plain
text. Each run keeps its records in one folder, `.hikari/runs/<run-id>/`, and
writes `messages.jsonl` only when the run has a message.

## The map draws its own canvas

The Map view is a force graph in the style of Obsidian's graph. `d3-force`
lays it out and `d3-zoom` pans and zooms, but the view draws every frame on a
2D canvas itself. The reason is the move from the topology into a system: the
camera, the fading nodes, the features that spread out of the system, and
the lens colors all change under one clock. A complete graph library owns its
draw loop and its camera, and a transition between two of its graphs fights
it. The graphs are small, tens of nodes and at most a few hundred, so a 2D
canvas is fast enough and WebGL would add weight for nothing.

Both maps live on one canvas, so no frame is ever empty. The engine knows
nodes and edges, not systems or features; the view tells it how two graphs
relate, and it opens one out of a node of the other or folds it back. The
simulation stops when the layout settles, so an idle map costs nothing.

A lens is a function from what a node stands for to how it is drawn. The
graphs and the lenses are plain functions with unit tests, and the canvas
only draws what they return. Each lens answers one question with the fewest
colors that answer it, and a node a lens says nothing about stays neutral,
so color always means something. The status dots elsewhere in the UI use the
activity colors, so that one color means one thing everywhere.

A feature's age is the time its document was created, which the server
records. Its last edit would make a reworded feature look new. A document
from before the record existed has no known age and shows as the oldest.

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
- No agent sees another system's code. A system tests against another
  system's published releases when the topology grants them, and a hub's
  tester runs E2E tests against the releases of the hub's systems.
- Teammates' branches are merged by people with git. hikari pushes and
  syncs, but it never merges.
- Release grants change only when a container starts, and there are no
  schema grants yet: a system that owns a contract publishes it in its
  releases.
- Files that an owner creates in its scope belong to the container user. On
  Docker Desktop for macOS this is your user; on Linux, it is the user ID of
  the agent's image, 1000 in `hikari-agent`.
- A sidecar serves one agent. Agents cannot share a sidecar yet.
