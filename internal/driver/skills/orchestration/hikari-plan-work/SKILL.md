---
name: hikari-plan-work
description: How the orchestration agent turns a request into work for the partition agents from the feature maps - which partitions, which features, and in what order - and how to read the user's plans and carry one out. Load it when a request needs work in more than one partition, and when the user asks you to carry out a plan.
---

# Plan the work

You have no code. You plan from the feature maps: yours, in `/hikari/self/features/`, and each partition's, which `get_partition` and `get_document` read.

1. Find the features that the request touches. Start from your own features: each names the partition features of a flow. `search_wiki` finds the rest.
2. For each partition, name the features that it changes, creates, or removes, and the interfaces between the partitions that change.
3. Order the work: a partition that provides an interface goes before the partitions that consume it, and a partition that tests against another's release waits for that release.
4. Post the plan in the room, and give each partition agent its part with @name, in terms of features and interfaces, not code. The partition agent decides how to make a change, and can push back with a reason.
5. When a part is done, read the partition's health, and the health of your features that link its features. Then send the next part.
6. When the work changes a flow, update your feature map with `change_wiki`: the `hikari-map-features` skill has the format.

## Plans and notes

The user's plans and notes are in your feature map and in the public wiki. A plan is work the user intends; its `status` says how far it is. Read them for context: they say where the user wants the project to go. Act on a plan only when the user asks you to. Never write, rename, or remove one.

## Carry out a plan

When the user asks you to carry out a plan, you track it; the actors do the work. A plan's steps each name an actor, what changes, what the step needs first, and when it is done.

1. Read the plan, and each partition plan that it names.
2. Post the steps in the room as a list, each with its state: waiting, sent, done, or blocked.
3. When the needs of a step are met, send the step to its actor by direct message: the step's id, what changes, what it needs, and when it is done. Name the partition plan in the actor's wiki, where the partition agent reads its part. Send a step only after the releases that it needs are announced.
4. A step whose actor is `session` or the user is the user's: ask the user, and wait for their answer.
5. When an actor reports a step done, check what you can see: the release in the announcement, the health in the map, the health of your features. Then post the new state of the steps.
6. When a step cannot be done as written, tell the user, with the reason and the actor's words. Never change the plan; it is the user's.
