---
name: change-topology
description: Change the topology of an onboarded hikari project - add a repository, split or merge partitions, extract a shared library, split a library into its server and client parts, or move folders between partitions - through hikify's analysis, plan, review, approval, and execution, scoped to the partitions that change. Use for split this partition, add this repo to the project, extract a library, give X its own partition, move this folder to another partition, after onboarding. Onboarding itself is containerize-repo and hikify; work inside the partitions as they are is room.
---

# Change the topology

You work for the user in their own Claude Code session, in an onboarded hikari project. A change of topology moves code between partitions, so it goes through the stages of hikify, scoped to the partitions it touches. You design the change with the user, fresh reviewers check it, the user approves it, and then you and hikari's agents carry it out.

The project root is the folder of `topology.yaml`. Run `cd <root>` once, as a command of its own, so that later commands run there, and give file tools and subagents absolute paths. When the root lies outside the session's folders, the shell goes back to its start folder after each command; then give each command the root instead, as `git -C <root> …` and `hikari <command> <root>/topology.yaml`. Do not join `cd` and `git` in one command, which Claude Code stops to ask about.

Each change has a folder, `changes/<id>/`, laid out as `onboarding/` is. `<id>` is the date and a short name in lowercase letters, digits, and dashes, such as `2026-11-02-split-api`. You follow hikify's references, and read each one with these substitutions:

| hikify's reference says | For this change |
|---|---|
| `onboarding/` | `changes/<id>/` |
| `onboarding/baseline.md` | `changes/<id>/baseline.md` |
| the plan id `hikify-plan` | `change-<id>-plan` |
| the partition plan id `hikify-plan-<partition>` | `change-<id>-plan-<partition>` |
| the stage `hikify: <stage>` | `change: <stage>` |

## Rules

1. Start only in an onboarded project: `topology.yaml` has partition agents, and `onboarding/state.md` says `hikify: done`, or the project has no `onboarding/`. During onboarding, the change belongs to hikify: tell the user, and stop.
2. Before approval, change no file in a clone in `repos/`, and do not change `topology.yaml` or the vault. The design goes into `changes/<id>/` and `topology.target.yaml`.
3. Commit each change at once, with the user's git identity (a plain `git commit`), on the branch that is checked out.
4. Put no secret in any file. Push or create a remote repository only with the user's approval in the conversation, for that one action.
5. Keep `changes/<id>/state.md` current: the stage, the decisions the user made, and the open questions.
6. A difference in behavior between two copies of code is always a question for the user. Never pick the behavior yourself.

The stages, in `state.md`: `change: scope`, `change: analysis`, `change: topology`, `change: plan`, `change: review`, `change: approved`, `change: restructure`, `change: execute`, `change: verify`, and `change: done`. When `changes/` holds a change that is not done, read its `state.md`, and continue from its stage.

## Stage 1: Scope

1. Agree `<id>` with the user, and write `changes/<id>/state.md`: the line `Stage: change: scope`, the date, the request in the user's words, and the sections `## Scope`, `## Decisions`, and `## Open questions`.
2. Name with the user, under Scope, the partitions that the change touches: the ones whose folders change, and the ones that consume what moves, whose tests must still pass.
3. A new repository joins first, through "Add a repository" of the `containerize-repo` skill, so that it has a partition, a map, and a baseline before the design starts.
4. Write `changes/<id>/baseline.md`: the date, the commit of each touched repository's project branch (`git -C repos/<name> rev-parse HEAD`), and, in a code block, the lines of `hikari status -tests topology.yaml` for the touched partitions.
5. Commit, and set the stage to `change: analysis`.

## Stage 2: Analysis

1. For each touched partition, send one fresh subagent with the brief in `../hikify/references/analysis.md`, read with the substitutions, all in one message so that they run in parallel. Each writes `changes/<id>/analysis/<repo>.md` for the repository of its partition's first folder.
2. Write `changes/<id>/analysis/synthesis.md` in the form of `../hikify/references/synthesis.md`, for the touched partitions only.
3. Commit, set the stage to `change: topology`, and ask the user the questions in one message. Record each answer in `state.md` under Decisions.

## Stages 3 to 5: Topology, plan, review

Follow hikify's Stages 2, 3, and 4 and its Approval, with `../hikify/references/principles.md`, `../hikify/references/topology.md`, `../hikify/references/plan.md`, and `../hikify/references/review.md`, read with the substitutions, and scoped:

- `topology.target.yaml` starts from `topology.yaml`, and changes only the partitions in scope.
- `changes/<id>/topology.md` describes what changes: the partitions before and after, a diagram of the touched partitions and those they meet, and the reason for each split or merge, of the three kinds.
- Draft only the orchestration features and the rules that the change adds or changes, in `changes/<id>/vault/`.
- The ledger, `changes/<id>/verification.md`, lists each test of `changes/<id>/baseline.md` once.
- The review runs at least one round, with two or more reviewers, and stops by hikify's rules of Stage 4.

When the user approves, set `status: approved` on each plan, set the stage to `change: approved`, commit, and ask whether to start the restructure now.

## Stages 6 to 8: Restructure, execute, verify

Follow `../hikify/references/execute.md` from the stage in `state.md`, read with the substitutions. Its message to the orchestration agent names `[[change-<id>-plan]]`. Its dry run in a fresh clone is needed only when the change adds or removes a repository.

When every check holds, set the stage to `change: done`, with the date, and commit. Sum up for the user: the partitions before and after, the releases, the tests and where they went, and the branches that wait for the user's merge.

## References

- `../hikify/references/principles.md`: hikari's two principles.
- `../hikify/references/analysis.md`, `../hikify/references/synthesis.md`: the analysis.
- `../hikify/references/topology.md`, `../hikify/references/plan.md`, `../hikify/references/review.md`: the target, the plan and the ledger, and the review.
- `../hikify/references/execute.md`: the restructure, the run of the plan, and the verification.
