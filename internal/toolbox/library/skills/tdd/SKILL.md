---
name: tdd
description: Change a feature test first - write the failing test for the behavior, make it pass with the smallest change, then clean up with the suite green, recording each green run with record_test_run. Load it when a request adds or changes a behavior of a feature in your partition.
---

# Test-driven change

Work in small loops. Each loop ends with the suite green and recorded.

1. **Name the behavior.** Write one sentence: "when X, the feature does Y." Find the feature in `/hikari/self/features/` and the suite that covers it in `/hikari/private/tests.yaml`.
2. **Red.** Write one test for that sentence, in the suite's style, named for the behavior. Run the suite and see it fail for the right reason. A test that passes at once tests nothing new; sharpen it.
3. **Green.** Make the smallest change that passes the test. No refactor yet, no second behavior.
4. **Record.** Run the whole suite, and call `record_test_run` for it. If something else broke, fix it before you go on.
5. **Refactor.** Now clean the code the test protects: names, duplication, structure. Run and record again.
6. **Map.** Add the test's key to `tests.yaml` with the feature it touches, and update `code-map.yaml` and the feature document when the behavior is new. Then `apply` with a message that says the behavior.

Repeat from 1 for the next behavior. When a behavior needs another partition, stop at 1 and ask, as the plan-work skill says.
