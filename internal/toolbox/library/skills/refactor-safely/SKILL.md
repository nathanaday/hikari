---
name: refactor-safely
description: Change the shape of code without changing what it does - characterize the current behavior with tests, move in steps that each keep the suite green, and keep the feature map true. Load it when a request asks to restructure, extract, rename, or split code in your partition.
---

# Refactor safely

A refactor changes structure and keeps behavior. The suite is how you know.

1. **Scope.** Name the features whose code moves, from `code-map.yaml`. Read their documents. If the refactor changes an interface another partition consumes, stop: that is a plan for the orchestration agent, not a refactor.
2. **Characterize.** Run the suites that touch those features and record them. Where a behavior you are about to move has no test, write one that pins what the code does today, even when you dislike it. Record the run.
3. **One step.** Make one behavior-preserving change: extract a function, move a file, rename a symbol, split a module. Run the affected suites. Green means the step holds; red means undo the step, not the test.
4. **Record and apply.** Call `record_test_run`, update `code-map.yaml` for the paths that moved, and `apply` with a subject that names the step: `refactor(stock): extract reservation rules`.
5. **Repeat** until the shape you wanted is there. Then update the feature documents if the structure changed what a feature is, and `mark_map_current`.

Never mix a refactor and a behavior change in one step. When you find a bug while refactoring, finish the step, record it, then fix the bug in its own step with its own test.
