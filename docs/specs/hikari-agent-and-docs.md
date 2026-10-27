# Spec: the Hikari agent, the docs, and the migration

Status: approved.
M1–M6 and M9–M12 built; M7–M8 pending.
Changes made during the build:

1. **The old topology keys name their move.** `x-hikari.store`, `x-hikari.hubs`, and a service's `system` and `tests` failed with the decoder's "field not found"; each now fails with the move and the section of the guide. Every refusal of an old file, key, or vault directory ends with `docs/guides/migrating.md#<section>`, through `vault.Migrating`.
2. **`hikari check` also refuses `hubs/` in the vault, and a vocabulary file without `terms.yaml`.** V11 listed neither, and a start would silently lose the stories and the terms (M5, step 8).
3. **A migrated test command starts with `cd <scope> &&`.** Decision 2 keeps the paths, but a partition's commands run in `/workspace`, where a system's ran in its scope; the suites of the test map take the same `cd`. A system with no scope has no `at:` that keeps it at `/workspace`, so it mounts at `/workspace/<repo>`, and its volumes and instructions take `<repo>/`.
4. **Only file and directory keys of the test map take the place.** An id key stays, because the runner names a test the same way after the `cd`.
5. **The guide's vault script uses `mv` and `git add -A vault`,** so that ignored releases and staging areas move too. It writes the new `.gitignore`. The new README comes from a scratch `hikari init`: hikari writes a README only into a vault without one, and one written at start would be an uncommitted file that blocks the next start.
6. **The partition plan is `hikify-plan-<id>.md`** in `onboarding/vault/partitions/<id>/read-only/features/`. The restructure copies with `cp -R onboarding/vault/. vault/`, and `git status` shows any vault file that a draft replaced.
7. **M4 is a test, `TestNamesBuilt`:** each backquoted snake-case name of a host skill is a tool of `internal/mcpserver`, each `hikari-` name a built-in skill or the image, each `hikari <command>` a command, and no skill drafts in `onboarding/wiki/` or `onboarding/shared/`.
8. **The vault's README no longer names stories.** The sweep found it outside the files of M10.
9. **`docs/design.md` was rewrapped paragraph by paragraph:** only the paragraphs with a line over 78 columns, with no word changed. `containerize.md` names the orchestrator where it named the lead, and keeps its other old words as history.
10. **The live check (M12) ran without Docker.** On `make example-onboard`, the topology written from `containerize-repo`'s reference passed `hikari check` after `hikari sync`, and a target from hikify's reference, with the library `accounts` to create and a partition `signup` of the whole `server` and `cli` repositories, passed `hikari check -draft`. No folder may lie inside another partition's writable folder, and each repository of the example keeps its code at its root, so the partition of both repositories takes both whole. The guide was rehearsed on a scratch copy of yami's old project: `hikari check` named each move, passed after them, and `hikari up -no-launch` converted both maps and committed them at stop. No image was built and no agent turn ran.

Sources: `docs/brainstorm/shared-language.md` (questions C4, F5, and the "Decided" note on "lead") and `shared-language-answers.md`. This is the last of the seven steps of the shared language. The names are those of `docs/glossary.md`.

## Summary

The host skills of the Hikari agent learn the model that steps 1 to 6 built: `containerize-repo` writes partitions and one orchestration agent, and `hikify` designs partitions of folders and drafts the orchestration feature map. A migration guide lists every move from a project of before the shared language, and your two projects, `yami` and `cs566-project`, are migrated by following it. The docs get one last pass for the glossary's words and for line width.

## Decisions

From the round and its answers:

1. Hikari ships no migrator; the guide lists the moves, and your two projects are migrated by hand (C4).
2. `containerize-repo` wraps each repository as one partition with one folder and writes one orchestration agent. `hikify` designs partitions of folders, drafts the orchestration feature map instead of stories, records E2E suites as left for integration, and drafts in `onboarding/vault/` (F5).
3. The stale mention of "lead" in `docs/specs/containerize.md` goes ("Decided").

## Decisions made in this spec

1. **`onboarding/vault/` mirrors the vault.** Hikify drafts the public wiki in `onboarding/vault/public/wiki/`, the orchestration feature map in `onboarding/vault/orchestration/read-only/features/`, and each partition's first documents in `onboarding/vault/partitions/<id>/read-only/features/`. Its execution copies the tree into the vault as it is, so a draft is already where it will live.
2. **A migrated partition keeps its old place with `at:`.** A partition of today mounted its scope at `/workspace/<scope>`. Its folder sets `at: <scope>`, so its test command, its volumes, and its instructions keep their paths. Only the code map and the test map change, because their paths become workspace paths.
3. **The migration commits on each project's `hikari` branch, after a tag.** That is the branch that `hikari up` checks out and that you merge, so the next start works without a merge. Each repository gets the tag `pre-shared-language` at the commit before the migration, so `git reset --hard pre-shared-language` undoes it.
4. **A service named `lead` is renamed to `orchestrator`.** The glossary retires the word, and nothing outside the topology file is keyed by the orchestration agent's id except its own vault part, which the migration creates new.

## Requirements

### Host skills

- **M1.** `containerize-repo` writes, for each repository, one partition agent whose partition is one folder: the whole repository, at its default place. It writes one orchestration agent named in `x-hikari.orchestration`. Its references describe folders, places, apply, the vault, and the orchestration agent, and nothing of systems, owners, hubs, or the store.
- **M2.** `hikify` designs the target as partitions of folders, and may give one partition folders from several repositories. It drafts in `onboarding/vault/` (decision 1): the public wiki's rules and principles, and the orchestration feature map's features, each linking the partition features it is made of. It writes `topology.target.yaml`. A suite that needs the whole application is recorded as left for integration.
- **M3.** The execution of hikify copies `onboarding/vault/` into the vault, and says to start the orchestration agent with "Carry out a plan" of `hikari-plan-work`.
- **M4.** `internal/onboard`'s tests and the lint test pass, and the host skills name only built skills and tools.

### The migration guide

- **M5.** `docs/guides/migrating.md` lists, in order, every move from a project of before the shared language to one of now, each with the command or the edit that makes it:
  1. `compose.yaml` to `topology.yaml`.
  2. `x-hikari.store` to `x-hikari.vault`, and the directory `store/` to `vault/`.
  3. `x-hikari.hubs` to `x-hikari.orchestration`, and each agent with no system to a partition agent or out of the topology.
  4. `system: {repo, scope, read, test}` to `partition: {folders, test}`, with `at: <scope>` (decision 2).
  5. The vault's moves: `shared/vocabulary/<hub>.yaml` to `public/vocabulary/terms.yaml` without `hub:`; `hubs/<hub>/wiki/` to `public/wiki/`, with each story to `orchestration/read-only/features/` as a `feature`; `shared/policies/` and `shared/deliverables/` to `public/`; `shared/skills/<name>/` to `skills/<name>/` with a grant for each agent that used it; `shared/agents/` removed; `systems/<id>/map.yaml` and `wiki/` to `partitions/<id>/read-only/features/`; `systems/<id>/releases/` to `partitions/<id>/read-only/releases/`; `systems/<id>/staging/` to `partitions/<id>/staging/`.
  6. In each partition's first writable folder, `.hikari/private/layout.yaml` to `code-map.yaml`, and each path in the code map and the test map prefixed with the folder's place.
  7. Instructions that name an old container path, such as `/hikari/public/readonly/systems/<name>/releases/`, to the new path, such as `/hikari/grants/<name>/releases/`.
  8. `hikari check`, which names any move left.
- **M6.** The README's section on project repositories links the guide.

### Your projects

- **M7.** `myprojects/yami` and `myprojects/cs566-project` are migrated by following M5, with the tags and commits of decision 3, and the lead renamed (decision 4). In the yami code repository at `~/projects/software/yami`, only the private files of its two partitions change, and no other file is staged; its untracked files stay as they are.
- **M8.** `hikari check` passes on both projects, except for warnings that predate the migration, which the build notes list.

### Docs

- **M9.** `docs/design.md` is wrapped at 78 columns again, and its principles section says "partition" and "orchestration agent". `docs/specs/containerize.md` no longer names a lead.
- **M10.** No file in `README.md`, `docs/design.md`, `docs/glossary.md`, or `docs/guides/` names a retired word for a current concept. The history in `docs/brainstorm/`, `docs/archive/`, and the specs' bodies stays.

### Checks

- **M11.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass.
- **M12.** A live check of the `onboard` example: `make example-onboard` writes the project and the host skills; a target topology written by following hikify's topology reference, with one partition of folders from both repositories, passes `hikari check -draft topology.target.yaml`; and `containerize-repo`'s topology reference, followed for the two repositories, gives a topology that passes `hikari check` after `hikari sync`. When Docker runs, `hikari up` on the shop runs one turn of each agent to the end, and the agents' tool calls use the new tools.
