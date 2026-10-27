# Spec: the topology is a compose file

Status: built.

Hubs were removed in `remove-hubs.md`.

Terms changed in `shared-language-names.md`; see `docs/glossary.md`.

Changes made during the build:

- hikari pulls an agent's image with `docker pull` only when the image is not on the machine. `docker compose pull --policy missing` tried to pull local images, such as `hikari-agent`, from a registry.
- Sidecars may not set the host-level keys of C7 either (`privileged`, `cap_add`, `devices`, `pid`, `ipc`, `userns_mode`, `volumes_from`), not only the network keys. A privileged sidecar could reach the host, and so the repos.
- Every compose command of a run gets the token variables, not only `up`, because `ps`, `logs`, `events`, and `down` also read the override file.
- The shop example has a sidecar, `orders-mock`, so that the demo shows one.

## Summary

The topology becomes a Docker compose file. Compose describes each agent's environment: its image or Dockerfile, its env files and secrets, its volumes, and any helper containers such as a database. hikari's own model stays in `x-hikari` keys, which compose ignores: the repos, the systems, the grants, the hubs, and the store. `hikari up compose.yaml` starts the server and the UI, writes a second compose file with what hikari adds to each agent, and runs `docker compose` with both files. hikari still computes every mount that forms a boundary, and it refuses a compose file that would cross one.

This replaces the topology fields that copied compose: `packages`, `setup`, `path`, `container_only`, the derived images, and the fixed list of passed variables.

## Decisions

The brainstorm settled one file, `x-` keys, and an image contract instead of a base image. You approved these points with the spec.

1. **hikari reads compose's resolved YAML.** `hikari up` runs `docker compose -f <file> config` and reads its output, so that hikari sees the file exactly as compose will run it: anchors, `extends`, interpolation, and `env_file` resolved. A test showed that the JSON output drops `x-` keys on services, so hikari reads the YAML output.
2. **A top-level `name:` is required.** The compose project name names the containers, the networks, and the volumes. A stable name keeps volumes such as `node_modules` between runs, and stops two projects in one directory from sharing them.
3. **Every service is an agent or a sidecar.** A service with `x-hikari` agent fields is an agent. A service with `x-hikari: {serves: <agent>}` is a sidecar of that agent. hikari refuses a service with neither.
4. **hikari owns the agent's process and its network.** An agent service may not set `entrypoint`, `command`, `working_dir`, `networks`, `network_mode`, `links`, or `external_links`. hikari sets them in the override file.
5. **hikari mounts its own binary.** The image needs no hikari binary, so it never holds an old one after an upgrade. `make build` also builds `bin/hikari-linux-amd64` and `bin/hikari-linux-arm64`. `hikari up` mounts the one that matches each image's architecture at `/hikari/bin/hikari`.
6. **The reference image is `hikari-agent`.** The repo's `Dockerfile` builds an image that meets the image contract: Debian, git, curl, ripgrep, Claude Code, and the `agent` user. It no longer holds the hikari binary. Your Dockerfiles can start `FROM hikari-agent` or from any image that meets the contract.
7. **hikari makes named volumes writable.** A new named volume belongs to root, so the agent user cannot write it; a test confirmed this. For each agent with named volumes, the override file adds a prepare service. It runs once as root before the agent, and gives the volumes to the image's user.
8. **Claude credentials are ordinary variables.** hikari no longer passes `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` by itself. You pass them like any other variable. The examples pass them by name from your environment, so the value stays in your `.env`. `hikari up` warns about an agent that has neither variable.
9. **hikari's tokens never reach the disk.** The override file says `HIKARI_TOKEN: ${HIKARI_TOKEN_<AGENT>}`, and hikari sets those variables only in the environment of the `docker compose` process.
10. **The old topology format goes away.** hikari does not read it and does not convert it. The three examples move to compose files.
11. **Extra mounts are allowed, outside the boundaries.** An agent can bind any host path, such as a dataset, except a path in a topology repo, the store, or the Docker socket. Its targets stay out of `/hikari` and out of `/workspace`, except named volumes and tmpfs inside its own scope.
12. **A sidecar serves one agent.** It joins only that agent's network, so only that agent can reach it, by its service name. Sharing a sidecar between agents is out of scope.
13. **One run per project.** `hikari up` refuses to start when containers of the project are running.

## Terms

- **Compose file**: the topology. A Docker compose file with `x-hikari` keys.
- **Agent service**: a compose service that runs a hikari agent.
- **Sidecar**: a compose service that serves one agent, such as a database for its tests. It has no hikari powers.
- **Override file**: the compose file that hikari writes for each run, with what it adds to each service.
- **Image contract**: what an agent's image must provide (section 2).

## 1. The compose file

```yaml
name: shop

x-hikari:
  store: ./store                  # optional; the default is store/ next to this file
  repos:
    shop: {path: ../.hikari/demo/shop, branch: hikari/demo}
  hubs:
    shop:
      description: Builds the shop.
      orchestrator: lead
      members: [orders, inventory]

x-claude: &claude                 # an anchor; any name works
  environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]

services:
  lead:
    <<: *claude
    image: hikari-agent
    x-hikari:
      description: Product lead for the shop. Plans changes from the system maps.

  orders:
    <<: *claude
    build: {context: ., dockerfile: python.Dockerfile}
    volumes: [orders-venv:/workspace/services/orders/.venv]
    x-hikari:
      description: Owns order intake.
      system:
        repo: shop
        scope: services/orders
        test: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/all.xml
      grants:
        releases: [inventory]

  orders-db:
    image: postgres:17
    environment: {POSTGRES_PASSWORD: dev}
    x-hikari: {serves: orders}

volumes:
  orders-venv: {}
```

The agent fields of a service's `x-hikari` are `description`, `instructions`, `model`, `system` (`repo`, `scope`, `read`, `test`), and `grants`. `system.test` is the command of the suite `all` in a new test map; it must write a JUnit XML report to `/hikari/private/reports/all.xml` (`docs/specs/test-health.md`, rule O9). The top-level `x-hikari` holds `store`, `repos`, and `hubs`. Unknown fields are errors.

- **C1.** The file has a top-level `name`.
- **C2.** Each service has `x-hikari`. It holds agent fields or `serves`, not both.
- **C3.** Agent names are service names, and follow the rule for agent names.
- **C4.** The topology rules T1 to T7 and T9 to T12 apply to the `x-hikari` keys. T8 goes away with `packages` and `path`. T4 no longer covers `container_only`.
- **C5.** Relative paths in `x-hikari` start at the compose file's directory, and `~` expands.
- **C6.** `serves` names an agent service.

## 2. Agent services

Compose owns the environment: `image` or `build`, `environment`, `env_file`, `secrets`, `volumes`, `ports`, `extra_hosts`, `user`, `restart`, `deploy` (for example GPU reservations), and the rest.

hikari owns the process and the boundaries:

- **C7.** An agent service sets none of `entrypoint`, `command`, `working_dir`, `networks`, `network_mode`, `links`, `external_links`, `volumes_from`, `privileged`, `cap_add`, `devices`, `pid`, `ipc`, or `userns_mode`.
- **C8.** No bind mount of an agent service or sidecar has a source in a topology repo, in the store, or at the Docker socket, and no source contains one of them.
- **C9.** No mount of an agent service targets `/hikari` or a path below it. A mount that targets `/workspace` or a path below it is a named volume or tmpfs inside the agent's own scope, and not inside `<scope>/.hikari`.
- **C10.** No `environment` entry of an agent service starts with `HIKARI_`.

The image contract:

- **C11.** `claude` is on the `PATH` of the image.
- **C12.** The image's user is set, and is not root.
- **C13.** The image has `chown`, when the agent has named volumes (decision 7).

`hikari up` checks C11 and C12 for each agent image before it starts any container, and names the agent and the missing part.

## 3. Sidecars

- **C14.** A sidecar joins only the network of the agent it serves. hikari sets its `networks`, so the sidecar sets none of the keys in C7 except `entrypoint`, `command`, and `working_dir`.
- **C15.** C8 applies to sidecars. A sidecar gets no hikari mounts and no token.

## 4. What hikari adds

The override file gives each agent service:

- **O1.** The mounts of the system maps and shared store specs (M1 to M3, M5 to M9), unchanged. The volumes for `container_only` (M4) go away: you declare named volumes in compose.
- **O2.** `/hikari/bin/hikari`, read-only, from the binary that matches the image's architecture (decision 5).
- **O3.** `entrypoint: [/hikari/bin/hikari, node]`, `init: true`, and for an owner `working_dir: /workspace/<scope>`.
- **O4.** Its own network, `<project>-agent-<name>`, and `host.docker.internal` pointed at the host, so that it reaches the server and nothing else of the topology.
- **O5.** `HIKARI_SERVER_URL`, `HIKARI_TOKEN` (decision 9), and `DISABLE_AUTOUPDATER=1`.
- **O6.** A prepare service, `<name>-prepare`, when the agent has named volumes. It runs the agent's image as root with no network, gives each named volume of the agent to the image's user, and exits. The agent `depends_on` it with `service_completed_successfully`.
- **O7.** The label `hikari.run=<run id>` on every container and network, so that `make clean` finds what a crashed run left.

The override file goes to `.hikari/runs/<run id>/compose.override.yaml`, next to the run's log and messages. It holds no secret.

## 5. Running

`hikari up [-listen addr] [-no-launch] <compose.yaml>`:

1. Run `docker compose config`, then read and check the file (C1 to C15, except C11 to C13).
2. Refuse to start when containers of the project are running (decision 13).
3. Open the listener, then prepare the repos and the store, as today.
4. Run `docker compose build`, pull each agent image that is not on the machine, then check the image contract (C11 and C12).
5. Write the override file, and run `docker compose -p <name> -f <file> -f <override> up`.
6. Show each line of container output in hikari's log with the service name. When an agent's container stops, the UI shows it as exited.
7. On Ctrl-C: `docker compose down --remove-orphans`, which keeps named volumes. Then the final commits and the summary, as today.

With `-no-launch`, hikari does steps 1 to 3 and prints each agent's credentials, as today.

- **R1.** `hikari up` warns about each agent with neither `ANTHROPIC_API_KEY` nor `CLAUDE_CODE_OAUTH_TOKEN` set (decision 8).
- **R2.** `make clean` removes the containers and networks with a `hikari.run` label, as today.

## 6. Removed

- The topology fields `packages`, `setup`, `path`, and `container_only`, and rules T8 and I1 to I3.
- Derived images (`internal/launcher/images.go`) and the per-agent `docker run` and network calls.
- `launcher.PassEnv` and the `-image` flag.

## 7. Examples and docs

- `examples/studio/`, `examples/team/`, and `examples/shop/` each hold a `compose.yaml`. The shop also holds a Dockerfile for its Python agents, its demo seed, and its store.
- `make up COMPOSE=examples/shop/compose.yaml` replaces `TOPOLOGY=`.
- The README shows the compose file and the image contract. `docs/design.md` explains the split: compose owns the environment, hikari owns the boundaries. The system maps spec points here for the removed fields.

## 8. Out of scope

- More than one compose file per run (`-f a -f b` from the user).
- A sidecar shared by several agents, and services with no hikari role.
- Running agents without Docker.
- Changing the compose file while a run is active.

## 9. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Topology.** Parse resolved compose YAML into the topology (C1 to C6), with the old rules carried over and the removed fields gone. Tests use YAML strings, as today; only `Load` runs `docker compose config`.
2. **Checks.** C7 to C10, C14, and C15 on the resolved services, with one test per rule.
3. **Override.** Generate the override file (O1 to O7) from the topology. Tests compare the generated YAML. Cross-compile the Linux binaries in `make build`.
4. **Runner.** `hikari up` on compose (section 5): config, build, contract check, up, logs, exit status, down. Remove images, `PassEnv`, and `-image`.
5. **Examples.** Move the three examples and the demo to compose files, and rename the image `hikari-agent`.
6. **Live test.** Run the shop with a sidecar for orders. Check that orders reaches the sidecar by name and inventory cannot, that orders still tests against inventory's release, and that no container sees what the specs forbid.
7. **Docs.** README, `docs/design.md`, and the system maps spec.
