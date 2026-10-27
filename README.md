# hikari

Status: in development.

## About

Hikari runs each Claude Code agent in its own Docker container and connects the agents through one room. Every post in the room reaches everyone, and anyone can send a direct message to anyone.

One agent can be the orchestration agent, and every other agent is a partition agent: it has a partition, a directory of a git repo on your machine, mounted into its container. The partition agent is the only agent that can read or change that code, and it keeps a feature map of what the partition does. The orchestration agent never sees code. It plans from the feature maps and asks the partition agents for changes.

You are in the room too, so you can post to everyone and send a direct message to any agent.

The words of this README, such as vault, partition, and feature map, are defined in [docs/glossary.md](docs/glossary.md).

## Quickstart

### Prerequisites

- Go 1.25 or later
- Node.js 22.12 or later, to build the web UI
- Docker (Docker Desktop on macOS)
- A Claude subscription or an Anthropic API key

### Setup

1. Make a long-lived token for the containers:

   ```sh
   claude setup-token
   ```

2. Write the token to `.env` in the repository root. Git ignores this file.

   ```sh
   CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-...
   ```

   You can use `ANTHROPIC_API_KEY=...` instead.

### Start

```sh
make demo-repo
make up
```

`make demo-repo` writes the repo of the shop example, two small Python services, to `.hikari/demo/shop`, and seeds the example's vault. `make up` starts the shop, and asks you to run `make demo-repo` first when its repo is missing. It builds the web UI, the `hikari` binaries, and the `hikari-agent` image. Then it starts the server and the shop's compose project, with one container for each agent: the orchestration agent and the partition agents `orders` and `inventory`. The terminal shows each start-up step, then keeps the web UI address, the state of each agent, and the cost so far pinned at the bottom, with one line for each message above them. Open the web UI address. It contains a secret token, so do not share it. Press Ctrl-C to stop all containers.

The web UI's Topology view shows the topology file in two tabs. Structure is a form: a list of the project, repos, agents, and sidecars, and an inspector for the one you select, where you rename, add, and remove them, pick the orchestration agent, and change partitions, grants, and models. YAML is an editor of the file with the checks of `hikari check` as you type. Every save shows the lines of the file it changes before it writes them, and a rename of an agent offers to carry the new `@name` into the wikis and the vocabulary, place by place. An edit changes only the lines it needs, so your comments and formatting stay. In a project repository, a save is a commit as you. The agents read the topology when they start, so a change applies at the next start; Stop and Start are in the view.

`hikari up` opens `topology.yaml` in the current directory when you name no file. It prints the web UI address before it reads the file. When the file is missing or has errors, or a start check fails, it does not exit: it waits with the web UI open, so that you can create the project or fix the file there, and start the agents from the UI. You can stop the agents from the UI too, change the topology, and start them again in the same process. Ctrl-C stops everything.

Each run keeps its records in `.hikari/runs/<run-id>/`: `hikari.log` with everything the containers and compose print, including each tool call of each agent, `messages.jsonl` with one JSON line for each message, when there are messages, and the run's compose override file.

To have the latest `hikari` on your `PATH` everywhere, run `make install` after you pull or commit. It builds the binaries and copies `hikari` and its Linux builds to `/opt/hikari/bin`, which it adds to the system `PATH` in `/etc/paths.d/hikari`. The first run asks for `sudo` to make the directory and that entry; later runs do not. Set `HIKARI_PREFIX` to install somewhere else.

To start another topology file:

```sh
make up TOPOLOGY=path/to/topology.yaml
```

## Usage

The topology is a Docker Compose file. Compose describes each agent's container, and the `x-hikari` keys, which compose ignores, describe the project: the orchestration agent at the top, and each agent's role in its service. The name `user` always means you.

```yaml
name: shop                     # required; it names the containers, networks, and volumes

x-hikari:
  repos:
    shop: {path: ~/code/shop, branch: hikari/demo}
  orchestration: orchestrator  # optional; the service id of the orchestration agent

x-agent: &agent                # a YAML anchor, so that each service repeats less
  image: hikari-agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  orchestrator:
    <<: *agent
    x-hikari: {description: Orchestrates the shop. Plans changes from the feature maps.}
  orders:
    <<: *agent
    x-hikari:
      description: Owns order intake.
      partition: {folders: [{repo: shop, path: services/orders}]}
  inventory:
    <<: *agent
    x-hikari:
      description: Owns stock levels and reservations.
      model: haiku
      partition: {folders: [{repo: shop, path: services/inventory}]}
```

`x-hikari.orchestration` names the orchestration agent by its service id. It is optional, and the orchestration agent has no partition. Every other agent has a partition: `hikari check` refuses an agent that is neither, as `agent "writer" has no partition and is not the orchestration agent`. Each agent sees the `description` of everyone in the room. The `instructions` field of an agent goes into the system prompt of that agent only.

You and every agent are in one room, and everyone in it receives every post. A post starts a turn at once only for:

- the orchestration agent,
- each agent that the post mentions as `@name`,
- every partition agent, when the orchestration agent posts without mentions.

When the project has no orchestration agent, a post without mentions starts a turn for every agent. Other agents read the post at the start of their next turn. Anyone can send a direct message to anyone, and a direct message always starts a turn for its recipient. These rules keep a room of agents from all answering at once.

Compose reads the whole file, including `x-hikari`, so write `$$` for a literal `$` in instructions.

Inside a container, every agent has the tools `post`, `send_direct_message`, `list_agents`, `check_inbox`, and `get_vocabulary`, and it sees its hikari documents under `/hikari`. `list_agents` lists everyone in the room with their role, `orchestration`, `partition`, or `operator` for you, and their description.

### Names and ids

Every agent and repo has an id, the key that the file writes, and a name. The name is the id unless a `name` field sets it, and the project's name is `x-hikari.name`:

```yaml
x-hikari:
  name: Shop Planning
  orchestration: orchestrator

services:
  inventory:
    <<: *agent
    x-hikari:
      name: stock
      description: Owns stock levels and reservations.
      partition: {folders: [{repo: shop, path: services/inventory}]}
```

Agents and the UI use names: here you mention `@stock`. References in the file use ids, such as `orchestration: orchestrator` or `releases: [inventory]`, so a rename changes one line. Everything outside the file is keyed by id: the vault directories, the release manifests, the containers, and the clones. A rename therefore moves nothing; it takes effect at the next start. Names follow the rules of ids, and no name may be another agent's id or `user`.

### Partitions

Give an agent a partition to make it a partition agent. A partition is a list of folders, each a path in any repo of the project, writable or read-only:

```yaml
x-hikari:
  repos:
    shop:
      path: ~/code/shop          # relative paths start at the topology file
      branch: hikari/checkout-v2 # agents commit here; you merge it
    protos: {path: ~/code/protos, branch: hikari/checkout-v2}

services:
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      description: Owns order intake.
      partition:
        folders:
          - {repo: shop, path: services/orders}          # at /workspace/shop/services/orders
          - {repo: shop, path: libs/money, read_only: true}
          - {repo: protos, path: orders, at: protos}     # at /workspace/protos
        test: python3 -m pytest -q shop/services/orders/tests -o pythonpath=shop/services/orders -p no:cacheprovider -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml  # optional; runs in /workspace, and seeds the first suite, all
      grants:
        deliverables: [python-library]  # optional; the releases this partition agent can publish
        releases: [inventory]     # optional; read-only access to another partition's releases
```

The `test` command seeds the partition agent's test map once: when the partition has no `tests.yaml` yet, hikari makes the command its first suite, `all`, whose report must be `/hikari/private/reports/all.xml`. After that, the partition agent keeps its suites in `tests.yaml`, and a change of `test` in the topology file has no effect.

A folder's `repo` is the repo's id, and its `path` is relative to the repo's root; with no `path`, the folder is the whole repo. The container sees each folder at `/workspace/<repo>/<path>`, with the repo's name, unless the folder sets `at:`, and it sees no `.git`. The agent's working directory is `/workspace`, and it works in its folders as in one monorepo. A partition needs at least one folder without `read_only`. The writable folders of two partitions never overlap, and no folder lies inside another partition's writable folder; read-only folders of two partitions may overlap.

When `hikari up` starts, it checks that each repo has no uncommitted changes, then creates each repo's branch if needed and checks it out. The agent cannot commit by itself. An apply commits its writable folders: one commit in each repo whose folders changed, on that repo's branch, with the agent as author, you as committer, and one message. The agent calls `apply` to name the change, and hikari applies what is left at the end of every turn and when hikari stops. You review the branches and merge them.

Four documents describe each partition, and the partition agent is the only author of all four. The first two are the feature map, which every agent can read; the last two are private:

- `vault/partitions/<id>/read-only/features/<doc>.md` holds the documents of the feature map: one markdown document for each feature, principle, and rule of the partition, with a property header that hikari checks and a body that says why it exists and what it promises. Documents link the documents that work together with `[[id]]` links. A link has no direction and no kind, and there are no groups: the links are the structure. The partition agent changes the wiki only through `change_wiki`, which applies a set of writes as a whole or not at all.
- `vault/partitions/<id>/read-only/features/map.yaml` is the rest of the feature map: a summary, the interfaces the partition provides and consumes, written through `set_partition_info`, and a list of the features that hikari writes from the wiki, with the health and state of each. It holds no file paths and no commands.
- `<folder>/.hikari/private/code-map.yaml` is the code map: where each feature lives in the code. `<folder>` is the partition's first writable folder, and every path in the private documents is a workspace path, such as `shop/services/orders/app.py`.
- `<folder>/.hikari/private/tests.yaml` is the test map: the suites that run tests, the features that each test touches, and what no test checks. Each suite's command writes a JUnit XML report to `/hikari/private/reports/<suite>.xml`, and `record_test_run` hands it to hikari. hikari keeps the last result of each test in `results.yaml` beside it. Results and reports are never committed.

The partition agent edits the private documents directly. The `hikari-map-features` skill in its container explains each document, what counts as a feature, and how to link; it comes with three read-only agents, `hikari-feature-survey`, `hikari-feature-review`, and `hikari-test-survey`, that read the code for a partition agent. Health is computed from the tests that touch each feature: `failing` when one of them failed, `pending` when one has not run since its file or the code it tests changed, `untested` when no test touches the feature, and otherwise `passing`. The partition agent never reports a count: the test runner writes the results. State is computed too: a feature is `stale` when its code changed since the documents were marked current. On its first start, a partition agent reads its code and builds the documents. When the code changes, hikari tells the partition agent which features are stale and which changed paths belong to no feature, and the partition agent updates them and marks the map current. You can open the vault in Obsidian: every link resolves.

Each kind of agent gets a pack of skills from hikari, which the driver installs in its container before each turn. A partition agent's pack is `hikari-plan-work`, which posts the plan in the room before a change of more than one feature; `hikari-do-work`, which makes the change, keeps the code map true, applies, and publishes a release; `hikari-validate-work`, which runs every suite and reads the health before the agent says that a change is done; and `hikari-map-features`, which builds and keeps the feature map, the code map, and the test map. The orchestration agent's pack is `hikari-plan-work`, which turns a request into work for the partition agents and carries out a plan; `hikari-health-check`, which reads the health of every partition and the vocabulary's flags and asks the partition agent of what fails; and `hikari-map-features`, which keeps its own feature map, the public wiki, and the vocabulary. The prompts name the skill of each task. A skill of the same name in two packs is two skills, and a container gets only its own pack. The prefix `hikari-` is hikari's, so a skill you grant cannot use it. Each agent's inspector lists its pack, marked as from Hikari, above the skills you grant.

The orchestration agent reads the feature map of every partition, and it keeps the project's one vocabulary in `vault/public/vocabulary/terms.yaml`: the names that every agent uses for domain terms and interfaces. Every agent reads it with `get_vocabulary`, and the orchestration agent changes it with `define_term`, `define_interface`, and `remove_entry`. With no orchestration agent, only you change it. hikari reads only `terms.yaml` and ignores the other files of the directory. The orchestration agent's view flags interface names that are not in the vocabulary, and consumed interfaces that no partition provides.

The UI shows each partition with its feature map, health, flags, and releases, and, marked private, its code map and test map. It also shows the vocabulary on the room's page, and the Vault page: the policies and deliverables of the public area, with each deliverable's rules and holders, and your skills, with the agents they are granted to. Each agent's inspector in the Structure tab shows its mounts.

### Wikis

Every partition's feature map has a wiki, the orchestration agent has a feature map of its own, and the project has one public wiki: one markdown document per entry, each with a kind.

| Kind | In | Who writes it | What it holds |
|---|---|---|---|
| `feature` | partition, orchestration | the partition agent, the orchestration agent | one thing the partition does that someone relies on; in the orchestration feature map, a flow across partitions |
| `principle` | partition, orchestration, public | the partition agent, the orchestration agent | a lasting choice that guides the work, with its reason |
| `rule` | partition, orchestration, public | the partition agent, the orchestration agent | a statement that must always hold |
| `plan` | partition, orchestration, public | you | work you intend, with a `status` |
| `note` | partition, orchestration, public | you | anything else; a file with no `kind` is a note |

The public wiki is `vault/public/wiki/`, and the UI and the API call it `public`. The orchestration agent writes it through `change_public_wiki` and the `hikari-map-features` skill. Every agent reads it at `/hikari/public/wiki/`, and the orchestration agent also reads every feature map. Ids are unique across the vault, so `[[reserve-stock]]` names one document wherever it is written: an orchestration feature links the features of the partitions, and a feature can link the rules and principles of the public wiki. The links across wikis show how the project fits together. A rename rewrites every link to the document in every wiki.

The orchestration feature map is `vault/orchestration/read-only/features/`, and the UI and the API call it `orchestration`. The orchestration agent writes it through `change_wiki`, as a partition agent writes its own, and reads it at `/hikari/self/features/`. An orchestration feature is a flow across partitions, such as a customer who orders items with stock held for each, and it links the partition features that carry it out. It has no code and no tests: its health is computed from the features it links, `failing` when one of them is failing, `pending` when one is pending, `untested` when none is tested, and otherwise `passing`. It can link every partition's feature map and the public wiki, and neither of them links it. Only the orchestration agent reads it, so a partition agent's `get_document` and `search_wiki` show only the backlinks from documents that it can read. A principle or a rule of the orchestration feature map binds only the orchestration agent; one for every agent belongs in the public wiki. The public wiki had a `story` kind before; a document of that kind is now a problem of that document, which says to move it to the orchestration feature map as a feature.

Plans and notes are yours. Write them in Obsidian, where the vault is one vault, or in the UI. Agents read them and never change them, and the prompts say to act on a plan only when you ask. You can write any document by hand; one of an agent's kinds must keep the rules of its kind. A hand edit that breaks one document affects only that document: hikari keeps its last valid version and shows the problem. Before each turn, an agent is told which documents of the wikis it reads were changed by someone else.

`get_document` and `search_wiki` read any document of the wikis an agent can read, with the documents that link it. A rule or principle of the public wiki or of the orchestration feature map shows its support: the health of the features it links.

The Map view draws the topology as a force graph: the orchestration agent in the middle with an edge to each partition, and each partition with the repository its code is in and the deliverables it holds. A dashed edge joins two partitions for each release grant, and where one partition uses the other's interfaces. Lenses color the partitions: Activity (cyan while the partition agent works, muted cyan while it waits), Tests (red for a failing feature, yellow for a pending one, green when they pass), and Git (magenta for uncommitted changes). Click a partition and the camera moves into it, and it opens into its wiki map; click the orchestration agent and it opens its feature map, with the partition features it links drawn around it. A wiki map has one node per document, shaped by kind, with smaller nodes for the documents of other wikis that it links or that link it. The wiki lenses are Kind, Tests (a feature's health, a rule's or a principle's support), and Age. Click a document to read it beside the graph, with the documents that link it, and to edit it; the New button writes a document. The room's Wiki tab lists the documents of the public wiki, and the orchestration agent's page has a Feature map tab that opens its feature map. Back returns to the topology.

### Suites of the whole application

Each partition's tests cover only its own code. A suite that needs a display, a browser, the whole application, or another partition stays in its repository, outside every test command, and hikify records it in `onboarding/` as left for integration. hikari does not run these suites yet, so an orchestration feature has no test of its own: its health is computed from the partition features it links, and the rules and principles of the public wiki show their support.

### Each agent's container

Compose owns each agent's environment: its image or Dockerfile, its variables, env files, and secrets, its volumes, its ports, and anything else compose offers. hikari owns the boundaries. It writes an override file for each run that gives every agent its mounts, its token, its own network, and the hikari binary, and runs `docker compose` with both files.

An agent's image must meet one contract: `claude` is on its `PATH`, and its user is not root. The `hikari-agent` image meets it, so a Dockerfile can start `FROM hikari-agent` and add a toolchain. Any other image works when it installs Claude Code and sets a user. `hikari up` checks each image before it starts any container.

- **Dependencies.** A named volume inside one of the agent's writable folders keeps Linux builds out of your repo, for example `volumes: [modules:/workspace/shop/web/node_modules]`. hikari makes the volume writable for the agent's user, and it stays between runs.
- **Secrets.** Give each agent only its own variables and env files. An agent can read every variable that its container has.
- **Sidecars.** A service with `x-hikari: {serves: orders}` runs beside orders, for example a database for its tests. It joins only orders' network, so only orders can reach it, by its service name.
- **Extra mounts.** An agent can mount other host paths, such as a dataset, but not a path in a topology repo, the vault, or the Docker socket. In a project repository, only the vault, `.git`, and the writable folders of its partitions are refused.

hikari sets an agent's `entrypoint`, `command`, `working_dir`, and networks, and it refuses a file that sets them or that gives a service host-level access, such as `privileged` or `network_mode`. Every service needs `x-hikari`, so no container runs outside the rules.

### The vault and releases

The vault is one directory, `vault/` next to the topology file unless `x-hikari` sets `vault:`. It holds every hikari document that is not code, so you can see your whole ecosystem in one place. The directory a file is in decides which containers mount it and whether they can change it. [docs/guides/vault.md](docs/guides/vault.md) shows where each file of the shop example lives on the host and where each agent sees it.

```
vault/
  public/                      every agent reads it
    vocabulary/terms.yaml      the names the project agreed on; the orchestration agent writes it
    wiki/<id>.md               the public wiki: rules, principles, and your plans and notes
    policies/<name>.md         your rules for every agent; they go into every system prompt
    deliverables/<name>/       what a release must contain and how to make it
  skills/<name>/SKILL.md       your Claude Code skills, granted with grants.skills
  partitions/<id>/
    read-only/                 its partition agent and the orchestration agent read it
      features/                the feature map: map.yaml, which hikari writes, and the documents
      releases/<deliverable>/<x.y.z>/   published releases, which never change
      releases/<deliverable>/latest     the newest release of that deliverable
      granted/                 files you give the partition to read
    can-edit/                  files you give the partition, which its agent can change
    staging/<deliverable>/     where the partition agent builds the product of its next release
  orchestration/               only when the project has an orchestration agent
    read-only/features/        the orchestration feature map: features across partitions, and your plans and notes
    read-only/granted/         files you give the orchestration agent to read
    can-edit/                  files the orchestration agent can change
```

A partition's directory is named by its partition agent's id, so a rename keeps it. hikari writes `README.md` once, makes every directory the topology needs at start, and never writes `policies/`, `deliverables/`, `skills/`, or a `granted/`. The orchestration agent's tools write only the vocabulary and the public wiki.

A deliverable is a named package that says what a release must contain and how its partition agent makes it. You write it, in `vault/public/deliverables/<name>/`, and grant it to a partition agent with `grants.deliverables`. It holds two files:

- `deliverable.yaml`: a description, optional layout rules on the files of the product as glob lists `require`, `allow`, and `forbid`, and optional targets, which promise that the product is also published to `github-release`, `dockerhub`, or `huggingface`.
- `SKILL.md`: a Claude Code skill that tells the partition agent how to build the product. hikari installs it under the deliverable's name in each holder's container.

A partition agent can hold several deliverables, so one partition can publish, for example, a library and a container image. To publish, the partition agent loads the deliverable's skill, builds the product in `/hikari/staging/<deliverable>/`, calls `check_release` to check it against the layout rules, then calls `publish_release` with a version and notes. When the deliverable has targets, the partition agent publishes there first, with the tools and credentials that compose gives its container, and gives one certificate for each target in the same call; hikari checks each certificate's shape and never uploads anything. hikari then writes the manifest, `release.yaml`, with who published, when, from which commit, with which hikari version, the certificates, and the digest of the product, writes `SHA256SUMS`, moves the product into `releases/<deliverable>/<version>/product/` with a copy of the feature map in `features/`, and announces the release in the room. An older manifest can still have a `hubs:` field, which hikari ignores, and a `system:` field, which hikari reads as `partition:`. The shop example's `python-library` deliverable shows the format.

Your own skills live in `vault/skills/<name>/`, each with a `SKILL.md`. Grant one to any agent with `grants.skills: [name]`: its container mounts the skill, and Claude Code loads it as one of the agent's skills. `hikari check` refuses a granted skill that does not exist, that has no `SKILL.md`, whose name is a deliverable's, or whose name starts with `hikari-`.

The orchestration agent reads the feature maps and releases of every partition, and the deliverables in the public area, so it can check each release against its deliverable. Grant a partition agent read-only access to another partition's releases with `grants.releases`, for example so that it can integrate a library and test against it. Grants take effect when the container starts.

Each container mounts a few directories of the vault at fixed paths, and nothing else of it:

| Container path | Partition agent | Orchestration agent | Mode |
|---|---|---|---|
| `/hikari/public` | `public/` | `public/` | read-only |
| `/hikari/self` | `partitions/<id>/read-only/` | `orchestration/read-only/` | read-only |
| `/hikari/can-edit` | `partitions/<id>/can-edit/` | `orchestration/can-edit/` | writable |
| `/hikari/staging` | `partitions/<id>/staging/` | none | writable |
| `/hikari/private` | its private documents, `.hikari/private/` of its first writable folder | none | writable |
| `/hikari/partitions/<name>` | none | each partition's `read-only/` | read-only |
| `/hikari/grants/<name>/releases` | each granted partition's `read-only/releases/` | none | read-only |
| `/hikari/skills/<name>` | each granted skill | each granted skill | read-only |

No container mounts another partition's `can-edit/`, `staging/`, or private documents.

### Project repositories

A project can be a git repository of its own, which holds the topology, its Dockerfiles, and the vault. Make one with:

```sh
hikari init ~/code/myproject
```

This writes a starter `topology.yaml`, a `.gitignore`, an `.env.example`, and the vault, and makes the first commit. Copy `.env.example` to `.env`, set a credential, and run `hikari up ~/code/myproject/topology.yaml`.

The key `x-hikari.project: {branch: hikari}` makes the topology file's directory a project repository. Then:

- The vault is versioned: the feature maps, the wikis, the vocabulary, your public files and skills, and every `granted/` and `can-edit/`. Releases and staging areas are not, and `vault/.gitignore` keeps them out.
- hikari commits each change that an agent makes through a tool, with the agent as author and you as committer, on the project branch: `orders: change features: create create-order`, or `orchestrator: define order in the vocabulary`. A test run changes the health in the map but makes no commit; the health goes into the partition's next commit. An apply also commits the agent's changes in its `can-edit/`, and the orchestration agent's changes there are committed at the end of each of its turns. At stop, hikari commits what each agent's paths still hold.
- hikari never commits your policies, deliverables, or skills. Commit them yourself. Your edits in the wikis are committed with your git identity: before an agent changes the same wiki, when you make them in the UI, at stop, and at start when the project branch is checked out and only the wikis changed. Your changes in a `can-edit/` or a `granted/` are committed the same way at start and at stop.
- The project repository follows the rules of every repo: it must be clean when hikari starts, and agents commit only to the project branch, which you merge. When the checkout of the branch changes the topology file, hikari reads it again.
- A partition's folder can live in the project repository too: `partition: {folders: [{repo: project, path: app}]}`. The folder cannot overlap the vault or `repos/`.
- The project repository must ignore `.env` and `/.hikari/`. Ignore `/.hikari/` at the top only: a `.hikari/` pattern also hides the private documents in each partition's folder.

#### Linked repositories and teammates

A repo can be linked by its remote URL instead of a path. hikari keeps its clone in `repos/<name>` in the project repository, which git ignores:

```yaml
x-hikari:
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  repos:
    p3-cloud: {url: https://github.com/you/p3-cloud.git}   # its branch is the project branch
    tools: {path: ~/code/tools, branch: hikari}             # a path repo, on this machine only
```

Three commands work on the whole project. Like `hikari up` and `hikari status`, each opens `topology.yaml` in the current directory when you name no file:

- `hikari sync` clones each missing linked repo. Where the remote has your branch and you do not, it makes your branch from the remote's. It reports how each branch compares with the remote, and each map whose commit is missing because a teammate did not push it. It never merges, rebases, resets, or checks out.
- `hikari check` runs every check of `hikari up`, and starts nothing. `hikari up` refuses a missing clone, and names `hikari sync`.
- `hikari push` pushes your branch of each linked repo, then of the project repository. It never forces. When a push is refused, it does not push the project repository, whose maps describe the linked repos' commits.

`HIKARI_BRANCH` in `.env` gives each person their own branch, so that two people's agents never commit to one branch. A teammate starts like this:

1. Run `git clone <project-url> myproject`.
2. Run `hikari sync -from hikari-alex myproject/topology.yaml`. This makes your branch in each repo from `hikari-alex`, a teammate's branch.
3. Copy `.env.example` to `.env`. Set a credential, and set `HIKARI_BRANCH` to your own branch, for example `hikari-bob`.
4. Run `hikari check myproject/topology.yaml`, then `hikari up myproject/topology.yaml`.

A fresh clone has no test results and no releases, because git does not version them. The maps keep the health that the teammate's machine recorded. On its first turn, each partition agent runs its suites again.

The web UI shows the room, a direct-message thread for each agent, and an All activity view with every message, including private messages between agents. The room's page, `#/room`, has three tabs: Messages, Vocabulary, and Wiki. Under each post in the room, it shows which agents the post woke. While you write, the composer shows which agents your draft will wake.

Each turn of an agent appears in the timeline as a card, after the message that woke it. While the agent works, the card shows its steps live: what it reads, edits, runs, and writes, each with its duration. Click a tool call to see its input and output. When the turn ends, the card folds into one line, such as `26s · 14 tool calls · 3 files changed · $0.18`, which opens again on click.

The text an agent writes at the end of a turn reaches no one; its messages are the only way to reach you or another agent. The card shows that closing note, marked when the agent sent no message during the turn. A note that asks something is highlighted, because the agent may be waiting for your answer: reply to it by message.

#### Bring existing repositories in

For a step-by-step guide, see [docs/guides/onboarding.md](docs/guides/onboarding.md).

`hikari init` also writes host skills into the project's `.claude/skills/`: skills for your own Claude Code session, not for an agent's container. `hikari sync` writes them again, so they always match your hikari binary. git ignores them.

`containerize-repo` wraps your repositories into the project, one partition for each repository, with no change to their code. Open Claude Code in the project and run `/containerize-repo`, then give it the URL of each repository. The skill:

1. links each repository and clones it with `hikari sync`;
2. surveys each repository with a subagent: its toolchain, its tests, and the suites that cannot run in a container, such as browser tests;
3. proposes ignore lines for build products that a repository does not ignore, and adds them only with your approval;
4. writes an agent Dockerfile, a test command, and instructions for each partition, and an orchestration agent, which it names in `x-hikari.orchestration`, then runs `hikari check`;
5. asks you to start `hikari up`, and waits until each partition agent has mapped its repository and recorded its tests;
6. writes the baseline, every test with its outcome, to `onboarding/baseline.md`.

It keeps its progress in `onboarding/state.md`, so a new session continues where the last one stopped.

`hikari status` shows what the files say of each partition: the commit that its map describes, its features by health, and the last run of each suite. With `-tests`, it lists each test with its outcome. It reads files only, so it works while hikari runs and when it does not.

`hikify` starts where containerize ends, and refactors the project into isolated partitions with you. Run `/hikify` in the same project. It works in stages, and keeps each one in `onboarding/`:

1. **Analysis.** A subagent reads each repository with its partition agent's map and the baseline. The session then writes a synthesis of the duplicated code, the boundaries it proposes, and the suites it leaves for integration, and asks you its questions. A difference in behavior between two copies of code is always your decision.
2. **Target topology.** It drafts `topology.target.yaml`, explains it in `onboarding/topology.md` with a diagram, and states the reason for each boundary. `hikari check -draft topology.target.yaml` checks a target whose new repositories and folders do not exist yet. With you, it drafts the features of the orchestration feature map in `onboarding/wiki/orchestration/`, and the rules and principles of the public wiki in `onboarding/wiki/public/`.
3. **Change plan.** It writes the project plan, `onboarding/wiki/public/hikify-plan.md`, a plan for each partition, and a ledger that says where each baseline test goes.
4. **Review.** Fresh reviewer subagents look for what would break, in rounds, until a round finds only minor problems. After four rounds with serious findings, the session stops and asks you.
5. **Execution.** After you approve, the session makes the new repositories, moves code with its history, and installs the drafts and the target topology. You create each new remote. Then you start hikari, and ask the orchestration agent to carry out the plan: it sends each step to its partition agent when the releases it needs exist. The session follows the run from files.
6. **Verification.** The session checks that every baseline test is kept, left for integration, or dropped with your approval, and that the health of each orchestration feature is passing. Then it clones the project into a temporary folder, syncs, checks, and starts it, as a teammate would.

To try it, run `make example-onboard`. It writes two small repositories as local remotes, and an empty project in `.hikari/demo/onboard/project`.

### Work on the UI

The UI is a Vite, React, and shadcn/ui app in `web/`. The `hikari` binary embeds the built UI. To change the UI with hot reload, start a project with `make up`, then run:

```sh
make dev-ui
```

Open http://localhost:5173/?token=... with the token that `make up` printed. The dev server sends API requests to the running hikari server. `make test` runs the Go tests and the UI tests.

## Patterns and conventions

The design notes in [docs/design.md](docs/design.md) give the full reasons.

- The server is the only authority on the topology. It checks every message against the topology, and decides whom each post wakes. A container cannot change who it is.
- Only the server writes to git. Agents never get `.git`, so they cannot commit outside their folders, rewrite history, or switch branches.
- The feature map is a public surface. Paths, commands, and test names stay in the private documents, which only the partition agent and you read.
- The partition agent is the only author of its documents. The server applies each agent's changes to its wiki and checks them, so the wiki and the map never hold an invalid document.
- Containers cannot reach each other directly. Each container has its own Docker network. The only path between agents goes through the server.
- Compose owns the environment; hikari owns the boundaries. You describe each container in compose, and hikari computes every mount, network, and token that keeps agents apart.
- One binary does all jobs. `hikari up` runs on the host. hikari mounts its Linux build into each container, where it runs as `hikari driver` and `hikari mcp`.

## Layout

- `cmd/hikari`: the command-line entry point
- `internal/server`: message routing, the log, the HTTP API, and the web UI
- `internal/partitions`: repos, feature maps, the vocabulary, and who may read and change them
- `internal/featuremap`: the feature map and the private documents, their rules, health, and feature state
- `internal/wiki`: the documents of a wiki, their kinds, links, and the change that writes them
- `internal/testmap`: the test map, JUnit reports, recorded runs, and the health of each feature
- `web/src/components/graph-engine.ts`: the canvas of the Map view: layout, drawing, input, and transitions
- `internal/vocab`: the vocabulary format
- `internal/vault`: the vault's directories, deliverables, and releases
- `internal/gitrepo`: branch preparation, commits of a partition's folders, and the remote operations of sync and push
- `internal/workspace`: a partition's folders, their places under `/workspace`, and the host folder of a workspace path
- `internal/linked`: the checks of a start, and `hikari sync` and `hikari push` across the project's repos
- `internal/driver`: the driver that gives messages to Claude Code
- `internal/mcpserver`: the MCP tools for the agent
- `internal/launcher`: writes the override file and runs the compose project
- `internal/topology`: reads and checks topology files
- `internal/version`: the version of hikari, which `hikari version`, the terminal banner, and the web UI show; change it there for each release
- `web/`: the browser UI
- `examples/`: `shop` (an orchestration agent and two partitions in the demo repo, with a sidecar), and `onboard` (two repositories before hikari, for `make example-onboard`)

External:

- Docker Compose: https://docs.docker.com/compose/
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code headless mode: https://docs.claude.com/en/docs/claude-code/headless
