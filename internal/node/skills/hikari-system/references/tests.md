# test.yaml: the test plan

`/hikari/private/test.yaml` says how each feature is tested. Only you and the human operator read it.

```yaml
version: 1
suites:                         # commands that run tests, from your scope
  unit:
    command: python3 -m unittest discover -s tests
    description: Fast unit tests with fakes. No network.
features:                       # how each feature is tested
  create-order:
    covers: Valid orders reserve each item; empty orders and bad quantities fail.
    tests:
      - suite: unit
        files: [tests/test_create.py]
        command: python3 -m unittest tests.test_create   # optional: runs only these tests
    gaps:
      - Not tested against the real inventory.
```

- Every test entry names a suite under `suites` (P4). Add a suite for each kind of test that you run, for example unit, integration, or lint.
- `files` are the test files of the feature. They must exist (P3). hikari hashes the feature's code and test files when you record results; when the files change, the feature's health becomes `stale` until its tests run again.
- `covers` says in one sentence what the tests prove. `gaps` says what they do not test.
- A feature without tests has no entry under `features`, and its health is `untested`.
- Every key under `features` is a feature id in the wiki (P2).

Health is computed: `untested`, `stale`, `failing`, or `passing`. The map shows it beside each feature's state.
