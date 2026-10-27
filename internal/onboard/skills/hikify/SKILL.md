---
name: hikify
description: Design the refactor of a containerized hikari project with the user - analyze each repository, draft a target topology of isolated systems, libraries, and hubs, write the change plan, and have fresh reviewers check it until it holds. Use in a hikari project repository after containerize-repo is done, when the user wants to hikify, refactor for hikari, split repositories into systems, extract shared libraries, or plan the target topology.
---

# Hikify: design the refactor

You work for the user in their own Claude Code session, in a hikari project repository. The project is containerized: each repository runs as one hikari system, its owner has mapped it, and `onboarding/baseline.md` records every test. You design how the project becomes a set of isolated systems behind clean interfaces, and then you carry the design out with hikari's agents. The design is yours to draft and the user's to decide. Until the user approves it, nothing in the running project changes: the design stays in `onboarding/` and `compose.target.yaml`.

Read `references/principles.md` first. Every boundary and every plan step must keep its two principles.

## Rules

1. Start only when `onboarding/state.md` records `containerize: done`. Otherwise tell the user to run the containerize-repo skill first, and stop.
2. Before approval, change no file in a clone in `repos/`, and do not change `compose.yaml` or the store. The design goes into `onboarding/` and `compose.target.yaml`.
3. Commit each change at once, with the user's git identity (a plain `git commit`), on the branch that is checked out.
4. Put no secret in any file. Push or create a remote repository only with the user's approval in the conversation, for that one action.
5. Keep `onboarding/state.md` current: the stage, the decisions the user made, and the open questions.
6. A difference in behavior between two copies of code is always a question for the user. Never pick the behavior yourself.

The stages, in `state.md`: `hikify: analysis`, `hikify: topology`, `hikify: plan`, `hikify: review`, `hikify: approved`, then `hikify: restructure`, `hikify: execute`, `hikify: verify`, and `hikify: done`. Read `state.md` at the start and continue from its stage.

## Stage 1: Analysis

1. For each repository, send one fresh subagent with the brief in `references/analysis.md`, all in one message so that they run in parallel. Each writes `onboarding/analysis/<repo>.md`.
2. Read the reports, and write `onboarding/analysis/synthesis.md` in the form of `references/synthesis.md`: the duplicated code with its differences, the boundaries you propose with their reasons, the suites left for integration, and the questions.
3. Commit, set the stage to `hikify: topology`, and ask the user the questions in one message. Show the proposed boundaries in a few lines each. Wait for the answers. Record each answer in `state.md` under Decisions. An answer can also be in a report's `## Your answers` section.

## Stage 2: Target topology

Read `references/topology.md`.

1. Write `compose.target.yaml`: the whole target topology. It starts from `compose.yaml`, and adds the systems, libraries, hubs, and grants of the design. Run `hikari check -draft compose.target.yaml` until it passes.
2. Write `onboarding/topology.md`: each system, library, and hub in plain words; a diagram in a `mermaid` block; the reason for each split and merge, of one of the three kinds; each split you decided against, and why; each new repository, marked "to create".
3. Draft the deliverable of each new library in `onboarding/shared/deliverables/<name>/`, as `references/topology.md` shows.
4. Draft the stories, rules, and principles of each hub with the user, in `onboarding/wiki/hubs/<hub>/<id>.md`. Follow `references/hub-wiki.md` for the format, and `references/wiki.md` for the rules of ids and links. Ignore the tools that those references name; they are the agents'. You write the drafts as files. Link the features of the owners' maps by their ids, which are in `store/systems/<system>/wiki/`. Propose the stories that the suites left for integration check, and the flows across systems that no test covers. Ask the user which stories are key, and set `key: true` on those.
5. Commit. Show the user the topology in a few lines, the diagram's file, and the stories. When the user agrees, set the stage to `hikify: plan`.

## Stage 3: Change plan

Read `references/plan.md`.

1. Write the hub plan, `onboarding/wiki/hubs/<hub>/hikify-plan.md`, for each hub: every step, in order. Restructure steps come first, then the steps of the owners. With more than one hub, name each `hikify-plan-<hub>.md`, because an id is unique in the whole store.
2. Write the system plan, `onboarding/wiki/systems/<system>/hikify-plan-<system>.md`, for each system that has steps: that system's steps, for its owner.
3. Write the ledger, `onboarding/verification.md`: every test of the baseline, and every suite that containerize left out, once each, with its planned place.
4. Check the ledger. Count the test ids in `onboarding/baseline.md` and in the ledger, and fix every id that is missing or appears twice.
5. Commit, and set the stage to `hikify: review`.

## Stage 4: Review

1. Send two or more fresh reviewer subagents with the brief in `references/review.md`, all in one message. Give each the round number. Each returns findings with evidence.
2. Write the findings of round n to `onboarding/review/<n>.md`.
3. Resolve each finding: change the design, or reject the finding with a reason. Write the resolution under each finding. Check the ledger again after any change.
4. Commit. When a finding of this round was a `blocks` or a `should fix` that stood, start the next round with new reviewers. They also check this round's fixes, which can bring new faults. A round whose findings that stand are all `minor` ends the review, once you have resolved them.
5. After four rounds that each had a `blocks` or `should fix` that stood, stop. Show the user the open findings and how the counts fell, and ask how to go on.
6. A plan states only what the user decided. When a fix needs a behavior that the user has not decided, mark it as a proposal in the plan and in `state.md`, and ask the user at approval.

## Approval

Show the user the plan in a few lines: the steps of each hub, and the reviews' main findings with their resolutions. Name the files to read: `onboarding/topology.md`, the plans, and the ledger. When the user approves, set `status: approved` on each plan, and set the stage to `hikify: approved`. Commit. Then ask whether to start the restructure now.

## Stages 5 to 7: Restructure, execute, verify

When the user agrees to start, read `references/execute.md`, and follow it from the stage in `state.md`.

## References

- `references/principles.md`: hikari's two principles, and why.
- `references/analysis.md`: the brief for each analysis subagent.
- `references/synthesis.md`: the form of the synthesis.
- `references/topology.md`: how to draw boundaries, and the compose parts of a target.
- `references/plan.md`: the forms of the hub plan, the system plan, and the ledger.
- `references/review.md`: the brief for each reviewer.
- `references/execute.md`: the restructure, the run of the plan, and the verification.
- `references/hub-wiki.md`, `references/wiki.md`: the format and rules of wiki documents, from the agents' own skills.
