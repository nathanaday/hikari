# Spec: partitions of many folders

Status: approved.

Sources: `docs/brainstorm/shared-language.md` (questions E1 to E8, and G2 for folders) and `shared-language-answers.md`. This is step 5 of the seven steps of the shared language. The names are those of `docs/glossary.md`.

## Summary

A partition is a list of folders, each a path in any repository of the project, writable or read-only. The partition agent sees every folder under `/workspace`, at `/workspace/<repo>/<path>` unless the folder sets `at:`, and works in it as in one monorepo. An apply commits every writable folder, one commit per repository, on that repository's branch, and Hikari still applies at the end of each turn and at stop. The private files live in the first writable folder. Paths in the code map are workspace paths, and a feature map records one commit per repository.

## Decisions

From the round and its answers:

1. A partition is `folders`, each with `repo`, `path`, and optional `read_only` (E1).
2. A folder mounts at `/workspace/<repo>/<path>` unless it sets `at:`; the working directory is `/workspace` (E2, agreed).
3. Folders keep today's isolation rules (E3).
4. The private files live in the first writable folder, and the code map's paths are workspace paths (E4).
5. An apply commits every writable folder, one commit per repository, and Hikari applies at the end of each turn and at stop (E5, agreed).
6. Branches stay one per repository (E6).
7. Secrets stay compose's `env_file` and `environment` (E7, agreed).
8. A repository linked by URL is still cloned into `repos/<name>` (E8).

## Decisions made in this spec

1. **A folder's default place uses the repository's name.** The agent sees names, never ids. A rename of a repository in the editor therefore moves the folders that use the default place, and the plan of the rename lists each partition whose workspace moves.
2. **A partition has at least one writable folder.** The private files and the commits need one.
3. **A feature map and each feature document record one commit per repository,** as a list of `<repo>@<commit>`, in the order of the partition's writable folders. A partition of one folder records one entry. The map's format version goes up by one.
4. **A change in a read-only folder does not make a feature stale.** The read-only code belongs to no feature of the partition, as today's read paths do not.
5. **`scope` and `read` are gone, and the strict decoder refuses them.** A partition of today becomes one writable folder for its scope and one read-only folder for each read path.

## Requirements

### The topology

- **F1.** A partition is:

```yaml
partition:
  folders:
    - {repo: shop, path: services/orders}
    - {repo: shop, path: libs/money, read_only: true}
    - {repo: protos, path: orders, at: protos}
  test: python3 -m pytest -q shop/services/orders/tests --junitxml=/hikari/private/reports/all.xml
```

`repo` names a repository by id. `path` is relative to the repository's root; empty means the whole repository. `at` is a relative path under `/workspace`. `test` runs in `/workspace`.

- **F2.** A partition has at least one folder without `read_only`. No two folders of one partition have places that nest or are equal, and no place is `.hikari` or lies under it.
- **F3.** No two partitions have writable folders that nest or are equal in one repository. No folder of a partition, read-only or not, lies inside another partition's writable folder. Read-only folders of different partitions may overlap. A folder in the project repository cannot overlap the vault or `repos/`.

### Mounts

- **F4.** Each folder mounts at `/workspace/<place>`, writable or read-only. A folder that is a whole repository has an empty read-only directory over its `.git`. Every writable folder has an empty read-only directory over its `.hikari`, and the first writable folder's `.hikari/private` mounts at `/hikari/private`. The working directory is `/workspace`.
- **F5.** A named volume or tmpfs of the compose service may sit inside a writable folder's place, as inside a scope today, and not under `.hikari` or `.git`.

### Private files and health

- **F6.** The private files live in `<first writable folder>/.hikari/private/`. Paths in the code map, keys in the test map, and the files of a JUnit report are workspace paths, such as `shop/services/orders/app.py`. Hikari maps a workspace path to its folder on the host to hash it and to find what changed.
- **F7.** A feature is stale when a path of its code in a writable folder changed after the commit that its document records for that folder's repository. The changed paths of each writable folder are listed in the agent's next prompt, as workspace paths.
- **F8.** `map.yaml` records `described_state.commits` as a list of `<repo>@<commit>`, and each feature document records `described_commits` in the same form. Hikari reads a map or document of the format before, with one commit, as a partition of one folder in the repository of its first writable folder, and writes the new form at the next change.

### Commits

- **F9.** An apply commits the writable folders of each repository, one commit per repository, on that repository's branch, with the agent as author and one message. It commits nothing in a repository whose folders did not change. The end of each turn and the stop apply.
- **F10.** `hikari up` refuses a repository whose writable folders hold uncommitted changes, as it refused a dirty scope. It prepares each repository's branch once, however many folders use it.

### Agents

- **F11.** The partition agent's prompt lists its workspace: each folder's place, its repository, and whether it is read-only. The skills say workspace paths.
- **F12.** `get_my_partition` and the orchestration agent's `get_partition` show the folders by place, with no host path.

### Editor and UI

- **F13.** `set_partition` takes `folders` and `test`. The operations' plan lists the partitions whose workspace moves when a repository is renamed (decision 1).
- **F14.** The partition agent's inspector edits the folder list: a row for each folder, with a repository picker, the path, a read-only switch, and `at`, and buttons to add, remove, and reorder rows. The first writable row is marked as the home of the private files.
- **F15.** The partition agent's page shows the folders and, for each repository, its branch and whether it is dirty.

### Host skills, examples, docs

- **F16.** `containerize-repo` writes each repository as one partition with one folder, the whole repository. `hikify` designs partitions as folder lists, and may give a partition folders from more than one repository.
- **F17.** `examples/shop` uses folders, and its test commands use workspace paths. The `onboard` example's documents follow.
- **F18.** `README.md`, `docs/design.md` ("The repo stays on the host", "Only the server writes to git"), `docs/guides/vault.md`, and `docs/guides/onboarding.md` describe folders, places, and apply.

### Checks

- **F19.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover a partition with folders from two repositories: its mounts, a stale feature in each repository, an apply that makes one commit in each repository and none in an unchanged one, and the reading of a one-commit map.
- **F20.** A live check of a project of two repositories, under `.hikari/demo/`, where one partition takes a folder from each: `hikari check` passes; the override file mounts both folders at their places; an apply through the agent API commits in both repositories; and the inspector edits the folders.
