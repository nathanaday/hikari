# Spec: hikify execution

Status: built, except the onboarding of P3 (decision 7), which waits for the user. Checked in a live run on 2026-10-23 on the onboard example, after the user approved the plan of the design run. The session did the restructure steps S1 to S3 with hikari stopped. It made `accounts` from the server's copy of the rules with its history, created its bare remote and pushed `main` with the user's approval, and cloned it with `hikari sync`. It installed the images, the two deliverables, the drafts, and the target topology, and moved the two rule features into the library's wiki. `hikari check` passed, and every tree was clean. After `hikari up`, the user's post in `#main` started the lead, which sent each step only after the release it needed was announced. accounts released `python-library` 1.0.0, then server and cli moved to it, deleted their copies, and released `python-program` 1.0.0, cli with 11 tests where it had 3. The tester ran the server release in its environment, drove the sign-up page in Chromium and the cli release against it, and all six documents of `#main` passed, the three key stories included. The session filled the ledger: accounts 5, server 1, cli 11, and main-e2e 7 tests passed, with no row missing or dropped. It pushed the hikari branches with the user's approval. A clone of the project passed `hikari sync` and `hikari check`, and its `hikari up` connected all five agents.

The tester, environments, and image releases were removed in `remove-e2e.md`.

Changes made during the build:

1. **A tester's environment override is written to its private `environment/` folder.** hikari wrote `.environment.override.yaml`, which holds this machine's paths, into the tester's scope, where the tester's commit picked it up. The session found it during verification. The folder is already out of git (LG2), and a Docker test now checks that the scope stays clean.
2. **Decision 1 held in the run.** The plan put the restructure commits on each repository's hikari branch, the agents found them there at start, and no merge was needed before `hikari up`.

Observed, not changed:

- **In a headless run, the user follows the agents.** A headless session turn cannot wait for the agents, so in the live run the user read the messages and brought the session back when the lead reported the last step. In an interactive session, the session follows the run as EX3 says.
- **The orchestrator answers posts before the plan starts.** The tester's first post woke the lead, which asked the owners to publish before the user started the plan. The owners refused, rightly, because the release that they needed did not exist yet. The plan then ran in order.

Sources: `docs/brainstorm/onboarding.md` (round 1, sections G, H, and I3) and `onboarding-answers.md`. This is the last of four steps of the onboarding work, after linked repositories, containerize, and hikify design.

## Summary

After you approve the plan, `hikify` carries it out in three stages:

1. **Restructure.** With hikari stopped, the session does the plan's steps whose actor is `session`. It makes each new repository from moved code, with its history; you create its remote, and `hikari sync` clones it. It installs the drafts: the wiki documents, the deliverables, and the target topology. Then `hikari check` passes.
2. **Execute.** You start hikari, and ask the orchestrator to carry out the plan. The orchestrator sends each step to its actor when the step's needs are met, and posts its progress in the hub. Owners change their own code, publish releases, and move to the releases of others. The tester proves the stories. The session follows the run from files, and does the steps that come back to it.
3. **Verify.** The session fills the ledger: every baseline test is kept, moved, or dropped with your approval. Then it runs a dry run from a fresh clone of the project, which proves that a teammate gets the same project.

## Decisions

From round 1 and its answers:

1. The session does the restructure; hikari's agents do the code changes (G1).
2. You create the remote of each new repository; with your approval in the conversation, the session runs the command (G3).
3. E2E tests move to the tester (G4).
4. The orchestrator tracks the plan in the hub (G5).
5. Hikify is done when the baseline is accounted for and the key stories pass (H1).
6. A dry run from a fresh clone ends onboarding (H2).
7. P3 is the first real onboarding (I3).

## Decisions made in this spec

1. **The restructure commits go on the hikari branch of each repository, as yours.** G2 put them on an `onboard` branch that you merge first. But `hikari up` checks out the hikari branch, so commits on another branch reach no agent until you merge them into it. A merge there adds a step and changes nothing about your review. The hikari branch is already the branch that you review and merge into your own, and the restructure on it carries your identity, apart from the agents' commits. Your own branch stays untouched until you merge. The plan of the live run of hikify design came to the same choice.
2. **A new repository reaches `repos/` only by a clone from its remote.** The session makes the repository in a temporary folder, and you create the remote and push its main branch to it. Then `hikari sync` clones it, as it clones every linked repository. So the clone's origin is always the topology's URL (SY2), and a teammate's sync gets the same repository.
3. **The session moves the documents of a feature that changes systems, while hikari is stopped.** An id is unique in the whole store, so the new owner cannot create a feature that the old owner's wiki still holds. The session moves the document from one system's wiki to the other, as a hand edit of yours. The new owner then keeps it.
4. **You start the plan with a post in the hub.** Agents act on a plan only when you ask (the wikis spec). The skill gives the text to post, and the orchestrator follows the `Carry out a plan` procedure of the `hikari-hub` skill.
5. **The session follows the run from files.** It reads `hikari status`, the run's `messages.jsonl`, and git. It never talks to an agent. A step that comes back to the session comes through you, or through the orchestrator's post in the hub, which you see.
6. **The dry run starts hikari until the agents connect, and no further.** The restore turns of a fresh clone are covered by the linked repositories spec. The dry run proves the clone, the sync, the check, the build of every image, and the start.

## Terms

- **Restructure step**: a step whose actor is `session`.
- **Dry run**: a start of the project from a fresh clone in a temporary folder.

## 1. The skill

- **HX1.** `hikify` continues after `hikify: approved` with the stages `hikify: restructure`, `hikify: execute`, `hikify: verify`, and `hikify: done`.
- **HX2.** It keeps the rules of its design stages: commits as you, at once; no secret; `state.md` current. It pushes and creates a remote only with your approval in the conversation, for that action.

## 2. Restructure

- **RS1.** The session does the restructure steps in the order of the plan, with hikari stopped. After each step, it checks the step's "Done when", and records the result in `state.md`. A step whose check fails stops the stage, and the session tells you.
- **RS2.** A new repository: the session makes it in a temporary folder, from the moved code with its history, with a README and an ignore file. It gives you the commands that create the remote and push `main`. With your approval, it runs them. Then it runs `hikari sync`.
- **RS3.** The session copies:
  - each draft of `onboarding/wiki/` into the store's wikis;
  - each deliverable of `onboarding/shared/deliverables/` into `store/shared/deliverables/`;
  - `compose.target.yaml` over `compose.yaml`, and removes the target.
- **RS4.** The session moves the documents of features that change systems (decision 3). It removes their entries from the old owner's code map and test map, in the old scope's private documents.
- **RS5.** The session commits in each repository it changed, on its hikari branch, and in the project. Then `hikari check compose.yaml` must pass, and every repository is clean.

## 3. Execute

- **EX1.** The session asks you to start `hikari up compose.yaml`, then to post the start text in the hub. The start text names the plan, and asks the orchestrator to carry it out from the first step that is not the session's.
- **EX2.** The orchestrator follows `Carry out a plan` (`hikari-hub`): it sends each step to its actor when the step's needs are met, and posts the state of the steps in the hub.
- **EX3.** The session follows the run (decision 5). When a step's actor is `session` or you, the session does it or asks you.
- **EX4.** When an owner reports that a step cannot be done as written, the orchestrator tells you. The session proposes a change to the plan, which you approve, and it updates the plan documents in the store as your edit.
- **EX5.** The stage ends when the orchestrator reports every step done, and the session confirms each "Done when" that it can check from files.

## 4. Verify

- **VF1.** The session fills in the result of each row of the ledger from `hikari status -tests`: the test's outcome in its planned system, or `missing`.
- **VF2.** Hikify is done when:
  - every row has a result, and none is `missing`;
  - every drop has your approval;
  - every owner's suite passes;
  - every key story is passing, as the orchestrator reports from `get_test_plan`;
  - `hikari check` reports nothing.
- **VF3.** **Dry run.** The session asks you to push, with `hikari push`. With your approval, it runs it. Then it clones the project, from its origin or else from its folder, into a temporary folder. It runs `hikari sync` and `hikari check`, and starts hikari until every agent connects, then stops it and removes the folder. A dry run that fails is a finding of verification.
- **VF4.** The session sets the stage `hikify: done`, and sums up the project for you: its systems and releases, the moved tests, and the stories with their health.

## 5. Out of scope

- Merging the hikari branches into your own.
- Teammates' runs: the linked repositories spec covers them.

## 6. Build plan

1. **The skill.** The execution stages of `hikify` (HX, RS, EX, VF), in its `SKILL.md` and references.
2. **The orchestrator.** `Carry out a plan` in `hikari-hub` (EX2), with its test. Done before this spec.
3. **Live check.** On the onboard example, after the design is approved: the session restructures, you start hikari and post the start text, the agents carry out the plan, and the session verifies and runs the dry run. Then the README, `docs/design.md`, and the status of this spec and of `hikify-design.md`.
4. **P3.** The first real onboarding, with you (I3).
