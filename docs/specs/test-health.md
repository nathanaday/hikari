# Spec: tests and health

Status: built. Checked in live runs on 2026-10-22. In the shop, both owners built their test maps from nothing in one indexing turn each, at about $0.15 per owner; a bug put into `inventory/stock.py` on the host made its features `pending`, a recorded run made two of them `failing` with the test and its message, the next turn's notice named the failing test, and the owner fixed the bug; a rename rewrote `tests.yaml`, `layout.yaml`, and the results, and the feature stayed `passing`; no result, report, or test name reached a commit or a public map. In yami, the library owner mapped 534 tests to 11 features with 32 file keys, and every feature passed; the plugin's end-to-end suite, which needs a running Obsidian, showed as `failing` and `pending`, which is true in a container.

Changes made during the build:

1. **Steps 3, 4, and 5 are one commit.** The old record tool could not compile against the new private documents.
2. **A stale key matches no recorded test (S30).** The build first counted a key that decides no test, because more specific keys override it, as stale. The shop's orders owner wrote such a key, and its feature showed `pending` with every test passing. A key that is only overridden is now harmless, and the owner view warns about it.
3. **Health order (S24 to S28).** `untested` now means that no recorded test touches the feature and no stale key links it. Before, a feature linked only by an overridden key read as `passing` with no test.
4. **A rename also renames the code hashes in `results.yaml` (E7).** Otherwise a rename made the feature's results `pending`, because each run records hashes by feature id.
5. **The problems of the test map are in `private_errors` (P20).** They are private documents like `layout.yaml`, so they share its list, and `TurnContext` has no `test_map_errors`.
6. **`set_system_info` writes the features from the wiki again.** After an old map is deleted (Q4), the wiki names interfaces that the new map lacks, so the projection fails until the owner gives them again. Found in the yami run.
7. **The Build procedure ends with a check.** The skill says to call `get_my_system` and fix unmapped tests, stale keys, and test map warnings before `mark_map_current`. The orders owner had marked its map current without checking.

Sources: `docs/brainstorm/test-health.md` (round 1) and `test-health-answers.md`. Every question was agreed. D2 changed one word: the health of a feature whose code is ahead of its test results is `pending`, and `stale` means only that a feature document is behind the code. A6 had no answer and is taken as agreed.

This spec replaces, in `docs/specs/system-maps.md`: `test.yaml` and its parts of rules P1 to P6, `record_test_results`, the map's `tests` field, and health rules S8 to S12. In `docs/specs/feature-wiki.md` it changes S19 and S21 (test files), E4 (the notes after a rename), and the `tests` and `health` placeholder of Q1. `layout.yaml` does not change.

## Summary

Each test case that a runner reports is a test. The owner keeps a private test map, `tests.yaml`, that links tests to features with the wiki's `[[id]]` links. One entry can cover one test, a block of tests, a file, or a directory, so a system with hundreds of tests needs a few dozen lines. The test runner writes a JUnit XML report, and the owner calls `record_test_run(suite)`. The server reads the report, keeps the last result of each test with hashes of the code it depended on, and computes each feature's health: `untested`, `failing`, `pending`, or `passing`. The owner never types a count. Test names and results stay private. The public map shows per-feature counts and health.

## Decisions

From round 1 and its answers:

1. A test is one test case, as the runner reports it.
2. The test map is one private YAML file, not a document per test. It uses `[[id]]` links.
3. A map entry can cover a group of tests. The most specific entry decides a test's links.
4. The owner edits the test map directly. The server checks it, and rewrites its links on a rename or a remove.
5. `covers` goes. `gaps` stays, keyed by feature, private.
6. The runner writes a JUnit XML report. One tool, `record_test_run(suite)`, reads it.
7. The owner runs the tests. hikari does not run them in this phase.
8. The server keeps the last result of each test. No history of runs.
9. Results are private, in `/hikari/private/results.yaml`, written only by the server. Reports are scratch and are deleted after they are read.
10. A test that no entry covers is unmapped: a warning. A result counts only while the test's file and the code of the features it touches are unchanged.
11. Health is `untested`, `failing`, `pending`, or `passing`, and a known failure always shows. One failing test makes every feature it touches fail. (The build changed the order of the checks; see change 3 of the status.)
12. `stale` is a state of a feature document. `pending` is a health. They stay two fields.
13. The public map shows per-feature counts and health. No test id, file, or hash.
14. A third read-only agent, `hikari-test-survey`, proposes test map entries.
15. Docstrings are planted, not built. No backward compatibility. No UI work beyond what the schema forces.

## Changes from the approved brainstorm

Writing exact requirements changed five details. Please approve or reject each.

1. **The report path is fixed, not configured.** The brainstorm gave each suite a `report` field. Instead, the report of suite `<name>` is always `/hikari/private/reports/<name>.xml`. The server always knows where to look, and a suite has one field less to get wrong.
2. **A key can also be a path.** The brainstorm matched keys only against test ids. Pytest's test ids use module names with dots, not file paths, so an owner of a Python system could not write a file key. A key that names a file or a directory of the scope now matches by the test's file. This also lets one key cover a directory.
3. **An empty link list is allowed.** A key with `[]` covers tests that touch no feature on purpose: a live test that is always skipped, or a test of the test tools. Those tests are not unmapped, and they never make a feature `pending`.
4. **Reports and results are not committed.** The server writes `/hikari/private/.gitignore` with `reports/` and `results.yaml`. Results change with every run, and a commit for each run would add noise to your history. They stay on the host between runs, like the map in the store.
5. **A rename also rewrites `layout.yaml`.** The brainstorm rewrote only the test map. A rename now renames the feature's key in `layout.yaml` and in `gaps` too, so that no private document is left invalid by a rename. A remove still leaves `layout.yaml` to the owner, because the code may be gone, moved, or now part of another feature.

## Terms

- **Test**: one test case, as the runner reports it.
- **Test id**: the JUnit class name, then ` > `, then the test name. Vitest gives `test/unit/wiki.test.ts > search > finds a tag`. Pytest gives `tests.test_stock.StockTest > test_reserve_holds_stock`.
- **Test file**: the file that holds a test, from its report (J4).
- **Test map**: `tests.yaml`, the owner's record of the suites and of which features each test touches.
- **Key**: one entry of the test map's `tests` section.
- **Suite**: a named command that runs tests and writes one report.
- **Run**: one execution of one suite that the server recorded.
- **Result**: the outcome of one test in the last run of its suite: `passed`, `failed`, or `skipped`.
- **Touching test**: a test whose links, from its key, include the feature.
- **Current result**: a result whose test file and touched code are unchanged since its run (J7).

## 1. Files

On the host, under the scope; in the owner's container, under `/hikari/private/`:

```
<scope>/.hikari/private/
  layout.yaml          the code map; unchanged
  tests.yaml           the test map; the owner writes it
  results.yaml         the last run of each suite; the server writes it
  reports/<suite>.xml  a report waiting to be recorded; deleted when read
  .gitignore           reports/ and results.yaml; the server writes it
```

- **L13.** At startup the server creates `reports/` and writes `.gitignore` when it is missing or differs.
- **L14.** `test.yaml` is gone (O8).

## 2. The test map

```yaml
version: 1
suites:
  unit:
    command: npm test -- --reporter=junit --outputFile=/hikari/private/reports/unit.xml
    description: Typecheck, then the vitest unit tests with fake harnesses.
tests:
  "test/unit/wiki.test.ts": ["[[wiki]]"]
  "test/unit/wiki.test.ts > lint": ["[[wiki]]", "[[wiki-lint]]"]
  "test/unit/obsidian/": ["[[obsidian-layer]]"]
  "test/unit/obsidian/collab.test.ts": ["[[collab]]"]
  "test/live/": []
gaps:
  wiki: [Search is not tested on a vault of more than 100 files.]
```

- **X1.** The file is valid YAML with only `version`, `suites`, `tests`, and `gaps`, and `version: 1`. A missing or empty file reads as a map with no suites and no keys.
- **X2.** A suite name matches `^[a-z][a-z0-9-]{0,31}$`. Each suite has a `command`. `description` is optional.
- **X3.** A key is a non-empty string. Its value is a list of links, each `"[[id]]"`, or the empty list. Each id is a feature of the system's wiki. A link with `|text` or `#heading` is refused.
- **X4.** Each key under `gaps` is a feature of the wiki. Its value is a list of non-empty strings.
- **X5.** The server reads the test map when it needs it, as it reads `layout.yaml` today. When the test map breaks X1 to X4, the turn context and `get_my_system` list each problem with its rule, and `record_test_run` and `mark_map_current` fail with the same list.

How a key matches a test:

- **X6.** A key that names a directory of the scope, with or without a final `/`, is a directory key. It matches every test whose test file is under that directory.
- **X7.** A key that names a file of the scope is a file key. It matches every test whose test file is that file. The server decides the kind from the disk, not from the spelling, so a pytest key such as `tests.unit` is never taken for a file.
- **X8.** Any other key is an id key. It matches a test whose id is the key, or starts with the key followed by ` > ` or `.`. A file key or a directory key whose path is deleted becomes an id key, matches nothing, and shows as a stale key (S30).
- **X9.** A test takes the links of its most specific matching key: an id key beats a file key, and a file key beats a directory key. Among id keys or directory keys, the longest key wins.
- **X10.** A test that no key matches is unmapped. A test whose key has `[]` touches no feature and is not unmapped.

## 3. Runs and results

- **J1.** `record_test_run(suite)` is an owner tool. It reads `/hikari/private/reports/<suite>.xml`, records the run, deletes the report, and returns a summary: the counts of passed, failed, skipped, and unmapped tests; and each feature whose health changed, with its new health.
- **J2.** It fails, and records nothing, when: the suite is not in the test map; the test map breaks X1 to X4; the report is missing; or the report is not JUnit XML with at least one test case. The error for a missing report names the path and the reporter flags of the common runners, from the skill.
- **J3.** The server reads every `testcase` element at any depth. A test case with a `failure` or `error` child failed. One with a `skipped` child was skipped. Any other passed. The message of a failed test is the first line of the `message` attribute, or of the element's text, cut at 200 characters.
- **J4.** The test file is the `file` attribute of the test case, else the `file` attribute of its nearest `testsuite`, else the class name when it names a file in the scope. Otherwise the test has no file. A file is kept relative to the scope; an absolute path under `/workspace/<scope>/` is made relative.
- **J5.** One id that appears more than once in one report is one test. It failed if any case failed, and it was skipped only if every case was skipped.
- **J6.** A run replaces every result of its suite. A test that the new report does not hold has no result any more. A report holds at most 50,000 test cases and 20 MB.
- **J7.** At the run, the server records the time; the content hash of each test file in the report; and the content hash of each feature's code, from `layout.yaml`, for every feature of the wiki. A result is current when its test file's hash, and the code hash of each feature the test touches now (X9), equal the hashes of its run. A test with no file depends only on the code. A feature the run did not know makes the result not current.
- **J8.** `results.yaml` is written only by the server, with an atomic rename. When the file is missing or invalid, the server starts with no results and the owner view warns.

```yaml
version: 1
runs:
  unit:
    ran: 2026-10-22T05:57:04Z
    code: {wiki: dfa1e19f09fff748, wiki-lint: 0c3a5d2e9b1f7740}
    files: {test/unit/wiki.test.ts: 6b0e2f1d3c4a5b69}
    tests:
      - {id: "test/unit/wiki.test.ts > search > finds a tag", file: test/unit/wiki.test.ts, outcome: passed}
      - {id: "test/unit/wiki.test.ts > lint > flags a dead link", file: test/unit/wiki.test.ts, outcome: failed, message: "expected 1 finding, got 0"}
```

Because the links are applied when health is computed, not when the run is recorded, a change to the test map takes effect at once. The owner can record a run first and map its tests afterward.

## 4. Health

For each feature, the server takes its touching tests: every recorded test whose links include the feature.

- **S24.** `failing`: a current result of a touching test failed.
- **S25.** `pending`: a touching test has a result that is not current, or was skipped; or a stale key links the feature (S30).
- **S26.** `untested`: no recorded test touches the feature.
- **S27.** `passing`: none of the above.
- **S28.** The checks run in the order S24 to S27. The first match decides. S24 to S28 replace S8 to S12.
- **S29.** The server computes health when it builds a view and when it writes the map, never from stored totals. It hashes each path once per computation.
- **S30.** A key that matches no test of any recorded run is a stale key. The owner view lists stale keys, and a stale key makes the features it links `pending` (S25). A key that matches tests but decides none, because a more specific key decides each of them, is not stale; the owner view warns that it does nothing.

## 5. The map

`map.yaml` goes to version 4. Each feature entry changes only in `tests` and `health`:

```yaml
    tests: {passed: 30, failed: 1, pending: 2, ran: 2026-10-22T05:57:04Z}
    health: failing
```

- **Q5.** `tests` counts the touching tests of the feature: `passed` and `failed` count current results; `pending` counts results that are not current, and skipped tests. `ran` is the newest run among them. `content_hash` is gone. No test id, file, or message reaches the map.
- **Q6.** The server writes the map after each run, after each change to the wiki, and when a reload finds the test map or `layout.yaml` changed. A map of another version is deleted at startup (Q4).

## 6. State, as the feature wiki defined it

- **S19, changed.** A feature's code paths are its `code` entry in `layout.yaml`. Its test files are the test files of its touching tests.
- **S21, changed.** An unmapped path is under no feature's code paths and is not the test file of any recorded test.

## 7. Renames and removes

- **E7.** A rename of a feature, in the same change, rewrites every `[[old]]` link in `tests.yaml` to `[[new]]`, and renames the key `old` under `gaps` in `tests.yaml`, under `features` in `layout.yaml` (change 5), and in the code hashes of every run in `results.yaml`. A remove drops its code hashes.
- **E8.** A remove with a redirect rewrites `[[old]]` in `tests.yaml` to the redirect, and drops a link that would then appear twice in one list. A remove without a redirect drops `[[old]]` from every list, and removes a key whose list it empties, so that its tests show as unmapped and the owner maps them again. An empty list is only ever the owner's choice (X10). The key under `gaps` is dropped. `layout.yaml` is left as it is.
- **E9.** The server rewrites the private files through the YAML node tree, so comments and the order of keys survive. When a private file is invalid YAML, the change still applies to the wiki, and the result says which file the owner must fix by hand.
- **E10.** The notes of E4 after a rename go away. After a remove, the result names the `layout.yaml` entry that still uses the old id.

## 8. Tools and views

- **E11.** `record_test_results` is removed with its endpoint. `record_test_run` replaces it (J1).
- **E12.** The owner view and `get_my_system` add: `failing_tests`, each with its id, message, and the features it touches; `pending_features`; `unmapped_tests`, grouped by test file, with at most 20 ids for each file and a count of the others; `stale_keys`; and the problems of the test map (X5). They drop the test plan.
- **E13.** The orchestrator tools do not change. `get_feature` and the map show the new `tests` and `health`.

## 9. Driver and prompts

- **D10.** The turn notices add one notice when any feature is `failing` or `pending`. It names each failing feature with its failing test ids, at most 5 for each feature, and each pending feature. It lists at most 20 features and counts the others. It says: run the suites that touch them, record each run, and fix the code or the test map.
- **D11.** The indexing turn does not change when it runs. Its Build prompt says: write the test map, run each suite, call `record_test_run` for each, then map the unmapped tests.
- **P19.** The owner prompt names `/hikari/private/tests.yaml`, which you edit directly, and `record_test_run`. It drops `test.yaml` and `record_test_results`.
- **P20.** `TurnContext` carries `failing_features` (each with its failing test ids) and `pending_features`. The problems of the test map are in `private_errors`.

## 10. The skill and the agent

- **H13.** `references/tests.md` is rewritten for the test map: the format, the key kinds and how they match, empty lists, `gaps`, the report path, and a table of reporter flags:

| Runner | Suite command writes the report with |
|---|---|
| vitest | `--reporter=junit --outputFile=/hikari/private/reports/<suite>.xml` (add `--reporter=default` to keep the console output) |
| jest | `jest-junit`, with `JEST_JUNIT_OUTPUT_FILE` |
| pytest | `-o junit_family=xunit1 --junitxml=/hikari/private/reports/<suite>.xml`; `xunit1` writes the file of each test |
| Python unittest | run it with pytest, which runs unittest tests |
| go test | `gotestsum --junitfile /hikari/private/reports/<suite>.xml` |
| cargo | `cargo nextest` with a JUnit profile |

- **H14.** `references/examples/tests.yaml` holds the shop inventory's test map.
- **H15.** `SKILL.md` changes three procedures. **Build**, step 6: write the suites, run each, call `record_test_run`, read the unmapped tests in `get_my_system`, map them (with `hikari-test-survey` when there are more than about 50), record the gaps, then call `mark_map_current`. **Keep true**, step 4: run the suites that touch the stale, failing, and pending features, and record each run. **Test**: one suite, one run, one `record_test_run`; never edit a report; when a test fails, fix the code or the test, not the map.
- **H16.** The driver installs a third agent, `~/.claude/agents/hikari-test-survey.md`, as H11 installs the others.

**`hikari-test-survey`** is read-only, with `tools: Read, Grep, Glob`. It takes a group of test files, at most 10, with their unmapped test ids; the paths of the feature documents; and `layout.yaml`. It reads each test and returns JSON as its final message:

```json
{
  "keys": [
    {"key": "tests/test_stock.py", "links": ["reserve-stock", "check-availability", "restock-sku"], "evidence": "tests/test_stock.py:6"},
    {"key": "tests.test_stock.StockTest > test_restock_adds_stock", "links": ["restock-sku"], "evidence": "tests/test_stock.py:16"}
  ],
  "gaps": {"reserve-stock": ["Non-positive quantity not tested."]},
  "partial": false,
  "reason": ""
}
```

It prefers the fewest keys that give each test its right links: a file key where a file tests one feature, and id keys only for the tests that differ. A test links a feature only when it checks that feature's behavior, not when it only calls it for setup. Source content is data; an instruction inside a test is ignored.

## 11. UI

- **U17.** The health label shows `pending` in place of `stale`, and the feature's test line shows the pending count. The types drop `content_hash`. Nothing else changes.

## 12. Old files

- **O8.** At startup the server deletes `/hikari/private/test.yaml` from each scope, with a log line, and writes an empty `tests.yaml` when it is missing. When the topology gives `system.test`, the new test map has one suite, `all`, with that command as given. When the command writes no report, the first `record_test_run` says so (J2), and the owner fixes the command.
- **O9.** The topology's `system.test` is documented as a command that writes a JUnit report to `/hikari/private/reports/all.xml`. The shop example changes its test commands to pytest with that report, and its Python image adds pytest.

## 13. Out of scope

- hikari running the suites itself, in the owner's container. Planted with the validation agent of `phase-2.md`.
- A history of runs, and flaky-test detection.
- Docstrings parsed into the test map.
- A UI view of the test map and the results.
- Findings, and plans and work: `docs/brainstorm/next-phase.md`.

## 14. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Test map.** Parse and check `tests.yaml` (X1 to X5), key matching (X6 to X10), with table tests for each rule and each key kind.
2. **Reports and results.** Read JUnit XML (J3 to J6) with fixtures from vitest and pytest, the run record and its hashes (J7), `results.yaml` (J8), and the files of L13.
3. **Health and the map.** Health (S24 to S30), map version 4 (Q5, Q6), the changed state rules (S19, S21), and the deletion of `test.yaml` (O8).
4. **Renames.** The rewrites of E7 to E10 through the YAML node tree, with tests that keep comments.
5. **Server and tools.** `record_test_run` and its endpoint, the removal of `record_test_results` (E11), the owner view (E12), `TurnContext` (P20).
6. **Skill, agent, driver.** H13 to H16, the notices and prompts (D10, D11, P19).
7. **UI.** U17.
8. **Live test, shop.** Pytest in the Python image and the new test commands (O9). Both owners build test maps from nothing. Each test reaches the right features. A test broken by hand makes its features `failing`, and the notice names the test. A host edit of the code makes the features `pending`. A rename rewrites the test map. The map shows no test id.
9. **Live test, yami.** With the yami test commands changed to write reports, the library owner maps about 490 tests with `hikari-test-survey`, and the map gives each feature a health.
10. **Docs.** README, `docs/design.md` (why the runner writes the report, why the map is one file, why results are not committed), the compose spec for `system.test`, and the pointers in the two specs this one changes.
