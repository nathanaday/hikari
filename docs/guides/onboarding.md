# Bring your repositories into hikari

This guide shows you how to turn a set of existing git repositories into a hikari project. You work with Claude Code, in the skills of the Hikari plugin:

- `hikari`, the front door, installs hikari, finds where you stand, and hands you to the skill of the stage.
- `containerize-repo` runs each repository in hikari, with no change to its code.
- `hikify` refactors the repositories into isolated partitions, shared libraries, and flows across partitions whose parts are tested, as you decide.

Two more serve before and after onboarding: `propose-topology` sketches a topology from reading the code, before anything runs, and `change-topology` changes the partitions of an onboarded project. `room` posts your requests to the running agents from your session.

You are the authority on the layout of your project. The skills propose, show, and ask. They do not refactor code before you approve a plan. They do not push or create a remote repository unless you approve that action in the conversation.

When you finish, the project repository holds the topology, the wikis, and the URL of each repository. A teammate can clone it and get the same project. See [Teammates](#teammates).

## Contents

- [Before you start](#before-you-start)
- [The stages](#the-stages)
- [1. Make the project](#1-make-the-project)
- [2. Containerize](#2-containerize)
- [3. Design the refactor](#3-design-the-refactor)
- [4. Carry out the plan](#4-carry-out-the-plan)
- [5. Verify](#5-verify)
- [After onboarding](#after-onboarding)
- [Teammates](#teammates)
- [Stop and continue later](#stop-and-continue-later)
- [Where things are](#where-things-are)
- [Commands](#commands)
- [Problems and fixes](#problems-and-fixes)
- [Try it on the example](#try-it-on-the-example)

## Before you start

You need:

- Claude Code on your machine, with the Hikari plugin:

  ```
  /plugin marketplace add nathanaday/hikari
  /plugin install hikari@hikari
  ```

  The plugin installs the hikari binary of its own version in `~/.hikari/bin` on first use. `hikari doctor` gives you the one `sudo` line that puts that folder on your terminal's PATH, where you run `hikari up`.
- Docker (Docker Desktop on macOS). `hikari up` builds the `hikari-agent` image on its first start.
- A credential for the agent containers: a token from `claude setup-token`, or an Anthropic API key.
- Read access to each repository from your machine. `hikari sync` uses your own git and SSH settings.

Onboarding costs agent time. Each partition agent reads its whole repository on its first start, and the review rounds of the design send several reviewers. Start with a few repositories, or try the [example](#try-it-on-the-example) first.

## The stages

| Stage | Who works | What you do | What you get |
|---|---|---|---|
| 1. Make the project | your Claude Code session | name the repositories and the project, set a credential | an empty project repository |
| 2. Containerize | your Claude Code session, then the partition agents | give the repository URLs, approve ignore lines, start hikari | one partition for each repository, and a baseline of every test |
| 3. Design | your session, with reviewer subagents | answer questions, approve the orchestration features and the plan | a target topology, features of the orchestration feature map, a change plan, and a test ledger |
| 4. Carry out | your session, then the agents | approve new remotes, start hikari, post one message | new repositories and libraries, refactored partitions |
| 5. Verify | your session | approve the push | a checked ledger, and a proof that a fresh clone starts |

Each stage records its progress in `onboarding/state.md`. You can stop after any stage and continue later.

## 1. Make the project

1. Open Claude Code in the folder that holds your repositories, and say what you want:

   ```
   Take these repos, build a hikari-friendly topology, and start working in hikari containers.
   ```

   The `hikari` skill runs `hikari doctor`, which installs hikari on first use, and asks you to fix what it reports, such as a Docker that does not run. It lists the git repositories it finds, with their origins, and asks which ones the project takes. To see a sketch of the topology before anything runs, ask for one: `propose-topology` reads the code and changes nothing.

2. The skill agrees a name with you, and makes the project in a new folder with `hikari init`, which writes `topology.yaml`, `.gitignore`, `.env.example`, and the vault, and makes the first commit. To make a project by hand, run `hikari init ~/code/myproject`, and open Claude Code there.

3. Copy `.env.example` to `.env` in the project, and set your credential. The session never reads or writes it:

   ```sh
   cp .env.example .env
   ```

   ```sh
   CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-...
   ```

   Git ignores `.env`. Never commit it.

4. If other people will run agents for this project, also set your own branch in `.env`:

   ```sh
   HIKARI_BRANCH=hikari-alex
   ```

   Your agents then commit to `hikari-alex` in every repository, and a teammate's agents commit to their own branch.

## 2. Containerize

1. The `hikari` skill hands you to `containerize-repo` with the repositories you chose. In a project you made by hand, run it yourself, and give it each repository's URL, or the folder of a clone, with one sentence on what it does:

   ```
   /hikari:containerize-repo
   The repositories:
   - https://github.com/acme/server.git: the HTTP service that registers accounts.
   - https://github.com/acme/cli.git: the command that registers an account from the terminal.
   The project builds an account sign-up product.
   ```

3. The skill links and clones each repository into `repos/`, and sends one subagent to survey each one. The survey finds its toolchain, its tests, and the suites that cannot run in a container, such as browser tests.

4. **Approve the ignore lines.** When a repository does not ignore its build products, such as `__pycache__/` or `node_modules/`, the skill shows you the lines to add to its `.gitignore`. This is the only change that containerize makes in a repository. Without it, every machine commits the build products again.

5. **Review the partitions.** The skill writes an agent Dockerfile, a test command, and instructions for each repository, plus an orchestration agent. Each partition has one folder, the whole repository, which its agent sees at `/workspace/<name>`; its test command runs in `/workspace` and starts with `cd <name> &&`. The skill names the orchestration agent in `x-hikari.orchestration`. It builds each image and runs each test command once. Then it shows you the partitions and the suites that it left out, with the reason for each. A suite that is left out is not changed. It stays in its repository, and the design stage records it as left for integration.

6. **Start hikari** when the skill asks, in a terminal of your own, and leave it running:

   ```sh
   hikari up topology.yaml
   ```

   The terminal shows the web UI address. On their first start, the partition agents read their code, write their feature maps, and run their tests. A large repository takes many minutes.

7. The skill waits until each partition agent has recorded its tests. Then it writes `onboarding/baseline.md`, with every test and its outcome. A failing test is part of the baseline, not a reason to change code.

8. Stop hikari with Ctrl-C when the skill says that containerize is done.

You can stop here. The project now runs each repository in hikari as it is.

## 3. Design the refactor

The `hikari` skill hands you to the second skill when containerize is done, or you run it in the project:

```
/hikari:hikify
```

When `propose-topology` sketched a topology before containerize, its proposal is `onboarding/proposal.md`. Hikify takes your answers in it as decisions, and checks the rest against the maps and the baseline.

### Analysis

One subagent reads each repository, its partition agent's map, and the baseline. Each writes a report in `onboarding/analysis/`. The session then writes `onboarding/analysis/synthesis.md`: the duplicated code, the boundaries it proposes, and the suites it leaves for integration. It then asks you its questions in one message.

Answer each question in the conversation. You can also write an answer in the `## Your answers` section of a report. Some answers are yours alone:

- **A difference between two copies of code.** For example, one copy lowercases an email address and the other does not. The skill never picks the right behavior; you do.
- **Names and remotes.** The name of each new library, and the URL of its future remote repository.
- **Scope.** Which problems to fix now, and which to leave out of this refactor.

### Target topology

The session writes:

- `topology.target.yaml`: the whole target topology. `hikari check -draft topology.target.yaml` checks it, even though its new repositories do not exist yet.
- `onboarding/topology.md`: each partition and library in plain words, with a diagram, the reason for each boundary, and the splits it decided against. Read this file. It is what you approve, not the YAML.
- Drafts of the features of the orchestration feature map, in `onboarding/vault/orchestration/read-only/features/`, and of the rules and principles of the public wiki, in `onboarding/vault/public/wiki/`. `onboarding/vault/` mirrors the vault, so each draft is already at the path where it will live.

**Agree on the orchestration features.** An orchestration feature is a flow across partitions, such as a sign-up from the terminal to the server. Its health, computed from the partition features it links, must pass at the end.

Every boundary states one of three reasons: a library that two or more partitions need, a client and a server, or two concerns that stay in one repository, each a folder of it. A partition is a list of folders, so the design can also give one partition folders from more than one repository, such as a client and the protocol files that only it changes, and read-only folders for code it reads but does not own. If you disagree with a boundary, say so, and the session revises the design.

### Change plan and review

The session writes:

- a project plan, `onboarding/vault/public/wiki/hikify-plan.md`, with each step in order, the actor of each step, what the step needs first, and when it is done;
- a partition plan for each partition agent;
- the ledger, `onboarding/verification.md`, which gives the planned place of every baseline test: kept, left for integration, or dropped with a reason. A drop waits for your approval.

Fresh reviewer subagents then look for what would break, in rounds. Each round's findings and their resolutions are in `onboarding/review/<n>.md`. A round with only minor findings ends the review. After four rounds that still have serious findings, the session stops and asks you how to go on.

When a fix needs a behavior that you have not decided, the session marks it as a proposal and asks you. Read the questions at approval with care.

**Approve the plan.** Read `onboarding/topology.md`, the project plan, and the ledger, then tell the session that you approve. It sets each plan to `approved`.

## 4. Carry out the plan

Tell the session to start the restructure. hikari must be stopped.

1. **The restructure.** The session does the steps whose actor is `session`:
   - it makes each new repository from the moved code, with its history;
   - it installs the drafts, the deliverables, and the target topology;
   - it moves the documents of each feature that changes partitions, and their entries in the code maps and test maps, whose paths are workspace paths such as `server/accounts/__init__.py`.

   It commits each change as you, on the hikari branch of each repository. Your own branches do not change.

2. **Approve each new remote.** The session gives you the commands that create the remote of a new repository and push its `main` branch. Run them yourself, or approve them in the conversation and the session runs them. Then `hikari sync` clones the new repository into `repos/`.

3. **Start hikari** again:

   ```sh
   hikari up topology.yaml
   ```

4. **Start the plan.** When the new partitions have finished their first turns, the session shows you the message that starts the plan, and posts it to the room with `hikari send` once you agree. It looks like this:

   ```
   @orchestrator Please carry out the plan [[hikify-plan]], from step S5. Follow the Carry out a plan procedure of the hikari-plan-work skill.
   ```

   Agents act on a plan only when you ask. You can also post it yourself in the web UI.

5. **Watch.** The orchestration agent posts the state of each step in the room. It sends a step to its partition agent only after the releases that the step needs exist. For example, the consumers move to a new library only after the library's first release. The partition agents change only their own code. When a step waits for you or for the session, the orchestration agent says so in the room.

   If a partition agent reports that a step cannot be done as written, the orchestration agent tells you. The session proposes a change to the plan, and changes it only with your approval.

6. When the orchestration agent reports every step done, stop hikari, and tell the session.

## 5. Verify

The session:

1. fills in the result of each row of the ledger from the recorded tests;
2. asks you to approve each test that the plan drops;
3. checks that every suite passes, and that the health of every orchestration feature is passing;
4. asks your approval to run `hikari push`, which pushes the hikari branch of each linked repository and then of the project;
5. runs a dry run: it clones the project into a temporary folder, runs `hikari sync` and `hikari check`, and starts hikari there until every agent connects.

The dry run proves that a teammate gets the same project from one clone. When every check holds, the session sets the stage to `hikify: done`, and sums up the partitions, their releases, the suites left for integration, and the orchestration features.

## After onboarding

- **Review and merge.** The agents' work and the restructure are on the hikari branch of each repository, or on your own branch when you set `HIKARI_BRANCH`. Review them and merge them into your own branches with git. hikari never merges.
- **Keep `onboarding/`.** It records why the project has its shape: the reports, the decisions, the reviews, and the ledger.
- **Keep working.** The project is now an ordinary hikari project. Ask your session for work, and `room` posts it to the orchestration agent and follows the replies; or post in the web UI. Plan larger work as plan documents in the wikis, and ask the orchestration agent to carry them out.
- **Change the partitions.** To add a repository, split or merge partitions, or extract a library later, ask your session; `change-topology` runs the stages of hikify for the partitions that change, with its record in `changes/<id>/`.

## Teammates

A teammate starts from a clone of the project repository:

1. Clone it:

   ```sh
   git clone https://github.com/acme/myproject.git
   cd myproject
   ```

2. Clone the linked repositories, and start from a teammate's branch:

   ```sh
   hikari sync -from hikari-alex topology.yaml
   ```

   `sync` checks out your branch in each repository it clones, and never merges, rebases, or resets. It reports how each branch compares with the remote, and names each map whose commit was not pushed.

3. Copy `.env.example` to `.env`. Set a credential, and set your own branch:

   ```sh
   HIKARI_BRANCH=hikari-bob
   ```

4. Check and start:

   ```sh
   hikari check topology.yaml
   hikari up topology.yaml
   ```

A fresh clone has no test results and no releases, because git does not version them. The maps keep the health that the other machine recorded. On its first turn, each partition agent runs its tests again.

To share your agents' work, run `hikari push topology.yaml`. It pushes your branch of each linked repository first, then of the project. It never forces.

## Stop and continue later

`onboarding/state.md` records the stage, your decisions, and the open questions. To continue, open Claude Code in the project; the start of the session names the stage. Ask to continue, and the `hikari` skill hands you to the skill of the stage, which reads `state.md` and continues from there. You can also run that skill yourself: `/hikari:containerize-repo` during containerize, `/hikari:hikify` after it.

The skills commit each of their files at once, as you, so the project is always clean for the next `hikari up`.

## Where things are

| Path | What it holds |
|---|---|
| `topology.yaml` | the topology that hikari runs |
| `topology.target.yaml` | the target topology, until the restructure replaces `topology.yaml` with it |
| `repos/<name>/` | the clone of each linked repository; git ignores it |
| `onboarding/state.md` | the stage, your decisions, and the open questions |
| `onboarding/proposal.md` | the sketch of `propose-topology`, with your answers, when it ran before containerize |
| `onboarding/baseline.md` | every test and its outcome at the end of containerize |
| `onboarding/analysis/` | a report for each repository, and the synthesis |
| `onboarding/topology.md` | the target topology in plain words, with a diagram |
| `onboarding/vault/` | drafts of the vault's documents, plans, and deliverables, at the paths where they will live, until the restructure copies them into `vault/` |
| `onboarding/review/<n>.md` | the findings of each review round, with their resolutions |
| `onboarding/verification.md` | the ledger: where each baseline test goes, and the result |
| `changes/<id>/` | the record of each topology change after onboarding, laid out as `onboarding/` |
| `.hikari/runs/<run>/` | the log and the messages of each run of hikari |
| `.hikari/host.json` | the address of the running `hikari up`, readable only by you, for `hikari send`; it goes when hikari stops |

## Commands

| Command | What it does |
|---|---|
| `hikari doctor [topology.yaml]` | checks the binary, the PATH, Docker, the image, git, and the project's credential, and says how to fix each problem |
| `hikari init <dir>` | makes a new project repository |
| `hikari sync [-from <branch>] topology.yaml` | clones missing repositories, makes your branches from the remote and checks them out in the new clones, removes the skill copies of hikari before the plugin, and reports what was not pushed |
| `hikari check topology.yaml` | runs every check of `hikari up`, and starts nothing |
| `hikari check -draft topology.target.yaml` | checks a target topology whose new repositories do not exist yet |
| `hikari status [-tests] topology.yaml` | shows each partition's map and recorded tests; with `-tests`, each test |
| `hikari up topology.yaml` | starts the project, and builds the `hikari-agent` image when it is missing |
| `hikari send [-to <agent>] <message>` | posts a message as you to the room of the running project, or to one agent |
| `hikari image` | builds the `hikari-agent` image again, such as for a newer Claude Code |
| `hikari push topology.yaml` | pushes your branches, linked repositories first |

## Problems and fixes

| What you see | What to do |
|---|---|
| `hikari doctor` reports a problem | The `hikari` skill says how to fix each line; ask it, or follow the fix that doctor prints. |
| `the hikari that the PATH finds first … is not hikari <version>` | An older install comes first on your PATH, such as one in `/opt/hikari`. Remove it with the command that doctor prints. |
| `hikari is not running for topology.yaml` | `hikari send` needs a running `hikari up` for that topology file. Start it in your own terminal. |
| `the linked repo <name> is not cloned` | Run `hikari sync topology.yaml`. |
| `<repo> has uncommitted changes` | Commit or stash the changes it names, then start again. |
| `cannot listen on 127.0.0.1:7420` | Another hikari runs. Stop it, or start with `hikari up -listen 127.0.0.1:7431 topology.yaml`. |
| `the linked repo <name> is not used by any partition yet` | A warning during containerize, before the skill writes the partitions. It is normal. |
| An image fails to build, or a partition agent cannot run its tests | Tell the session. It fixes the Dockerfile or the test command, and asks you to restart hikari. |
| A change to a partition's `test` command has no effect | The `test` key seeds only the first suite of a new partition. Ask the partition agent to change its suite instead. |
| `hikari sync` reports a map whose commit is missing | A teammate pushed the project but not their linked repositories. Ask them to run `hikari push`. |
| The review stops after four rounds | Read the open findings in `onboarding/review/`, then tell the session to revise, to run one more round, or to go on. |
| The session asks to push or to create a remote | Approve only the action that it names, or run the command yourself. |

## Try it on the example

`examples/onboard` holds two small Python repositories before hikari: a server and a command that share a copied `accounts` module, with one difference between the copies. The server also has a browser test that cannot run in a container.

From a checkout of this repository:

```sh
make example-onboard
cp .env .hikari/demo/onboard/project/.env
cd .hikari/demo/onboard/project
claude --plugin-dir ../../../../plugin
```

`--plugin-dir` loads the plugin of the checkout for that session; with the plugin installed, a plain `claude` does. The command prints the `file://` URLs of the two repositories. Give them to `/hikari:containerize-repo`, then run `/hikari:hikify`. The skill must find the copied module, ask you which behavior is right, propose an `accounts` library, and leave the browser test in its repository for integration.
