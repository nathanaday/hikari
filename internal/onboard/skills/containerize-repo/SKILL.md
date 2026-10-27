---
name: containerize-repo
description: Wrap existing git repositories into a hikari project, one partition for each repository, with no change to their code. Use in a hikari project repository when the user wants to bring repositories into hikari, link repos, containerize repos, or start onboarding. Refactoring the repositories for hikari is the hikify skill, which starts after this one.
---

# Containerize repositories

You work for the user in their own Claude Code session, in a hikari project repository (the directory with `topology.yaml`, made by `hikari init`). The result is a containerized project: one hikari partition for each repository, with one folder, the whole repository, and no change to any code. Each partition agent then maps its repository and records its tests, and those results become the baseline that the hikify skill starts from.

The user is the authority on the project. Propose, show, and ask; do not decide for them what they did not say. Where the request already answers a question, use the answer and do not ask again.

## Rules

1. Change no file inside a clone in `repos/`, except the ignore lines of step 4, and only with the user's approval.
2. Never push, never create a remote repository, and never force a git operation.
3. Commit each change to `topology.yaml`, the Dockerfiles, and `onboarding/` at once, with the user's git identity (a plain `git commit`, no author option), on the branch that is checked out. An uncommitted file stops the next `hikari up`.
4. Put no secret in any file: no token, password, or key from a repository, its `.env`, or the user's machine.
5. Keep `onboarding/state.md` current after each step, so that a new session can continue from it.

## Procedure

### 1. Start

Read `onboarding/state.md` if it exists, and continue from its stage. Otherwise ask the user for:

- each repository: its remote URL, and one sentence on what it does;
- the project's purpose in one sentence, for the orchestration agent's description.

Write `onboarding/state.md` from `references/state.md`, and commit.

### 2. Link

Add each repository to `x-hikari.repos` in `topology.yaml` with its `url` and no `path` or `branch`. A linked repo's clone is `repos/<name>`, and its branch is the project branch. Until step 5 writes its partition, `hikari` warns that no partition uses it; that is expected. Use the repository's own name, in lowercase letters, digits, and dashes. Then run:

```sh
hikari sync topology.yaml
```

Stop and tell the user if a clone fails. Their machine's git settings decide access.

### 3. Survey

For each repository, send one fresh subagent with the brief in `references/survey.md`, all in one message so they run in parallel. Each subagent reads one repository and returns its findings. Write the findings of each repository into `state.md` under its name, and commit.

### 4. Ignore lines

A repository that does not ignore its build products (`__pycache__/`, `node_modules/`, `target/`, `build/`, `.venv/`, and so on) makes each machine commit them again. For each such repository, show the user the lines to add to its `.gitignore`, and ask. With approval, add them and commit in that repository (`git -C repos/<name> commit`), on the branch that is checked out there. This is the only change containerize makes in a repository.

### 5. Write the partitions

Read `references/topology.md`. For each repository, write:

- `<name>.Dockerfile` in the project, from `hikari-agent`, which installs the toolchain that the survey found;
- a service in `topology.yaml` with a `description`, `instructions`, and a partition of one folder, the whole repository: `partition: {folders: [{repo: <name>}], test: <command>}`. A folder with no `path` is the whole repository, and the agent sees it at `/workspace/<name>`.

The test command runs every suite that can run in the container, and writes one JUnit XML report to `/hikari/private/reports/all.xml`. hikari makes the command the partition agent's first suite, `all`, and reads its report from that path. The partition agent can split it into more suites later. Leave out each suite that needs a display, a browser, hardware, a service that the container lacks, network access, or credentials. Do not change the suite to make it run. Name each suite that you left out, with the reason, in the partition agent's `instructions` and in `state.md`. hikify records them as left for integration; they stay in the repository.

Write `instructions` from the repository's `CLAUDE.md` and contributor docs: the conventions an agent must follow in that code. Leave out what only applies to a person's machine. Add the suites that the partition agent must not run, and why.

Then write the orchestration agent, and name it in `x-hikari.orchestration`, as `references/topology.md` shows.

### 6. Check

Run `hikari check topology.yaml`, and fix each finding. Then prove each image before the partition agents need it. Build it, and run its test command once against the clone:

```sh
docker compose -f topology.yaml build <service>
docker run --rm -v "$PWD/repos/<name>:/workspace/<name>" -v "$(mktemp -d):/hikari/private/reports" \
  -w /workspace <project>-<service> sh -c '<test command>'
```

`<project>` is the topology file's `name`. Fix the Dockerfile or the test command until the run reaches the tests. A failing test is a finding, not a reason to change code. Show the user:

- the partitions, with the toolchain of each image;
- the test command of each partition;
- the suites left out, with the reasons;
- anything the survey could not decide.

When the user agrees, commit, and set the stage in `state.md` to `containerize: run`.

### 7. Run

Ask the user to start hikari in a terminal of their own and leave it running:

```sh
hikari up topology.yaml
```

The first start builds each image. On their first turns, the partition agents read their code, write their wikis and maps, and run and record their suites. Check progress with:

```sh
hikari status topology.yaml
```

A partition agent is done when its map is described and its suites have a recorded run. Wait between checks; a large repository takes many minutes. When an image fails to build or an agent cannot run its tests, `hikari up` shows it, and the user can tell you. Fix the Dockerfile or the test command, commit, and ask the user to restart hikari. A test that fails is a finding of the baseline, not a reason to change code.

### 8. Baseline

When every partition agent is done, write `onboarding/baseline.md`:

- the date, and the commit of each repository's hikari branch (`git -C repos/<name> rev-parse HEAD`);
- the output of `hikari status -tests topology.yaml`, in a code block;
- for each partition, the failing tests and the suites left out, from `state.md`.

Set the stage in `state.md` to `containerize: done`, with the date, and commit. Tell the user that the project is containerized. Summarize the baseline in a few lines. Name the hikify skill as the next step, which refactors the repositories against a target topology.

## References

- `references/topology.md`: the topology file of a containerized project.
- `references/survey.md`: the brief for each survey subagent.
- `references/state.md`: the form of `onboarding/state.md`.
