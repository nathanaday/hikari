# Tests and health: round 1

## What I take from your proposal

From the plant in `next-phase.md`:

- Each unit test is an entity that links the features it touches, one or many.
- A feature is healthy only when every test that touches it passes.
- When the tests run, the owner publishes the results and which features each test touches. The server computes the health of every feature from them and the feature wiki.
- A hikari skill explains how to build the test map.
- The test map stays inside the owner's container. It names code paths, functions, and classes, which no one outside may see.
- Later: hikari-format docstrings on a test are parsed into the test map.

This replaces `test.yaml`, `record_test_results`, and health rules S8 to S12 of `docs/specs/system-maps.md`. The feature wiki kept those only as a placeholder.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## What is wrong today, in two real systems

**The shop's inventory.** Three features, one test file, three test cases. The test plan gives all three features the same file. The owner ran the suite once and then reported a pass count and a fail count for each feature. Those counts are the owner's guess at which test belongs to which feature. The server cannot check them.

**The yami library.** About 490 test cases in 37 files, in one suite (`npm test`, vitest). The test plan names files per feature. If 1 of the 30 tests in `wiki.test.ts` fails, the owner must notice which feature it belongs to and report it there. Nothing makes that happen.

The common fault: the owner rolls the results up per feature by hand, and the server stores the totals. The design below moves the roll-up into the server.

## Terms

- **Test**: one test case, as the test runner names it. Not a file, not a suite.
- **Test id**: the name of a test in its report: the JUnit class name, then ` > `, then the test name. Vitest gives `test/unit/wiki.test.ts > search > finds a tag`. Pytest gives `tests.test_stock.StockTest > test_reserve_holds_stock`. It is the join key between the test map and the results.
- **Test map**: the owner's private record of which features each test touches.
- **Run**: one execution of one suite, which writes one report.
- **Report**: the file a run writes, with one result per test.
- **Result**: the outcome of one test in one run: passed, failed, or skipped.

---

## A. The test map

**A1. A test is a test case.** The entity is the single test case that the runner reports, not the file or the suite. Results come per test, so health can say which test fails and which feature it touches.

Do you agree?

**A2. One file, not one document per test.** I recommend one private file, `/hikari/private/tests.yaml`, not a markdown document per test. Yami's library alone would have about 490 documents, each with a one-line body. A test has little to say that its name and code do not say already. The feature wiki earns its documents because a feature has a purpose to explain. A test does not.

The file still uses the wiki's links, so a reader sees one link syntax everywhere:

```yaml
version: 2
suites:
  unit:
    command: npm test -- --reporter=junit --outputFile=/hikari/private/reports/unit.xml
    report: reports/unit.xml          # relative to /hikari/private/
tests:
  "test/unit/wiki.test.ts": ["[[wiki]]"]                  # every test in the file
  "test/unit/wiki.test.ts > lint": ["[[wiki]]", "[[wiki-lint]]"]
  "test/unit/review.test.ts > review refuses non-review sessions": ["[[review]]", "[[agents]]"]
gaps:
  wiki: [Search is not tested on a vault of more than 100 files.]
```

Do you agree, or do you want one markdown document per test?

**A3. An entry covers a test or a group of tests.** A key in `tests` is a test id, or the start of one: a file, or a file and a describe block. The server matches each test in a report to the longest key that it starts with. The owner writes one line for a file that serves one feature, and a longer key only where a test serves other features. Each test still gets its own result and its own links.

Without this, the yami owner must write about 490 entries, and each new test needs a new entry before it counts.

Do you agree?

**A4. The owner edits the file directly.** As `test.yaml` today, the test map is the owner's working file in `/hikari/private/`. The server checks it whenever it reads it. On a rename in the wiki, the server rewrites the links in the test map in the same change, as it rewrites the links between documents. On a remove with a redirect, it rewrites them to the redirect target. On a remove without one, it drops the link and warns.

The alternative is a tool such as `change_tests`, as `change_features` does for the wiki. That gives a stricter check on each write, but the file is private and mostly mechanical, and a tool adds a step to every new test.

Do you agree?

**A5. What the server checks.** Every link names a feature of the system's wiki. Every suite has a command and a report path. Every key is a non-empty string. A file with an error stays the owner's to fix, and the turn context names the errors, as it does for `layout.yaml` today.

The server does not check that a key matches a real test until a report arrives (C3).

Do you agree?

**A6. `covers` goes, `gaps` stays.** Today each feature's tests have a `covers` sentence and a `gaps` list. The test names now say what the tests cover, so `covers` goes. `gaps` stays, keyed by feature id, because no test can say what no test checks. It stays private, as today.

Do you agree?

## B. Running the tests and recording results

**B1. The runner writes the report, not the agent.** I recommend that each suite writes a JUnit XML report, and that the owner calls one tool, `record_test_run(suite)`, after the run. The server reads the report from the suite's `report` path and records each result. The agent never types a count.

JUnit XML is the one report format that almost every runner writes:

| Runner | How it writes JUnit XML |
|---|---|
| vitest (yami) | `--reporter=junit --outputFile=<path>`, built in |
| pytest | `--junitxml=<path>`, built in |
| Python unittest (the shop) | needs the `unittest-xml-reporting` package, or a switch to pytest |
| go test | needs `gotestsum --junitfile <path>` |
| cargo | needs `cargo nextest` with its JUnit output |

The cost: a system whose runner cannot write JUnit needs a tool added to its image. The system's `test` command in the topology must then write a report too.

Do you agree?

**B2. hikari does not run the tests, yet.** The owner runs the suite. The server only reads what the runner wrote. An agent could still edit a report, but it has no reason to, and the skill says not to.

The stronger option is that the server runs the suite in the owner's container, with `docker compose exec`, at the owner's request or at the end of a turn. Then no result passes through the agent. It costs a long blocking call for a slow suite, timeouts, and a second way that tests run. I recommend that we plant it with the validation agent of `phase-2.md`, which needs the same thing.

Do you agree?

**B3. The server keeps the last result of each test.** A run of one suite updates the results of that suite's tests and leaves the others. Each result keeps its run time and a content hash (C2). The map shows only per-feature totals. There is no history of runs in this phase.

Do you agree?

**B4. Where results live.** The results of each test name tests, so they are private. I recommend `/hikari/private/results.yaml`, written only by the server, so that the owner can read why a feature is failing. The public map gets only the per-feature totals and health (D3).

The report files are scratch. The server deletes each report after it reads it, so reports never reach a commit.

Do you agree?

## C. Matching results to the map

**C1. A test with no links is unmapped.** A test in a report that no key in the test map covers touches no feature. The owner view lists unmapped tests, as it lists unmapped paths today. It is a warning, not an error.

Do you agree?

**C2. A result counts only while its code is unchanged.** When the server records a result, it hashes the test's file and the code of each feature the test touches, from `layout.yaml`. A later change to any of them makes that result outdated. The test then counts as not run for health.

The test's file comes from the report's file attribute. Not every runner writes one: pytest does not by default. Where it is missing, the server cannot hash the test file, and only the feature's code makes the result outdated. The spec sets this for each runner in B1.

Do you agree?

**C3. A test that disappears.** A key in the test map that matches no test in the last report of any suite is a warning in the owner view: the test was removed or renamed. A result of a test that a new run of its suite no longer reports is deleted.

Do you agree?

## D. Health

**D1. The health rules.** For one feature, the server takes every test that touches it:

1. `untested`: no test touches the feature.
2. `failing`: a current result of a touching test failed.
3. `outdated`: a touching test has no current result: it never ran, or its result is outdated (C2).
4. `passing`: every touching test has a current result, and none failed. A skipped test counts as not run.

The checks run in this order, and the first match decides. `failing` comes before `outdated`, so a known failure always shows, even while other tests wait for a run.

Do you agree?

**D2. Two words, not two `stale`s.** Today a feature can be `stale` in two senses: its document is behind the code (state, from the feature wiki), and its test results are behind the code (health). I recommend `outdated` for the health value, as in D1, and `stale` only for the state. They measure different things and can differ, so they stay two fields with two words.

Do you agree?

**D3. What the public map shows.** Each feature in the map shows `health` and `tests: {passed, failed, outdated, ran}`, where the counts are over the tests that touch the feature, and `ran` is the newest run among them. No test id, file, or hash reaches the map. Releases copy the map as today.

Do you agree?

**D4. One test fails, many features fail.** A test that touches three features makes all three `failing`. This is what the plant says ("passing tests for all unit tests that touch it"), and it is the reason to link a test to more than one feature only when it really checks each.

Do you agree?

## E. Tools, prompts, and the skill

**E1. Tools.** `record_test_results` goes. `record_test_run(suite)` replaces it (B1). `get_my_system` adds the failing tests by id, the unmapped tests, the stale keys, and the errors of the test map. Orchestrator tools do not change.

Do you agree?

**E2. Turn notices.** The notice at the start of a turn names each `failing` feature with its failing tests, and each `outdated` feature. The Keep true procedure in the skill runs the suites that touch them, records each run, and fixes the code or the test map.

Do you agree?

**E3. The skill.** `references/tests.md` is rewritten for the test map, with one worked example from the shop. I also recommend a third read-only agent, `hikari-test-survey`: it reads a group of test files with the feature wiki, and returns the keys and links for the test map as JSON, as `hikari-feature-survey` does for features. A system with 490 tests needs it for the first build.

Do you agree?

## F. Docstrings

**F1. Plant only.** The test map format leaves room for docstrings: a line such as `Features: [[wiki]] [[wiki-lint]]` in a test's docstring could become its entry. That needs a parser per language. I recommend that it stays planted until one system needs it.

Do you agree?

## G. Old files and the UI

**G1. No backward compatibility.** At startup the server deletes a `test.yaml` of version 1 with a log line, as it does for an old map, and writes an empty `tests.yaml`. The owner builds the test map again in its next turn. When the topology gives `system.test`, the new test map gets one suite, `all`, with that command, and an empty `report`, which the owner must fill.

Do you agree?

**G2. No UI work.** The UI keeps its health label and counts. The test map and the results get no view in this phase, as decided for the feature wiki (G2 there).

Do you agree?
