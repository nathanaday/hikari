# Design notes

This document gives the reasons for the main decisions in hikari.

## Hikari's principles

Two principles shape every topology, and the hikify skill checks a design
against both.

Each partition has one concern. hikari isolates a library from its consumers,
and a client from its server: each agent has one concern behind clean
interfaces, in a container that sees only its own code, and the orchestration
agent links the partitions. An agent that has one concern holds all of it in
context and cannot break code it cannot see. Code copied between two
partitions drifts, and the drift is a bug that no unit test finds, so a
library serves both instead.

Each partition tests only its own code, with the unit tests of its language.
Inside one partition, a suite that needs a display, a browser, the whole
application, or another partition depends on code the partition does not own,
so its failure names the wrong partition agent, and its tools weigh down the
partition's container. Such a suite stays in its repository, outside every
test command, and hikify records it as left for integration. A way to run
it is planted.

## Ids and names

Every agent and repo has an id and a name. The id keys what lives outside
the topology file: vault directories, release manifests, containers,
networks, and clones. The name is what people and
agents use. A rename then changes one field, and the vault and the
releases stay as they are; a release is
immutable, so it could not follow a rename anyway.

The id is the key that the file already writes, not a generated code, so a
topology file stays readable and every file from before names existed keeps
its meaning. References in the file name ids, so that a rename changes one
line and the file never holds a stale name. hikari checks the file in ids,
so each error names the key that the file writes, and then keys the
topology by name, so that everything agents and the UI reach works in
names. hikari writes every mount, so a mount's source is keyed by id and its
target by name: an agent never sees an id.

## One room

A project is one room. The user and every agent are in it, everyone in it
receives every post, and any two of them can send direct messages to each
other. `x-hikari.orchestration` names the orchestration agent by its service
id, and every other agent has a partition. A key at the top of the file allows
one orchestration agent and makes two impossible to write. A project with no
orchestration agent is valid too.

One orchestration agent and its partition agents is the team that a project
needs today: one agent coordinates, the others do focused work, and everyone
can read the room. hikari once split a project into hubs, each with its own
orchestration agent and members, so that a tree of hubs gave a hierarchy in
which each agent saw only its own rooms. The wake rules, the direct messages,
the partitions that an orchestration agent saw, the vocabularies, and the
wikis all followed from the hub list. A tree of hubs can come back when a
project needs islands.

## Not every post starts a turn

Every participant receives every post, but a post starts a turn at once only
for the orchestration agent, for each agent that the post mentions as `@name`,
and for every partition agent when the orchestration agent posts without
mentions. With no orchestration agent, a post without mentions starts a turn
for every agent. Other agents read the post at the start of their next turn. A
direct message always starts a turn for its recipient.

If every post started a turn for every agent, one message in a room of N
agents would start N turns, and each reply would start N more. With these
rules, the orchestration agent decides who works, a partition agent wakes
another partition agent only by name, and nothing is lost: every post still
reaches every participant. The server records in each message which
participants it woke, and the UI shows this under each post.

## Partition agents, maps, and orchestration agents that never see code

An agent can have a partition: a list of folders, each a path in a git repo,
writable or read-only. The partition agent is the only agent that changes the
code of its writable folders. Orchestration agents own no code. They plan from
feature maps, which describe what each partition does, which interfaces it
provides and consumes, and how healthy its features are. A team works the same
way: an orchestration agent knows which features exist and who owns them, and
asks the partition agent whether a change is feasible. The partition agent
decides how to make it.

The full spec is `docs/specs/system-maps.md`. The rest of this section gives
the reasons for its main decisions.

## The repo stays on the host

Each folder of a partition is a bind mount of your repo, not a clone. You keep
using the repo as usual, and the change in a folder is the work itself; there
is no second copy to sync. The container sees its folders under `/workspace`,
each at `/workspace/<repo>/<path>` unless the topology gives it another place
with `at:`, and the agent works in them as in one monorepo. A boundary can
then follow a concern instead of a repo: a partition can take a service from
one repo and the protocol files that only it changes from another. The working
directory is `/workspace`, so the code map, the test map, and the test
commands name workspace paths, which hikari maps back to the folders on the
host.

A default place names the repo by its name, which the agent sees, not by its
id, which only the file and the vault use. A rename of a repo in the editor
therefore moves those places, and the plan of the rename lists each partition
whose workspace moves.

The writable folders of two partitions never overlap, and no folder of a
partition, read-only or not, lies inside another partition's writable folder,
so no agent sees the code of another partition. Read-only folders of two
partitions may overlap.

The container never sees `.git`, because `git status` would show every file
outside the folder as deleted, and because git access would let an agent
rewrite history or switch branches.

## Only the server writes to git

Local commits need no credentials, so the server commits on the host. An apply
commits the writable folders of each repo: `git add -A -- <folders>`, then
`git commit` of the staged files, one commit in each repo whose folders
changed, with one message, and nothing in a repo whose folders did not. A
commit holds only the partition agent's files, even when two partition agents
share a repo and one has staged files. The agent is the author and you are the
committer, so `git log` shows who wrote each change and who accepted it into
the repo.

One working tree has one branch, so agents that share a repo share its branch,
and the topology declares the branch once for each repo. `hikari up` refuses a
repo with uncommitted changes, so that your unfinished work never enters an
agent commit, and prepares each repo's branch once, however many folders use
it. An agent calls `apply` to name its commits; to keep the next start
possible, hikari also applies what each partition agent left when a turn ends
and when hikari stops.

## A public map and private documents

The map is the boundary of a container. Other agents never see the code, so
the map holds only what they need: what the partition does, its interfaces,
and whether each feature exists and is tested. A file path or a test command
in the map would invite them to reason about code that they cannot see. The
partition agent still needs that knowledge, so it lives in two private
documents in the same directory: `code-map.yaml`, the code map, which says
where each feature lives in the code, and `tests.yaml`, a test map that says
which features each test touches.

Only the partition agent can describe its partition accurately, so it writes
all three documents. It changes the map through tools, not by editing the
file: the server applies each change to a copy, checks it, and writes the file
with an atomic rename. A typo cannot corrupt the public surface, and the
server knows at once when it changed. The private documents are the partition
agent's working notes, so the partition agent edits them with its normal file
tools, and the container mounts them read-write while the map stays read-only.
The server reads them when it computes health, and it refuses to record a test
run or to mark the map current while they are invalid, so that they stay true.
A skill in the container, not a file in your repo, teaches the partition agent
their formats.

The private documents live in the partition's first writable folder, in
`.hikari/private/`, so they are versioned and reviewed with the code that they
describe. A partition with folders in several repos keeps them in one place,
and the first writable folder is the one that the topology names first.

## The feature wiki

Each feature is one markdown document in the vault, beside the map, and the
map's feature list is written from those documents. A map entry of a few
fields had no room for the reasons behind a feature, and nothing said when one
feature's description was last true. A document has a header that the server
checks and a body for people and agents: why the feature exists, what it
promises, and what it refuses. The body cites no code, so the document is the
same for every reader; nothing in it is hidden from one reader and shown to
another. The code paths of a feature stay in the partition agent's code map,
`code-map.yaml`, which only the partition agent needs.

The documents live in the vault, not in the repo, because they are the public
surface: the map and the documents share one directory, `features/`, in the
partition's read-only part, which the orchestration agent already mounts; a
release copies that directory whole; and the vault opens as one Obsidian vault
of every partition. Each document records the commit it describes, so
staleness needs no git history.

Links are the only structure. A document links the features it works with as
`[[id]]`, anywhere in its text. A link has no direction, so the partition
agent writes it once and the server reads the graph both ways. A link has no
kind: a labeled relation is a judgment that goes stale, and nothing would read
it. There are no feature groups, because how broad a group should be is always
an opinion; the clusters of linked features show the groups by themselves.

The partition agent changes the wiki through one tool that applies a set of
writes as a whole, so that the files always have the schema and features in
one change can link each other. A rename rewrites every link, which an agent
editing files by hand would get wrong. The partition agent reads the files
directly, and a user edits a document's `## Notes` section in Obsidian; the
server reloads hand edits and keeps the last valid wiki when one breaks a
rule. Findings, plans, and test health are not in the documents: they will be
entities of their own that link features, so that each relation is computed
instead of kept fresh by hand.

Staleness is per feature. The server knows each feature's code paths from the
code map, so a change in the code names the features it touches, and a
changed path under no feature is how a missing feature gets found.

A map records the commits that it describes, one for each repo of the writable
folders. When files in a writable folder change after its repo's commit, the
map is `stale`, and the driver lists the changed paths, as workspace paths, in
the partition agent's next prompt. A change in a read-only folder belongs to
no feature of the partition, so it makes nothing stale.

## Feature maps and the public wiki

The feature wiki became one of many wikis: every partition has one, the
orchestration agent has one, and the project has a public wiki for what every
agent keeps. A document has a kind. A partition's features, principles, and
rules, the orchestration agent's, and the public wiki's rules and principles
share one format and one change model, because the reasons to check a header,
to keep links as the only structure, and to apply a change as a whole are the
same for every kind.

Ids are unique across the vault, which the feature wiki already required
across partitions. So `[[reserve-stock]]` names one document in any wiki, in
hikari and in Obsidian, and an orchestration feature links the features of
its partitions with no other syntax. A link names a document by its last path
segment, because Obsidian writes a path when two files share a name, as a
release copy does.

The user's plans and notes live beside the agents' documents, so that each
wiki is also where the user keeps what they intend for its partition or for
the project. Agents read them and never change them. A file with no kind is a
note, so that a page dropped into the folder in Obsidian is valid as it is. A
hand edit is checked file by file: a broken document keeps its last valid
version, and a broken link is a warning, because a wiki that refuses a note is
a worse place to write than one that warns. A tool write checks links and
uniqueness only for the documents it writes, so a warning that a hand edit
left never blocks an agent.

A change reads the ids of every wiki and can rewrite the links of all of
them, so hikari applies one wiki change at a time. Every tool change is
committed when it is made, so an uncommitted change in a wiki is always the
user's, and hikari commits it with the user's identity: before an agent's
next change of that wiki, at stop, and at start. Configuration in the public
area stays the user's to commit, because it changes how agents behave.

Every agent reads the public wiki, because its rules and principles apply to
every agent, and only the orchestration agent writes it. It lives in
`vault/public/wiki/`, in the public area that every container already mounts
read-only, so it needs no mount of its own. The orchestration agent also reads
every feature map. Before each turn, an agent learns which documents of the
wikis it reads someone else changed.

## The orchestration feature map

The orchestration agent has a feature map of its own, in
`vault/orchestration/read-only/features/`. Its features are flows across
partitions, and each links the partition features that carry it out. The
public wiki's stories did this before. They became features because a flow
across partitions is something the project does, as a partition's feature is
something the partition does, and one tool, `change_wiki`, for "my feature
map" keeps the two kinds of agent alike. Rules and principles for every agent
stay in the public wiki, because only the orchestration agent reads its own
map.

An orchestration feature has no code and no test of its own. Its health is
computed from the features it links: failing when one of them is failing,
pending when one is pending, untested when none is tested, and otherwise
passing. A flow across partitions can be tested only with every partition at
once, which is the work of the integration agent planted for later. Until that
agent exists, the health of the parts is the best evidence that the flow
works, and computing it needs no new rule: a story of the public wiki showed
its support the same way.

The orchestration feature map can link every partition's feature map and the
public wiki, and neither of them links it, so no partition agent depends on a
document it cannot read. A link has no direction, so a partition feature that
an orchestration feature links has a backlink. `get_document` and
`search_wiki` show each reader only the backlinks from documents it can read,
so that a partition agent learns nothing of a map it cannot open.

## Tests and health

Health is computed, never set, and no agent types a number into it. Each
suite's command makes the test runner write a JUnit XML report, the one format
that almost every runner writes, and the partition agent only tells the server
which suite ran. The server reads the report, keeps the last result of each
test, and deletes the report. An agent that summed results by hand could
miscount or credit a failure to the wrong feature; a runner's report cannot.
The partition agent still runs the suites. The server could run them in the
partition agent's container, which would take the agent out of the path
entirely, but that needs long blocking calls and timeouts, and it belongs with
the validation agent planted for later.

A test is one test case, as the runner names it, and it links the features it
touches with the same `[[id]]` links as the wiki. One test can touch many
features, so the links go from tests to features, and a feature's tests are
computed. The links live in one private file, not a document per test: a
library with five hundred tests would need five hundred documents with nothing
to say beyond their names. One key can cover a test, a describe block, a file,
or a directory, and the most specific key decides, so that a large partition
needs a few dozen lines. The links are applied when health is computed, not
when a run is recorded, so a partition agent can run the suites first and map
the tests afterward.

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
stay on the host between runs, like the map in the vault. A rename in the
wiki follows into the test map, the code map, and the recorded hashes, so
that a rename never makes results pending or leaves a private file invalid.

## The orchestration agent sees every partition

The orchestration agent reads the map of every partition, because it plans
across all of them. Partition agents read no other partition's map. When a
partition agent is blocked by another partition, it asks the orchestration
agent.

## One vocabulary

Partition agents cannot read each other's maps, so they cannot agree on names
by themselves. The orchestration agent keeps the project's vocabulary of terms
and interface names in `vault/public/vocabulary/terms.yaml`, which every agent
reads. hikari reads only `terms.yaml` and ignores the other files of the
directory. Matching interface names is how the orchestration agent connects
partitions: "orders consumes `Stock.reserve`, which inventory provides". The
server never rejects a map change because of the vocabulary, because a map
must describe what the code does. It flags names that the vocabulary lacks,
and consumed interfaces that no partition provides, and the orchestration
agent decides whether to add the name or to ask the partition agent to rename.

## One vault for everything that is not code

Feature maps, the vocabulary, shared rules, and releases live in one
directory, the vault, so that you can open it and see your whole ecosystem.
The private documents are the exception: they describe the code at a commit,
so they stay in the repo and change in the same commits as the code. Releases
are versioned by their immutable version directories, and they can hold large
binaries, which git handles badly, so git never holds them.

The vault is laid out by who reads a file and who writes it. `public/` holds
what every agent reads: the vocabulary, the public wiki, the policies, and
the deliverables. Each partition has a `read-only/` part, with its feature
map, its releases, and the files you grant it in `granted/`; a `can-edit/`
part, which its partition agent can change; and a staging area. The
orchestration agent has its own `read-only/` and `can-edit/` parts in
`orchestration/`, beside `partitions/` because it has no code and no
releases. Your skills sit in `skills/`, outside `public/`, because the
topology grants each one to the agents that use it.

Policies, deliverables, and skills stay yours. A policy changes every
agent's system prompt, so an agent that could change one would rewrite its
own rules. The orchestration agent's tools write only the vocabulary and the
public wiki, and hikari writes no `granted/`. An agent's changes in its
`can-edit/` are committed with its apply, so they are reviewed like its code.

hikari moves no file from an earlier layout. It refuses to start on one and
names the move that fixes it, because a silent start would show empty maps.

## The project repository

A vault outside git loses its history: no one can say who changed a
feature or when. So a project can be a git repository of its own, which
holds the topology and the vault, and hikari commits each change that an
agent makes through a tool, with the agent as author. The knowledge is
versioned; products and results are not.

A project opts in with `x-hikari.project`. hikari could find the
repository that holds the topology file by itself, but the examples sit
inside the hikari repo, and hikari would then check out a branch of its
own repo. An explicit key costs one line.

The project repository follows the branch rules of every repo: clean at
start, and agent commits only on a branch that you merge. The checkout of
that branch can change the topology file, so hikari reads it again after
the checkout, and the branch's file decides.

A test run changes the health in a map, but it makes no commit. A commit
for each run would bury the changes of the wiki in noise, so the health
goes into the partition's next commit. For the same reason, a time in a
document keeps whole seconds: a finer time in memory would rewrite the map
on each start.

An apply also commits the agent's changes in its `can-edit/`, with the agent
as author, and the orchestration agent's changes there are committed at the
end of each of its turns. Your own changes in a `can-edit/` or a `granted/`
are committed with your identity at start and at stop, as your wiki edits
are.

In a project repository, a compose service may bind mount a config directory
next to the topology file. The guard covers only what hikari mounts itself:
the vault, `.git`, and the writable folders of the partitions in it.

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

A fresh clone lacks what git does not version: test results and releases. A
missing results file is no evidence, so the map keeps the health that the
teammate's machine recorded, instead of dropping every feature to pending and
committing the drop. The partition agent's first turn runs its suites again.

## Onboarding runs in your own session

Turning repositories into a hikari project starts before any topology
exists, and every hikari agent needs one. It also needs a view across all
repositories, which no hikari agent has by design. So the onboarding
skills run in your own Claude Code session on the host, where you decide
as the work goes. Hikari's own agents make the code changes later, each in
its own folders.

The Hikari plugin carries these host skills, and its version is the binary's.
A skill must match the binary that checks the topology it writes, and an
earlier design kept the skills in the binary for that reason, which wrote them
into each project's `.claude/skills/`. That design reached a session only
inside a project, so a request such as "take these repos and build a hikari
topology" had no skill to reach before a project existed. A plugin reaches
every session on the machine. One version for both keeps the match: the
plugin's launcher runs only the binary of the plugin's own version, and
`hikari sync` removes the copies that the earlier binary wrote.

One skill, `hikari`, is the front door. Its description matches any request
that names hikari, and it finds where you stand, from the folder, the
repositories, and `onboarding/state.md`, before it hands you to the skill of
the stage. Each stage's skill then keeps a description of its own work, so
that "continue the design" reaches hikify without the front door. Talking to
running agents is its own skill, `room`, because it is the most frequent
request after onboarding and names no stage.

Containerize changes no code. It wraps each repository as one partition, and
the partition agents' first turns map the code and record the tests. That
gives the refactor of hikify three things it would otherwise rebuild: every
repository runs in hikari, every test has a recorded outcome, and each
repository has a map written by the agent that owns it. A suite that cannot
run in a container is a finding, not a change: hikify leaves it in its
repository for integration. The one exception is an ignore line for build
products, with your approval. Without it, every machine commits the products
again.

`hikari status` reads only files, so the session can watch the partition
agents' progress without a server API, and the baseline is the same text that
a person reads.

Hikify designs before it changes anything. The design stays in
`onboarding/` and `topology.target.yaml` until you approve it, because the
target's new partitions have no wiki in the vault yet. A difference
in behavior between two copies of code is always your question: the skill
never picks one. Fresh reviewers check the plan in rounds, and a reviewer
told to find faults always finds a minor one, so a round of minor findings
ends the review, and four rounds of serious ones stop for your judgment.

The restructure commits go on each repository's hikari branch, as yours.
That branch is the one the agents work on and the one you review and
merge, so a separate branch would only delay the agents. A new repository
reaches `repos/` only by a clone from its remote, so that every clone,
yours or a teammate's, has the same origin.

## The plugin pins the binary it installs

Claude Code has no install step for a plugin, so the plugin's `bin/hikari` is
a launcher: a short shell script that runs `~/.hikari/bin/<version>/hikari`,
and on its first run downloads that binary from the GitHub release of the
version. It downloads the binary of the machine and both Linux binaries, which
hikari mounts into the containers, and installs each one only when its sha256
matches the checksum pinned in the launcher. A release builds the same bytes
on every machine (the toolchain of `go.mod`, `-trimpath`, and the Node version
of `.node-version` for the UI), so `make version` pins the checksums of a
build on your machine, and the release workflow, which runs on the version's
tag, refuses to publish a build whose bytes differ. A release starts from its
tag, not from every push to main, because most commits to main change the
binary without being a release.

The launcher needs no sudo: it writes only to `~/.hikari`. A hook never
downloads, because a hook runs at every session start with a short timeout;
the first `hikari doctor` of the front door does. Your own terminal needs one
PATH entry for `~/.hikari/bin`, which only `sudo` can write in `/etc/paths.d`,
so `hikari doctor` prints that line for you to run. A build of a checkout
installs into the same folder with `make install`, so one PATH entry serves
both, and the launcher runs your build of its own version.

The binary embeds the Dockerfile of `hikari-agent`, so a machine without a
checkout can build it, and `hikari up` builds it when it is missing. The image
keeps its name, because every topology and partition Dockerfile names it, and
carries a label with the sha256 of the Dockerfile, so a new hikari rebuilds it
only when the Dockerfile changed. The image holds no hikari binary, since
hikari mounts its own.

`hikari send` lets your session post to the room. It reads
`.hikari/host.json`, which `hikari up` writes beside the topology file with
the UI's address and token, readable only by you, and removes at stop. The
session then posts as you, through the same endpoint as the web UI, so the
server needs no second way to sign in.

## /hikari: the same paths in every container

Every container sees its hikari documents under `/hikari`, not in its
working directory. Orchestration agents have no folders, so a `.hikari`
directory inside a folder could not work for them. Docker also creates an
empty directory on the host for every mount point, so mount points inside a
folder would leave directories in your repo. The `.hikari` directory of each
writable folder is hidden, so the private documents appear at one path only.

Each container mounts the vault in a few fixed places, one directory each:

| Container path | Partition agent | Orchestration agent | Mode |
|---|---|---|---|
| `/hikari/public` | `public/` | `public/` | read-only |
| `/hikari/self` | `partitions/<id>/read-only/` | `orchestration/read-only/` | read-only |
| `/hikari/can-edit` | `partitions/<id>/can-edit/` | `orchestration/can-edit/` | writable |
| `/hikari/staging` | `partitions/<id>/staging/` | none | writable |
| `/hikari/private` | its private documents | none | writable |
| `/hikari/partitions/<name>` | none | each partition's `read-only/` | read-only |
| `/hikari/grants/<name>/releases` | each granted partition's releases | none | read-only |
| `/hikari/skills/<name>` | each granted skill | each granted skill | read-only |

The directory decides the mode. A file is read-only to an agent because it
sits in a `read-only/` or in `public/`, and writable because it sits in a
`can-edit/` or a staging area, so one mount for each directory needs no rule
for each file, and a file changes its mode by moving. No mount contains
another mount's directory, so no empty directory has to hide a staging area
inside a read-only mount, and no container mounts another partition's
`can-edit/`, staging area, or private documents. A partition agent's map
stays read-only, so it keeps the checks of the map tools, and its published
releases stay read-only, so they never change. The Structure tab shows each
agent's mounts from the function that writes the override file, so the table
cannot disagree with the container.

## Releases connect isolated partitions

A partition that another partition needs, such as a library, publishes it as a
release: the product and the docs that someone who cannot see the code
needs to use it. Publishing checks that the map is current, so that a
release matches what the map describes, and records the source commit and a
copy of the public map. Feature health does not block a release; the copy
of the map shows it.

## Deliverables put the rules of a release in the vault

Every partition delivers something different: a Python package, a container
image, a set of documents. The rules of a release therefore do not belong in
hikari. They live in the vault as a deliverable, a named package of
`deliverable.yaml` and `SKILL.md`, which you can read, change, and bind to
partition agents in the topology. The layout rules are three glob lists, so
that you can be as loose as a list of file types or as strict as an exact map
of files. The skill tells the partition agent how to make the product, and
hikari sets none of its frontmatter, so its author decides whether it forks
its context.

Deliverables live in the public area, which every container already mounts, so
an orchestration agent can check a release against its deliverable and a
change applies from the next turn. The grant, not the mount, decides who
publishes a deliverable: only a holder gets its skill and can call the publish
tools, and only a partition agent can hold one, because a partition agent has
code to build from, a map and a commit for provenance, and a place in the
vault.

hikari writes the whole manifest. The partition agent writes nothing in it, so
the manifest is a record that hikari vouches for: who published, when, from
which commit, with which hikari version, and the digest of every file. A
release keeps its product as a directory, not an archive, so that consumers
and the UI read it with normal tools; the read-only mounts make it immutable
and `SHA256SUMS` makes it checkable.

A target is a promise that the product also goes to a cloud service. The
partition agent uploads, with the credentials that compose gives its
container, and gives a certificate in the publish call. hikari checks the
certificate's shape against the promise and records it; it never uploads and
does not call the service, which would need credentials and network access on
the server.

Publishing moves the staging area's contents into the release one entry at a
time. It does not rename the staging directory, because the partition agent's
container mounts that directory: after a rename, the mount would follow the
directory into the release, and the partition agent could still write to it.

A container cannot get a new bind mount while it runs, so release grants
are part of the topology and take effect when the container starts. A grant
covers every version of a partition, and the consumer chooses a version.

## Shared knowledge reaches agents as Claude Code expects it

Your skills are linked into `~/.claude/skills` in each container that is
granted them, where Claude Code finds them, and so is the skill of each
deliverable that the agent holds. A skill is granted, not shared with every
agent: a copy in each partition's `granted/` would drift, and the grant
shows in the topology who has which skill. The grant also bounds what the
container can read, because only granted skills are mounted. Policies are
rules, so they go into every system prompt; a skill applies only when an
agent loads it. The driver connects the vault again before each turn, so a
change applies from the next turn without a restart. hikari's own skills and
its `hikari-` agents always come from the binary, so that they match the
binary's version; no skill of the vault can take their names. Shared agent
definitions are gone: no project defined one.

hikari's own skills come in packs, one for each kind of agent, with a skill
for each task: plan the work, do it, validate it, and map the features for a
partition agent; plan the work, check the health, and map the features for the
orchestration agent. A skill per task loads only what the task needs: an agent
that checks the health does not read how to publish a release, and an agent
that maps its features does not read how to carry out a plan. The prompts name
the skill of each task, such as `hikari-map-features` for the indexing turn. A
skill of the same name in two packs is two skills with one purpose, so that a
prompt names it the same way for either kind of agent, and a container gets
only its own pack.

The skill of a deliverable can run in its own context (`context: fork`), so
that building a product does not fill the partition agent's context; it must
then wait for the result (`background: false`), because a headless turn ends
when its prompt is answered.

## One server routes all messages

Every message goes through the server. The server knows the topology, checks
the sender and the recipient of each message, and records each message in
the log.

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

## The topology is edited as text, one node at a time

The UI changes the topology through operations, such as `rename` or
`set_orchestration`, and the server turns each operation into a change of the
topology file's text. A plain encoder would write the whole file again: it
drops blank lines, moves comments, and writes `<<: *agent` as
`!!merge <<: *agent`. So each operation splices only the YAML nodes that it
changes, and every other byte of your file stays. The server applies the
operations, not the UI, so that the edits, the checks, and their tests live
in one language.

A plan shows what a list of operations would do before it does it: the
diff of the file, what it leaves in the vault, and the checks of the new
text. A save names the revision it started from, because you, a host
session, or git can change the file at the same time. In a project
repository each save is a commit as you, because an uncommitted compose
file would block the next start, and because the topology's history is then
`git log topology.yaml`.

The editor reads its model from the source, without the checks, so that a
file with a broken reference opens in the form, which is where a form helps
most. Lint runs the whole load of `hikari check`, with the text on
compose's stdin: interpolation and merge keys decide the values, so no
check can run on the source alone.

The Structure tab shows the model of the plan, not a copy that the UI
changes by itself. Each edit adds an operation, the server plans the
operations, and the form draws the model of the planned text. The form and
the file then never disagree, and no rule of an operation is written in
TypeScript. The field checks that run as you type repeat the rules of
names, branches, and urls, and the tests of both languages run one file of
cases.

## hikari up serves the UI before it has a topology

A start refuses a broken file, so a UI that existed only during a run could
never fix one, nor shape a project before its first start. So `hikari up`
listens first, and waits in an editing state when the file is missing,
broken, or fails a start check. Start and Stop in the UI move between the
states. The server of a run is built from one topology, so each start is a
new run, and a change of mode closes the UI's event stream: the browser
opens it again, and its first event, a snapshot, says the mode.

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

A partition agent must run its tests, so its container needs the partition's
toolchain, and some partitions need more: secrets, a database, a GPU. The
topology once grew its own fields for this, such as Debian packages and a
setup script, and each new need would have copied another part of Docker
compose. So the topology is now a topology file, and every compose feature
works.

hikari keeps the parts that must not be written by hand: the mounts of each
folder and of the vault, the networks, the tokens, the entrypoint, and the
hikari binary. It writes them into an override file for each run, which
compose merges with the user's file. Before it runs anything, hikari reads the
file as compose resolves it (`docker compose config`) and refuses a file that
would cross a boundary: a bind mount of a repo or the vault, the Docker
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
forwarding it would flood the room. Instead the system prompt says that the
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
2D canvas itself. The reason is the move from the topology into a partition:
the camera, the fading nodes, the features that spread out of the partition,
and the lens colors all change under one clock. A complete graph library owns
its draw loop and its camera, and a transition between two of its graphs
fights it. The graphs are small, tens of nodes and at most a few hundred, so a
2D canvas is fast enough and WebGL would add weight for nothing.

Both maps live on one canvas, so no frame is ever empty. The engine knows
nodes and edges, not partitions or features; the view tells it how two graphs
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
- The log and the inboxes are in memory. The run's `messages.jsonl` is a
  record only. The server does not read it back at start.
- Claude Code loads MCP tools on demand, so the first turn of each agent
  spends one tool call to load the hikari tools.
- The first indexing turn of a partition agent reads its whole partition.
- No agent sees another partition's code. A partition tests against another
  partition's published releases when the topology grants them. No agent runs
  a suite of the whole application; such suites wait for integration.
- Teammates' branches are merged by people with git. hikari pushes and syncs,
  but it never merges.
- Release grants change only when a container starts, and there are no schema
  grants yet: a partition that owns a contract publishes it in its releases.
- Files that a partition agent creates in its folders belong to the container
  user. On Docker Desktop for macOS this is your user; on Linux, it is the
  user ID of the agent's image, 1000 in `hikari-agent`.
- Only the folders are writable, not `/workspace` itself, which Docker creates
  for the mount points. A test runner that writes a cache in its working
  directory needs its cache off or inside a folder.
- A sidecar serves one agent. Agents cannot share a sidecar yet.
