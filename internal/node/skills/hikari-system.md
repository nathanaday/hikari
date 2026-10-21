---
name: hikari-system
description: How to keep your hikari system documents true - the public map, which you change through the map tools, and your private layout.yaml and test.yaml - and how to publish a release. Load it before you build or update your map, after you change code, before you run or record tests, and before you publish a release.
---

# Your system documents

You own a system. Three documents describe it, and you are the only author of all three.

| Document | Who reads it | How you change it |
|---|---|---|
| `/hikari/public/export/map.yaml` | Your orchestrator, which plans from it, and every release you publish. | Only through the map tools. The file is read-only to you. |
| `/hikari/private/layout.yaml` | You and the human operator. | Edit the file. |
| `/hikari/private/test.yaml` | You and the human operator. | Edit the file. |

The map is the public surface of your system: what it does, its interfaces, and how healthy each feature is. Other agents never see your code, so the map must make sense without it. Do not put file paths, commands, or internal names into the map. Interface names are the exception: they are the contract between systems, so use the names in your hub's vocabulary.

The private documents are for your own work. They say where each feature lives in the code and how it is tested.

## The map

- `set_system_info`: the summary, the responsibilities, and the interfaces that the system provides and consumes.
- `upsert_features`: one entry for each feature, with its id, name, description, the interfaces it uses, findings (known problems), plans, and related features. A feature is something that a user or another system relies on. Most systems have 5 to 30.
- `record_test_results`: the passed and failed counts of each feature's tests.
- `mark_map_current`: declares that the documents describe the code as it is now.

## layout.yaml

```yaml
version: 1
parts:                          # the main parts of the code
  - path: orders/
    description: Order intake and cancellation.
  - path: tests/
    description: Unit tests.
features:                       # one entry for each feature id in the map
  create-order:
    code: [orders/create.py]
    notes: create_order() checks items, then reserves each one.
```

- Paths are relative to your scope and must exist.
- Every key under `features` must be a feature id in the map. Give every feature an entry.
- `notes` is optional. Use it for what you would otherwise have to find again: entry points, tricky parts.

## test.yaml

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

- Every test entry names a suite under `suites`. Add a suite for each kind of test that you run, for example unit, integration, or lint.
- `files` are the test files of the feature. They must exist. hikari hashes the feature's code and test files when you record results; when the files change, the feature becomes `stale` until its tests run again.
- `covers` says in one sentence what the tests prove. `gaps` says what they do not test.
- A feature without tests has no entry under `features`, and its health is `untested`.

## Build the documents

1. Read the code that you need to understand what the system does.
2. Call `set_system_info` and `upsert_features`.
3. Write `layout.yaml`: the parts, and the code of each feature.
4. Write `test.yaml`: the suites, and the tests of each feature.
5. Run the tests as the plan says, and call `record_test_results` for each feature that has tests.
6. Call `mark_map_current`.

## Keep them true

When you change the code, update the documents in the same unit of work:

- New or moved code: update `layout.yaml`.
- New, moved, or removed tests: update `test.yaml`.
- Changed behavior, a new feature, or a new interface: update the map.
- Run the tests of the affected features, record the results, call `commit_changes`, then call `mark_map_current`.

hikari checks the private documents when you record test results and when you mark the map current, and `get_my_system` lists their problems. The rules:

- P1: the file is valid YAML with the fields above and `version: 1`.
- P2: every feature key is a feature id in the map. After you remove or rename a feature, update both files.
- P3: every path exists in your scope.
- P4: every suite has a command, and every test entry names a defined suite.

## Publish a release

A release is a version of your deliverable that other systems can use, with docs for people who cannot see your code. After you publish it, it never changes.

1. Make sure the map is current and every feature that the release provides is tested.
2. Build the deliverable into `/hikari/public/export/staging/product/`. It is whatever another system needs to use your work: a package, a binary, a directory of files. Copy only what a user needs: no tests, no build caches such as `__pycache__` or `node_modules`, and no private documents. If it is hosted elsewhere, such as an image on Docker Hub, skip this step and use a remote product.
3. Run the `release-docs` skill. It writes the docs into `/hikari/public/export/staging/docs/`.
4. Write `/hikari/public/export/staging/release.yaml`:

```yaml
product:
  kind: files                 # files, or remote
  path: product/              # for files: inside the staging area
  # location: {type: docker, ref: docker.io/acme/orders:1.2.0}   # for remote: docker, github-release, huggingface, or url
runtime:
  requires: [python >= 3.11]  # what the user needs
  install: add product/ to PYTHONPATH
  run: python -m orders       # optional
docs:
  index: docs/README.md
interfaces: [create_order, cancel_order]   # interfaces from your map that this release provides
```

5. Call `publish_release` with a semver version greater than your last release, and notes that say what changed. hikari adds the version, the time, the source commit, and a copy of your public map, moves the staging area into `/hikari/public/export/releases/<version>/`, and announces the release in your hubs.

Your published releases are in `/hikari/public/export/releases/`, and `releases/latest` is the newest one.
