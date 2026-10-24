# The plan and the ledger

## The hub plan

`onboarding/wiki/hubs/<hub>/hikify-plan.md` is a plan document of the hub wiki. Its id is `hikify-plan`, which must be unique in the store: when two hubs have plans, use `hikify-plan-<hub>`.

```markdown
---
id: hikify-plan
kind: plan
name: Hikify the project
description: The steps that take the containerized project to its target topology.
status: proposed
---

## Purpose

Why the project changes, in two or three sentences, with links to the stories that the target proves.

## Steps

### S1. Create the units repository
- Actor: session
- Changes: makes the repository `units`, with the code of `cloud/app/units.py` and its history, by `git subtree split` in cloud and a pull into the new repository. The user creates the remote.
- Needs: nothing.
- Done when: `repos/units` exists, and its log holds the history of the moved files.

### S2. Release the units library
- Actor: units
- Changes: shapes the moved code into the interface of `[[convert-reading]]`, and publishes `python-library` 0.1.0.
- Needs: S1.
- Done when: the release `units/python-library/0.1.0` exists, and units' suite passes.

### S3. Move cloud to the units release
- Actor: cloud
- Changes: imports the conversion from the units release, and deletes its own copy.
- Needs: S2 (the release `units/python-library/0.1.0`).
- Done when: cloud's suite passes against the release, and cloud holds no copy of the conversion.
```

Fields of each step:

- **Actor**: `session` for a restructure step, which this session does on the host: it creates a repository, moves code with its history, writes a Dockerfile, copies drafts into the store, or replaces `compose.yaml` with the target. Otherwise, the agent of the target that does the step.
- **Changes**: what the step changes, in terms that its actor understands. An owner cannot see another system's code, so tell it what its own code must do.
- **Needs**: the steps, and the releases by name and version, that must exist first.
- **Done when**: what a test, a release, or a file shows when the step is done. "Works" is not a criterion; "the suite passes against the release 0.1.0" is.

Order: restructure steps first. Then a library's release before its consumers move. Each consumer moves in its own step, and its suite passes against the release before its own copy is deleted. No step deletes code that a consumer still uses. A move of an E2E test to the tester comes after the releases that the test needs.

## The system plan

`onboarding/wiki/systems/<system>/hikify-plan-<system>.md` holds the steps of one system, with the same fields, for its owner. It is a plan document of the system wiki. It repeats the hub plan's step ids, so that the owner and the orchestrator name a step the same way.

## The ledger

`onboarding/verification.md` lists each test of the baseline, and each suite that containerize left out, once:

```markdown
# Verification

| Test | Now in | Planned place | Story or reason |
|---|---|---|---|
| tests.test_units > test_round | edge | kept: units | |
| tests.test_api > test_store | cloud | kept: cloud | |
| tests/ui/test_dashboard.py (left out) | cloud | moved: main-e2e | [[operator-sees-reading]] |
| tests.test_old > test_v1_format | cloud | dropped | The v1 format is gone; waits for the user's approval. |
```

- **kept: <system>**: the test stays a unit test, in the system that holds its code after the plan.
- **moved: <tester>**: the test becomes an E2E test, with the story that it checks.
- **dropped**: the test goes, with a reason. A drop waits for the user's approval.

Execution fills in the result of each row. Every test id of the baseline appears once.
