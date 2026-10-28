---
name: hikari-scope-work
description: How the orchestration agent works the task pipeline - scope a stub with the user into a scope with scope_task, carry out a scope that a queue starts by sending each part to its partition agent and following their statuses, and validate it before asking for done with set_task_status. Load it when the user asks you to scope a stub, when hikari tells you that a queue started a scope or that its parts are done, and when the user asks you to arrange queues.
---

# Work the task pipeline

The pipeline holds the user's work as markdown documents in `/hikari/tasks/`, which you read; you change them only through your tools. A stub is the user's idea in a few words. A scope says what changes in which partitions, and what shows that it is done. A queue orders scopes; a scope in a queue is approved and is the user's, and a running queue starts its scopes one after another, one scope at a time in each partition. `list_tasks` shows the whole pipeline.

## Scope a stub

The user asks you to scope a stub in your direct messages.

1. Read the stub with `get_task`, and the feature maps that it concerns: yours, and each partition's with `get_partition` and `get_document`. Find the features it changes or creates, the partitions whose code holds them, and the interfaces between them.
2. Ask the user your questions in that conversation, all in one message: what the stub leaves open, each choice between two behaviors, and what done means. You may ask a partition agent a question about its own features with @name, but do not ask it to start work.
3. When the user has answered, call `scope_task`: the partitions, the features, for each partition its part in terms of its features and interfaces, the goal, what shows that it is done, and the questions with their answers. Name in `features` the features whose health decides that the work is done, and `needs` for a scope of another queue that must be done first.
4. Tell the user that the scope is a draft until it enters a queue. When the user asks, put it in one with `change_queue`. Only the user starts or pauses a queue.

## Carry out a scope

hikari tells you when a queue starts a scope.

1. Read the scope with `get_task`. Order its parts: a part that provides an interface or a release goes before the parts that use it.
2. Send each part to its partition agent by direct message when its needs within the scope are met: the scope's id, what its part changes, and what it needs from the other parts. Tell it to follow the `hikari-task-work` skill.
3. Follow the statuses. hikari tells you when a part is done, blocked, or no longer blocked, and the notice of each turn lists them. When a part is blocked, read why, and resolve what you can: ask another partition agent, or ask the user. When a part waits for another, send it once the other is done.
4. When a part reports that its work cannot be done as the scope says, tell the user, with the reason. Never change a scope in a queue; it is the user's.

## Validate a scope

hikari tells you when every part is done, and the scope is `validating`.

1. Read the health of the scope's features, and of your own features that link them: the flows across the partitions.
2. When every one passes, and `## Done when` holds, call `set_task_status` with `done` and a note on what you checked. hikari grants it only when the scope's features in your feature map pass.
3. When a flow fails, call `set_task_status` with `blocked` and what failed, send the part back to the partition agent whose feature fails, and clear the block with `started` once it is fixed.
