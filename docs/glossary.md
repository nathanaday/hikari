# Glossary

These are the words of hikari. Each concept has one name in the code, the docs, the UI, the prompts, and the files, and a lint test (`internal/driver/lint_test.go`) keeps the retired words below out of the prompts, the skills, and the UI. The meanings come from the Shared Language note in the Hikari journal, with the answers of `docs/brainstorm/shared-language-answers.md`.

## Terms

**Hikari.** The binary itself: the tooling, the API, and the UI.

**Project.** One or more repositories worked on for one goal, such as an application, an ecosystem, or a service. In hikari, a project is one topology file, and it has one room.

**Repository.** One git repository on the host. You link one that exists anywhere on your machine by its path, or hikari clones it by its URL into `repos/<name>` in the project repository.

**Topology file.** The Docker Compose file with `x-hikari` keys that defines a project. Every Compose feature keeps working; only hikari's keys are its own. hikari opens `topology.yaml` when you name no file.

**Vault.** The directory of every hikari file that is not code: the shared knowledge, the feature maps, the health, and the releases. It is `vault/` next to the topology file. Its public files, in `public/`, reach every container; each partition has a read-only part, a part its agent can edit, and a staging area; and the directory a file is in decides who mounts it and how. Private knowledge, the code map and the test map, stays inside the partition, so that only the partition agent sees it. [guides/vault.md](guides/vault.md) shows each file of an example on the host and in each container.

**Vocabulary.** The project's shared dictionary, so that every agent maps features with the same names. Every agent reads it, and only the orchestration agent changes it, through tools. It is a directory, `vocabulary/`, whose file `terms.yaml` holds the terms and the interfaces; hikari reads only that file, so that later vocabulary-like files can join it.

**Partition.** The folders that one container mounts as its workspace. It may or may not be a whole repository. The agent inside sees what is mounted as one monorepo, without any concern for the topology or the mounts. It is a flat list of folders, each a path in any repository of the project, writable or read-only, and the container sees each folder at `/workspace/<repo>/<path>`, or at the folder's `at:`.

**Secrets.** A partition's secrets are Compose's `env_file` and `environment` on its service. A secret system of hikari's own is left for later.

**Agent.** A Claude Code session with skills for its work: mapping, testing, validation, and the like.

**Partition agent.** The agent in a partition's container. It reads and changes only its own partition, its private files (the code map and the test map), the read-only files granted to it (its feature map, its releases, and what you grant), and the files you let it edit.

**Orchestration agent.** The agent that reads the feature map and health of every partition, and every release, and no code. It works only through the knowledge of the features, the releases, and the other documents of the vault, and it alone changes the public files, through tools. A project has zero or one orchestration agent, named by `x-hikari.orchestration`.

**Integration agent.** Planted, not built. It will get every release and knowledge file and run the end-to-end suites, with mocks where they are needed, once the home of those suites is settled.

**Hikari agent.** A Claude Code session on the host with hikari's host skills, such as `containerize-repo` and `hikify`. It helps you work with a hikari project, and does not work on the code.

**Driver.** The part of the hikari binary that runs in every agent's container, as `hikari driver`. It takes the agent's messages from the server and gives them to Claude Code.

**Room.** The one conversation of a project. You and every agent are in it, everyone receives every post, and anyone can send a direct message to anyone.

**Feature map.** A partition's public description: its feature documents and `map.yaml`, which hikari writes from them. Every agent that may read the partition reads it; only its partition agent writes it, through tools.

**Orchestration feature map.** The orchestration agent's own feature map, in `orchestration/read-only/features/`: features that cross partitions, each made of the partition features it links, with its health computed from theirs, and principles and rules that bind only the orchestration agent. Only the orchestration agent reads it, and it writes it through `change_wiki`, as a partition agent writes its own.

**Code map.** Private to the partition: where each feature lives in the code, in `code-map.yaml` in the partition's `.hikari/private/`.

**Test map.** Private to the partition: how its suites run and which features each test touches, in `tests.yaml`.

**Skill pack.** The built-in skills that hikari installs in an agent's container for the tasks of its kind. A partition agent's pack is `hikari-plan-work`, `hikari-do-work`, `hikari-validate-work`, and `hikari-map-features`; the orchestration agent's is `hikari-plan-work`, `hikari-health-check`, and `hikari-map-features`. A skill of the same name in two packs is two skills, and the prefix `hikari-` is reserved for them.

**Apply.** Commit a partition's changes to its source repositories: one commit in each repository whose writable folders changed. An agent calls `apply` to name the commit, and hikari applies what is left at the end of every turn and at stop.

## Retired Words

| Retired | Now | Why |
|---|---|---|
| hub | the room | Each project is one room; islands within a project are not needed yet. |
| lead | orchestration agent | An old name of the orchestrating agent in default configs. |
| store | vault | The vault says that it keeps what agents may and may not see. |
| system | partition | A partition is the folders of one container, possibly from several repositories. |
| system map, system wiki | feature map | The canvas calls a partition's public description the feature map. |
| owner | partition agent | Agents are named by where they run and what they see. |
| orchestrator | orchestration agent | The same naming. Code identifiers say `Orchestration`. |
| node | driver | Node was both the driver and the partition; a partition is now only the folders. |
| compose file | topology file | Compose stays the name of Docker Compose, its keys, and its commands. |
| story | orchestration feature | A flow across partitions is a feature of the orchestration feature map. |
| layout, code layout | code map | The private document of where each feature lives. |
| `commit_changes` | `apply` | The name of the event in the glossary. |
