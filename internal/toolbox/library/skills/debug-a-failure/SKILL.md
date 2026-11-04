---
name: debug-a-failure
description: Find and fix the cause of a failing test or a reported fault - reproduce it, isolate it, state the cause with evidence, fix it, and keep the test that catches it. Load it when a suite fails, when health says a feature is failing, or when the orchestration agent reports a fault in your partition.
---

# Debug a failure

Fix the cause, not the symptom, and leave a test behind.

1. **Reproduce.** Run the failing test alone and read the whole message. If the fault came as a report, write the test that reproduces it first; if you cannot reproduce it, say so with what you tried, and stop.
2. **Isolate.** Narrow the input until the smallest case fails. Read the code path with `code-map.yaml` as the map. Add a temporary assertion or a log line where you expect the value to be right, and find where it stops being right.
3. **State the cause.** One sentence with evidence: "`reserve()` reads the stock before the lock, `stock.py:52`, so two orders can both see one unit." If you cannot write that sentence, you have not found it; keep isolating.
4. **Fix.** Change the code at the cause. Remove the temporary lines. Make the reproducing test the regression test, named for the behavior it protects.
5. **Verify.** Run every suite that touches the feature and call `record_test_run`. Health should read passing for the feature.
6. **Report.** In the room or the task, say the cause in that one sentence, the fix, and the test that now guards it. When the cause was in another partition's release or in a contract, say so and open an issue with `create_issue`.
