# Migrate a Project to the Shared Language

This guide is for a project that you made before hikari took the words of the [glossary](../glossary.md): a project with a `compose.yaml`, a `store/`, hubs, and systems. hikari ships no migrator. It moves none of these files itself, and refuses to start on each of them with a message that names the move and the section of this guide that makes it. The moves are below, in the order that you make them. Each move is a command or an edit.

The examples use a project of two systems, `yami-library` and `yami-obsidian-plugin`, that each had a scope of one repository, `yami`. Replace the names with yours.

## Before You Start

1. Stop hikari, and install the new binary: `make build`, or `make install`.
2. Check out the branch that `hikari up` checks out: in the project repository, the branch of `x-hikari.project`; in each repository of a system, its `branch` in `x-hikari.repos`, or the project branch for a linked repository. The migration commits there, so that the next `hikari up` starts without a merge.

   ```sh
   git switch hikari
   git -C ~/code/yami switch hikari
   ```

3. Tag the commit before the migration in each repository, so that `git reset --hard pre-shared-language` undoes it:

   ```sh
   git tag pre-shared-language
   git -C ~/code/yami tag pre-shared-language
   ```

4. Check that each repository is clean with `git status`. hikari refuses to start on uncommitted changes, so the migration ends with one commit in each repository.

## The Topology File

hikari opens `topology.yaml` when you name no file, and tells you to rename an old `compose.yaml`:

```sh
git mv compose.yaml topology.yaml
```

Change each comment or script that names `compose.yaml`, such as `hikari up compose.yaml`.

## The Vault

The vault was the store. It is `vault/` next to the topology file, where the store was `store/`, so rename the directory:

```sh
git mv store vault
```

A topology file that sets `x-hikari.store` names a store elsewhere. Rename the key to `x-hikari.vault`, and rename the directory it names if you like:

```yaml
x-hikari:
  vault: ../acme-vault        # was: store: ../acme-store
```

The files inside still have the old layout. [The Vault Layout](#the-vault-layout) moves them.

## The Orchestration Agent

A project has one room, and zero or one orchestration agent. `x-hikari.hubs` goes, and `x-hikari.orchestration` names the agent that was the orchestrator of the hub:

```yaml
x-hikari:
  orchestration: orchestrator
  # was:
  # hubs:
  #   yami-project: {description: ..., orchestrator: lead, members: [yami-library, yami-obsidian-plugin]}
```

The glossary retires the word "lead", so rename a service named `lead` to `orchestrator`: rename its key under `services`, and each `@lead` in the instructions of the other agents. Nothing else is keyed by its id. Its part of the vault, `orchestration/`, is new, and hikari makes it at the next start. Move the hub's `description` into the orchestration agent's `description` if it says something that the agent's does not.

Every agent other than the orchestration agent now has a partition. Give each agent that had no system a partition, as [Partitions](#partitions) shows, or remove its service. A hub's tester goes with the hub: remove its service and its `tests:` key. Its suites stay in their folder, left for integration.

## Partitions

A system is now a partition: a list of folders, each a path in a repository, writable or read-only. Write `system: {repo, scope, read, test}` as `partition: {folders, test}`:

```yaml
    x-hikari:
      partition:
        folders:
          - {repo: yami, path: yami, at: yami}
        test: cd yami && npm run build && npm test -- --reporter=default --reporter=junit --outputFile=/hikari/private/reports/all.xml
      # was:
      # system:
      #   repo: yami
      #   scope: yami
      #   test: npm run build && npm test -- ...
```

- **The scope** becomes the folder's `path`. A folder mounts at `/workspace/<repo>/<path>` by default; `at: <scope>` keeps it at `/workspace/<scope>`, where the system's scope was, so that its volumes, such as `/workspace/yami/node_modules`, and the paths in its instructions keep working.
- **A whole repository**, a system with no scope, becomes a folder with no `path` and no `at`. It mounts at `/workspace/<repo>`, where the system's scope was `/workspace`, so add `<repo>/` to each container path of its volumes and its instructions that started at `/workspace`.
- **Each read path** becomes a folder with `read_only: true` and `at: <path>`, which keeps it at `/workspace/<path>`: `{repo: yami, path: proto, read_only: true, at: proto}`.
- **The test command** runs in `/workspace` now, not in the scope. Start it with `cd <scope> &&`, or `cd <repo> &&` for a whole repository, so that the runner works as it did.

A partition that you wrote between the two forms, with `partition: {repo, scope, read}`, takes the same edit.

## The Vault Layout

The directory that a file is in now decides who mounts it and how. [The vault of the shop](vault.md) shows every file of the new layout, and where each agent sees it.

| Old path in the vault | New path |
|---|---|
| `shared/vocabulary/<hub>.yaml` | `public/vocabulary/terms.yaml`, without its `hub:` line |
| `shared/wiki/` | `public/wiki/` |
| `hubs/<hub>/wiki/`, each rule, principle, plan, and note | `public/wiki/` |
| `hubs/<hub>/wiki/`, each story | `orchestration/read-only/features/`, as a `feature` |
| `shared/policies/`, `shared/deliverables/` | `public/policies/`, `public/deliverables/` |
| `shared/skills/<name>/` | `skills/<name>/`, granted to each agent that used it |
| `shared/agents/` | removed |
| `systems/<id>/wiki/` and `systems/<id>/map.yaml` | `partitions/<id>/read-only/features/` |
| `systems/<id>/releases/` | `partitions/<id>/read-only/releases/` |
| `systems/<id>/staging/` | `partitions/<id>/staging/` |
| `README.md`, `.gitignore` | the new text of each |

This script makes every move of the table. Save it as `migrate-vault.sh` outside the vault, and run it with `sh` in the vault; it uses only the tools of macOS and Linux:

```sh
# Run in vault/: sh ../migrate-vault.sh
mkdir -p skills partitions orchestration/read-only/features

# shared/ is now public/; its skills move out, and its agents go.
if [ -d shared ]; then
  if [ -e public ]; then echo "public/ exists: merge shared/ into it by hand"; exit 1; fi
  mv shared public
  if [ -d public/skills ]; then
    for s in public/skills/*/; do
      if [ -d "$s" ]; then mv "$s" skills/; fi
    done
    rmdir public/skills
  fi
  rm -rf public/agents
fi
mkdir -p public/vocabulary public/wiki public/policies public/deliverables

# The vocabulary of the one hub is terms.yaml, without its hub: line and
# the comments that hikari wrote.
if [ ! -f public/vocabulary/terms.yaml ]; then
  for v in public/vocabulary/*.yaml; do
    if [ -f "$v" ]; then
      sed -e '/^hub:/d' -e '/^#/d' "$v" > public/vocabulary/terms.yaml && rm "$v"
      break
    fi
  done
fi

# A story of a hub is a feature of the orchestration feature map; the
# rest of a hub's wiki is the public wiki.
if [ -d hubs ]; then
  for f in hubs/*/wiki/*.md; do
    if [ ! -f "$f" ]; then continue; fi
    if sed -n '2,/^---$/p' "$f" | grep -q '^kind: story$'; then
      sed -i.bak 's/^kind: story$/kind: feature/' "$f" && rm "$f.bak"
      mv "$f" orchestration/read-only/features/
    else
      mv "$f" public/wiki/
    fi
  done
  rmdir hubs/*/wiki hubs/* hubs
fi

# Each system is a partition, with its feature map in read-only/features/.
if [ -d systems ]; then
  for d in systems/*/; do
    id=$(basename "$d")
    mkdir -p "partitions/$id/read-only"
    if [ -d "$d/wiki" ]; then mv "$d/wiki" "partitions/$id/read-only/features"; fi
    mkdir -p "partitions/$id/read-only/features"
    if [ -f "$d/map.yaml" ]; then mv "$d/map.yaml" "partitions/$id/read-only/features/"; fi
    if [ -d "$d/releases" ]; then mv "$d/releases" "partitions/$id/read-only/releases"; fi
    if [ -d "$d/staging" ]; then mv "$d/staging" "partitions/$id/staging"; fi
    rmdir "$d"
  done
  rmdir systems
fi

printf 'partitions/*/staging/\npartitions/*/read-only/releases/\n' > .gitignore
```

A `rmdir` that fails names a directory that still holds a file the table does not move: move the file by hand, and remove the directory. Then finish the moves that the script cannot make:

- **The README.** `vault/README.md` still describes the store. hikari writes a README only into a vault that has none, so take the new text from a scratch project:

  ```sh
  d=$(mktemp -d) && hikari init -name scratch "$d" && cp "$d/vault/README.md" vault/README.md && rm -rf "$d"
  ```

- **More than one hub.** The script makes `terms.yaml` from one vocabulary file. Merge the `terms` and `interfaces` of each other `<hub>.yaml` into it by hand, and delete the file. Two hub wikis that held a document of the same id need one of them renamed: an id is unique in the whole vault.
- **The stories.** A story is now a feature of the orchestration feature map, which only the orchestration agent reads, and it keeps its links to the partition features. Its `key:` line, if any, does nothing now; delete it. Its health now follows the partition features it links.
- **The skills.** A skill of `shared/skills/` reached every agent. A skill of `skills/` reaches only the agents that the topology grants it to, so grant it to each agent that used it:

  ```yaml
      x-hikari:
        grants:
          skills: [python-style]
  ```

- **The subagents.** hikari no longer installs the subagent definitions of `shared/agents/`. Move what an agent needs from them into its instructions or into a skill.
- **The feature maps** need no edit. hikari converts a `map.yaml` of version 4, the last version of the store, at the next start, and commits it when it stops: it reads its `system:` key, and reads the one commit that the map and each document describe as a commit of the repository of the partition's first writable folder. A map of an earlier version is deleted, and the partition agent describes its partition again.

Stage the moves:

```sh
git add -A vault
```

## The Code Map and the Test Map

The private files live in `.hikari/private/` of each partition's first writable folder, which is the scope of the system you migrated. The code map is now `code-map.yaml`:

```sh
git -C ~/code/yami mv yami/.hikari/private/layout.yaml yami/.hikari/private/code-map.yaml
```

For a whole repository, the folder is the repository's root: `git -C ~/code/app mv .hikari/private/layout.yaml .hikari/private/code-map.yaml`.

The paths of both files were relative to the scope, and they are now workspace paths, relative to `/workspace`. Put the folder's place in front of each: the scope with `at: <scope>`, or the repository's name for a whole repository. In `code-map.yaml`, edit each `path` of `parts` and each path of a feature's `code`:

```yaml
version: 1
parts:
  - {path: yami/src/, description: "Library source; one folder per package export."}   # was: src/
features:
  module-sdk:
    code: [yami/src/index.ts, yami/src/module]                                        # was: [src/index.ts, src/module]
```

In `tests.yaml`, start each suite's command with the same `cd` as the topology's test command, and put the place in front of each key that names a file or a directory. Leave each key that names a test by its id, such as `tests.test_stock.StockTest`: the runner names a test the same way as before.

```yaml
version: 1
suites:
  all:
    command: cd yami && npm run build && npx vitest run test/unit --reporter=default --reporter=junit --outputFile=/hikari/private/reports/all.xml
tests:
  yami/test/unit/agents.test.ts: ["[[module-sdk]]", "[[agents]]"]   # was: test/unit/agents.test.ts
```

Leave `results.yaml` as it is. Its results name the old paths, so each feature's health is pending until its suites run again, which each partition agent does on its first turn. A path that you missed is not lost: at its next turn, hikari tells the partition agent each path of its code map that is in no folder of its workspace (rule P3), and asks it to fix them.

## Instructions

The vault mounts at new paths in each container. Change each container path in the agents' `instructions` in the topology file, and in the text of the vault that agents read: the policies, the skills, and the `SKILL.md` of each deliverable.

| Old container path | New container path |
|---|---|
| `/hikari/public/readonly/shared/vocabulary/<hub>.yaml` | `/hikari/public/vocabulary/terms.yaml` |
| `/hikari/public/readonly/shared/` | `/hikari/public/`: `wiki/`, `policies/`, `deliverables/` |
| `/hikari/public/readonly/shared/skills/<name>/` | `/hikari/skills/<name>/` |
| `/hikari/public/readonly/hubs/<hub>/wiki/` | `/hikari/public/wiki/`; the orchestration agent reads its features in `/hikari/self/features/` |
| `/hikari/public/export/wiki/`, `/hikari/public/export/map.yaml` | `/hikari/self/features/` |
| `/hikari/public/export/releases/` | `/hikari/self/releases/` |
| `/hikari/public/export/staging/` | `/hikari/staging/` |
| `/hikari/public/readonly/systems/<name>/releases/`, in a partition agent's text | `/hikari/grants/<name>/releases/` |
| `/hikari/public/readonly/systems/<name>/`, in the orchestration agent's text | `/hikari/partitions/<name>/`, with `features/` for `wiki/` and `map.yaml` |
| `/workspace/<scope>/` | unchanged with `at: <scope>` |
| `/hikari/private/` | unchanged |

For a project like the example, whose orchestration agent's instructions name no container path, one `sed` makes the changes of a partition agent's text. Run it in the project repository; it changes no release or staging area:

```sh
grep -rl '/hikari/public/' topology.yaml vault/public vault/skills | xargs sed -i.bak \
  -e 's#/hikari/public/readonly/shared/skills/#/hikari/skills/#g' \
  -e 's#/hikari/public/readonly/shared/#/hikari/public/#g' \
  -e 's#/hikari/public/readonly/hubs/[a-z0-9-]*/wiki/#/hikari/public/wiki/#g' \
  -e 's#/hikari/public/export/staging/#/hikari/staging/#g' \
  -e 's#/hikari/public/export/wiki/#/hikari/self/features/#g' \
  -e 's#/hikari/public/export/map.yaml#/hikari/self/features/map.yaml#g' \
  -e 's#/hikari/public/export/#/hikari/self/#g' \
  -e 's#/hikari/public/readonly/systems/\([a-z0-9-]*\)/releases/#/hikari/grants/\1/releases/#g'
rm -f topology.yaml.bak && find vault/public vault/skills -name '*.bak' -delete
grep -rn '/hikari/public/readonly\|/hikari/public/export' topology.yaml vault/public vault/skills
```

The last `grep` lists each path left, which you change by hand. A vocabulary path, `/hikari/public/vocabulary/<hub>.yaml`, also needs `terms.yaml` in place of `<hub>.yaml`. Read the instructions once more for the old words, too: an agent that is told to brief "the owners in the hub" learns a model that hikari no longer has.

## Commit and Check

Commit in each repository, on the branch of [Before You Start](#before-you-start):

```sh
git add topology.yaml vault
git commit -m "Migrate to the shared language"
git -C ~/code/yami add yami/.hikari/private obsidian_plugin/.hikari/private
git -C ~/code/yami commit -m "Rename the code maps, and give their paths the folders' places"
```

Then run, in the project repository:

```sh
hikari sync
hikari check
```

`hikari sync` also writes the host skills in `.claude/skills/` again, for the new binary. `hikari check` runs every check of `hikari up`, and names each move left: `compose.yaml`, the keys `store`, `hubs`, `system`, and `tests`, a partition of `repo`, `scope`, and `read`, the directories `store/`, `shared/`, `hubs/`, and `systems/`, a vocabulary file without `terms.yaml`, and a `layout.yaml` without `code-map.yaml`. Each message ends with the section of this guide that makes the move. Fix each one, commit, and run `hikari check` again until it passes.

`hikari check` cannot see three things, which show at the next start instead:

- a document of the public wiki whose kind is still `story`, which hikari shows as a problem of the wiki: move it to `vault/orchestration/read-only/features/` as a `feature`;
- a path of a code map that is in no folder of the workspace, which hikari asks the partition agent to fix;
- an old container path in a text, which an agent finds empty. The last `grep` of [Instructions](#instructions) lists them.

Start the project with `hikari up`. On its first turn, each partition agent runs its suites again, and the health of its features follows. If the migration went wrong, `git reset --hard pre-shared-language` in each repository puts it back as it was.
