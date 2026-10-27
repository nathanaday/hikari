# tests.yaml: the test map

`/hikari/private/tests.yaml` says how your tests run and which features each test touches. Only you and the human operator read it. You edit it directly.

A test is one test case, as your test runner reports it. hikari never counts tests itself and never takes counts from you: each suite writes a JUnit XML report, `record_test_run` reads it, and hikari computes the health of every feature from the tests that touch it.

```yaml
version: 1
suites:
  unit:
    command: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/unit.xml
    description: Fast unit tests with fakes. No network.
tests:
  tests/test_stock.py: ["[[reserve-stock]]", "[[check-availability]]"]
  "tests.test_stock.StockTest > test_restock_adds_stock": ["[[restock-sku]]", "[[check-availability]]"]
  tests/live/: []
gaps:
  reserve-stock: [A quantity that is not positive is not tested.]
```

See [examples/tests.yaml](examples/tests.yaml) for the shop's inventory.

## Suites

- A suite name is lowercase letters, digits, and dashes (X2). Each suite has a `command`, run from your working directory.
- The command must write a JUnit XML report to `/hikari/private/reports/<suite>.xml`. That path is fixed: hikari reads the report there and deletes it once it is recorded.
- Add a suite for each kind of test that you run separately, for example `unit` and `integration`.

| Runner | Add to the command |
|---|---|
| vitest | `--reporter=default --reporter=junit --outputFile=/hikari/private/reports/<suite>.xml` |
| jest | the `jest-junit` reporter, with `JEST_JUNIT_OUTPUT_FILE=/hikari/private/reports/<suite>.xml` |
| pytest | `-o junit_family=xunit1 --junitxml=/hikari/private/reports/<suite>.xml`. `xunit1` writes the file of each test; without it, file keys match nothing. |
| Python unittest | run the tests with pytest, which runs unittest tests unchanged |
| go test | `gotestsum --junitfile /hikari/private/reports/<suite>.xml -- ./...` |
| cargo | `cargo nextest run` with a JUnit output in its profile |

When the runner your partition uses is not in the image, ask the user to add it.

## Test ids

hikari names a test by its JUnit class name, then ` > `, then its name:

- vitest: `test/unit/wiki.test.ts > search > finds a tag` (the file, then the describe blocks, then the test).
- pytest: `tests.test_stock.StockTest > test_reserve_holds_stock` (the module and class, then the test).

`get_my_partition` lists the ids of the tests that no key covers under `unmapped_tests`, grouped by file. Copy ids from there; do not guess them.

## Keys

Each key under `tests` covers one test or a group of tests. Its value is a list of links to the features those tests touch, each `"[[feature-id]]"` (X3). Quote the links: YAML reads a bare `[[...]]` as a list.

- A **directory key** names a directory of your scope, such as `tests/live/`. It covers every test whose file is under it.
- A **file key** names a file of your scope, such as `tests/test_stock.py`. It covers every test in that file.
- An **id key** is anything else. It covers the test with that id, and every test whose id starts with the key followed by ` > ` or `.`: `tests.test_stock.StockTest` covers the class, `test/unit/wiki.test.ts > lint` covers the describe block.

hikari decides the kind from the disk. When keys overlap, the most specific decides a test's links: an id key beats a file key, a file key beats a directory key, and between two keys of the same kind the longer wins. The links do not add up: write every feature the test touches on the key that decides it. A key that a more specific key overrides for every test it matches does nothing, and `get_my_partition` warns about it.

Write the fewest keys that give each test its right links: a file key where a file tests one feature, and an id key only for the tests that differ.

A test touches a feature when it checks that feature's behavior, not when it only calls the feature to set up something else. Link a test to more than one feature only when it really checks each: one failing test makes every feature it touches fail.

An empty list, `[]`, says that the tests touch no feature on purpose: a live test that is always skipped here, or a test of your test tools. They are not unmapped, and they never make a feature pending.

## gaps

`gaps` says, for a feature, what no test checks. It is for you and the human operator. Keep it short and true.

## Health

hikari computes each feature's health from its touching tests, in this order. The first match decides.

1. `failing`: a touching test failed in its last run, and its code has not changed since.
2. `pending`: a touching test has not run since its test file or the code of a feature it touches changed, or it was skipped, or a key that links the feature matches no recorded test (a stale key: the suite has not run, or the key is wrong).
3. `untested`: no recorded test touches the feature.
4. `passing`: every touching test passed in a run that is still current.

The code of a feature is its `code` entry in `layout.yaml`. A test's result stays current while its test file and the code of every feature it touches are unchanged.

Health is not the feature's state. `stale` is a state: the feature's document is behind the code, and `mark_map_current` clears it. `pending` is a health: the tests are behind the code, and only a run clears it.

The public map shows each feature's health and counts of its touching tests: passed, failed, and pending. It never shows a test id, a file, or a message.

## Rules

- X1. The file has only `version`, `suites`, `tests`, and `gaps`, and `version: 1`.
- X2. Each suite name is valid and each suite has a command.
- X3. Each key is a non-empty string, and each link names a feature of the wiki.
- X4. Each key under `gaps` is a feature of the wiki.

`record_test_run` and `mark_map_current` fail while these rules are broken. `get_my_partition` lists the problems, the keys that match no test (`stale_keys`), and the failing tests with their messages.

When you rename or remove a feature with `change_wiki`, hikari rewrites its links and its `gaps` key in this file for you. A remove without a redirect drops the key whose only link it was, so its tests show as unmapped and you map them again.
