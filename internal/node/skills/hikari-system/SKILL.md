---
name: hikari-system
description: How to keep your hikari system documents true - the feature wiki, which you change through change_features, the public map that hikari writes from it, and your private layout.yaml and test.yaml - and how a release of a deliverable is published. Load it before you build or update your documents, after you change code, before you run or record tests, and before you publish a release.
---

# Your system documents

You own a system. These documents describe it, and you are the only author of all of them.

| Document | Who reads it | How you change it |
|---|---|---|
| `/hikari/public/export/features/<id>.md` | Your orchestrator, every release, and the human operator. | Only through `change_features`. The files are read-only to you. |
| `/hikari/public/export/map.yaml` | The same readers. | hikari writes it: the `system` section from `set_system_info`, the `features` list from the wiki. |
| `/hikari/private/layout.yaml` | You and the human operator. | Edit the file. |
| `/hikari/private/test.yaml` | You and the human operator. | Edit the file. |

The feature wiki is the public surface of your system: one document for each feature, with links between the features that work together. Other agents never see your code, so a document must make sense without it. The map carries the system's summary, its interfaces, and a projection of the wiki with the health and state of each feature.

The private documents are for your own work. `layout.yaml` is your code map: where each feature lives in the code. `test.yaml` is your test plan.

References:

- [features.md](references/features.md): the feature document, its rules, what counts as a feature, and how to link.
- [layout.md](references/layout.md): the code map.
- [tests.md](references/tests.md): the test plan.
- [examples/](references/examples/): three feature documents of a small shop.

## The tools

- `set_system_info`: the summary, the responsibilities, and the interfaces that the system provides and consumes. Use the names in your hubs' vocabularies.
- `change_features`: create, modify, rename, or remove feature documents, as one change. hikari checks every write and applies all of them or none.
- `record_test_results`: the passed and failed counts of each feature's tests.
- `mark_map_current`: declares that the documents describe the code as it is now. It commits your pending changes and records the commit in every document.
- `get_my_system`: the map with each feature's health and state, the stale features with their changed paths, the changed paths that belong to no feature, warnings about the wiki, and problems in the private documents.

## Build

For a system with no wiki:

1. Orient. Read the instruction files, the manifests, the entry points, and the names of the test directories. Write `parts` in `layout.yaml`.
2. Survey. For each part, send the `hikari-feature-survey` agent with the part's paths, the system's interfaces, and the vocabulary's terms. When the scope has about 40 source files or fewer, read the part yourself instead. Each survey returns candidate features with their code paths, evidence, and candidate neighbors.
3. Decide the feature set against [features.md](references/features.md): merge, split, name, and give ids. Call `set_system_info` first, so that the interfaces exist for the documents to name.
4. Link against the link rules. Every feature should have a neighbor.
5. Write one `change_features` call with every create.
6. Write the code map in `layout.yaml`: a `code` entry for every feature. Write `test.yaml`: the suites, and the tests of each feature.
7. Run the tests as the plan says, and call `record_test_results` for each feature that has tests.
8. Call `mark_map_current`.

## Keep true

When your prompt lists stale features, undescribed features, or unmapped paths:

1. For each stale feature, read its changed paths and decide: the description still holds (no write); the behavior changed (modify its description or body); the feature is gone (remove, with a redirect to the feature that took its place).
2. For each unmapped path, decide: it belongs to a feature (add it to that feature's `code` in `layout.yaml`); it is a new feature (create it, and add its code); it is a part and not a feature (add it to `parts`).
3. An undescribed feature is one you created since the last `mark_map_current`. It needs nothing but the mark.
4. When more than about ten paths changed, send the `hikari-feature-review` agent first and work from its findings.
5. Run the tests of the affected features, record the results, call `commit_changes`, then `mark_map_current`.

When you change code yourself, do the same in the same unit of work: new or moved code updates `layout.yaml`; new, moved, or removed tests update `test.yaml`; changed behavior, a new feature, or a new interface updates the wiki and the map.

## Problems

hikari refuses a change that breaks a rule and names the write and the rule; fix the write and send the change again. `get_my_system` lists the warnings of the wiki and the problems of the private documents. `record_test_results` and `mark_map_current` fail while the private documents have problems.

## Publish a release

A release is a published product of one of your deliverables: the named packages that your system prompt lists under "Your deliverables". Each deliverable has a skill with its name that says how to make its product, and `deliverable.yaml` in `/hikari/public/readonly/shared/deliverables/<name>/` says what the product must contain.

1. Make sure the map is current and every feature that the release provides is tested.
2. Load the deliverable's skill and build the product in `/hikari/public/export/staging/<name>/`. Put only what the deliverable asks for there: no tests, no build caches such as `__pycache__` or `node_modules`, and no private documents, unless the deliverable asks for them.
3. Call `check_release` with the deliverable's name, and fix every problem it reports.
4. Call `publish_release` with the deliverable's name, a semver version greater than its last release, notes that say what changed, and a certificate for each target of the deliverable.

hikari writes the manifest, moves the product into `/hikari/public/export/releases/<name>/<version>/` with a copy of the map and the wiki, and announces the release in your hubs. A release never changes. `releases/<name>/latest` is the newest release of that deliverable.
