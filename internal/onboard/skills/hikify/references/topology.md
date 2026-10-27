# The target topology

## Drawing boundaries

Each boundary of the target states one reason, of one of three kinds. Propose no boundary without one.

- **Library.** Two or more consumers need the code, or their copies already differ. The library becomes its own partition, and the consumers use its releases.
- **Client and server.** They change at different rates, or a different team owns each. Each becomes a partition, and they meet at an interface that the vocabulary names.
- **Folder in a repository.** A repository holds two concerns, but splitting its git history is not worth the cost. Each concern becomes a partition with its own folder of the same repository.

A partition is a list of folders, and its agent works in them as one monorepo. A boundary follows the concern, not the repository: a partition may take folders from more than one repository, such as a service and the protocol files that only it changes, and a folder may be read-only, for code that it reads but another partition changes or no partition owns.

Also name each split you considered and decided against, and why. Small code with one consumer stays where it is. A library for one consumer adds a release cycle for no gain.

## The parts of a target topology

`topology.target.yaml` is a whole topology file. It keeps every part of `topology.yaml` that the design does not change.

A new library in a new repository, and a consumer of its releases:

```yaml
x-hikari:
  repos:
    units: {url: https://github.com/acme/units.git}   # to create, in execution
  orchestration: orchestrator

services:
  units:
    <<: *agent
    build: {context: ., dockerfile: units.Dockerfile}
    x-hikari:
      description: Owns the units library, which converts sensor readings.
      partition:
        folders: [{repo: units}]
        test: cd units && python3 -m pytest -q tests -p no:cacheprovider -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml
      grants:
        deliverables: [python-library]      # the releases it publishes

  cloud:
    <<: *agent
    build: {context: ., dockerfile: cloud.Dockerfile}
    x-hikari:
      description: Owns the cloud API.
      partition: {folders: [{repo: cloud}], test: ...}
      grants:
        releases: [units]                   # reads units' releases, read-only
```

A consumer finds a release at `/hikari/grants/<library>/releases/<deliverable>/<version>/product/`, or `latest/product/`. For a Python library, it adds that folder to `PYTHONPATH` in its test command.

Two partitions in one repository, each with its own folder. The writable folders of two partitions never overlap, and no folder of a partition lies inside another partition's writable folder:

```yaml
      partition: {folders: [{repo: cloud, path: api}], test: ...}
      partition: {folders: [{repo: cloud, path: consumer}], test: ...}
```

A partition with folders from two repositories, and a read-only folder. Each folder mounts at `/workspace/<repo>/<path>`, or at its `at:`. The first writable folder holds the private files, and an apply makes one commit in each repository that changed:

```yaml
      partition:
        folders:
          - {repo: cloud, path: consumer}
          - {repo: cloud, path: libs/money, read_only: true}
          - {repo: protos, path: consumer, at: protos}
        test: python3 -m pytest -q cloud/consumer/tests -p no:cacheprovider --junitxml=/hikari/private/reports/all.xml
```

## Deliverables

A library publishes a release of a deliverable: a kind of product, with rules for its files. The topology grants it with `grants.deliverables`. Draft each new deliverable in `onboarding/shared/deliverables/<name>/`. Execution copies it to `vault/public/deliverables/<name>/`. A deliverable that the vault already has needs no draft.

`deliverable.yaml`:

```yaml
version: 1
description: A Python package that other partitions add to their PYTHONPATH, with docs for engineers who cannot see the code.
layout:
  require: [README.md, "*/__init__.py"]
  allow: [README.md, "docs/**/*.md", "*/**/*.py"]
  forbid: ["**/__pycache__/**", "**/*.pyc", "**/tests/**", "**/test_*.py"]
```

`SKILL.md` beside it tells the partition agent how to make the product in `/hikari/staging/<name>/`: what to copy there, the docs to write for a reader who cannot see the code, and to run `check_release` and then `publish_release`. Give it front matter with `name: <name>` and a `description` that says when to load it.

## Checks

Run `hikari check -draft topology.target.yaml`. It runs every rule of a topology, and allows repositories and folders that do not exist yet. Fix each finding.
