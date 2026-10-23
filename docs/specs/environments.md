# Spec: environments

Status: draft.

Sources: `docs/brainstorm/e2e.md` (round 1, sections E, F, and I) and `e2e-answers.md`. This is the fourth step of the E2E work, after the project repository, the wikis, and the tester.

## Summary

A tester can now run the released products of its hub as services beside its tests. A deliverable can make a container image: hikari builds it on the host from the commit that the release describes, and records its id in the release. The tester describes the services of its test system in `environment.yaml`, a compose file in its scope, where a service runs an image release, or any image with a release's product mounted into it. `start_environment` starts the environment for a scenario of the plan, waits until its services are healthy, and connects the tester's container to its network. The tester runs its suites against the services, reads their logs, and calls `stop_environment` when it is done. hikari checks the environment against the plan and against the boundaries of a topology, keeps one environment per hub, stops it when its time ends, and shows it in the UI.

## Decisions

From round 1 and its answers:

1. hikari does not learn kinds of products: a release is mounted into a container, or it is an image (E1).
2. hikari builds image releases on the host, from the commit that the map describes (E2).
3. Foreign platforms run under emulation, and the UI says so (E3).
4. hikari runs the environment on the host; the Docker socket stays refused (F1).
5. The environment is a compose file in the tester's scope; boundary checks apply (F2).
6. The tester joins the environment's network and runs its suites itself (F3).
7. One environment per hub, a time limit, and limits from the topology (F4).
8. The tester reads the logs of each service (F5).
9. No credentials reach an environment in this phase (F6).
10. The UI shows each environment and can stop it (F7).
11. A small example shaped like P3 exercises every mechanism (I2), and a live run on a slice of P3 is the last check (I3).

## Decisions made in this spec

1. **`start_environment` waits.** The brainstorm had it return at once and wake the tester with a message. hikari has no sender for such a message, and a compose start with health checks takes seconds to a few minutes, so the tool waits until the services are healthy, or up to five minutes, and says what it found. A start that is still running when the call returns can be read with `environment_status`.
2. **An image stays in the host's Docker.** The release records the image's id and tag; hikari does not save an archive into the store. A missing image is a problem that names the release, and publishing the next version builds again. An archive of a large image in every release would cost more than a rebuild.
3. **One platform per image deliverable.** A deliverable may name one platform, such as `linux/arm64`. A multi-platform image needs a registry or the containerd image store, which is outside this phase.
4. **The tester's own fixtures can be mounted.** A service can mount a directory of the tester's scope read-only, such as recorded sequences that a simulator replays. Nothing else of the host is mounted.
5. **Actors scale services.** An actor whose `runs` names a service sets that service's replica count for the scenario; an actor that runs `tests` is the tester's suite.

## Terms

- **Image deliverable**: a deliverable whose `deliverable.yaml` has an `image` section.
- **Image release**: a release of an image deliverable. Its product holds the docs; the image is in the host's Docker.
- **Environment**: the containers of `environment.yaml`, started for one scenario of the plan.
- **Environment project**: the compose project of an environment, `<project>-e2e-<hub>`.

## 1. Image releases

```yaml
# store/shared/deliverables/http-service/deliverable.yaml
version: 1
description: An HTTP service that runs from a container image.
layout:
  require: [README.md]
image:
  dockerfile: Dockerfile    # relative to the owner's scope
  context: .                # relative to the owner's scope
  platform: linux/amd64     # optional; the host's platform by default
```

- **IM1.** `image` is optional. `dockerfile` is required and `context` defaults to `.`. Both are relative paths that stay inside the scope. `platform`, when set, is `<os>/<arch>`.
- **IM2.** `publish_release` of an image deliverable, after every other check, extracts the owner's scope at the described commit into a temporary directory and runs `docker build` there, with the deliverable's dockerfile, context, and platform. A failed build fails the publish, with the build's last lines, and changes nothing.
- **IM3.** The image is tagged `hikari/<project>-<system>-<deliverable>:<version>`. The manifest gets an `image` section: the tag, the image id, and the platform.
- **IM4.** `check_release` of an image deliverable checks that the dockerfile exists in the scope. It does not build.
- **IM5.** The build has no network restriction and no credentials beyond what Docker on the host has.

## 2. The environment file

```yaml
# e2e/<hub>/environment.yaml
services:
  cloud:
    x-hikari: {release: cloud/http-service}
    depends_on: {db: {condition: service_healthy}}
    environment: {DATABASE_URL: "postgres://app:app@db/app"}
    healthcheck: {test: [CMD, wget, -qO-, http://localhost:8000/health], interval: 2s, retries: 30}
  edge:
    image: debian:bookworm-slim
    x-hikari:
      mount:
        edge/tarball: /opt/edge           # the product of a pinned release
        models/model-folder: /opt/models
        scope:fixtures/readings: /data    # a directory of the tester's scope
    command: [/opt/edge/bin/edge, --cloud, http://cloud:8000, --replay, /data]
  mqtt:
    image: eclipse-mosquitto:2
```

- **EN1.** `environment.yaml` sits at the root of the tester's scope. It is a compose file without `name`, `x-hikari` at the top, `networks`, or `volumes` at the top.
- **EN2.** A service's `x-hikari` may set `release` (`<system>/<deliverable>`, an image release that the lock pins) or `mount`, a map from a source to a container path. A source is `<system>/<deliverable>`, a release that the lock pins, whose product is mounted read-only; or `scope:<path>`, a directory of the tester's scope, mounted read-only. A service with `release` sets no `image` or `build`; every other service sets `image`. No service sets `build`.
- **EN3.** hikari refuses a service that sets `networks`, `network_mode`, `ports`, `privileged`, `cap_add`, `devices`, `pid`, `ipc`, `userns_mode`, `volumes_from`, `env_file`, a bind mount, or a variable whose value comes from the host environment. Named volumes and tmpfs are allowed. hikari gives every service one network, the environment's, so services reach each other by name and nothing else.
- **EN4.** hikari checks the environment against the plan: each service is a component of the plan; each component with `image` or `simulates`, and each component whose release is an image release, is a service. A finding is a warning in the plan's findings; `start_environment` refuses a start while one stands.
- **EN5.** A scenario's actors set the replicas: an actor whose `runs` names a service runs `count` replicas of it, times the scenario's count for the actor. Every other service runs one replica.

## 3. Starting and stopping

- **ST1.** `start_environment(scenario)` is a tester tool. It refuses while the private documents have problems, while the environment or the plan has findings, or while another environment of the hub runs.
- **ST2.** It writes the environment project's compose override, which adds the network, the mounts, the images of image releases, the replicas, the labels of the run, and the limits; then runs `docker compose up --detach --wait` for the project. It waits for the services to be healthy, up to five minutes.
- **ST3.** When the services are up, hikari connects the tester's container to the environment's network, so that the tester reaches each service by its name. The tool returns the services with their state, image, platform, and replicas, and where the logs are.
- **ST4.** A service whose image platform differs from the host's runs under emulation. Its status says `emulated`, and the result of the tool says so.
- **ST5.** `stop_environment` disconnects the tester, and runs `docker compose down --volumes` for the project. hikari also stops the environment when the scenario's `minutes` end (30 by default), and when hikari stops.
- **ST6.** `environment_status` returns the state of the hub's environment: none, starting, running, failed with the reason, or stopped, with its services.
- **ST7.** The topology can limit each environment: `x-hikari.e2e: {cpus: 4, memory: 8g}` on the tester's service. hikari divides nothing: every service of the environment gets the limit as its maximum.

## 4. Logs

- **LG1.** While an environment runs, hikari writes each service's logs to `/hikari/private/environment/logs/<service>.log` in the tester's container, one file per service, replicas together with their names. A start clears the files of the last run.
- **LG2.** The logs are private documents that are never committed: the scope's private ignore file lists `environment/`.

## 5. Credentials

- **CR1.** No variable of the host reaches an environment: compose's `environment` may set only literal values (EN3), and hikari passes no `.env`.

## 6. The UI

- **UE1.** The Tests tab of a hub shows its environment: its state, scenario, services with their state, image, platform, replicas, and whether they are emulated, the time it started and when it stops, and a Stop button.
- **UE2.** The snapshot's hub wiki carries the environment's status in its test plan, and a change of the status sends the wiki again.

## 7. The example

- **EX1.** `examples/sensors` is a project repository with four systems, a lead, and a tester, small enough to build in seconds:
  - `edge`: a Go binary that reads readings and sends each one over MQTT, released as a tarball (`linux-tarball`) and run from a mount in a Debian image;
  - `cloud`: an HTTP API that reads readings from MQTT, stores them, and opens an incident when a reading is over its threshold, released as an image (`http-service`) that hikari builds;
  - `console`: a web page that shows the incidents, released as a static build (`static-site`) and served by nginx;
  - `thresholds`: a data release of the threshold of each sensor (`data-files`) that the cloud loads.
- **EX2.** Its plan uses Mosquitto as a substitute for a cloud MQTT broker, a sensor simulator that replays fixture files, five edges in parallel, and names a gap: the hardware watchdog of a real edge device. Its key story is "A reading over its threshold opens an incident that the console shows."
- **EX3.** `make example-sensors` makes the example's repository in `.hikari/demo/sensors`, as `make demo-repo` does for the shop.

## 8. Out of scope

- Images for more than one platform in one release.
- Pushing images to a registry.
- Hikari running the suite in a runner container; the tester runs it.
- Credentials for real cloud services in an environment.
- The bench results of the fifth step.

## 9. Build plan

1. **Image deliverables.** The `image` section (IM1), the build at publish (IM2, IM3), the check (IM4), with a test that builds a tiny image when Docker is present.
2. **The environment file.** Parsing and checks (EN1 to EN5), and the override file, as plain functions with tests.
3. **Running.** Start, stop, status, the network connection, the time limit, stop at shutdown, logs (ST1 to ST7, LG1, LG2), with a test against Docker that starts a two-service environment when Docker is present.
4. **Tools, prompts, skill, UI.** `start_environment`, `stop_environment`, `environment_status`; the tester's skill and prompt; UE1, UE2.
5. **The example** (EX1 to EX3).
6. **Live check and docs.** A run of the example in which the tester starts the environment with five edges, proves the key story, and the story becomes `passing`. Then the P3 slice (decision 11), and the README, `docs/design.md`, and this spec's status.
