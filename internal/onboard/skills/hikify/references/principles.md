# Hikari's principles

## 1. Each system owns one concern

hikari isolates a library from its consumers, and a client from its server. Each agent owns one system: one concern, behind clean interfaces, in a container that sees only its own code. The orchestrator links the systems, so that together they work as the architecture describes.

Why: an agent that owns one concern holds all of it in its context, tests all of it, and cannot break code it cannot see. A system that does two things makes its owner trade one concern against the other. A copy of code in two systems drifts, and the drift is a bug that no unit test finds.

What it rules out:

- one system that holds a client and its server;
- code copied between systems, where a library would serve both;
- a system that reaches into another system's code instead of using its release or its interface.

## 2. Each system tests only its own code

Each system tests its own code with the unit-test framework of its language. A test that needs a display, a browser, the whole application, or another system is an E2E test, and it belongs to the hub's tester, which runs it against the released products of the systems.

Why: a dry run of the whole application inside one system's tests depends on code that the system does not own, so its failures name the wrong owner. It is slow, and it needs tools that the system's container should not carry. The tester runs such a test against releases, where a failure names the story that broke and the systems behind it.

What it rules out:

- a system's suite that starts a browser, a display, or another system's service;
- a test that checks a flow across systems from inside one of them.

A sign: when a container needs a display to run its tests, that test is an E2E test.
