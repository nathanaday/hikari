# Spec: skill packs and the orchestration feature map

Status: approved.

Sources: `docs/brainstorm/shared-language.md` (questions F1 to F4, B7, and G1) and `shared-language-answers.md`. This is step 6 of the seven steps of the shared language. The names are those of `docs/glossary.md`.

## Summary

Each kind of agent gets a pack of built-in skills from the binary. A partition agent's pack is `hikari-plan-work`, `hikari-do-work`, `hikari-validate-work`, and `hikari-map-features`. The orchestration agent's pack is `hikari-plan-work`, `hikari-health-check`, and `hikari-map-features`. Today's `hikari-partition` and `hikari-orchestration` split across them. The orchestration agent gets a feature map of its own, in `orchestration/read-only/features/`: features that link the partition features they are made of, with health computed from theirs. Stories leave the public wiki for it.

## Decisions

From the round and its answers:

1. The packs come from the binary, and the UI lists them as granted (F1).
2. Your own skills are granted in the topology (F2, built in step 4).
3. This step sets the packs' shape, and carries today's text into them; `hikari-validate-work` runs the suites and reads the health, and `hikari-plan-work` asks for a plan in the room before a change of more than one feature (F3).
4. The orchestration feature map links partition features into larger features, with the kinds `feature`, `principle`, and `rule`, and its health is computed from theirs (F4).
5. A story of the public wiki moves to the orchestration feature map (B7).

## Decisions made in this spec

1. **Built-in skills keep the `hikari-` prefix.** It marks a skill that comes from the binary, and a granted skill cannot take it (V7). A skill of the same name in two packs is two skills, and a container gets only its own pack.
2. **`change_wiki` writes the caller's own feature map, for both kinds of agent.** The orchestration agent's `change_wiki` writes `orchestration/read-only/features/`; `change_public_wiki` still writes the public wiki. One tool name for "my feature map" is what the glossary's symmetry asks for.
3. **A principle or rule of the orchestration feature map binds only the orchestration agent.** Partition agents never mount `orchestration/`. A principle or rule for every agent belongs in the public wiki.
4. **A reader sees only the links it can follow.** A link has no direction, so a partition feature that an orchestration feature links has a backlink. `get_document` and `search_wiki` show a partition agent only the backlinks from documents it can read.
5. **`story` is no longer a kind.** A public wiki document of kind `story` is a problem of that document, as any invalid document is: it keeps no valid version, and the problem says to move it to the orchestration feature map as a `feature`.

## Requirements

### Packs

- **K1.** The binary embeds the skills under `internal/driver/skills/partition/` and `internal/driver/skills/orchestration/`. The driver installs a container's pack into `~/.claude/skills/` before each turn, and no built-in skill of the other pack.

| Skill | Partition agent | Orchestration agent |
|---|---|---|
| `hikari-plan-work` | Before a change of more than one feature, post the plan in the room: the features it changes, creates, or removes, the interfaces it touches, and what it needs from other partitions. Wait for an answer when it needs another partition. | Turn a request into work for partition agents from the feature maps: which partitions, which features, and in what order. Carry out a plan you hand it (today's "Carry out a plan"). |
| `hikari-do-work` | Make the change in the workspace, keep the code map true, apply, and publish a release (today's "Build" and "Publish a release"). | none |
| `hikari-validate-work` | Run every suite, record each run, read the health, and fix or report what fails, before saying that a change is done (today's "Test" and "Problems"). | none |
| `hikari-map-features` | The feature map, the code map, and the test map: their formats, `change_wiki`, `set_partition_info`, and `mark_map_current` (today's "Your partition documents", "Keep true", and the references). | The orchestration feature map, the public wiki, and the vocabulary: their kinds, `change_wiki`, `change_public_wiki`, and the vocabulary tools. |
| `hikari-health-check` | none | Read every partition's health and staleness, the vocabulary's flags (names it lacks, consumed interfaces that no partition provides), and the orchestration features' health; report what needs work in the room, and ask the partition agent that owns it. |

- **K2.** `hikari-partition` and `hikari-orchestration` are gone. The prompts name the skill of each task: the indexing turn names `hikari-map-features`, the restore turn `hikari-validate-work`, and a request to change code `hikari-plan-work`.
- **K3.** The `hikari-` agent definitions (`hikari-feature-survey`, `hikari-feature-review`, `hikari-test-survey`) stay, and the skills that used them name them.

### The orchestration feature map

- **K4.** The orchestration feature map is a wiki at `orchestration/read-only/features/`, with the ref `orchestration`. Its kinds are `feature`, `principle`, and `rule` for the orchestration agent, and `plan` and `note` for you. It has no `map.yaml`, no code map, and no test map.
- **K5.** It can link every partition's feature map and the public wiki. A partition feature map and the public wiki cannot link it.
- **K6.** An orchestration feature's health is its support: failing when a linked feature is failing, pending when one is pending, untested when none is tested, and otherwise passing. A principle or a rule shows its support the same way.
- **K7.** A rename of a partition feature rewrites its links in the orchestration feature map, as in every wiki that links it.
- **K8.** The orchestration agent reads its feature map at `/hikari/self/features/`, and through `get_document` and `search_wiki`. Before each turn, it learns which of its documents someone else changed.
- **K9.** In a project repository, a change of the orchestration feature map is committed with the orchestration agent as author, and your hand edits as you, as for every wiki.

### Kinds and links

- **K10.** `story` leaves the kinds, with its support rule. The public wiki's kinds are `rule` and `principle`, and `plan` and `note`.
- **K11.** `get_document` and `search_wiki` show only backlinks from documents the reader can read (decision 4).

### UI

- **K12.** The orchestration agent's page has a Feature map tab: its documents, their health, and their links, in the same wiki views as a partition's. You can write and edit plans and notes there.
- **K13.** Clicking the orchestration agent on the Map opens its feature map's graph, with the partition features it links drawn around it.
- **K14.** Each agent's inspector lists its skills: the built-in pack, marked as from Hikari, and its granted skills.

### Docs

- **K15.** `README.md`, `docs/design.md`, and `docs/glossary.md` describe the packs and the orchestration feature map. `design.md` gives the reasons: a skill per task loads only what the task needs, and an orchestration feature computes its health from the features it links, so it needs no test of its own until the integration agent exists.

### Checks

- **K16.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover: each kind of agent's installed pack, with no skill of the other pack; an orchestration feature whose health follows a linked partition feature from passing to failing; a rename of a partition feature that rewrites the orchestration feature's link; and a partition agent's `get_document` that hides the orchestration backlink.
- **K17.** A live check of the shop: the orchestration agent writes an orchestration feature that links `create-order` and `reserve-stock`; the UI's Feature map tab shows it with its health; `orders`' `get_document` of `create-order` shows no orchestration backlink.
