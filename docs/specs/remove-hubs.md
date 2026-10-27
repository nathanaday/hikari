# Spec: one project, one room

Status: built. Docker was not running, so the live check ran `hikari up -no-launch` on the shop and drove the node API with each agent's run token in place of agent turns: `messages.jsonl` shows that the user's post woke only the orchestrator, the orchestrator's post woke both owners, and an owner's direct message woke the other owner, and inventory read the term and the rule that the orchestrator wrote into `terms.yaml` and `shared/wiki/`. Headless Chrome showed the room with its Messages, Vocabulary, and Wiki tabs, and the Map with the orchestrator in the middle; no agent turn ran.

Changes made during the build:

1. **A rename leaves `x-hikari.orchestration` alone** (H5). The key holds a service id, which a rename keeps; `change_id` rewrites it, as it rewrites a grant.
2. **`set_orchestration` does not check the agent's system** (H18). `hikari check` reports an orchestrator that owns one, and the project's inspector offers only agents without a system, and None.
3. **The user's wiki routes take the wiki's ref**: `/api/ui/wiki/{ref}/docs/{id}`, with `public` or `system:<name>`, because the public wiki has no scope and name. The key route went with the key.
4. **The wake cases are one file**, `web/src/lib/fixtures/wake.json`, which `TestWakeRules` and `rules.test.ts` both run (H6). It also holds the direct-message cases of H7.
5. **The wire types follow the room.** `Self` has `participants` and `orchestrator` instead of hubs, and `owner_grants` and `owner_deliverables` replace the member fields. The snapshot has `participants` and one `vocabulary`, each agent view has its `role`, and the role `member` is `owner`. A turn in no conversation, such as indexing, shows in the room.
6. **`terms.yaml` is decoded strictly.** A `hub:` key in it is an error; hikari never reads the old `<hub>.yaml` files, so only a hand copy can have one.
7. **The orchestrator's node opens the public wiki's map** (`#/room/map`), as a hub's node opened its wiki, and the Activity lens colors it by the orchestrator's state.
8. **`docs/design.md` had a heading pasted into a sentence** of "The feature wiki". The renamed section "System wikis and the public wiki" now stands on its own.

Sources: `docs/brainstorm/shared-language.md` (questions B1 to B8, and G1 to G3 for hubs) and `shared-language-answers.md`. This is step 2 of the seven steps of the shared language.

## Summary

Hubs leave. A project is one room: every agent and the user are in it, every agent receives every post, and any two of them can send direct messages to each other. The topology names its orchestrator, if it has one, at the top, in `x-hikari.orchestration`. Every other agent owns a system. The hub vocabularies become one vocabulary, and the hub wikis become one public wiki that every agent reads and the orchestrator writes. This step still uses today's words for everything that is not a hub: store, system, owner, orchestrator. Step 3 renames them.

## Decisions

From the round and its answers:

1. The orchestrator is named at the top of the topology, by service id (B1).
2. A project has zero or one orchestrator (B2).
3. Every agent is an owner or the orchestrator (B3).
4. The room keeps today's wake rules without the hub, and with no orchestrator a post without mentions wakes every agent (B4).
5. Any agent can send a direct message to any agent (B5).
6. The vocabulary is a directory, and today's file moves into it as `terms.yaml` (B6 and its answer).
7. A hub's rules and principles become the public wiki, which the orchestrator writes through `change_public_wiki`. Stories move to the orchestration feature map in step 6. The key-story flag goes (B7).
8. A release is announced in the room, and new manifests have no `hubs:` field (B8).

## Decisions made in this spec

1. **The public wiki lives at `store/shared/wiki/` in this step.** Every agent already mounts `store/shared` read-only, so the public wiki needs no new mount, and step 4 moves `shared/` to `public/` whole. The wiki's ref is `public`.
2. **Stories stay in the public wiki until step 6.** The `story` kind is valid there, written by the orchestrator, and its health is its support, as in step 1. Step 6 moves stories into the orchestration feature map.
3. **The vocabulary is `store/shared/vocabulary/terms.yaml`.** The directory already exists, so step 4 moves it with `shared/`. Hikari reads only `terms.yaml` and ignores every other file in the directory, including the old `<hub>.yaml` files.
4. **The orchestrator's skill is `hikari-orchestration`.** A skill named after a removed concept would fail the lint of step 3. Its content is today's `hikari-hub` without the hub parts; step 6 splits it into the pack.
5. **The Map draws the orchestrator in the middle.** Its hub hexagons go in this step, so G3 is built here instead of in step 6: the orchestrator's node, each system's node, an edge from the orchestrator to each system, and an edge for each release grant.
6. **A message to the room has no channel field.** `Message.Hub` goes. A message with `to` is direct; one without is a post to the room. The journal's lines lose `hub` in the same way.
7. **Removing the orchestrator in the editor clears `x-hikari.orchestration`.** Zero orchestrators is valid, so the editor no longer refuses the removal.

## Requirements

### Topology

- **H1.** `x-hikari.hubs` is gone, and the strict decoder refuses it. `x-hikari.orchestration` is optional. When set, it names an agent's service id. It cannot name a sidecar or `user`.
- **H2.** The orchestrator cannot own a system. Every other agent must own one: an agent with neither fails with `agent "writer" owns no system and is not the orchestrator`.
- **H3.** `Hub`, `Topology.Hubs`, `HubIDs`, `HubNames`, `OrchestratedHubs`, `InHub`, `HubsOf`, `Contacts`, `CanDirect`, `ChildHubs`, `ParentHubs`, `MemberHubs`, and `ReadableHubWikis` are gone. `Topology.Orchestrator` holds the orchestrator's name, or is empty. `Participants()` lists the orchestrator first, then the other agents by name, then `user`.
- **H4.** `VisibleSystems(agent)` is every system when the agent is the orchestrator, and none otherwise. The orchestrator therefore mounts every system's directory read-only, as an orchestrator mounted its member owners' before.
- **H5.** Names share one namespace among agents and `user`. The rules of ids and names otherwise stay as they are, and a rename or a change of id of an agent rewrites `x-hikari.orchestration`.

### Messages

- **H6.** A post goes to the room. Every participant but the sender receives it. It wakes the orchestrator, each agent that it mentions as `@name`, and every owner when the orchestrator sends it without mentions. When the project has no orchestrator, a post without mentions wakes every agent. A user's post follows the same rules. The server and `web/src/lib/rules.ts` implement the rule, and both test the same cases.
- **H7.** Any participant can send a direct message to any other participant, and a direct message always wakes its recipient.
- **H8.** `Message.Hub`, `SendRequest.Hub`, `HubInfo`, and the `hub` field of the journal are gone. A message with `to` is direct; one without is a post to the room.
- **H9.** The MCP tools are `post` (body), `send_direct_message` (to, body), `list_agents`, and `check_inbox`. `post_to_hub` and `list_hubs` are gone. `list_agents` lists every participant with its role (`orchestrator`, `owner`, or `operator` for the user) and description, and the system prompt shows the same list.
- **H10.** A turn's trigger reads `X in the room` or `X directly`, and the UI's turn parser reads both.

### Vocabulary

- **H11.** The project has one vocabulary, `store/shared/vocabulary/terms.yaml`, with no `hub` field. `get_vocabulary`, `define_term`, `define_interface`, and `remove_entry` take no hub. Only the orchestrator writes it; with no orchestrator, it can be read and stays empty unless you write it. The interface flags and providers of a system come from it and from every other system.

### The public wiki

- **H12.** The hub wikis become one wiki, `store/shared/wiki/`, with the ref `public`. Its kinds are `story`, `rule`, and `principle` for the orchestrator, and `plan` and `note` for you. The orchestrator writes it through `change_public_wiki`, which replaces `change_hub_wiki`. Every agent reads it at `/hikari/public/readonly/shared/wiki/`.
- **H13.** A system wiki can link the public wiki, and the public wiki can link every system wiki. An agent reads its own system wiki and the public wiki; the orchestrator also reads every system wiki.
- **H14.** The key-story flag, its route, and rule K3 are gone. A `key:` field in an old document is ignored.
- **H15.** `store/hubs/` is no longer read, prepared, or mounted. A rename's propagation scans `systems/*/wiki`, `shared/wiki`, and the vocabulary, and only for `@name`: the `#name` sigil is gone.

### Store and releases

- **H16.** `Store.Hubs`, `HubID`, `HubName`, `HubDir`, and `HubWiki` are gone, and `VocabularyFile()` takes no hub. The store's README describes the vocabulary and the public wiki.
- **H17.** `publish_release` announces the release in the room. A new manifest has no `hubs:` field, and an old manifest that has one still loads.

### Editor

- **H18.** The operations `add_hub`, `set_hub`, `remove_hub`, `add_member`, and `remove_member` are gone, and so are the `hubs` field of `add_agent` and the hub cases of `rename` and `change_id`. `set_orchestration {agent}` sets or clears the orchestrator. `remove_agent` of the orchestrator clears the key. `docs/specs/topology-api.md` lists the operations as they are.

### Prompts and skills

- **H19.** The system prompt has a section "## The room", which lists the participants and states the wake and message rules. No prompt, skill, or agent definition in `internal/node` mentions a hub.
- **H20.** The `hikari-hub` skill becomes `hikari-orchestration`, about the public wiki, the vocabulary, and carrying out a plan. `hikari-system` and its references say "the public wiki" and "the vocabulary".
- **H21.** `containerize-repo` and `hikify` write `x-hikari.orchestration` and no hubs. Hikify drafts the public wiki in `onboarding/wiki/public/` instead of `onboarding/wiki/hubs/<hub>/`, and its plan is `onboarding/wiki/public/hikify-plan.md`.

### Examples and scaffold

- **H22.** `examples/studio` and `examples/team` are deleted. `examples/shop` names its orchestrator in `x-hikari.orchestration`. `hikari init` writes `orchestration: orchestrator`. The Makefile's default `COMPOSE` is the shop, and `make up` says to run `make demo-repo` first when the shop's repo is missing.

### UI

- **H23.** The UI has no hub pages, hub diagram, network map, or hub group in the sidebar. The sidebar shows the room, then the agents. The room's page has the tabs Messages, Vocabulary, and Wiki, and its route is `#/room`. The composer posts to the room or sends a direct message, and its wake hint follows H6.
- **H24.** The Structure tab lists the project, the repos, the agents, and the sidecars. The project's inspector picks the orchestrator, or none. The agent inspector has no hub section, and the add form has no hub variant.
- **H25.** The Map follows decision 5.

### Docs

- **H26.** `README.md` and `docs/design.md` describe one room. The section "Hubs, not a free graph" becomes "One room", with the reason: one orchestrator and its owners is the team that the project needs today, and a tree of hubs can come back when a project needs islands. "A vocabulary for each hub" becomes "One vocabulary". `docs/guides/onboarding.md` follows H21.
- **H27.** Each spec that describes hubs as built gets one line under its status: "Hubs were removed in `remove-hubs.md`."

### Checks

- **H28.** `go vet ./...`, `go test -race ./...`, `npm test`, and `npm run build` pass.
- **H29.** No file in `internal/`, `cmd/`, `web/src/`, or `examples/` contains `hub` as a word, `#hub`, `post_to_hub`, or `list_hubs`, except `GitHub` and `github-release`.
- **H30.** A live run of the shop with two owners and an orchestrator: a post from the user without mentions wakes only the orchestrator; the orchestrator's post without mentions wakes both owners; an owner sends a direct message to the other owner; the orchestrator defines a term and writes a rule into the public wiki, and an owner reads both. The UI shows the room, its vocabulary, and its wiki.
