---
name: room
description: Work with the agents of a running hikari project from this session - post a request to the orchestration agent or a partition agent with hikari send, follow their replies in the run's messages, and read the partitions' health and the agents' commits. Use for ask the agents to, tell the orchestrator, start working in the hikari containers, what did the agents say, is hikari running, how are the partitions doing, what did the agents change, once a project is containerized or onboarded. A change of the partitions themselves is change-topology.
---

# Work in the room

You work for the user in their own Claude Code session, in a hikari project whose agents run under `hikari up`. The agents talk to each other and to the user only through one room, and you post there as the user, with `hikari send`. A request that spans partitions goes to the orchestration agent, which plans it from the feature maps and sends each part to the partition agent whose folders hold the code. You do not change the partitions' code yourself: that is the agents' work, each in its own container.

The project root is the folder of `topology.yaml`. Run each command there, as `cd <root> && …`.

## Rules

1. Post only what the user asked for. Show the user each message before you send it, unless the user gave you its text.
2. Never merge, push, or reset a branch without the user's approval in the conversation, for that one action.
3. Wait for the agents' replies between posts; a second request before the first is answered splits their work.

## 1. Is hikari running?

The session's start says whether hikari runs for the project, and `hikari send` says so too. When it does not run, ask the user to start it in their own terminal, and to leave it running:

```sh
hikari up topology.yaml
```

It prints the address of the web UI, where the user can follow each agent's turns. Its first start builds the images, which takes minutes. A run that this session starts would stop when the session ends, so start one here only when the user asks, in the background, and say that it stops with the session. A message waits in an agent's inbox until the agent reads it.

## 2. Post

`x-hikari.orchestration` in `topology.yaml` names the orchestration agent, and each service with a `partition` is a partition agent.

- Work that spans partitions, or when you are not sure: post to the room, naming the orchestration agent:

  ```sh
  hikari send "@<orchestration agent> <request>"
  ```

- A question for one partition agent about its own code: `hikari send -to <agent> "<question>"`.
- A long message: `hikari send - <<'EOF'`, the message, then `EOF`.

Write the request as the user means it: the outcome, the limits the user named, and what shows that it is done. Name a feature by its id, as `[[id]]`, when a feature map has it; the partitions' feature maps are in `vault/partitions/<id>/read-only/features/`, and the orchestration feature map is in `vault/orchestration/read-only/features/`.

## 3. Follow

- `.hikari/runs/<newest>/messages.jsonl` holds every message of the run, one JSON object to a line: `from`, then `to` for a direct message, then `body` and `time`. Read its newest lines.
- `hikari status topology.yaml` shows each partition's map and recorded tests; `hikari status -tests topology.yaml` shows each test.
- `git -C repos/<name> log <project branch>` shows each partition agent's commits. `x-hikari.project.branch` names the project branch, which `HIKARI_BRANCH` in `.env` can set.

Check every few minutes; a step takes an agent many minutes. Tell the user in a few lines who answered, what changed, and what waits for them. When an agent asks the user a question, give the user the question, and post their answer.

## 4. When the work is done

The agents' commits wait on the project branch of each repository, for the user to review and merge into their own branches. Show the user the commits, and the health from `hikari status`. Push only with the user's approval: `hikari push topology.yaml` pushes the project branch of each linked repository, then of the project.
