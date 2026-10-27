---
name: hikari-validate-work
description: How a hikari partition agent checks its work before it says that a change is done - run every suite of tests.yaml, record each run with record_test_run, read each feature's health with get_my_partition, and fix or report what fails. Load it after you change code, when your prompt lists failing or pending features, and when this machine has no results of your tests.
---

# Validate the work

A change is done when every suite ran since it, every run is recorded, and no feature that it touches is failing or pending. Say that a change is done only then, and say what you ran.

1. Run every suite of `/hikari/private/tests.yaml`, and call `record_test_run` for each run (Test, below).
2. Call `get_my_partition` and read the health of each feature, the failing tests with their messages, the pending features, the unmapped tests, and the keys that match no test.
3. Fix what fails: the code, or a test that checks the wrong thing. Map each unmapped test in `tests.yaml`. Then run the suites again.
4. When a failure is not yours to fix, such as a release of another partition that does not do what its feature map says, report it in the room: the feature, the failing test, and its message.

[tests.md](../hikari-map-features/references/tests.md) explains the test map, the flags that make each runner write its report, and how hikari computes health.

## Test

1. Run one suite with its command from `tests.yaml`. The command writes the report.
2. Call `record_test_run` with the suite's name. One run, one call.
3. When a test fails, fix the code or the test. Never change the test map to hide a failure, and never edit a report.
4. A skipped test leaves its features pending. A test that is always skipped here, such as a live test, gets a key with an empty list.

## Problems

hikari refuses a change that breaks a rule and names the write and the rule; fix the write and send the change again. `get_my_partition` lists the warnings of the wiki and the problems of the private documents. `record_test_run` and `mark_map_current` fail while the private documents have problems.
