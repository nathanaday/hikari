# Spec: hikify design

Status: built. Checked in a live run on 2026-10-23 on the containerized onboard example, in a headless Claude Code session that took the user's answers in follow-up turns. Two analysis subagents wrote their reports, and the synthesis found the copied `accounts` module and its difference. The session asked which behavior is right, among seven questions, and did not choose. With the answers, it drafted a target of an `accounts` library in a new repository, `server` and `cli` as its consumers, and a tester `main-e2e`. `hikari check -draft` passed. It gave each boundary its reason, and named two splits it decided against. To check the tester's image, it built it and ran the browser test in it. It drafted four stories, a rule, and a principle of `#main`, and set `key: true` on the three that the user picked. It wrote a hub plan of nine steps, four system plans, and a ledger that places each of the nine baseline tests once. Five review rounds found 12, 10, 7, 6, and 3 problems, all resolved, one of them blocking: the `test` key of a system seeds only a new test map, so a `PYTHONPATH` there never reached the suites. The user approved the plan after round 5.

The tester, environments, and image releases were removed in `remove-e2e.md`.

Changes made during the build:

1. **A round of minor findings ends the review** (RV4). Reviewers who are told to find faults always find a minor one. The live run needed five rounds on a project of two small repositories, and its findings still stood. Now a round whose findings that stand are all minor ends the review, once the session resolves them. Four rounds with serious findings still stop for the user.
2. **A plan states only what the user decided** (RV6). A reviewer of round 4 found that the plan gave two behaviors of cli as the user's decisions, which the session had made up while it resolved findings. Such a behavior is now a proposal until the user decides it.
3. **The README says that a system's `test` key seeds only the first suite.** A reviewer found this in the source. Without it, a target topology that changes `test` has no effect on an existing system.

Sources: `docs/brainstorm/onboarding.md` (round 1, sections D, E, and F) and `onboarding-answers.md`. This is the third of four steps of the onboarding work, after linked repositories and containerize. Execution and verification are the fourth.

## Summary

`hikify` is the second host skill. It starts from a containerized project and designs the refactor with you, in three stages:

1. **Analysis.** A fresh agent reads each repository with its owner's map and the baseline, and writes a report. The session then writes one synthesis across the reports: duplicated code, candidate boundaries, and candidate E2E tests. You answer its questions.
2. **Target topology.** The session drafts the target compose file and explains it in plain words, with a diagram and the reason for each boundary. With you, it drafts the stories, rules, and principles of each hub, and you mark the key stories.
3. **Change plan.** The session writes the plan as plan documents for each hub and system, and a ledger that says where each baseline test goes. Fresh reviewers look for what would break, in rounds, until no finding stands. Then you approve.

Nothing runs differently until step 4. The design lives in `onboarding/` until then, because the target's new hubs and systems do not exist in the store yet. One new hikari feature supports the design: `hikari check -draft` checks a target topology whose new repositories and scopes do not exist yet.

## Decisions

From round 1 and its answers:

1. One fresh agent for each repository writes an analysis report with fixed sections; one synthesis follows; you answer the open questions before the topology stage (D1).
2. The target topology is a draft compose file with a plain explanation and a diagram; you approve the explanation (D2).
3. Each boundary has a stated reason of one of three kinds, and the splits decided against are named (D3).
4. You and the skill write the hub wikis; owners keep writing their own features (D4).
5. Your two principles go into the hikify skill and into `docs/design.md` (E1).
6. The change plan is a set of plan documents in the wikis (F1).
7. Fresh agents review the plan adversarially, in rounds, and you see the findings (F2).
8. Steps follow the order of releases (F3).

## Decisions made in this spec

1. **The target compose file is `compose.target.yaml` at the top of the project, not in `onboarding/`.** Compose resolves a relative path from the file's own folder. The target names the same Dockerfiles and build contexts as `compose.yaml`, so it sits beside it.
2. **Drafts of wiki documents live in `onboarding/wiki/` until step 4.** They mirror the store: `onboarding/wiki/hubs/<hub>/<id>.md` and `onboarding/wiki/systems/<system>/<id>.md`. A hub or system of the target may not exist yet, and hikari prepares a wiki only for a hub or system in the running topology. Step 4 copies the drafts into the store when the target becomes `compose.yaml`.
3. **The session writes the hub wikis' stories, rules, and principles as your documents.** The UI and Obsidian already let you write a document of any kind, and hikari checks it as a hand edit. The session acts for you, so its drafts are your edits. After step 4, the orchestrator keeps them.
4. **The ledger is a table that can be checked mechanically.** `onboarding/verification.md` lists every baseline test once, with its planned place. The skill counts the ids against the baseline before each review, so no review round is spent on a missing row.
5. **Review rounds stop after four.** If a finding still stands after the fourth round, the session stops and asks you. A plan that four rounds of fresh reviewers still fault needs your judgment, not a fifth round.
6. **The host skill carries the wiki reference of the agents' skill.** The binary writes the document format of `hikari-system` and `hikari-hub` into `hikify`'s references, so the session writes drafts that hikari accepts, from one source.

## Terms

- **Target topology**: `compose.target.yaml` and its explanation, `onboarding/topology.md`.
- **Draft**: a wiki document in `onboarding/wiki/`, written for the target.
- **Change plan**: the plan documents `hikify-plan*.md` of each hub and system, in `onboarding/wiki/`.
- **Step**: one unit of the change plan, with an id, an actor, what it changes, what it needs first, and when it is done.
- **Restructure step**: a step that the session does on the host: it creates a repository, moves code between repositories, or changes the topology.
- **Ledger**: `onboarding/verification.md`.

## 1. `hikari check -draft`

- **DR1.** `hikari check -draft <file>` loads the file and runs the topology's rules. It does not refuse a linked repository that is not cloned, a scope or read path that does not exist yet, or a path repository that does not exist yet.
- **DR2.** It skips the checks of the running project: ignore rules, uncommitted changes, and credentials.
- **DR3.** It prints `the draft is valid` with the counts of agents and systems, or each finding, and exits with status 1 on a finding.

## 2. The skill

- **HK1.** `hikify` refuses to start until `onboarding/state.md` records `containerize: done`. Then it names the `containerize-repo` skill.
- **HK2.** Its stages, in `state.md`: `hikify: analysis`, `hikify: topology`, `hikify: plan`, `hikify: review`, and `hikify: approved`. Step 4 adds its own stages after these.
- **HK3.** It follows the rules of `containerize-repo`: it commits its files at once, as you; it puts no secret in a file; it never pushes; and it keeps `state.md` current.
- **HK4.** In this step it changes no file in a clone, and it does not change `compose.yaml`.

## 3. Analysis

- **AN1.** For each repository, one fresh subagent reads the code, the owner's map and wiki in the store, and the baseline. It writes `onboarding/analysis/<repo>.md` with these sections:
  - **Releases**: what the repository makes for others;
  - **Consumes**: what it takes from the other repositories, and how (copied code, a package, an API, files);
  - **Interfaces**;
  - **Duplicated code**, with the files on both sides and each difference in behavior;
  - **E2E tests**: the tests that need a display, a browser, or the whole application, and the flows that no test covers because they cross systems;
  - **Run files**;
  - **Open questions**.
- **AN2.** The session writes `onboarding/analysis/synthesis.md` from the reports:
  - each duplicated piece of code, with its copies and their differences;
  - the boundaries it proposes, each with its reason (TP3);
  - the E2E candidates;
  - the questions for you.

  A difference in behavior between two copies is always a question: only you decide which behavior is right.
- **AN3.** The session asks you the questions, and records each answer in `state.md` under Decisions before the topology stage. An answer can also go into a report's `## Your answers` section, which the session reads.

## 4. Target topology

- **TP1.** `compose.target.yaml` is the whole target topology, and `hikari check -draft` passes on it.
- **TP2.** `onboarding/topology.md` explains the target in plain words:
  - each system, library, and hub;
  - a diagram in a `mermaid` block, which Obsidian and GitHub draw;
  - the reason for each split and merge;
  - each split that it decided against, and why;
  - each new repository, marked "to create".
- **TP3.** Each boundary states one reason, of one of three kinds:
  - **library**: two or more consumers need the code, or their copies already differ;
  - **client and server**: they change at different rates, or a team owns each;
  - **scope in a repository**: a repository holds two concerns, but splitting its history is not worth its cost.

  A boundary with no stated reason is not proposed.
- **TP4.** Each hub of the target that has E2E candidates gets a tester, with `tests: <hub>` and a scope in the project repository, `e2e/<hub>`.
- **TP5.** A new repository has the URL that you choose, in `x-hikari.repos`. Step 4 asks you to create it.
- **TP6.** The session drafts the stories, rules, and principles of each hub with you, in `onboarding/wiki/hubs/<hub>/`. The drafts follow the hub wiki format, and link the features of the owners' maps by their ids. You choose the key stories, and the session sets `key: true` on them for you.

## 5. Change plan

- **PL1.** `onboarding/wiki/hubs/<hub>/hikify-plan.md` (or `hikify-plan-<hub>.md` with more than one hub, since an id is unique in the store) is a plan document (`kind: plan`, `status: proposed`). It holds the steps of the hub in order:
  - restructure steps first, then the steps of the owners and the tester;
  - for each step: its id (`S1`, `S2`, ...), the actor (`session`, or an agent of the target), what changes, what it needs first (steps or releases), and when it is done, in terms that a test or a file can show.
- **PL2.** `onboarding/wiki/systems/<system>/hikify-plan-<system>.md` holds the steps of one system, with the same fields, for its owner.
- **PL3.** The steps follow the order of releases:
  - a library is extracted and released before its consumers move to it;
  - each consumer moves in its own step, and its suite passes against the release before its old copy is deleted;
  - no step deletes code that a consumer still uses.
- **PL4.** A restructure step names how code moves with its history, for example `git subtree split` and a pull into the new repository.
- **PL5.** `onboarding/verification.md` lists each test of the baseline once, with its planned place:
  - kept, in the system that has the code after the plan;
  - moved to the tester, with the story that it checks;
  - dropped, with a reason that waits for your approval.

  The tests that containerize left out are rows too. The skill checks that every baseline id appears once (decision 4).
- **PL6.** Your two principles are rules of the plan: each system owns one concern behind clean interfaces, and each system tests only its own code with unit tests. A test that needs a display, a browser, or the whole application moves to the tester.

## 6. Review

- **RV1.** Each round sends two or more fresh reviewer subagents in parallel, with the brief in the skill's references. Each reads the plan, the reports, the target topology, the ledger, and the code, and returns findings with evidence. They look for:
  - a step that breaks a behavior that the baseline covers;
  - a step that needs a release that no earlier step makes;
  - a boundary without a reason of TP3, or one that breaks the principles;
  - a test that the ledger loses;
  - a step whose "done when" nothing can show.
- **RV2.** The findings of round n go to `onboarding/review/<n>.md`.
- **RV3.** The session resolves each finding with a change to the design, or rejects it with a reason. It records the resolution beside the finding.
- **RV4.** A round whose findings that stand are all minor ends the review, once the session resolves them. After four rounds that each had a finding that blocks or should be fixed, the session stops and asks you (decision 5).
- **RV6.** A plan states only what you decided. A fix that needs a behavior you have not decided is marked as a proposal, and the session asks you at approval.
- **RV5.** The session shows you the plan, the target topology, and the reviews. When you approve, it sets `status: approved` on the plans and the stage `hikify: approved`.

## 7. Principles

- **PR1.** The skill's references hold the two principles of your note, with the reason for each, as rules of TP3 and PL6.
- **PR2.** `docs/design.md` gets a section on hikari's principles.

## 8. Out of scope

- Execution: the restructure, the owners' steps, and verification (step 4).
- Creating remote repositories (step 4).

## 9. Build plan

1. **`check -draft`** (DR1 to DR3), with tests.
2. **The skill.** `SKILL.md` and its references: the analysis brief, the synthesis, the topology guidance, the principles, the plan and ledger forms, the review brief, and the wiki reference from the agents' skills (HK, AN, TP, PL, RV, PR1).
3. **Docs.** The principles in `docs/design.md` (PR2), the README.
4. **Live check.** On the containerized onboard example, run the skill headless. Let it ask its questions, answer them in the conversation, and let it review and finish. Check that it finds the copied `accounts` module and its difference, asks which behavior is right, proposes a library with a reason, puts the browser test with the tester, drafts a key story, and writes a ledger with every baseline test. Then update this spec's status.
