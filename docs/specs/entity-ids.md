# Spec: ids and names

Status: built. Checked in a live run on 2026-10-25 of a two-agent project, `ids-check`, named `Ids Check`, whose hub `eng` is named `platform` and whose agents `eng-orchestrator` and `dev` are named `chief` and `builder`. A post in `#platform` that mentioned `@builder` woke `chief` and `builder`, and `builder` answered in `#platform`: "builder, in #platform." The store held `hubs/eng`, the container of `dev` listed `/hikari/public/readonly/hubs/platform`, and the containers and networks were `ids-check-dev-1` and `ids-check-agent-dev`. The sidebar showed the project's name.

The tester, environments, and image releases were removed in `remove-e2e.md`.

Changes made during the build:

1. **A map takes the system's new name** (ID12). The public map records `system.name`, and hikari refused a map whose name was not its system's. The map's place in the store, keyed by id, already says which system it describes, so hikari now writes the new name into the map at start and shows a note. Agents that read `map.yaml` then see names too. Found in the tests.
2. **An agent's compose service is found through its id.** The credential check of `hikari check` and `hikari up` looked up the service by the agent's name, and warned that a renamed agent had no credential. `ServiceOf` now looks up every agent's service. Found in the live run.
3. **The vocabulary files are named by id.** The shared area is mounted whole, so an agent that lists `vocabulary/` sees `eng.yaml` for `#platform`. The tools take and show names; only the directory listing shows ids.

Sources: `docs/brainstorm/topology-editor.md` (round 1), `topology-editor-round-2.md` (questions I1 to I6), and `topology-editor-answers.md`. This is the first of four steps of the topology editor.

## Summary

Every project, repo, hub, and agent has an id and a name. The id is the key that the compose file writes today, and it never changes once the entity has state outside the file. The name is what people and agents use, and a rename changes only the name. A new optional `name` field sets it; without the field, the name is the id, so every compose file of today keeps its meaning.

On the host, everything outside the file is keyed by id: store directories, vocabulary files, release manifests, the tester's lock, containers, networks, and clones. Everything that an agent or the UI sees is keyed by name: messages, mentions, prompts, tool inputs and outputs, and the paths inside a container. This step adds no editor. It makes a hand-written `name` work everywhere, so that the editor of step 2 can rename by changing one field.

## Decisions

From the rounds and their answers:

1. Entities are keyed by an internal id, and a rename changes a display name (B3).
2. The id is the YAML key, and the name defaults to it (I1).
3. Agents use names only (I2).
4. Names and ids share one namespace among agents, hubs, and `user` (I3).
5. An id can change only while nothing is keyed by it (I4). The editor of step 2 enforces this; this step only defines the ids.
6. The project's name is `x-hikari.name`; the compose `name` is its id (I6).

## Decisions made in this spec

1. **Paths inside a container use names.** Round 2 (I2) kept the container paths keyed by id. But hikari writes every mount itself, so the mount's source can be keyed by id while its target is keyed by name: `store/hubs/eng/wiki` mounts at `/hikari/public/readonly/hubs/platform/wiki`. An agent then never sees an id at all, and its prompt needs no second word for each hub.
2. **References in the file use ids.** `orchestrator`, `members`, `grants.releases`, `tests`, `serves`, and `system.repo` name ids. A rename then changes one line, and the file never holds a name that a rename has made stale. The editor shows names beside the ids.
3. **hikari checks the file in ids, then works in names.** The topology is read and checked with ids as keys, as today, so each error names the key that the file writes. Then hikari keys the topology by name, and every part of hikari that agents or the UI reach works in names, as it does today.
4. **A sidecar has no name.** Nothing addresses a sidecar: no message, no mention, no store directory. Its service key is all it has.
5. **The project's name is free text.** No one mentions a project, so its name needs no pattern: one line of at most 64 characters, such as `P3 Cloud`.
6. **The store maps names to ids in one place.** `store.Store` gets the id of each system and hub, and every path function takes a name. The release manifest and the tester's lock hold ids on disk and names in memory.

## Terms

- **Id**: the key of an entity in the compose file: the compose `name` of the project, the key of a repo or a hub under `x-hikari`, the service key of an agent.
- **Name**: the entity's `name` field, or its id when the field is absent.
- **Renamed entity**: an entity whose name differs from its id.

## 1. The File

```yaml
name: shop                     # the project's id

x-hikari:
  name: Shop                   # the project's name
  repos:
    shop: {name: storefront, path: ~/code/shop, branch: hikari}
  hubs:
    eng: {name: platform, orchestrator: eng-orchestrator, members: [orders]}

services:
  eng-orchestrator:
    image: hikari-agent
    x-hikari: {description: Orchestrates engineering.}
  orders:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      name: intake
      description: Owns order intake.
      system: {repo: shop, scope: services/orders}
```

- **ID1.** `x-hikari.name`, the `name` of a repo or a hub, and the `name` of an agent's `x-hikari` set the name. Without it, the name is the id.
- **ID2.** The name of a repo, a hub, or an agent follows the rule of ids: a lowercase letter, then lowercase letters, digits, and dashes, at most 32 characters. The project's name is one line of at most 64 characters, not empty.
- **ID3.** A sidecar's `x-hikari` holds only `serves`, as today, so a sidecar has no `name`.
- **ID4.** The names of the agents, the hubs, and `user` are unique together. The ids of the agents and the hubs are unique together, as today. A name may not be the id of another agent or hub. The same three rules hold among repos, with `project` reserved as both an id and a name.
- **ID5.** References name ids: a hub's `orchestrator` and `members`, an agent's `grants.releases`, `tests`, and `system.repo`, and a sidecar's `serves`. An error about a reference names the id, as it is written in the file.

## 2. On the Host

- **ID6.** The store keys by id: `store/systems/<id>`, `store/hubs/<id>`, and `store/shared/vocabulary/<id>.yaml`. The vocabulary file's `hub` field holds the id.
- **ID7.** A release manifest's `system` holds the system's id. hikari writes the id when it publishes and shows the name when it reads a manifest.
- **ID8.** The tester's `releases.lock` names each system by id. `pin_releases` takes names, and `get_test_plan` and the UI show names.
- **ID9.** Compose keys by id: each service and its prepare service, each agent's network (`<project id>-agent-<agent id>`), and the variable that carries each token.
- **ID10.** A linked repo's clone is `repos/<id>`.
- **ID11.** A commit names the agent by name, as author and in the message, because the log is for people.

## 3. In a Run

- **ID12.** Everything an agent sees uses names: its prompt, its hubs and peers, the `from`, `hub`, and `to` of each message, the mentions that wake it, the inputs and outputs of its tools, and the paths of its mounts under `/hikari`.
- **ID13.** Everything the UI shows uses names, and the snapshot carries the project's name, which the UI shows in the sidebar header.
- **ID14.** The terminal shows the project's name in the line `Read <name>`.

## 4. Out of Scope

- The editor, and the rename operations with propagation (step 2).
- Changing an id. A hand edit of an id is what it is today: a new entity with no store state.

## 5. Tests

1. **Topology.** Names from each field; the defaults; ID2 to ID5, one case for each rule; the id-keyed checks still name ids; the name-keyed topology after parse, with references resolved; every example file loads with names equal to ids.
2. **Store.** Each path function of a renamed system and hub gives the id's directory; a manifest written under a name holds the id, and reads back with the name; the lock does the same; the vocabulary file holds the id.
3. **Launcher.** The override of a topology with a renamed agent, hub, and repo: service keys, networks, and token variables by id; mount sources by id; mount targets by name.
4. **Server and systems.** A message to a renamed agent and hub, and a mention of a renamed agent, route and wake by name; `Self` lists names; a renamed owner's map, wiki, and releases come from its id's directory.

## 6. Build Plan

1. **Topology.** ID1 to ID5, the ids of each entity, and the name-keyed topology, with tests.
2. **Store and launcher.** ID6 to ID10, with tests.
3. **Systems, server, and UI.** ID7, ID8, ID11 to ID14, with tests.
4. **Live check and docs.** Run `examples/team` with a renamed hub and a renamed agent, and check that the agents talk by name and the store keys by id. Then update the README, `docs/design.md`, and this spec's status.
