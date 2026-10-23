# Spec: the project repository

Status: draft.

Sources: `docs/brainstorm/e2e.md` (round 1, section H) and `e2e-answers.md`. Every recommendation of section H was agreed. This is the first of four steps of the E2E work.

## Summary

A hikari project can be a git repository of its own: the project repository. It holds the topology, the Dockerfiles, and the store, and later the E2E scaffold. A topology opts in with one key. Then hikari versions the knowledge in the store: maps, feature wikis, vocabularies, and your shared area. It keeps products and results out of git. hikari commits each change that an agent makes in the store through a tool, with the agent as author, on the project's hikari branch. The project repository follows the branch rules of every repo: clean at start, one hikari branch, agent commits only there. A new command, `hikari init`, writes a new project repository.

## Decisions

From round 1 and its answers:

1. A project repository holds the topology, the Dockerfiles, the store, and the E2E scaffold (H1).
2. Knowledge is versioned; products and results are not. Releases and staging areas stay out of git (H1, H2).
3. hikari commits the store changes of each tool, with the agent as author and you as committer (H3).
4. The project repository follows the branch rules of every repo (H4).
5. A system's code can live in the project repository; separate system repos stay the normal case (H5).
6. `hikari init` writes a new project repository, and yami moves to its own repository (H6).

## Decisions made in this spec

Writing exact rules needed five more choices. Each is the simplest one that keeps the rules above.

1. **A topology opts in with `x-hikari.project`.** The brainstorm said hikari finds the project repository as the repository that holds the topology file. But the examples in this repo, and `myprojects/yami`, sit inside the hikari repo, so hikari would take its own repo as their project and check out a branch in it. An explicit key avoids that. Without the key, nothing changes: the store is a plain directory, as today.
2. **hikari reads the topology again after it checks out the project branch.** The checkout can change the compose file, when the hikari branch has commits that are not on your branch. The topology of the checked-out branch decides.
3. **A test run does not make a commit.** A run changes the health in `map.yaml`. A commit for each run would fill the history with noise, so the health is committed with the system's next change, and at stop.
4. **The project repository is guarded part by part.** Today a compose service cannot bind mount anything inside a repo. In the project repository, that would also refuse a mount of a config directory next to the compose file. So for the project repository, the guard covers the store, `.git`, and the scopes of its systems, not the whole repository.
5. **`hikari init` ignores `.obsidian/` and `.DS_Store`.** Obsidian rewrites its workspace file each time you open the vault, and macOS writes `.DS_Store` files. Either would leave the repository dirty, and `hikari up` would refuse to start.

## Terms

- **Project repository**: the git repository that holds a topology that sets `x-hikari.project`.
- **Project branch**: the branch of the project repository that agents commit to.
- **Agent paths**: the store paths that one agent changes through tools: an owner's system directory, and a hub orchestrator's vocabulary.

## 1. Topology

```yaml
name: p3

x-hikari:
  project: {branch: hikari}       # the compose file's directory is a project repository
  repos:
    p3-cloud: {path: ~/code/p3-cloud, branch: hikari}
  hubs: ...

services:
  console:
    x-hikari:
      system: {repo: project, scope: console}   # a system in the project repository (rule PR5)
```

- **PR1.** `x-hikari.project` is optional. Its only field is `branch`, which is required and follows the rules of a repo's branch.
- **PR2.** With `project` set, the directory of the compose file must be the top directory of a git repository.
- **PR3.** With `project` set, the store must be inside the project repository, and not its top directory. The default, `store`, is.
- **PR4.** `project` is a reserved repo name. `x-hikari.repos` cannot name a repo `project`. A system with `repo: project` needs `x-hikari.project`.
- **PR5.** A system in the project repository has a scope that is not empty and does not overlap the store.
- **PR6.** The project repository need not hold a system. The rule that every repo is used by a system does not apply to it.
- **PR7.** For the project repository, the bind-mount guard of the compose checks covers the store, `.git`, and the scope of each system in the project repository. A service can bind mount any other path of the project repository. Every other repo stays guarded as a whole.

## 2. Start and branch

- **PR8.** `hikari up` prepares the project repository first, as it prepares every repo: it refuses uncommitted changes, creates the project branch from HEAD when the branch does not exist, and checks it out.
- **PR9.** When the checkout changed HEAD, hikari reads the topology again from the checked-out compose file. When the second topology names another project branch, or no project, hikari stops with an error that says so.
- **PR10.** When `hikari up` refuses a repo with uncommitted changes, the error names up to ten of the changed paths. This applies to every repo.
- **PR11.** With `project` set, hikari refuses to start when git does not ignore `.env` or `.hikari/` in the project repository. The error names the lines to add to `.gitignore`.

## 3. What is versioned

```
<project>/
  .gitignore                     .env, .hikari/, .obsidian/, .DS_Store (written by hikari init)
  compose.yaml
  store/
    .gitignore                   written by hikari
    README.md                    written once by hikari
    shared/                      versioned; you write it, except the vocabularies
    systems/<s>/map.yaml         versioned; hikari writes it
    systems/<s>/features/        versioned; hikari writes it
    systems/<s>/staging/         ignored
    systems/<s>/releases/        ignored
```

- **PR12.** When the store is in a project repository, hikari writes `store/.gitignore` with the lines `systems/*/staging/` and `systems/*/releases/`, and keeps it so. Outside a project repository, hikari writes no ignore file.
- **PR13.** Every other file of the store is versioned.

## 4. Commits

hikari commits the store as it commits a scope: `git add --all` and `git commit` on the given paths only, with the agent as author and you as committer, and only on the project branch.

- **PR14.** The agent paths of an owner are `systems/<owner>/map.yaml` and `systems/<owner>/features/`. The agent path of a hub's orchestrator is `shared/vocabulary/<hub>.yaml`.
- **PR15.** After each tool change, hikari commits the agent paths of the agent that made it. The tool changes are `change_features`, `set_system_info`, and `mark_map_current` for an owner, and the vocabulary tools for an orchestrator. The message names the agent and the change:

  | Tool | Message |
  |---|---|
  | `change_features` | `<owner>: change features: <what changed>`, such as `orders: change features: write create-order, rename hold to reserve-stock` |
  | `set_system_info` | `<owner>: set the system info` |
  | `mark_map_current` | `<owner>: mark the map current at <short commit>` |
  | `define_term`, `define_interface`, `remove_vocabulary_entry` | `<orchestrator>: <define or remove> <name> in the #<hub> vocabulary` |

- **PR16.** `record_test_run` does not commit. The health that it writes into `map.yaml` goes into the next commit of the owner's agent paths.
- **PR17.** When hikari stops, it commits what each agent's paths still hold, with the message `<agent>: store changes left when hikari stopped`.
- **PR18.** hikari commits the files it writes itself, `store/README.md` and `store/.gitignore`, when it creates or changes them, with the author `hikari` and the message `hikari: <add or update> <path>`.
- **PR19.** hikari never commits your shared files: policies, skills, agents, and deliverables. You commit them.
- **PR20.** A change that you make by hand in an agent's paths, such as a note in a feature document, goes into that agent's next commit, as a hand edit in a scope does today.
- **PR21.** A failed store commit does not fail the tool call. The server logs it, and the result of the tool notes it.
- **PR22.** The summary at stop has a line for the project repository: the number of store commits on the project branch since the start.

## 5. `hikari init`

```sh
hikari init ~/code/p3-project
```

- **PR23.** `hikari init <dir>` refuses a directory that exists and is not empty. It creates the directory and writes:
  - `.gitignore` with `.env`, `.hikari/`, `.obsidian/`, and `.DS_Store`;
  - `compose.yaml`: a starter topology named after the directory, with `x-hikari.project: {branch: hikari}`, one hub with a lead, and comments that show how to add a system and a repo;
  - `store/README.md` and `store/.gitignore`;
  - `.env.example` with the two credential variables, and a comment that says to copy it to `.env`.
- **PR24.** It runs `git init` with the branch `main`, and makes the first commit with your git identity. When git has no identity, it leaves the files uncommitted and says how to commit them.
- **PR25.** The project name is the directory name in lowercase, with every character that compose does not allow replaced by a dash. When the result is not a valid name, `hikari init` asks for `-name`.
- **PR26.** `hikari up <dir>/compose.yaml` starts the new project without changes, once `.env` holds a credential.

## 6. The UI

- **PR27.** The repositories of the snapshot include the project repository, named `project`, only when a system is in it. The Map view then draws it as any repository.

## 7. yami

- **PR28.** `myprojects/yami` becomes a project repository where it is: `x-hikari.project: {branch: hikari}` in its compose file, a `.gitignore` as `hikari init` writes it, `git init`, and a first commit of its files as they are. It is ignored by the hikari repo already, so the hikari repo does not change.

## 8. Docs

- **PR29.** The README describes project repositories and `hikari init`. `docs/design.md` replaces "The store is a plain directory, not a git repo" with the reasons of decisions 2, 3, and 4 of this spec. The store README text describes the ignore file.

## 9. Out of scope

- Pushing, fetching, and remotes. They are planted as "Working with remote repositories" in `docs/brainstorm/next-phase.md`.
- Stories, the tester, and the E2E scaffold. They are step 2.
- Converting a store outside a project repository into one. A store can be moved into a project repository by hand.

## 10. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Topology.** `x-hikari.project`, the reserved name, rules PR1 to PR7.
2. **Git.** Commits on several paths, missing paths skipped; the changed paths in the refusal (PR10); the ignore check (PR11).
3. **Start.** Preparing the project repository first, and reading the topology again (PR8, PR9), in `hikari up`.
4. **Store commits.** The store ignore file (PR12), and the commits of PR14 to PR22 in the systems manager.
5. **`hikari init`** (PR23 to PR26), with a test that starts the topology it writes through `topology.Parse`.
6. **UI and yami.** PR27 and PR28.
7. **Live check and docs.** A run of a project repository from `hikari init` with one owner: the wiki changes and the vocabulary changes appear as commits by their agents, a test run leaves no commit, and the next start succeeds. Then PR29.
