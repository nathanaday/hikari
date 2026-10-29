---
name: hikari-do-work
description: How a hikari partition agent makes a change in its workspace - the code, the code map, the test map, and the feature map in one unit of work, then apply - and how a release of a deliverable is published. Load it when you change code, after hikari-plan-work for a change of more than one feature, and before you publish a release.
---

# Make the change

You work in `/workspace`: the folders of your partition, which you see as one monorepo. You can change only the folders that are not read-only, and you have no git: hikari commits for you.

1. Change the code and its tests in your writable folders.
2. Keep your documents true in the same unit of work: new or moved code updates `code-map.yaml`; new or moved tests get keys in `tests.yaml`; changed behavior, a new feature, or a new interface updates the wiki and the map. The `hikari-map-features` skill has the formats and the rules.
3. Load the `hikari-validate-work` skill, and run and record the suites that touch the features you changed.
4. Call `apply` with a message that says what changed and why: a subject line and an optional body, with no trailers, because hikari sets the author. hikari makes one commit with that message in each repository whose folders changed.
5. Call `mark_map_current`.

When the change needs something outside your partition, tell the orchestration agent, or the partition agent of that partition, what you need and why. Do not work around it.

## Publish a release

A release is a published product of one of your deliverables: the named packages that your system prompt lists under "Your deliverables". Each deliverable has a skill with its name that says how to make its product, and `deliverable.yaml` in `/hikari/public/deliverables/<name>/` says what the product must contain.

1. Make sure the map is current and every feature that the release provides is passing.
2. Load the deliverable's skill and build the product in `/hikari/staging/<name>/`. Put only what the deliverable asks for there: no tests, no build caches such as `__pycache__` or `node_modules`, and no private documents, unless the deliverable asks for them.
3. Call `check_release` with the deliverable's name, and fix every problem it reports.
4. Call `publish_release` with the deliverable's name, a semver version greater than its last release, notes that say what changed, a certificate for each target of the deliverable, and the tags to set on it, among those your prompt says you may set.

hikari writes the manifest, pushes the product to the deliverable's remote when it has one, moves the product into `/hikari/self/releases/<name>/<version>/` with a copy of the feature map in `features/`, and announces the release in the room. A release never changes. Its record travels with the project; its product is pulled through the remote on a machine that lacks it.

Tags are mutable names for versions, such as `dev`, `staging`, or `production`; the project gives them their meaning, and hikari gives them none. `releases/<name>/tags/<tag>` links the version a tag names, and `tags/latest` is the newest. Move a tag with `tag_release`. When a release of the deliverable is built and published elsewhere, such as by a CI workflow, record it with `record_release`: hikari pulls the version through the remote into the empty `/hikari/staging/<name>/` and publishes it as yours, with the same checks.

## Integrate a release

When your work uses a release of another partition, pin it with `pin_release`: a version, or a tag you follow. hikari records the pin, pulls the product onto this machine when it is absent, and `hikari sync` gets it on a clone of the project. `list_releases` shows every release you can read, with its tags and whether its product is on this machine. Read the product's docs, and name the partition and version in the description of the suite that tests against it.
