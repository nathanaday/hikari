# Hikari's principles

## 1. Each partition has one concern

hikari isolates a library from its consumers, and a client from its server. Each agent has one partition: one concern, behind clean interfaces, in a container that sees only its own code. The orchestration agent links the partitions, so that together they work as the architecture describes.

Why: an agent that has one concern holds all of it in its context, tests all of it, and cannot break code it cannot see. A partition that does two things makes its partition agent trade one concern against the other. A copy of code in two partitions drifts, and the drift is a bug that no unit test finds.

What it rules out:

- one partition that holds a client and its server;
- code copied between partitions, where a library would serve both;
- a partition that reaches into another partition's code instead of using its release or its interface.

## 2. Each partition tests only its own code

Each partition tests its own code with the unit-test framework of its language. A suite that needs a display, a browser, the whole application, or another partition is left for integration: it stays in its repository, outside every test command, and `onboarding/` records it with the reason. hikari does not run such suites yet. Never move one into another partition.

Why: a dry run of the whole application inside one partition's tests depends on code that the partition does not own, so its failures name the wrong partition agent. It is slow, and it needs tools that the partition's container should not carry.

What it rules out:

- a partition's suite that starts a browser, a display, or another partition's service;
- a test that checks a flow across partitions from inside one of them.

A sign: when a container needs a display to run its tests, that suite is left for integration.
