# Spec: linked repositories

Status: built. Checked in a live run on 2026-10-23 with a project from `hikari init` and two linked repos, `greet` and `hello`, on local bare remotes. `hikari check` named both missing clones, and `hikari sync` cloned them. Alex's run committed the owners' maps on `hikari`, and `hikari push` pushed both repos, then the project. Bob cloned the project, ran `hikari sync -from hikari` with `HIKARI_BRANCH=hikari-bob`, and his run started on `hikari-bob` from Alex's maps. A third clone found that a fresh clone dropped every health to pending and committed the drop; after the fix (change 2), Carol's owners took a restore turn of 7 to 10 seconds, ran their suites, and the maps changed only by the time of the run. `hikari up` on a clone that was not synced refused to start and started no container.

Changes made during the build:

1. **The first line of `git status` keeps its leading space.** The git helper trimmed the output on both sides, so the status line of a modified file lost the first letter of its path, and the refusal of PR10 named `gitignore` for `.gitignore`. It now trims the end only. Found in the tests.
2. **A fresh clone keeps the recorded health, and owners restore what git does not version** (SY10, SY11). Found in the live run: Bob's clone had no test results, so every feature became pending, and hikari committed the drop to his branch. A merge of that branch would have erased Alex's health. B6 also assumed that owners publish again on their first turns, but nothing told them to.
3. **A command that the user copies names the compose file as typed.** The instruction `run: hikari sync <file>` names the file relative to the working directory when it is inside it.
4. **Observed for the containerize step:** the sample repos had no `.gitignore`, so the owners committed `__pycache__` files, and each machine commits them again. Containerize must report a repo that does not ignore its build products.

Sources: `docs/brainstorm/onboarding.md` (round 1, questions A3 and B1 to B6) and `onboarding-answers.md`. This is the first of four steps of the onboarding work.

## Summary

A topology can name a repository by its remote URL. hikari keeps the clone of a linked repository in `repos/<name>` in the project repository, and git ignores that folder. Three new commands work on a whole project:

- `hikari sync` clones what is missing and makes local branches that track the remote's hikari branches. It reports what a teammate has not pushed, and it never changes a branch that exists.
- `hikari check` runs every check of `hikari up` and starts nothing.
- `hikari push` pushes the hikari branch of each linked repository and of the project repository.

A teammate clones the project repository, runs `hikari sync`, and has the same project. Each person's agents commit to that person's branch, which `.env` sets.

## Decisions

From round 1 and its answers:

1. A linked repository has a URL, and its clone is in the clone folder (B1).
2. `hikari sync` makes a clone of the project complete, and never merges, rebases, or resets (B2).
3. `hikari up` refuses a missing clone and names `hikari sync` (B3).
4. You push with `hikari push`; hikari never pushes by itself (B4).
5. Each teammate's agents commit to their own branch, set in `.env` (B5).
6. A fresh clone republishes its releases (B6).
7. `hikari check` runs the checks of `hikari up` and starts nothing (A3).

## Decisions made in this spec

1. **The branch comes from compose interpolation, not from a hikari variable.** Compose reads `.env` next to the compose file and substitutes `${VAR}` in every key, `x-hikari` too. So `branch: ${HIKARI_BRANCH:-hikari}` gives each person their own branch, with no new code to read `.env`, and the topology shows where the value comes from. `hikari init` writes it this way.
2. **A linked repository's branch defaults to the project branch.** With one `${HIKARI_BRANCH:-hikari}` in `x-hikari.project`, every repo follows it. A repo can still name its own branch.
3. **A linked repository needs a project repository.** The clone folder is a folder of the project repository, and its ignore rule is checked there. A topology without a project keeps path repositories, as today.
4. **A linked repository has no `path`.** Its clone is always `repos/<name>`, so every teammate has the same layout. A `path` beside `url` is an error, not a second way to say the same thing.
5. **A missing clone is not an error of the topology.** `hikari sync` must read a topology whose clones do not exist yet. The topology records the missing clones, and skips the checks that need their files. `hikari check` and `hikari up` refuse to go on while a clone is missing.
6. **`hikari sync -from <branch>` starts your branch from a teammate's.** A new hikari branch starts from HEAD, which is the default branch of a fresh clone. With `-from`, `sync` creates your branch in each repository from the remote's `<branch>`, so you start from a teammate's agents' work. It does this only where your branch does not exist yet.
7. **`push` pushes the linked repositories first, and the project repository last.** A map in the store records the commit that it describes. When the project repository is pushed last, a teammate never gets a map whose commit is not on the remote yet.
8. **Only the project repository and linked repositories are pushed and synced.** A path repository is on your machine only, so `sync` and `push` leave it alone, and `sync` warns that a teammate cannot get it.

## Terms

- **Linked repository**: a repository of `x-hikari.repos` with a `url`.
- **Path repository**: a repository of `x-hikari.repos` with a `path`, as before.
- **Clone folder**: `repos/` at the top of the project repository.
- **Your branch**: the hikari branch of a repository, after compose interpolation. `HIKARI_BRANCH` in `.env` sets it for a person.

## 1. Topology

```yaml
name: p3

x-hikari:
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  repos:
    p3-cloud: {url: https://github.com/rockfield-epsilon/p3-cloud.git}
    p3-edge:  {url: git@github.com:rockfield-epsilon/p3-edge.git}
    tools:    {path: ~/code/tools, branch: hikari}     # a path repository, as before
```

- **LR1.** A repo of `x-hikari.repos` has either `url` or `path`, not both, and not neither.
- **LR2.** A repo with `url` needs `x-hikari.project`. Its path is `repos/<name>` in the project repository.
- **LR3.** A `url` is not empty, has no spaces or control characters, and does not start with `-`. hikari passes it to git after `--`, so that it is never read as an option.
- **LR4.** A linked repository without `branch` uses the project branch. A path repository still needs `branch`.
- **LR5.** A system in the project repository has a scope that does not overlap the clone folder.
- **LR6.** When a linked repository's clone is missing, the topology lists the repository as missing, and skips the checks of the scopes and read paths of its systems. This is not an error of the topology.
- **LR7.** With a linked repository, hikari refuses to start when git does not ignore `/repos/` at the top of the project repository, as it does for `.env` (rule PR11 of the project repository spec).

## 2. `hikari sync`

```
hikari sync [-from <branch>] <compose.yaml>
```

For the project repository, and then for each linked repository, `sync` does the steps below and prints one line for each repository.

- **SY1.** **Clone.** When the clone of a linked repository is missing, `sync` runs `git clone -- <url> repos/<name>`. A failed clone is reported, and `sync` goes on with the other repositories.
- **SY2.** **Check the remote.** When the clone exists, its `origin` must be the topology's `url`. A clone with another origin is reported and left alone.
- **SY3.** **Fetch.** `sync` runs `git fetch origin` in each repository. The project repository is fetched only when it has an `origin`.
- **SY4.** **Track.** When your branch does not exist locally and `origin` has it, `sync` makes the local branch from `origin/<branch>`, with `origin/<branch>` as its upstream. With `-from <other>`, when neither the local branch nor `origin/<branch>` exists, `sync` makes your branch from `origin/<other>`.
- **SY5.** **Report.** When your branch exists, `sync` compares it with `origin/<branch>` and reports one of: up to date, ahead by n, behind by n, diverged (n ahead, m behind), or not on the remote. It never merges, rebases, resets, or checks out.
- **SY6.** **Maps.** For each system, `sync` reads the commit that its map records. When that commit is not in the system's repository, `sync` reports the system, the commit, and the repository: a teammate may not have pushed it.
- **SY7.** **Path repositories.** For each path repository, `sync` warns that a teammate's clone cannot get it, and does nothing else.
- **SY8.** `sync` exits with status 1 when a repository could not be cloned, has another origin, or a map's commit is missing. Being behind or ahead is not a failure.
- **SY9.** `sync` needs no `.env`. Compose warns about unset variables, and `sync` shows the warnings.
- **SY10.** **Test results.** An owner whose map records test runs, but whose scope has no results file, has run no tests on this machine. The map keeps the recorded tests and health of each feature until a run on this machine records results. The owner's first turn runs its suites and records them, as a restore turn when nothing else starts one.
- **SY11.** **Releases.** When a tester pins a release that this machine does not have, the owner's first turn publishes it again: at the same version when its code has not changed since, else at a new version. Then the owner sends the tester a direct message. The tester takes no restore turn while its pins are missing. The message wakes it, and it pins again and runs its tests.

## 3. `hikari check`

```
hikari check <compose.yaml>
```

- **CK1.** `check` loads the topology as `hikari up` does, and reports each finding, in this order:
  1. the topology's errors and warnings;
  2. each missing clone, with the instruction to run `hikari sync`;
  3. the project repository's ignore rules (`.env`, `/.hikari/`, `/repos/`);
  4. each repository with uncommitted changes, with up to ten of the changed paths. In the project repository, changes only to the wikis are not a finding, because `hikari up` commits them for you (rule C5 of the wikis spec);
  5. the credentials of each agent.
- **CK2.** `check` writes nothing, starts nothing, and makes no network call.
- **CK3.** `check` exits with status 1 when it reports anything but warnings, and prints one line that says that the project can start otherwise.
- **CK4.** `hikari up` runs the same checks, in the same order, before it prepares any repository. A missing clone stops it with the text of CK1.

## 4. `hikari push`

```
hikari push <compose.yaml>
```

- **PS1.** `push` pushes your branch of each linked repository, then your branch of the project repository, to `origin`. It uses the refspec `refs/heads/<branch>:refs/heads/<branch>`, and never forces.
- **PS2.** A repository is skipped, with the reason, when your branch does not exist locally or the repository has no `origin`.
- **PS3.** A push that git refuses, for example because it is not a fast-forward, is reported with the repository and git's reason. `push` then goes on, but it does not push the project repository, because its maps may describe commits that are not on the remote (decision 7).
- **PS4.** `push` prints one line for each repository: pushed n commits, up to date, skipped, or refused. It exits with status 1 when a push was refused.
- **PS5.** `push` sets the upstream of a branch that had none, so that `sync` and git can compare it with the remote.

## 5. `hikari init`

- **IN1.** The project's `.gitignore` also lists `/repos/`.
- **IN2.** The starter compose file sets `project: {branch: "${HIKARI_BRANCH:-hikari}"}`, and its comment shows a linked repository: `app: {url: https://github.com/you/app.git}`.
- **IN3.** `.env.example` explains `HIKARI_BRANCH`: set it to your own branch name, such as `hikari-alex`, when more than one person runs agents for the project.

## 6. A teammate's steps

The README gets this procedure:

1. `git clone <project-url> myproject`
2. `hikari sync myproject/compose.yaml`, or `hikari sync -from hikari-alex myproject/compose.yaml` to start from a teammate's work.
3. Copy `.env.example` to `.env`. Set a credential, and set `HIKARI_BRANCH` to your own branch.
4. `hikari check myproject/compose.yaml`
5. `hikari up myproject/compose.yaml`. Each owner publishes its releases again on its first turns (B6).

## 7. Out of scope

- Fetching releases from their targets.
- Merging teammates' branches. People do this with git.
- Creating remote repositories (step 4 of the onboarding work).
- Credentials for git. `sync` and `push` use the git and SSH settings of your machine.

## 8. Build plan

1. **git operations.** In `internal/gitrepo`: clone, fetch, the origin's URL, a tracking branch, ahead and behind counts, and push, with tests against local bare repositories.
2. **Topology.** LR1 to LR7, with tests.
3. **Commands.** `sync`, `check`, `push`, the shared checks of `up` (CK4), and `init` (IN1 to IN3), with tests that use `file://` remotes.
4. **Live check and docs.** Make a project with two linked repositories on local bare remotes. Run it, push, clone it as a teammate in another directory, sync with `-from`, check, and start it on the teammate's branch. Then update the README and `docs/design.md`, and this spec's status.
