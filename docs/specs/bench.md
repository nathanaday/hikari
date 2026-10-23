# Spec: bench results

Status: draft.

Sources: `docs/brainstorm/e2e.md` (round 1, question C7) and `e2e-answers.md`. This is the fifth and last step of the E2E work.

## Summary

Some stories can only be checked on real hardware: a model on an NPU, a relay on a digital output, a camera on a network. The tester writes such a check as a bench procedure in its plan: the steps you follow on the bench, and what proves it passed. You run it, and record pass or fail in the UI, with a note. A bench result counts like an E2E result for the stories it touches, and it goes pending when the release set or the procedure changes, because then it no longer says anything about what is pinned.

## Decisions

From round 1 and its answers:

1. The tester writes a bench check as a procedure; you run it and record the result in the UI (C7).
2. A bench result counts like an E2E result, and goes pending when the release set changes (C7).

## Decisions made in this spec

1. **Procedures live in the plan.** A procedure is part of the test system's design, as a gap is, and the plan already holds what touches which story.
2. **A result also goes pending when its procedure changes.** A result of an older procedure says nothing about the new one.
3. **Results are yours, in the store,** beside the approvals of gaps, and committed with your git identity.
4. **A story's health joins its E2E tests and its bench results.** A failure of either fails it; else a pending one makes it pending; else a pass passes it; it is untested only when neither has a result.

## 1. The procedure

```yaml
bench:
  npu-detect:
    title: The vehicle detector runs on the NPU
    touches: ["[[vehicle-suppresses-alarm]]"]
    procedure: |
      1. Flash the image of the pinned release onto the gateway.
      2. Point the camera at the parking lot, and drive a car through the frame.
      3. Pass when the incident list shows no alarm and the gateway's log shows backend tflite-npu.
```

- **BE1.** `bench` is an optional section of `plan.md`: a map from an id, which keeps the id rule of a gap, to a procedure with `title`, `touches` (links to stories, rules, or principles of the hub, at least one), and `procedure` (text, not empty).
- **BE2.** The plan's checks warn about a procedure that touches nothing of the hub, as they do for a gap.

## 2. Results

- **BR1.** You record a result in the UI: pass or fail, and an optional note. hikari keeps the last result of each procedure in `store/hubs/<hub>/bench.yaml`, with the time, a digest of the lock, and a digest of the procedure's text.
- **BR2.** A result is current while the lock and the procedure are unchanged since you recorded it; otherwise it is pending.
- **BR3.** A change of the results is committed at once with your git identity: `user: record <id> as <pass or fail> in #<hub>`.
- **BR4.** `get_test_plan` and the Tests tab show each procedure with its last result, whether it is current, and its note.

## 3. Health

- **BH1.** The health of a story, a rule, or a principle of a tested hub joins the E2E health of its tests and the results of the procedures that touch it: `failing` when a current test result or a current bench result failed; else `pending` when one of them is pending; else `passing` when one passed; else `untested`.
- **BH2.** A key story with a procedure and no test is covered: the plan does not warn about it.

## 4. Skill

- **BS1.** The `hikari-tester` skill says when to write a procedure instead of a gap: when the check needs hardware the test system lacks, and the user has it. A procedure names what to set up, what to do, and what proves a pass, in steps a person follows without the code.

## 5. Build plan

1. **Plan and results.** The `bench` section (BE1, BE2), the results file (BR1, BR2), with tests.
2. **Health and tools.** BH1, BH2, BR3, BR4; the UI endpoint and the Tests tab; the skill.
3. **Live check and docs.** In the sensors example, the tester writes a procedure for the edge watchdog; you record a pass in the UI; the watchdog's story is passing; a new pin makes it pending.
