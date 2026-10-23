# Spec: the tester, the plan, and the release lock

Status: built, except environments, which are the fourth step. Checked in live runs on 2026-10-22 and 23 in the vote project: a tester, `vote-e2e`, with the owners `tally` and `ballots`. On its first turn, with no release published, the tester wrote a plan that named the blocker and claimed no gap. After both owners published `python-library` 0.1.0, it pinned both, wrote 7 tests that walk the key story and check the principle, and recorded them: the story `ballots-decide-winner` became `passing` from E2E results. It added a gap for a real voting machine when asked; the gap was approved through the UI and committed as the user. When tally published 0.1.1, the tester's turn context named the newer release.

Changes made during the build:

1. **Tests can link principles too** (TT1, EH2). In the live run, the tester checked the principle `ballots-decide-validity` in the same tests as the story, and could not link it. A principle that a test can check is worth E2E health.
2. **The test map says what a link may name.** Its refusal said "not a feature", which misled the tester; it now names a feature of the wiki, or for a tester a story, rule, or principle of its hub.

Sources: `docs/brainstorm/e2e.md` (round 1, sections A, B, C, D, and G) and `e2e-answers.md`. This is the third step of the E2E work, after the project repository and the wikis. Environments for services are the fourth step, and bench results the fifth.

## Summary

A tester is an owner whose scope is the E2E scaffold of one hub. Its test map links the stories and rules of the hub's wiki, so each story and rule gets health from E2E tests, as a feature gets health from unit tests. The tester writes an architecture plan, `plan.md`: the components of the test system, the actors that drive it, the scenarios, and the gaps it cannot test. It pins the release set it tests in `releases.lock` through a tool, and a result counts only for the release set it ran on. You approve each gap in the UI. In this step, the tester runs its tests in its own container against the releases it can read: libraries, binaries, and data. Services that must run beside it come with environments, in the fourth step.

## Decisions

From round 1 and its answers:

1. A tester, not the orchestrator, writes and runs the E2E suite (A1).
2. The topology names the hub that a tester checks; the tester gets the releases of the hub's systems (A2).
3. Story health comes from E2E results; the UI shows the support beside it (B5).
4. The tester writes a plan for each hub, and you review it (C1); its header names components, actors, scenarios, and gaps (C2); substitutes and simulators say what they replace (C3); hikari checks the plan (C5); a gap needs your approval to count (C6).
5. The tester pins a release set in a lock file, with a tool (D1); a change to the lock makes every E2E result pending (D2); hikari tells the tester about newer releases (D3).
6. A failing story names its suspects: the systems whose features it links (G1).

## Decisions made in this spec

1. **The tester is an ordinary owner with one extension.** It has a system wiki, a map, a code map, and a test map, like any owner. Only its test map's links reach further: to the stories and rules of its hub. This keeps one test map format, one results file, and one health computation.
2. **The plan lives in the tester's scope, and hikari serves it.** The plan describes the scaffold, so it is versioned with it, as `layout.yaml` is. The orchestrator and you read it through hikari, because neither can read the scope.
3. **The lock is the "code" of a story.** A result of a test that links a story counts while the test's file and `releases.lock` are unchanged since the run. So a new pin makes every E2E result pending (D2) through the hashing that already decides when a result counts.
4. **Approvals are yours, in the store.** You approve a gap in the UI. hikari keeps each approval with a digest of the gap's text, so that a gap the tester rewrites needs a new approval.

## Terms

- **Tester**: the owner that a topology marks with `tests: <hub>`.
- **Tested hub**: the hub that a tester checks.
- **E2E health**: the health of a story or a rule of a tested hub, from the tester's results.
- **Plan**: `plan.md` at the root of the tester's scope.
- **Lock**: `releases.lock` at the root of the tester's scope.

## 1. Topology

```yaml
  vote-e2e:
    build: {context: ., dockerfile: python.Dockerfile}
    x-hikari:
      description: Proves the stories of #main against the releases of its systems.
      system: {repo: project, scope: e2e/main}
      tests: main
```

- **TS1.** `x-hikari.tests` names a hub. Only an agent with a system can have it.
- **TS2.** The tester is a member of the tested hub, not its orchestrator.
- **TS3.** A hub has at most one tester.
- **TS4.** A tester can read the releases of every system whose owner is a member of the tested hub, besides its own `grants.releases`. Its container mounts them as any release grant does.

## 2. The tester's test map

- **TT1.** A key of the tester's `tests.yaml` can link the stories, rules, and principles of the tested hub's wiki, besides the features of its own wiki. Any other link is a problem of the test map, as today.
- **TT2.** A rename or a remove of a story or a rule in the hub wiki follows into the tester's test map and results, as a rename of a feature does in an owner's.
- **TT3.** The code of a story or a rule, for the hashes of a run, is `releases.lock`. A result of a test that links one counts while the test's file and the lock are unchanged since the run (decision 3).

## 3. E2E health

- **EH1.** The E2E health of a story or a rule of a tested hub is computed from the tester's results and test map, with the rules of feature health: `failing`, `pending`, `passing`, or `untested`.
- **EH2.** The UI, `get_document`, and `search_wiki` show a story's or a rule's E2E health as its `health`, beside its `support`. A hub without a tester shows no `health`.
- **EH3.** A story or a rule whose E2E health is `failing` names its suspects: the systems whose features it links, in name order.
- **EH4.** A story or a rule with no test and a gap that touches it shows the gap: its id, its reason, and whether you approved it.

## 4. The plan

```markdown
---
components:
  tally:     {release: tally/python-library}
  ballots:   {release: ballots/python-library}
  sample:    {fixture: true, note: 100 ballots written by hand}
  machine:   {simulates: a voting machine, note: feeds ballots as a machine would}
actors:
  operator:  {runs: tests, count: 1}
scenarios:
  smoke:     {stories: [ballots-decide-winner], actors: {operator: 1}}
gaps:
  real-machine:
    what: Ballots read from a real voting machine
    why: no machine is reachable from the test system
    touches: ["[[accept-ballot]]"]
---

## The test system

(prose, and a diagram when one helps)
```

- **PL1.** `plan.md` is a markdown file with a property header. Its four sections are optional; no other property is allowed.
- **PL2.** A component has exactly one of: `release` (`<system>/<deliverable>`), `image` (an image reference), `fixture: true` (data that the tester makes), or `simulates` (text: the hardware it stands in for). A component with `release` or `image` may set `substitutes` (text: the external service it stands in for). Each may set `note`.
- **PL3.** An actor has `runs`: a component, or `tests` for the tester's own suite; and `count`, at least 1. It may set `note`.
- **PL4.** A scenario has `stories`: ids of stories or rules of the tested hub, at least one; `actors`: a count for each actor it uses; and may set `minutes`, at least 1.
- **PL5.** A gap has an id that matches the id rule of the wiki, and `what`, `why`, and `touches`: links to stories, rules, or features of the tested hub's systems, at least one.
- **PL6.** hikari checks the plan when it computes the tester's view. A plan that does not parse is a problem of the private documents, which blocks `record_test_run` and `mark_map_current` as a broken test map does. Every other finding is a warning:
  - a release component that the lock does not pin;
  - a link or a story id that resolves to nothing the plan may name;
  - a key story with no test and no gap.
- **PL7.** `get_test_plan(hub)` returns the plan of a hub, with its findings, the lock, the gaps with their approval, and the E2E health of each story and rule. The tester and the hub's orchestrator can call it.

## 5. Gaps and approval

- **GA1.** A gap is proposed until you approve it. You approve and withdraw approvals in the UI.
- **GA2.** hikari keeps approvals in `store/hubs/<hub>/approvals.yaml`: for each gap id, a digest of its `what`, `why`, and `touches`. An approval whose digest no longer matches is void, and the gap is proposed again.
- **GA3.** In a project repository, a change of approvals is committed at once with your git identity: `user: approve <gap> in #<hub>`, or `withdraw`.

## 6. The lock

- **LK1.** `pin_releases(releases)` is a tester tool. Each entry names a system, a deliverable, and a version of a release that the tester can read. hikari writes `releases.lock` at the root of the tester's scope with each entry and the release's digest, sorted, replacing what was there:

  ```yaml
  version: 1
  releases:
    - {system: ballots, deliverable: python-library, version: 0.1.0, digest: "sha256:..."}
  ```

- **LK2.** hikari checks the lock when it computes the tester's view. An entry whose release is missing, or whose digest differs from the release's, is a problem of the private documents.
- **LK3.** Before each turn, the tester learns which pinned releases have a newer version, as a notice: `ballots/python-library 0.2.0 is newer than your pinned 0.1.0`.
- **LK4.** In this step, the tester's suites use the pinned releases where its container mounts them: `/hikari/public/readonly/systems/<system>/releases/<deliverable>/<version>/product/`.

## 7. Prompts and skills

- **SK1.** A new built-in skill, `hikari-tester`, is installed for testers, beside `hikari-system`. It explains the plan, the lock, the test map's links to stories, and these procedures:
  - **Start**: read the hub wiki and the releases; write `plan.md`; pin the releases; ask the user to review the plan, through the hub.
  - **Test**: write tests that walk each story and check each rule, map them to the stories in `tests.yaml`, run them, and call `record_test_run`.
  - **Report**: tell the orchestrator in the hub about each failing story: the story, the test, the step that failed, what was expected, and what happened.
  - **Keep true**: after a new release or a changed story, pin again, run again, and update the plan.
- **SK2.** The tester's system prompt names its hub, its plan and lock, and the releases it can read, and tells it to load `hikari-tester`. Its first turn follows the Start procedure instead of the Build procedure.
- **SK3.** The orchestrator's prompt names the tester of each hub it orchestrates and `get_test_plan`.

## 8. The UI

- **UT1.** A hub with a tester gets a Tests tab: the tester, the plan's body and its four sections as tables, the findings, the lock, each gap with its approval and an Approve or Withdraw button, and each story and rule with its E2E health, its support, and its suspects.
- **UT2.** The Tests lens of a hub's wiki map colors a story or a rule by its E2E health, and by its support when the hub has no tester.

## 9. Out of scope

- Starting services beside the tests (the fourth step).
- Bench results (the fifth step).
- More than one tester for a hub, and a tester for a parent hub (B7).

## 10. Build plan

1. **Topology.** `tests`, rules TS1 to TS4.
2. **Plan and lock.** Parsing and checking the plan (PL1 to PL6) and the lock (LK1, LK2), with tests.
3. **Health.** The tester's test map links (TT1 to TT3), E2E health (EH1 to EH4), approvals (GA1 to GA3).
4. **Tools, notices, prompts, skill.** `pin_releases`, `get_test_plan`, LK3, SK1 to SK3.
5. **UI.** UT1, UT2.
6. **Live check and docs.** In the vote project, the tally and ballots owners publish their libraries; a tester writes a plan with one gap, pins the releases, tests the key story, and the story's E2E health becomes `passing`; you approve the gap in the UI; a new release makes the story `pending`.
