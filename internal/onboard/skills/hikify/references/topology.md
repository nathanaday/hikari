# The target topology

## Drawing boundaries

Each boundary of the target states one reason, of one of three kinds. Propose no boundary without one.

- **Library.** Two or more consumers need the code, or their copies already differ. The library becomes its own system, and the consumers use its releases.
- **Client and server.** They change at different rates, or a different team owns each. Each becomes a system, and they meet at an interface that the hub's vocabulary names.
- **Scope in a repository.** A repository holds two concerns, but splitting its git history is not worth the cost. Each concern becomes a system with its own `scope` in the same repository.

Also name each split you considered and decided against, and why. Small code with one consumer stays where it is. A library for one consumer adds a release cycle for no gain.

## The compose parts of a target

`compose.target.yaml` is a whole compose file. It keeps every part of `compose.yaml` that the design does not change.

A new library in a new repository, and a consumer of its releases:

```yaml
x-hikari:
  repos:
    units: {url: https://github.com/acme/units.git}   # to create, in execution
  hubs:
    main: {orchestrator: orchestrator, members: [units, cloud]}

services:
  units:
    <<: *agent
    build: {context: ., dockerfile: units.Dockerfile}
    x-hikari:
      description: Owns the units library, which converts sensor readings.
      system: {repo: units, test: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml}
      grants:
        deliverables: [python-library]      # the releases it publishes

  cloud:
    <<: *agent
    build: {context: ., dockerfile: cloud.Dockerfile}
    x-hikari:
      description: Owns the cloud API.
      system: {repo: cloud, test: ...}
      grants:
        releases: [units]                   # reads units' releases, read-only
```

A consumer finds a release at `/hikari/public/readonly/systems/<library>/releases/<deliverable>/<version>/product/`, or `latest/product/`. For a Python library, it adds that folder to `PYTHONPATH` in its test command.

Two systems in one repository, each with its own scope (scopes in one repository never overlap):

```yaml
      system: {repo: cloud, scope: api, test: ...}
      system: {repo: cloud, scope: consumer, test: ...}
```

## Deliverables

A library publishes a release of a deliverable: a kind of product, with rules for its files. The topology grants it with `grants.deliverables`. Draft each new deliverable in `onboarding/shared/deliverables/<name>/`. Execution copies it to `store/shared/deliverables/<name>/`. A deliverable that the store already has needs no draft.

`deliverable.yaml`:

```yaml
version: 1
description: A Python package that other systems add to their PYTHONPATH, with docs for engineers who cannot see the code.
layout:
  require: [README.md, "*/__init__.py"]
  allow: [README.md, "docs/**/*.md", "*/**/*.py"]
  forbid: ["**/__pycache__/**", "**/*.pyc", "**/tests/**", "**/test_*.py"]
```

`SKILL.md` beside it tells the owner how to make the product in `/hikari/public/export/staging/<name>/`: what to copy there, the docs to write for a reader who cannot see the code, and to run `check_release` and then `publish_release`. Give it front matter with `name: <name>` and a `description` that says when to load it.

## Checks

Run `hikari check -draft compose.target.yaml`. It runs every rule of a topology, and allows repositories and scopes that do not exist yet. Fix each finding.
