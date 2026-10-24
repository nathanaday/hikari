# Onboarding: round 1

Sources: your note `journals/Hikari/Principles.md` in the SoftwareProjects vault, and your message of 2026-10-23.

## What I take from your proposal

- **Start from an empty project repository.** You run `hikari init`, link the remote URLs of your repositories, and build the topology with an agent. The work is collaborative: at the start, you are the authority on how the ecosystem is laid out.
- **The clones live in the project repository, ignored by git.** The agent clones each repository into the project directory. git ignores the clones, so no repository is nested in another's history.
- **Any set of repositories can become a hikari project.** The agent helps refactor the repositories against the topology.
- **A teammate gets the same project from one clone.** The project repository holds the knowledge, the topology, and every remote URL. A teammate clones it, builds the same project on their machine, and starts work.
- **Two skills that stay apart.** `containerize-repo` wraps a repository in a container with no change to its code. `hikify` refactors it for hikari. Your note asks for two skills so that the guidance of one does not leak into the other.
- **Stages.** Hikify is long and needs much context, so it runs in stages: analysis and planning, topology design, change planning, change execution, and verification and dry run. You can add context at each stage, and fresh adversarial agents review the change plan before any code changes.
- **Two principles.** Each container owns one concern, isolated behind clean interfaces, and the orchestrator links the systems. Each system tests only its own code with unit tests. A test that needs a display, a browser, or the whole application is an E2E test.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Decided

- **You decide the layout.** The agent proposes and you approve: the topology, the boundaries, and the change plan. Nothing is refactored before you approve the plan.
- **The clones are ignored, not submodules.** The project repository records each repository's URL and branch, not its commits.
- **`containerize-repo` and `hikify` are two skills.**

## Terms

- **Onboarding**: the work that turns a set of repositories into a hikari project.
- **Linked repository**: a repository that the topology names with a remote URL.
- **Clone folder**: `repos/` in the project repository, where hikari clones linked repositories. git ignores it.
- **Containerized project**: a project with one system for each repository, whose scope is the whole repository, and no change to the code.
- **Hikified project**: a project whose systems, libraries, and hub stories follow the approved target topology.
- **Target topology**: the topology that hikify proposes and you approve.
- **Change plan**: the steps that take a containerized project to its target topology.
- **Restructure**: a change that moves code between repositories or creates a repository. Compare with a code change inside one system.
- **Baseline**: the test results of each repository before hikify changes anything.

## What exists today

- `hikari init <dir>` writes an empty project repository: a starter `compose.yaml`, `.gitignore`, `.env.example`, and the store, with a first commit.
- A linked repository is a local path: `x-hikari.repos.<name>` has only `path` and `branch` (`internal/topology/topology.go:73`). A path such as `~/projects/itl/p3/p3-cloud` means nothing on a teammate's machine.
- `hikari up` refuses a repository with uncommitted changes, creates the hikari branch from HEAD, and checks it out. hikari never clones, fetches, or pushes.
- hikari checks a topology only when `hikari up` loads it. No command checks a draft without starting containers.
- The built-in skills are `hikari-system`, `hikari-hub`, and `hikari-tester`. All three are for agents inside a running topology. They are embedded in the binary and copied into each container.
- Releases and test results stay out of git (`project-repo.md`). A fresh clone of a project has its wikis and maps, but no releases and no images.
- On its first turn, an owner reads its code and writes its feature map. The owner can do this only after the project runs.
- I wrote the Dockerfiles and test commands of every example by hand.

## Example: P3

P3 has nine repositories under `~/projects/itl/p3`, each with a GitHub remote under `rockfield-epsilon`. They include `p3-cloud` (a FastAPI service, an MQTT consumer, a Vue console, and its own `docker-compose.yml`), `p3-edge` (C++), `itl-cv` (Python), and `itl-compulab-p3-hostkit` (Go). Seven of them have a `CLAUDE.md`, and three have a `docker-compose.yml`.

An onboarding of P3 could go like this:

1. `hikari init ~/projects/itl/p3-hikari`, then open Claude Code in it and start the onboarding skill.
2. You name the repositories. The agent writes each URL into the topology, and hikari clones them into `repos/`.
3. Containerize: one system for each repository, a Dockerfile with its toolchain, a test command, and a lead. `hikari up` runs. Each owner maps its features, and the first test runs are the baseline.
4. Hikify analysis: a fresh agent reads each repository and its map, and writes a report. You add what the code cannot show, such as "hostkit's DIO commands are about to be replaced".
5. Target topology: for example (these are guesses, not findings), an `alarm-types` library if p3-edge and p3-cloud both define alarm types, p3-cloud split into `api`, `consumer`, and `console` systems, Playwright tests moved to the tester, and hub stories for the hotspot flow.
6. Change plan: ordered steps, reviewed by fresh agents, then approved by you.
7. Execution: the restructure, then each owner's part of the plan in hikari.
8. Verification: every baseline test is kept, moved, or dropped with a reason, and the key stories pass.

---

## A. Where onboarding runs

**A1. The onboarding skills run in your own Claude Code session, on the host.** Every hikari agent runs under `hikari up`, which needs a valid topology, and onboarding starts before one exists. A host session also has what the early stages need:

- you, in the conversation, deciding as the work goes;
- access to the host, to clone repositories and run git;
- fresh subagents, for the analysis of each repository and for the adversarial review;
- a view across all repositories, which no hikari agent has by design.

The alternative is a special hikari agent that reads every repository, with a chat in the hikari UI. That needs a second runtime, and it breaks the isolation that hikari exists to keep. I recommend the host session for the stages that design. Hikari's own agents do the code changes (section G). Do you agree?

**A2. The binary carries the skills, and writes them into the project.** `hikari init` and `hikari sync` (B2) write `containerize-repo` and `hikify` into `<project>/.claude/skills/`, and git ignores that folder. A Claude Code session in the project then finds the skills, and they always match the version of the hikari binary on that machine, its topology rules included. The alternatives:

- **Versioned in the project repository.** The skills would drift from the binary that checks the topology.
- **A Claude Code plugin installed once on each machine.** The plugin's version is separate from the binary's.

Do you agree?

**A3. A new command, `hikari check <compose.yaml>`.** It loads the topology and runs every check that `hikari up` runs: topology rules, compose guards, and whether each repository is cloned and clean. It starts no container. The skills run it after each draft, and it is useful by hand too. Do you agree?

**A4. Onboarding keeps its state in `onboarding/` in the project repository, versioned.** A hikify run spans many sessions, and a session can run out of context. I recommend:

```
onboarding/
  state.md                 the stage, the decisions you made, and what is open
  analysis/<repo>.md       one report for each repository (C1)
  topology.md              the target topology, explained, with a diagram (D2)
  review/<round>.md        the findings of each adversarial review (F2)
  verification.md          the baseline, and where each baseline test went (H1)
```

A new session reads `state.md` first and continues. When onboarding ends, the folder stays as the record of why the project has its shape. The change plan itself goes into the wikis (F1). Do you agree?

## B. Linked repositories and teammates

**B1. A linked repository has a URL, and its path defaults to the clone folder.**

```yaml
x-hikari:
  project: {branch: hikari}
  repos:
    p3-cloud: {url: https://github.com/rockfield-epsilon/p3-cloud.git, branch: hikari}
    p3-edge:  {url: https://github.com/rockfield-epsilon/p3-edge.git, branch: hikari}
```

- `url` is optional. With `url` set, `path` defaults to `repos/<name>` and must be inside the clone folder.
- Without `url`, `path` is required, as today. A single-user project keeps working with no change.
- In a project repository, hikari refuses to start when git does not ignore `/repos/`, as it does for `.env`. `hikari init` adds the line.

Do you agree?

**B2. A new command, `hikari sync <compose.yaml>`, makes a clone of the project complete.** For each linked repository with a URL, it:

1. clones the repository into the clone folder when it is missing;
2. fetches, and when the remote has the hikari branch, makes a local branch that tracks it, so that a teammate starts from the agents' work;
3. reports each map in the store whose recorded commit is not in the clone, for example because a teammate did not push it.

`sync` never merges, rebases, or resets a branch that exists. It reports a branch that is behind or has diverged. The teammate's steps are `git clone <project-url>`, `hikari sync compose.yaml`, copy `.env.example` to `.env`, and `hikari up compose.yaml`. Do you agree?

**B3. `hikari up` refuses a missing clone, and names `hikari sync`.** The alternative clones on start. I recommend keeping `up` free of network access and writes outside the hikari branch, so that a start never surprises you. Do you agree?

**B4. You push, with a command that pushes everything.** hikari never pushes by itself. `hikari push <compose.yaml>` pushes the project repository's branch and the hikari branch of each linked repository. It refuses a push that is not a fast-forward, and names the repository. Without this command, a teammate's clone has the wikis but not the code those wikis describe. Do you agree?

**B5. Each teammate's agents commit to their own branch.** Two people who run agents on the same `hikari` branch make histories that diverge. I recommend a variable in `.env`, which is already per machine and ignored: `HIKARI_BRANCH=hikari-alex` replaces the hikari branch of the project and of every linked repository for that machine. People merge each other's branches with git, as they do with their own work. The alternative for this phase: only one person runs agents at a time. Which do you want?

**B6. A fresh clone republishes its releases.** A clone has no releases or images. Owners publish again on their first turns. The tester gets the newer-release notice and pins the new releases. A rebuild can make a different digest for the same version, so the tester pins again in any case. Fetching releases from their targets (GitHub releases, Docker Hub) is planted for later. Do you agree?

## C. Containerize

**C1. `containerize-repo` makes a containerized project.** For each linked repository, the skill writes:

- a system whose scope is the whole repository;
- an agent Dockerfile that installs the repository's toolchain on `hikari-agent`;
- a test command that writes JUnit XML to `/hikari/private/reports/`;
- instructions for the owner, taken from the repository's `CLAUDE.md` and its docs.

It also writes a lead and one hub, then runs `hikari check` and `hikari up`. It changes no code, and stops when every owner has mapped its features and recorded a test run. Do you agree?

**C2. A suite that cannot run in its container is a finding, not a change.** Some suites need a display, a browser, hardware, or a database. Containerize keeps every test, and changes no code or test. It records each suite that cannot run in the container, with the reason, in `onboarding/state.md`. The test command runs the suites that can run, and the owner's map shows the rest as untested. Hikify moves those suites later (G4). Do you agree?

**C3. Containerize is the first stage of hikify.** Hikify starts from a containerized project, never from bare repositories. This gives hikify three things it would otherwise have to rebuild:

- every repository builds and runs in hikari;
- the baseline test results;
- a feature map of each repository, written by the agent that owns it.

The skills stay apart, as your note asks: `hikify` refuses to start until containerize is done, and the two skills share no text. Do you agree?

## D. Hikify: analysis and topology

**D1. One fresh agent for each repository writes an analysis report.** The report reads the code, the owner's map, and the baseline. It has fixed sections:

- what the repository releases;
- what it consumes from the other repositories, and how: copied code, a package, an API, files;
- its interfaces;
- code that another repository duplicates;
- tests that are E2E by your principle (they need a display, a browser, or the whole application);
- run files such as a `docker-compose.yml`, which can become the tester's environment;
- open questions for you.

The host session then writes one synthesis across all reports, with the boundaries and libraries it proposes. You answer the open questions in the reports, or add notes in them, before the topology stage. Do you agree?

**D2. The target topology is a draft compose file, with an explanation.** The draft is `onboarding/compose.target.yaml`, checked with `hikari check`. Beside it, `onboarding/topology.md` explains each system, library, and hub in plain words, with a diagram, and gives the reason for each split or merge. You approve the explanation, not YAML. When you approve, the target replaces `compose.yaml` only when execution begins (G1). Do you agree?

**D3. The skill splits by concern, and justifies each split.** By your service-isolation principle, a system owns one concern. The skill states the reason for each boundary that it proposes, from three kinds:

- **A library:** two or more consumers need the code, or their copies already differ.
- **A client and its server:** they change at different rates, or a team owns each one.
- **A scope in the same repository:** an existing repository holds two concerns, but splitting the git history is not worth its cost.

It also names the splits that it decided against, and why. A boundary with no stated reason is not proposed. Do you agree?

**D4. You and the skill write the hub wikis; owners write their own features.** During the topology stage, the skill drafts the stories, rules, and principles of each hub with you, from the reports and your answers. You mark the key stories. Each owner's feature map is already written (C3), and after the restructure, the owners update their own maps. Do you agree?

## E. Your principles in the skills

**E1. Both principles go into the hikify skill, and into `docs/design.md`.** The skill uses them as rules for D3 and G4. `docs/design.md` gets a section of hikari's principles, so that they guide hikari's own design too. They are hikari's principles, not a project's, so they do not go into a project's wiki. Do you agree?

## F. Hikify: the change plan

**F1. The change plan is a set of plan documents in the wikis.** Plans are your documents: agents read them and never change them, and hikari commits them under your git identity. The host session works for you, so it writes:

- one plan in each hub wiki: the order of the steps across systems, and which releases unblock which steps;
- one plan in each system wiki: what that owner changes, with acceptance criteria.

Each owner then sees its part of the plan where it already looks, and the orchestrator sees the whole. Do you agree?

**F2. Fresh agents review the plan adversarially, and you see the findings.** Before you approve, two or more reviewers with no context of the session read the plan, the reports, and the code. Each one looks for:

- a step that breaks a behavior the baseline covers;
- a step that depends on a release that no earlier step makes;
- a boundary that contradicts D3;
- a test that the plan loses.

The findings go to `onboarding/review/<round>.md`. The session revises the plan, and another round runs until no finding stands. You then approve, or ask for changes. Do you agree?

**F3. Steps follow the order of releases.** A library is extracted and released before its consumers move to it. A consumer moves in its own step, and its suite must pass against the release before the old copy is deleted. No step deletes code that a consumer still uses. Do you agree?

## G. Hikify: execution

**G1. The host session does the restructure; hikari's agents do the code changes.** An owner cannot read another repository, so an owner cannot extract a library from code it cannot see. The work divides:

- **The host session (restructure):** it creates repositories, moves code with its git history (for example `git subtree split`), writes the Dockerfiles and test commands of the new systems, and replaces `compose.yaml` with the target topology.
- **Hikari's owners (code changes):** each owner makes its part of the plan in its own scope. The new library's owner shapes the code that the session moved into a clean interface and releases it. Each consumer's owner moves to the release. The tester proves the stories.

This keeps service isolation for every change to behavior. Do you agree?

**G2. The restructure commits on an `onboard` branch that you merge.** hikari's agents commit only to the hikari branch, and `hikari up` creates that branch from your HEAD. The restructure is your work, so the session commits it on `onboard` in each repository that it changes. You review it, and merge it into your branch before `hikari up`. Do you agree?

**G3. You create the remote of each new repository.** Creating a repository on GitHub acts outside your machine. The session makes the local repository and gives you the command, such as `gh repo create rockfield-epsilon/alarm-types --private --source repos/alarm-types`. With your approval in the conversation, the session runs it. Do you agree?

**G4. E2E tests move to the tester.** A suite that C2 recorded as unable to run, or that D1 names as E2E, moves to the tester's scope in a plan step. The tester rewrites it against releases. The system keeps unit tests of the same code where they make sense. The verification (H1) records where each test went. Do you agree?

**G5. The orchestrator tracks the plan.** Plans are read-only to agents, so no agent ticks off a step. The orchestrator reports progress in the hub, and owners report each finished step to it. The hub's Tests tab and each system's map show health. At verification, the session reads them and records the result. The alternative is a progress file that agents can write. I recommend the hub, because it already carries this kind of report. Do you agree?

## H. Hikify: verification

**H1. Hikify is done when the baseline is accounted for and the key stories pass.** `onboarding/verification.md` lists each baseline test, and one of:

- kept, in the system that has the code now;
- moved to the tester, linked to a story;
- dropped, with a reason that you approved.

Hikify is done when:

- every test is accounted for;
- every owner's suite passes;
- every key story is passing, or its gap is approved;
- `hikari check` reports nothing.

Do you agree?

**H2. A dry run from a fresh clone ends onboarding.** As the last step, the session clones the project repository into a temporary directory, runs `hikari sync` and `hikari check`, and starts it. This proves the promise of section B: a teammate gets the same project from one clone. Do you agree?

## I. Order of work and examples

**I1. Four steps, each with its own spec.**

1. **Linked repositories:** `url`, the clone folder, `hikari sync`, `hikari push`, `hikari check`, and per-person branches (A3, B).
2. **Containerize:** the skill, written into the project (A1, A2, A4, C).
3. **Hikify design:** analysis, topology, and change plan (D, E, F).
4. **Hikify execution and verification** (G, H).

Step 1 is useful alone: it lets a teammate clone any project today. Do you agree with the order?

**I2. A small example of "before" repositories, for hikari's own tests.** Like the sensors example, but before hikari:

- two repositories, `server` and `cli`, whose remotes are local bare repositories (`file://` URLs), so that tests need no network;
- a validation module that both copy, with one small difference between the copies;
- a browser test in `server`'s suite that cannot run in a container.

Containerize must run it with no code change, and record the browser suite. Hikify must propose a `validation` library, move the browser test to the tester, and reach H1. Do you agree?

**I3. P3 is the first real onboarding.** After step 4, the last check is an onboarding of P3 with you, from `hikari init` to H2. It replaces the P3 slice that the environments spec left open (decision 11). It reads the GitHub remotes. It pushes nothing and creates no remote without your approval. Do you agree?
