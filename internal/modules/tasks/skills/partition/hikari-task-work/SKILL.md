---
name: hikari-task-work
description: How a hikari partition agent works its part of a scope from the task pipeline - read the scope with get_task, report each change of your part with set_task_status (started, testing, blocked, done), do the work with the plan, do, and validate skills, and plant a stub for work outside your part. Load it when a message or a notice names a scope with a part for you.
---

# Work your part of a scope

A scope is a piece of work across partitions, which the user approved by putting it in a queue. Its part for you says what your partition changes, in terms of your features and interfaces. The orchestration agent sends you the part when its turn comes, and follows its status, which you report. The status is how the user and the orchestration agent see your work, so report each change the moment it happens.

1. Read the scope with `get_task`: its goal, your part under `## Parts`, the features it names, and `## Done when`. Read the other parts too, for the interfaces you share with them.
2. Call `set_task_status` with `started`.
3. Do the work as for any request: plan it with the `hikari-plan-work` skill when it changes more than one feature, change the code with `hikari-do-work`, and keep your feature map true.
4. When your suites run, call `set_task_status` with `testing`. Run them with the `hikari-validate-work` skill.
5. When they pass, call `set_task_status` with `done`. hikari grants it only when the health of every feature of the scope in your partition is passing; otherwise your part stays `testing`, and the answer names each feature that is not passing. Fix them, run the suites again, and ask again.
6. The moment you cannot go on, call `set_task_status` with `blocked` and a note that says why and what you need: a release of another partition, a decision of the user, a change of an interface. When the block clears, set the status you are at again.

Report only your own part. Work outside it is not yours to do in this scope: when you find a fault in code you cannot see, or work that the scope did not plan, plant a stub with `plant_stub`, say so in your answer to the orchestration agent, and go on with your part.
