---
name: hikari-scope-work
description: How the orchestration agent works the task pipeline - scope an issue that a queue started into a scope with scope_task, carry out a scope by sending each part to its partition agent and following their statuses, validate it before asking for done with set_task_status, and plan an issue in public with comment_issue. Load it when hikari tells you that a queue started an issue, when it tells you that the parts of a scope are done, when the user asks you to scope, plan, or comment on an issue, and when the user asks you to arrange queues.
---

# Work the task pipeline

The pipeline's unit of work is an issue: a GitHub issue of one of the project's repositories, or a local issue in the vault. Each issue that enters a queue gets a task, a markdown document in `/hikari/tasks/`, which you read and change only through your tools. A scope is the task once you have written what changes in which partitions and what shows that it is done. A queue orders tasks; a running queue starts them one after another, one scope at a time in each partition. `list_tasks` shows every issue and each queue; `list_repositories` shows where the work stands in git.

## Scope an issue

hikari tells you when a queue starts an issue that is not scoped yet. The queue holds every other queue until you write the scope, so do it in this turn.

1. Read the issue with `get_issue`: its body and its comments hold the user's request and answers. Read the feature maps that it concerns: yours, and each partition's with `get_partition` and `get_document`. Find the features it changes or creates, the partitions whose code holds them, and the interfaces between them.
2. Decide what the issue leaves open. Take the most reasonable reading, and write each assumption with its reason in `answers`; do not wait for the user. Ask the user in your direct messages only when a choice would waste the partitions' work either way, and still write the scope with your best reading, so that the work starts.
3. Call `scope_task` with the issue (`<repo>#<number>`) or the task id: the partitions, the features whose health decides that it is done, for each partition its part in terms of its features and interfaces, the goal, what shows that it is done, and the assumptions. hikari posts the scope to the issue for the user.
4. Go on with Carry out a scope: the task is already started.

The user may also ask you to scope an issue that is in no queue, or to plan one in public. Then read it the same way, and either call `scope_task` (the task waits in no queue until the user queues it) or call `comment_issue` with the plan: the partitions, the features, and the open questions, in a few lines.

## Carry out a scope

1. Read the scope with `get_task`. Order its parts: a part that provides an interface or a release goes before the parts that use it.
2. Send each part to its partition agent by direct message when its needs within the scope are met: the task's id, what its part changes, and what it needs from the other parts. Tell it to follow the `hikari-task-work` skill.
3. Follow the statuses. hikari tells you when a part is done, blocked, or no longer blocked, and the notice of each turn lists them. When a part is blocked, read why, and resolve what you can: ask another partition agent, or ask the user. When a part waits for another, send it once the other is done.
4. When a part reports that its work cannot be done as the scope says, tell the user, with the reason. Never change a scope in a queue; it is the user's.

## Validate a scope

hikari tells you when every part is done, and the scope is `validating`.

1. Read the health of the scope's features, and of your own features that link them: the flows across the partitions.
2. When every one passes, and `## Done when` holds, call `set_task_status` with `done` and a note on what you checked. hikari grants it only when the scope's features in your feature map pass, and then tells the issue that the work is done and which branch holds it.
3. When a flow fails, call `set_task_status` with `blocked` and what failed, send the part back to the partition agent whose feature fails, and clear the block with `started` once it is fixed.

## Report where the work stands

When the user asks what is done, what is merged, or what comes next, call `list_repositories` and answer from it: your branch of each repository, how far it is ahead, whether it is pushed, and each open pull request with its checks. Opening a pull request, pushing, and merging are the user's clicks on the Repositories page; you may say that they are the next step.
