# The topology file of a containerized project

`topology.yaml` is a Docker Compose file. Its `x-hikari` keys tell hikari the repositories, the partitions, and the orchestration agent. Compose interpolates `${...}` everywhere, so write `$$` for a literal `$` in instructions.

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
      description: Orchestrates the acme product. Plans from the feature maps and reports to the user.

  server:
    <<: *agent
    build: {context: ., dockerfile: server.Dockerfile}
    volumes: [server-venv:/workspace/server/.venv]   # only for a dependency folder
    x-hikari:
      description: The acme HTTP server.
      instructions: |
        Format with ruff. Keep each handler in its own module.
        Do not run tests/browser: it needs Playwright and a browser, which this container lacks.
      partition:
        folders:
          - {repo: server}
        test: cd server && python3 -m pytest -q tests --ignore=tests/browser -p no:cacheprovider -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml

volumes:
  server-venv:
```

## What hikari does with it

- **Folders and places.** A partition is a list of folders. Each folder mounts in the partition agent's container at its place, `/workspace/<repo>/<path>`, or `/workspace/<repo>` for a whole repository, unless the folder sets `at:`. Leave `at:` out: the default place is the repository's name, which the test command and the instructions then use. The private files, the code map and the test map, live in `.hikari/private/` of the first writable folder, and the agent sees them at `/hikari/private/`.
- **Apply.** The partition agent has no git. hikari applies its changes: one commit in each repository whose writable folders changed, on the project branch (`hikari` unless `HIKARI_BRANCH` sets another), with the agent as author. The agent calls `apply` to name a commit, and hikari applies what is left at the end of every turn and at stop. The user reviews the branch and merges it.
- **The vault.** `vault/` next to the topology file holds every hikari file that is not code: the public wiki, the vocabulary, each partition's feature map and releases, and the orchestration agent's feature map. hikari makes its directories at start and mounts each part where its agents may see it. Write nothing there in containerize; the partition agents write their feature maps through their tools.
- **The orchestration agent.** It reads every partition's feature map and health, and every release, and no code. It needs no partition and no Dockerfile: `image: hikari-agent` is enough. It plans work across partitions, and in hikify it carries out the plan.

## Rules that hikari checks

- Names of services and repos: lowercase letters, digits, and dashes, at most 32 characters.
- `x-hikari.orchestration` names the service of the orchestration agent, which has no partition. Every other agent has a partition.
- A partition is a list of folders. A folder with only `repo` is the whole repository, and its container sees it at `/workspace/<repo>`. Write one folder for each repository; the hikify skill may later give a partition more.
- A service sets no `networks`, `ports`, `privileged`, `network_mode`, or bind mount of a repository. hikari mounts the repositories and the vault itself.
- Under `/workspace`, only named volumes and tmpfs inside one of the agent's writable folders are allowed, such as `/workspace/server/.venv`. Use them for dependency folders such as `node_modules` or `.venv`, so that installs do not land in git.
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

- It runs in `/workspace`, where the repository is `/workspace/<name>`. Start it with `cd <name> &&`, so that the runner works as it does in the repository. hikari reads the paths of the report relative to `/workspace`, and then relative to the folder.
- It writes JUnit XML to `/hikari/private/reports/all.xml`: hikari makes the command the suite `all`, and reads the report of a suite from `<suite>.xml`. Common forms:
  - pytest: `-o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml`
  - Go: `go test -v ./... 2>&1 | go-junit-report > /hikari/private/reports/all.xml` (install `go-junit-report` with `go install` in the Dockerfile)
  - Vitest: `--reporter=default --reporter=junit --outputFile=/hikari/private/reports/all.xml`
  - Jest: `jest-junit` with `JEST_JUNIT_OUTPUT_FILE=/hikari/private/reports/all.xml`
  - CTest: `ctest --output-junit /hikari/private/reports/all.xml`
  - Cargo: `cargo2junit` or `cargo nextest` with its JUnit profile
- It builds first when the tests need a build, for example `npm run build && npm test ...`.
- It leaves out each suite that cannot run in the container, with the runner's own option (`--ignore`, `-run`, `--exclude`, a marker), not by editing the suite.
