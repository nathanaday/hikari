# Spec: containerize

Status: built. Checked in a live run on 2026-10-23 of the onboard example: a headless Claude Code session in the project ran the skill with the two `file://` URLs and the user's approvals in the request. It linked and cloned both repositories, surveyed them with two subagents, and added `__pycache__/` and `.pytest_cache/` to each repository's `.gitignore`, the only change to their files. It wrote a Dockerfile and a system for each, left out `tests/browser` with the reason, and built each image and ran each test command once. `hikari check` passed. After `hikari up`, both owners mapped their code and recorded their suites (server 5 of 5, cli 3 of 3). The server's owner recorded the browser page as a gap of its test map. The session then wrote `onboarding/baseline.md` with every test and set the stage to `containerize: done`. Every commit of the session has the user's identity, and nothing was pushed.

Changes made during the build:

1. **A linked repo that no system uses yet is a warning** (topology). The session links the repositories in step 2 and writes their systems in step 5, but the topology refused a repo that no system used, so `hikari sync` failed. Found in the live run. A path repo that no system uses is still an error.
2. **The starter compose file sets `image: hikari-agent` on the lead only.** The anchor that every agent merges set it, so an owner that builds its own image would tag it `hikari-agent` and overwrite the base image. Found in the live run.
3. **The skill proves each image before the run** (CZ7). The session built each image and ran its test command once against the clone, on its own initiative. That finds a broken Dockerfile in seconds instead of in an owner's first turn, so step 6 now requires it.
4. **The test command writes `all.xml`.** hikari makes the system's `test` command the suite `all`, and reads its report from `all.xml`. The skill said `<suite>.xml`, the session named the report `unit.xml`, and the owner had to correct it. Found in the live run.
5. **Each host skill's folder ignores itself** (HS3 to HS5). A line in the project's `.gitignore` for each skill meant that each new host skill, such as `hikify`, broke every project made before it, until you added a line. Now `hikari` writes a `.gitignore` with `*` into each skill's folder, so git ignores the folder with no line at the top, and a skill of your own in `.claude/skills/` stays versioned. Found when `hikify` was added.

Sources: `docs/brainstorm/onboarding.md` (round 1, questions A1, A2, A4, C1 to C3, and I2) and `onboarding-answers.md`. This is the second of four steps of the onboarding work, after linked repositories.

## Summary

`containerize-repo` is a Claude Code skill that you run in your own session, in a project repository. You name your repositories, and the skill does the rest with you:

1. It links each repository by its URL and clones it.
2. It surveys each repository: its toolchain, its tests, and what its tests need.
3. It writes one system for each repository, with an agent Dockerfile and a test command, plus a lead and a hub.
4. You start hikari. Each owner maps its repository and records its tests.
5. The skill writes the baseline: every test, with its outcome.

The skill changes no code. The hikari binary carries the skill, and `hikari init` and `hikari sync` write it into the project's `.claude/skills/`, so the skill always matches the binary. A new command, `hikari status`, reads each system's map and recorded tests without starting anything. The skill uses it to know when the owners are done, and to write the baseline.

## Decisions

From round 1 and its answers:

1. The onboarding skills run in your Claude Code session on the host (A1).
2. The binary carries the skills, and writes them into the project's `.claude/skills/`, ignored by git (A2).
3. Onboarding keeps its state in `onboarding/` in the project repository, versioned (A4).
4. `containerize-repo` makes a containerized project: one system for each repository, scope the whole repository, an agent Dockerfile, a test command, instructions from the repository's `CLAUDE.md`, a lead, and one hub. It changes no code (C1).
5. A suite that cannot run in its container is a finding, not a change (C2).
6. Containerize is the first stage of hikify (C3).
7. A small example of "before" repositories, for hikari's own tests (I2).

## Decisions made in this spec

1. **The skills keep the names of your note: `containerize-repo` and `hikify`.** Only their folders are ignored, so a skill of your own in `.claude/skills/` stays versioned.
2. **`hikari status` reads files only.** It reads each map in the store and each owner's results file, so it works while hikari runs, and when it does not. It does not compute the map's state as the server does, which needs the code maps and hashes. It shows facts: the commit that the map describes, the features with their recorded health, and the outcome of each recorded test.
3. **You start hikari, not the skill.** `hikari up` is a long-running process with a dashboard and the UI's link, and it is yours. The skill tells you when to start it, and waits with `hikari status` until each owner is done.
4. **The skill commits its own files at once, as you.** These are `compose.yaml`, the Dockerfiles, and `onboarding/`. The skill commits them with your git identity, on the branch that is checked out. A file left uncommitted would stop the next `hikari up`.
5. **The only change in a repository is an ignore line, and only with your approval.** A repository that does not ignore its build products makes each machine commit them again, as the live run of linked repositories showed with `__pycache__`. Before the first start, the skill proposes the lines. With your approval, it commits them in that repository with your identity, and it changes nothing else.
6. **`hikari init` also ignores `.claude/settings.local.json`.** Claude Code writes that file when you approve a permission in the project. As an untracked file, it would stop `hikari up`.

## Terms

- **Host skill**: a skill that the hikari binary writes into a project for your own Claude Code session, not for an agent's container.
- **Baseline**: the outcome of every test of each system at the end of containerize, in `onboarding/baseline.md`.
- **Survey**: the skill's reading of one repository before it writes the system.

## 1. Host skills

- **HS1.** The binary embeds the host skills: `containerize-repo`, and later `hikify`.
- **HS2.** `hikari init` and `hikari sync` write each host skill into `<project>/.claude/skills/<name>/`, and replace what is there. A host skill belongs to hikari: an edit to it is lost at the next sync.
- **HS3.** Each host skill's folder holds a `.gitignore` with `*`, so that git ignores the folder, that file included. `hikari init` also ignores `/.claude/settings.local.json`.

## 2. `hikari status`

```
hikari status [-tests] <compose.yaml>
```

- **SU1.** For each owner, `status` prints one line: the commit that the map describes, or `not described`; the number of features; and the count of features for each recorded health.
- **SU2.** Below each owner, it prints each suite of the results file, with the time of its run and the count of each outcome. With no results file, it prints `no test results on this machine`.
- **SU3.** With `-tests`, it also prints each test of each suite, with its outcome, sorted by id.
- **SU4.** `status` reads files only: it writes nothing, starts nothing, and needs no server. A missing clone is printed as `not cloned`.

## 3. `onboarding/`

- **OB1.** `onboarding/state.md` holds the stage of onboarding (`containerize`, then the stages of hikify), the repositories with their survey findings, the decisions that you made, and the open questions. A new session reads it first.
- **OB2.** `onboarding/baseline.md` holds the output of `hikari status -tests`, with the time and the commit of each repository.
- **OB3.** The skill commits each change to `onboarding/`, `compose.yaml`, and the Dockerfiles at once, with your git identity, on the branch that is checked out (decision 4).

## 4. The procedure

The skill's procedure, in order. Where you gave an answer in your request, the skill uses it and does not ask again.

- **CZ1.** **Start.** Read `onboarding/state.md` when it exists, and continue from its stage. Otherwise, ask for the repositories: a URL for each, and a sentence on what each one does.
- **CZ2.** **Link.** Write each repository under `x-hikari.repos` with its `url`. Run `hikari sync`, and stop on a failed clone.
- **CZ3.** **Survey.** For each repository, a fresh subagent reads it and returns:
  - the languages, build files, and versions of its toolchain;
  - the commands that run its tests, and how each can write a JUnit XML report;
  - each suite that needs a display, a browser, hardware, a database, network access, or credentials;
  - the build products that git does not ignore;
  - its `CLAUDE.md` and contributor docs;
  - its run files, such as a `docker-compose.yml` or a Dockerfile.
- **CZ4.** **Ignore lines.** For each repository whose build products are not ignored, propose the lines. With your approval, commit them in that repository (decision 5).
- **CZ5.** **Write the systems.** For each repository, write:
  - `<repo>.Dockerfile`, from `hikari-agent`, which installs the toolchain as root, then switches back to `USER agent`;
  - a service with `x-hikari.system: {repo: <repo>, test: ...}`, whose scope is the whole repository;
  - a test command that runs every suite that can run in the container, and writes JUnit XML to `/hikari/private/reports/`;
  - a named volume, mounted inside the scope, for each directory of dependencies, such as `node_modules`;
  - a `description`, and `instructions` drawn from the repository's `CLAUDE.md`, with no secret and no path outside the repository.
- **CZ6.** **The hub.** Write a `lead` and one hub, `main`, whose members are the owners.
- **CZ7.** **Check.** Run `hikari check`, and fix each finding. Build each image, and run its test command once against the clone. Show you the systems, the suites that cannot run with the reason, and the ignore lines. Commit.
- **CZ8.** **Run.** Ask you to start `hikari up compose.yaml`. Wait with `hikari status`, until each owner's map is described and each owner has recorded a run of its suites. A suite that fails is a finding of the baseline, not a reason to change code.
- **CZ9.** **Baseline.** Write `onboarding/baseline.md` (OB2), and record in `state.md` that containerize is done, with the date. Commit.
- **CZ10.** **Never.** The skill never edits a file in a clone, except the ignore lines of CZ4. It never pushes, and never creates a remote repository.

## 5. The example

- **EX1.** `examples/onboard` holds two "before" repositories:
  - `server`: a Python HTTP service with an `accounts` module that checks email addresses, unit tests, and a browser test that needs Playwright, which the agent image lacks;
  - `cli`: a Python command that copies the same `accounts` module, with one difference: the server lowercases the address, and the command does not.
- **EX2.** `make example-onboard` writes the two repositories as bare remotes in `.hikari/demo/onboard/remotes/`, and an empty project from `hikari init` in `.hikari/demo/onboard/project/`.
- **EX3.** Containerize must run the example with no code change, exclude the browser test from the test command and name it in `state.md`, and write a baseline with every unit test. Hikify (steps 3 and 4) must find the copied module and propose a library.

## 6. Out of scope

- Hikify: analysis, target topology, change plan, and execution.
- Repositories that need credentials beyond your machine's git settings.
- Agent images for platforms other than the host's.

## 7. Build plan

1. **Host skills and init.** The embedded skills, their writing in `init` and `sync`, and the ignore rules (HS1 to HS5), with tests.
2. **`hikari status`** (SU1 to SU4), with tests.
3. **The skill.** `containerize-repo`'s `SKILL.md` and its references (OB1 to OB3, CZ1 to CZ10).
4. **The example** (EX1, EX2).
5. **Live check and docs.** Run the skill headless in Claude Code on the example, with the answers in the request. Start hikari, let the skill write the baseline, and check EX3. Then update the README, `docs/design.md`, and this spec's status.
