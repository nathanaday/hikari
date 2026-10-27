---
name: hikari-plan-work
description: How a hikari partition agent plans a change before it makes it - for a change of more than one feature, post the plan in the room with the features it changes, creates, or removes, the interfaces it touches, and what it needs from other partitions, and wait for an answer when it needs another partition. Load it when a request asks you to change code.
---

# Plan the work

Read the request against your feature map: which features it changes, which it creates, and which it removes. A change of one feature needs no plan; load the `hikari-do-work` skill and make it.

Before a change of more than one feature, post the plan in the room:

- the features it changes, creates, or removes, by id;
- the interfaces it touches, with the names in the vocabulary, and whether it changes what another partition consumes;
- what it needs from other partitions: a feature, a change of an interface, or a release.

When the plan needs another partition, mention its partition agent with @name, and wait for an answer from it or from the orchestration agent before you start. Their answer starts a new turn. When the plan needs no other partition, start at once with the `hikari-do-work` skill.

When the work turns out other than the plan, post what changed before you go on.
