I read the canvas. Before the questions, here is my understanding of the idea, so you can correct it first.

## What I take from the canvas

- **The orchestrator never sees code.** It works only from the system map: which features exist, who owns them, and their current health. It finds gaps between what you want and what exists, then negotiates with owners the way a team lead does. An owner can push back ("we don't do that, because…"), and that sends the orchestrator back to rethink the architecture.
- **Each sandbox is a bind-mounted directory with one owning agent.** The mount is a real physical link, not a symbolic one. The deliverable is the change to the sandbox itself, so the agent produces no separate output and makes no promises.
- **A feature shard is the unit of the map.** It records a feature's existence and health: related features, code references, test references, test results, findings and plans. The system map is the set of shards, plus each system's responsibilities and its inputs and outputs (IO).
- **A full cycle (from the canvas):** feature-set query → proposal → accept or reject → formal work order → change rounds checked by a validation agent → change log → map update.

I'll use these terms: a **system** is the code one agent owns (a repo or a subdirectory of one), and a **system map** is the feature map of one system.

## Round 1 questions

Each question has my recommendation. A short answer like "agree" or "no, because…" is enough.

**1. What is in this spec?** I suggest this spec covers:
- mounts and ownership,
- the system map (its schema, storage, building it and keeping it current),
- the MCP tools to query the map,
- a read-only Systems view in the UI.

The formal work order, the change log, and the validation agent go in the next spec. They depend on the map existing, and the free chat in hubs already covers the negotiation for now. Do you agree, or do you want work orders in this spec?

**2. How does an agent write to your repo?** Your repo is live on the host while the agent edits it. I recommend:
- The agent edits your working tree in place.
- It never commits, switches branches, resets or pushes. The prompt forbids these, and the container has no git credentials, so a push can't work anyway.
- You review the agent's work with `git diff` and commit it yourself.

This matches "the change to the sandbox is the work itself". The risk: if you edit the same files while the agent works, the edits mix. The other option is a separate git worktree on a branch for each agent. That keeps your checkout untouched, but it is a lot more complex, because git worktrees store absolute host paths that don't exist inside the container. Which do you want?

**3. How big is a scope?** I recommend:
- A system is a repo path plus an optional subdirectory, for example `~/code/shop` with scope `services/api`.
- The agent can write only inside its scope.
- Optional read-only paths for shared code, such as `proto/`.
- Two agents may share a repo only with scopes that don't overlap; hikari rejects overlapping scopes.

Is a subdirectory scope enough, or do you need a list of paths for one system?

**4. Where is the map stored, and who writes it?** I recommend:
- The map is a file inside the system, for example `services/api/.hikari/map.yaml`, so it is versioned and reviewed with the code it describes.
- The server is its only writer: the worker changes it through MCP tools, and the server validates each change against a schema.
- Inside the container, the map file is mounted read-only, so the agent can't bypass the tools.
- The orchestrator keeps no separate copy, which avoids two copies drifting apart. Its wiki is a live, computed view of all the maps it can see.

Are you OK with hikari putting files in your repos? If not, maps live in hikari's own data directory instead.

**5. What does a feature record hold, and what does the orchestrator see?** From the canvas plus your message, each system and feature has:
- **System:** purpose, responsibilities, IO (APIs, events or files it exposes and consumes), and a short layout summary.
- **Feature:** id, name, description, code references, test references, last test results, health (`passing`, `failing` or `untested`), findings, plans, and related features, which can be in other systems.

Two sub-questions:
- (a) **What does "test coverage" mean here?** I recommend: which tests cover the feature, plus their last result. Line-coverage percentages need separate tooling for each language, and they say little about whether the feature works.
- (b) **Should the orchestrator's view leave out code and test file paths?** I recommend yes. The orchestrator gets description, IO, health and test counts, findings and plans, but no paths. That enforces "guarded from implementation details" in the data itself, not only in the prompt.

**6. When is the map built, and how does it stay true?** I recommend:
- On first start with no map, the worker reads its system and builds the map. That one-time read costs a lot of tokens.
- The map records the repo state it describes, as the git commit plus a hash of the uncommitted changes in scope.
- At the start of each turn, the driver checks for changes since then, including your own edits on the host. If there are any, the worker's prompt lists the changed paths, and it updates the affected features.
- If you edit the map by hand, the server reloads it.

Do you want automatic building, or an explicit "index this system" action?

**7. Who can query whose map?** I recommend this follows the hub rules:
- An agent can read the maps of systems owned by agents it shares a hub with.
- The orchestrator gets the redacted view from question 5.
- Owners see the full view of their own map.
- You see everything in the UI.
- Only the owner can change a map.

Should workers see their peers' maps, or only their own?

**8. Can an orchestrator also own a system?** I recommend no, for now:
- An agent either owns one system or owns none.
- Orchestrators have no mount at all, so they physically can't read code.
- An owner can still be a hub member.

Your `team.yaml` pattern, where `eng-lead` passes work down a tree of hubs, keeps working, as long as `eng-lead` itself owns no system.

**9. How does a worker run the tests?** The current image has only Debian and Claude Code, so it can't run `go test` or `npm test` on your repos. I recommend:
- An optional `image:` for each agent, built `FROM hikari-node`, that adds the toolchain.
- An optional `test:` command for each system. If none is set, the agent finds the test command itself and records it in the map.

Do your target repos already have Dockerfiles or devcontainer configs that we should reuse instead?

**10. How strict is "the only thing the agent can reach"?** Today the containers have open internet, which Claude Code needs for the Anthropic API, and which `npm install` and web fetches also use. I recommend:
- Mounts are the hard boundary for this spec.
- Hikari reads no host files except what you mount.
- Limiting the network to an allowlist is its own later spec.

Is open internet acceptable for now?

## What these recommendations add up to

```yaml
agents:
  lead:
    description: Product lead.                 # no system: cannot read code
  api:
    description: Owns the order API.
    system:
      repo: ~/code/shop
      scope: services/api
      read: [proto]                            # optional read-only paths
      test: go test ./...                      # optional
    image: hikari-node-go                      # optional toolchain image
hubs:
  product: {orchestrator: lead, members: [api]}
```

The MCP tools would be:
- **Read tools, for everyone:** `list_systems`, `search_features`, `get_feature`, `get_system`.
- **Write tools, for the owner only:** `upsert_feature`, `remove_feature`, `set_system_info`, `record_test_results`.
