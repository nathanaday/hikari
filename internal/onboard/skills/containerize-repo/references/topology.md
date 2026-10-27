# The compose file of a containerized project

`compose.yaml` is a Docker compose file. Its `x-hikari` keys tell hikari the repositories, the systems, and the orchestrator. Compose interpolates `${...}` everywhere, so write `$$` for a literal `$` in instructions.

```yaml
name: myproject

x-hikari:
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  repos:
    server: {url: https://github.com/acme/server.git}
    cli:    {url: https://github.com/acme/cli.git}
  orchestration: orchestrator

x-agent: &agent
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  orchestrator:
    <<: *agent
    image: hikari-agent
    x-hikari:
      description: Orchestrates the acme product. Plans from the system maps and reports to the user.

  server:
    <<: *agent
    build: {context: ., dockerfile: server.Dockerfile}
    volumes: [server-venv:/workspace/.venv]          # only for a dependency folder
    x-hikari:
      description: Owns the acme HTTP server.
      instructions: |
        Format with ruff. Keep each handler in its own module.
        Do not run tests/browser: it needs Playwright and a browser, which this container lacks.
      system:
        repo: server
        test: python3 -m pytest -q tests --ignore=tests/browser -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml

volumes:
  server-venv:
```

## Rules that hikari checks

- Names of services and repos: lowercase letters, digits, and dashes, at most 32 characters.
- `x-hikari.orchestration` names the service of the orchestrator, which owns no system. Every other agent owns a system.
- A system without `scope` owns the whole repository. Its container sees it at `/workspace`.
- A service sets no `networks`, `ports`, `privileged`, `network_mode`, or bind mount of a repository. hikari mounts the repositories and the store itself.
- Under `/workspace`, only named volumes and tmpfs inside the agent's own scope are allowed. Use them for dependency folders such as `node_modules` or `.venv`, so that installs do not land in git.
- No `environment` variable starts with `HIKARI_`.

## The agent Dockerfile

Start from `hikari-agent`, install as root, and end as `agent`:

```dockerfile
FROM hikari-agent
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pytest python3-venv \
 && rm -rf /var/lib/apt/lists/*
USER agent
```

`hikari-agent` is Debian with Claude Code, git, and curl. Install the versions that the repository's build files name, where the distribution has them; otherwise use the language's own installer (for example `go install`, `npm install -g`, or a release tarball). Install what the tests need, and nothing for development on a person's machine.

## The test command

- It runs from `/workspace/<scope>`, which is `/workspace` for a whole repository.
- It writes JUnit XML to `/hikari/private/reports/all.xml`: hikari makes the command the suite `all`, and reads the report of a suite from `<suite>.xml`. Common forms:
  - pytest: `-o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml`
  - Go: `go test -v ./... 2>&1 | go-junit-report > /hikari/private/reports/all.xml` (install `go-junit-report` with `go install` in the Dockerfile)
  - Vitest: `--reporter=default --reporter=junit --outputFile=/hikari/private/reports/all.xml`
  - Jest: `jest-junit` with `JEST_JUNIT_OUTPUT_FILE=/hikari/private/reports/all.xml`
  - CTest: `ctest --output-junit /hikari/private/reports/all.xml`
  - Cargo: `cargo2junit` or `cargo nextest` with its JUnit profile
- It builds first when the tests need a build, for example `npm run build && npm test ...`.
- It leaves out each suite that cannot run in the container, with the runner's own option (`--ignore`, `-run`, `--exclude`, a marker), not by editing the suite.
