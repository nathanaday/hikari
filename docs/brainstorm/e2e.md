# E2E orchestration: round 1

This round was revised after your first note on it. Products are not only libraries: a project can release compiled engines, device software, APIs, web clients, data, and disk images. The E2E work needs an architecture plan, not only the links and interfaces. The plan must allow many clients at once, and it must name what the system cannot test. The shop is too simple to design from, so the examples come from the P3 ecosystem in the Movitherm-ITL vault.

## What I take from your proposal

- **Hub stories.** Each hub gets a wiki of business rules and user stories: what the ecosystem must do well. A story links the features of the systems that carry it out. It gets health from tests, as a feature does.
- **A tester.** One agent per hub designs the E2E scaffold, writes the tests, and runs them. It solves as much as it can, and it names the gaps where a test is not realistic. You help pick the few stories that make a good integration test.
- **An architecture plan.** A document that says how the released products are put together for a test: what runs, what stands in for what, which clients drive it and how many, and what cannot be tested.
- **E2E tests against releases.** The tests use only released products, never code or staging areas.
- **A project repository.** Each hikari project is a git repository that holds the topology, the shared area, and the E2E scaffold.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Decided

- **A tester, not the orchestrator, writes and runs the E2E suite.** The orchestrator writes the stories. The tester is an ordinary owner whose scope is the scaffold. The rule that an orchestrator cannot own a system stays.

## Terms

- **Story**: one document of a hub wiki. B2 asks whether a story and a business rule are two kinds.
- **Key story**: a story that you mark as part of the integration test that matters most (B4).
- **Tester**: the agent that plans, writes, and runs the E2E suite of a hub.
- **Architecture plan**: the tester's document that describes the test system: components, actors, scenarios, and gaps (section C).
- **Component**: one part of the test system: a release, a substitute, a simulator, or a fixture.
- **Substitute**: a component that stands in for an external service, such as Mosquitto for AWS IoT Core.
- **Simulator**: a component that stands in for hardware, such as a camera that replays recorded frames.
- **Fixture**: data that a test feeds in, such as recorded thermal sequences.
- **Actor**: a client that drives the test system, with a count, such as five gateways or twenty console users.
- **Scenario**: a set of stories, run with set actor counts for a set time, such as a smoke run or a load run.
- **Gap**: something that the plan cannot test, with the reason.
- **Release set**: the one version of each release that an E2E run uses.
- **Environment**: the containers that run the components while the E2E suite runs.

## What exists today

- An orchestrator cannot own a system. `internal/topology/topology.go:480` refuses it.
- A feature link is `[[id]]` inside one system's wiki. Links never cross systems.
- The test map links tests to the owner's own features. The owner runs its suites and records each JUnit report with `record_test_run`.
- A release is a directory of files with a manifest. Layout rules check the files, and hikari knows nothing about what kind of product they are.
- No container gets the Docker socket, so an owner cannot build a container image. Containers cannot reach each other. Only hikari, on the host, starts containers.
- The store is a plain directory, not a git repository. hikari commits only the scopes of owners.
- `docs/design.md` lists "an E2E test agent that runs tests across systems" as open work. `phase-2.md` plants a validation agent.

## What a P3 E2E test needs

P3 has six repositories. As I read the vault, an E2E test of P3 meets every hard case:

| System | What it releases | How a test can run it |
|---|---|---|
| p3-cloud | Two container images (API and MQTT consumer), a Vue console, docs | As images, with Postgres 16, RustFS for S3, and Mosquitto for AWS IoT Core, as its own compose file does today |
| p3-edge | C++ binaries as a Debian tarball, and a draft Yocto recipe | Installed into a Debian container. The `test-image-folder` camera driver replays stored frames, so no camera is needed |
| itl-cv | TFLite model folders under the model contract | Mounted into the edge container as data |
| itl-data-cap-s3 | A Go binary that writes thermal sequences | Its recorded sequences are fixtures for the camera simulator |
| itl-compulab-yocto | A disk image for the IOT-GATE-IMX8PLUS | Not at all. Its root file system can run as an arm64 container, which matches the board's CPU |
| itl-compulab-p3-hostkit | A Go binary for network, DIO, cellular, GPS, and the watchdog | Only its commands that need no hardware |

A key story could be: "A hotspot that grows in front of a camera opens an incident in the cloud, and a notification goes out." The test system for it:

- the p3-cloud images, with Postgres, RustFS, and Mosquitto in place of IoT Core, and Mailpit in place of SES;
- five edge containers in parallel, each with three cameras that replay recorded sequences;
- a model folder from itl-cv;
- a browser client that opens the incident in the console.

The gaps: the NPU (the edge runs the model on the CPU), real cameras, X.509 claims against IoT Core, SMS and voice notifications, the Yocto image, and the timing claims ("received in the cloud at +724 ms"), because an emulated or loaded host does not show real latency.

---

## A. The tester

**A1. The topology names the hub that a tester checks.** I recommend a key on the tester's service:

```yaml
  p3-e2e:
    build: {context: ., dockerfile: e2e.Dockerfile}   # pytest, Playwright, an MQTT client
    x-hikari:
      description: Proves P3's stories against its releases.
      system: {repo: project, scope: e2e/p3}
      tests: p3                   # the hub whose stories this agent checks
```

With `tests: p3`, hikari grants the tester the releases of every system in the hub and the hub's stories, read-only. The tester must be a member of the hub. A hub has at most one tester. Its image holds the test tools it needs, such as a browser for a web client, and compose builds it as for any owner. Do you agree?

## B. The hub wiki

**B1. Stories use the format of the feature wiki.** Each story is a markdown document in `store/hubs/<hub>/stories/<id>.md`, with the same header, body rules, and `[[ ]]` links as a feature. The orchestrator changes stories through one tool, `change_stories`, which applies a set of writes as a whole. The code reuses the `featurewiki` package. Do you agree?

**B2. Two kinds: story and rule.** I recommend a `kind` property with two values:

- A **story** has an actor, a goal, a flow across systems, and an outcome. Example: "A hotspot that grows opens an incident, and the incident carries its snapshot."
- A **rule** is a statement that every flow must keep. Example: "The cloud never recomputes an alarm that the edge raised."

A story links the rules that it must keep. The tester checks a story by walking its flow, and it checks a rule in every story that links it. The alternative is one type with no kind, as the feature wiki has no groups. Do you want two kinds or one?

**B3. A story links features across systems.** A story can link a feature of each system that the orchestrator sees, as `[[p3-edge/alarm-episodes]]`. A rename in a system's wiki also rewrites the links in stories, and a link to a removed feature is flagged in the orchestrator view. The link form must also resolve in Obsidian. I will check which form does before the spec. Do you agree?

**B4. You mark the key stories.** You can write a story yourself, in the UI or in Obsidian, and mark it `key: true`. Only you can set `key`. The tester must cover each key story with a test, or name it in a gap with the reason. The orchestrator can write any story, but it cannot make one key. Do you agree?

**B5. Story health comes from E2E results only.** The tester's test map links its tests to stories, and a story gets one of the four health values, as a feature does. A failing unit test in a system does not change a story's health. Beside it, the UI shows two more facts:

- the health of the features that the story links;
- the substitutes and gaps that the story's tests ran with, such as "IoT Core: Mosquitto" or "NPU: not tested".

A story that passes with substitutes is passing, but it never looks the same as a story that ran on the real parts. Do you agree?

**B6. Owners read the stories that link their features.** Today an owner reads no other system's map. A story names features of other systems, but it holds no code. I recommend that an owner reads each story that links one of its features, so that it knows which flows use the feature. The alternative keeps owners blind to stories, and the orchestrator passes on what they need. Which do you want?

**B7. One level of hubs in this phase.** A parent hub's orchestrator sees only the systems of its direct members, so a parent story cannot link the features of a child hub. I recommend that this phase supports E2E only for a hub whose members own systems. A parent hub's E2E is planted. Do you agree?

## C. The architecture plan

**C1. The tester writes a plan for each hub, and you review it.** The plan is `e2e/<hub>/plan.md` in the tester's scope. It has a property header that hikari checks, and a body that explains the test system in prose, with a diagram when one helps. The tester writes the plan before it writes tests, and it asks you to review it. The orchestrator reads it too, because it shows which stories the hub can prove. I considered computing the layout from the stories' links and the interfaces of the maps. That shows what depends on what, but it cannot say what stands in for a camera, or how many gateways run at once. Do you agree?

**C2. The header names the components, actors, scenarios, and gaps.** For the P3 example:

```yaml
components:
  cloud-api:      {release: p3-cloud/api-image}
  cloud-consumer: {release: p3-cloud/consumer-image}
  console:        {release: p3-cloud/console}
  edge:           {release: p3-edge/debian-package}
  model:          {release: itl-cv/model-folder, kind: fixture}
  sequences:      {release: itl-data-cap-s3/bench-sequences, kind: fixture}
  db:             {image: postgres:16}
  mqtt:           {image: eclipse-mosquitto:2, substitutes: AWS IoT Core}
  s3:             {image: rustfs/rustfs, substitutes: AWS S3}
  mail:           {image: axllent/mailpit, substitutes: AWS SES}
actors:
  gateway: {runs: edge, count: 5, note: three replayed cameras each}
  operator: {runs: browser, count: 2}
scenarios:
  smoke: {stories: [hotspot-opens-incident], actors: {gateway: 1, operator: 1}}
  load:  {stories: [hotspot-opens-incident, config-reaches-edge], actors: {gateway: 20, operator: 10}, minutes: 15}
gaps:
  - {what: NPU inference, why: no board in the test system, touches: ["[[p3-edge/ml-detect]]"]}
  - {what: alarm latency, why: an emulated and loaded host does not show real latency, touches: ["[[incident-latency]]"]}
```

The names are examples. The spec fixes the fields. Do you agree with these four parts?

**C3. Substitutes and simulators say what they replace.** A component with `substitutes:` stands in for an external service. A component with `simulates:` stands in for hardware. Each test result records the substitutes and simulators of its scenario, so that B5 can show them. Do you agree?

**C4. A simulator that needs a system's knowledge comes from that system.** The tester never sees code. It can build a substitute for an external service from public docs, or use an image such as Mailpit. A simulator of a device that a system talks to needs that system's protocol knowledge. Then the tester asks the owner, through the orchestrator, to release the simulator as a deliverable. In P3, p3-edge already has the `test-image-folder` driver, and DataCapS3 can release recorded sequences. Do you agree?

**C5. hikari keeps the plan honest.** hikari checks that:

- each component names a release in the lock or an image;
- each component of the plan appears in the environment, and each service of the environment appears in the plan;
- each gap links at least one story or feature;
- each key story has a test or a gap.

A plan that fails a check is shown with its problems in the UI, and the tester cannot start an environment from it. Do you agree?

**C6. A gap needs your approval to count.** A gap that the tester names is "proposed" until you approve it in the UI. A story whose only checks are proposed gaps is `untested`. A story whose checks are approved gaps shows "out of reach" with the reason. This keeps the tester from closing a hard story by calling it a gap. Do you agree?

**C7. You can record a bench test by hand.** Some stories can only be checked on real hardware: the NPU, DIO, a SEEK camera. The tester writes such a check as a procedure in its scope. You run it on the bench and record pass or fail in the UI, with a note. The result counts like an E2E result, and it goes pending when the release set changes. I recommend this, but in a later step than the rest (section I). Do you want it?

## D. Release sets

**D1. The tester pins a release set in a lock file.** `e2e/<hub>/releases.lock` names one version of each release that the plan uses, with its digest. The tester changes it through a tool, `pin_releases`, which reads the versions and digests from the store, so the tester never types a digest. hikari checks the digests when an environment starts. The alternative is to always use `latest`. That needs no lock, but a run cannot be repeated, and a result does not say what it tested. Do you agree with the lock?

**D2. A change to the lock makes every E2E result pending.** A result counts only for the release set it ran on. A narrower rule, such as "only the stories that link the changed system", would guess: an E2E test can depend on any component. Do you agree?

**D3. hikari tells the tester about new releases.** When a system publishes a version newer than the pin, the tester's next prompt names it, and the orchestrator view shows the hub as behind. The tester decides when to pin and run. Do you agree?

## E. Running releases

**E1. hikari does not learn kinds of products.** I recommend two ways to run a release, and no list of product kinds in hikari:

1. **Mount the product.** A service of the environment names a release, and hikari mounts its product read-only into that container. The service's image is any image that the plan chooses. This covers binaries and tarballs (the p3-edge package in a Debian image), data (a model folder, recorded sequences), web builds (the console served by nginx), and libraries.
2. **Run an image release.** A deliverable that makes a container image releases the image itself (E2), and the service runs it.

A web client is tested by a browser in the tester's container, against the served build. A disk image, such as the Yocto image, runs nowhere, and the plan names it in a gap. Do you agree?

**E2. hikari builds image releases on the host.** An owner cannot run Docker, so it cannot build an image. I recommend that a deliverable declares the build:

```yaml
image:
  dockerfile: Dockerfile          # relative to the owner's scope
  context: .
  platforms: [linux/amd64, linux/arm64]
```

At `publish_release`, hikari builds the image on the host from the commit that the map describes, tags it `hikari/<system>/<deliverable>:<version>`, and writes its digest into the manifest. The build context never enters the release, so no source reaches a reader of the release. The alternatives:

| Option | For | Against |
|---|---|---|
| hikari builds from the commit (recommended) | The owner needs no Docker. The image matches a commit. No source in the release. | hikari runs builds, which can be slow. A multi-platform build needs buildx with QEMU. |
| The owner builds in its container with a rootless builder, such as BuildKit rootless, and stages an archive | hikari runs no builds. | Rootless builders need container settings that the boundary checks refuse today. Archives of 1.5 GB in the store. |
| The owner pushes to a registry, through the `dockerhub` target | Nothing large on the host. | Every test needs a push and a pull, and hikari needs credentials. |

Which do you want?

**E3. Foreign platforms run under emulation, and results say so.** An arm64 release can run on an amd64 host through QEMU. A result from an emulated container records that fact, and the plan names timing claims as gaps when they run emulated. Do you agree?

## F. The environment

**F1. hikari runs the environment on the host.** A container cannot start containers, and the Docker socket stays refused. So the tester describes the environment, and hikari starts it. Do you agree?

**F2. The environment is a compose file in the tester's scope.** `e2e/<hub>/environment.yaml` realizes the plan. A service names a release to mount or run, and the lock gives the version:

```yaml
services:
  cloud-api:
    x-hikari: {release: p3-cloud/api-image}
    depends_on: {db: {condition: service_healthy}}
  edge:
    image: debian:bookworm-slim
    x-hikari: {mount: {p3-edge/debian-package: /opt/release, itl-cv/model-folder: /opt/p3-edge/models/yolo}}
    command: [/opt/release/install-and-run.sh]
    deploy: {replicas: 5}
  db:
    image: postgres:16
    healthcheck: {test: [CMD, pg_isready]}
```

Scenarios map to compose profiles and replica counts. hikari applies the boundary checks of the topology: no bind mount of a repo or the store, no Docker socket, no host network, no privileges. Do you agree?

**F3. The tester joins the environment and runs its suite itself.** Two tools, `start_environment(scenario)` and `stop_environment`:

1. `start_environment` returns at once. hikari builds or loads the images, runs compose up with the scenario's profile and counts, and waits for the health checks.
2. hikari connects the tester's container to the environment's network, then wakes the tester with a message: ready, or failed with the reason.
3. The tester runs its suite and records each report with `record_test_run`, as owners do today. It drives the actors from its suite, and it can call the services by hand to find a fault.
4. `stop_environment` disconnects the tester and removes the containers, networks, and volumes, so that each start is fresh. hikari also stops the environment when hikari stops, or when the scenario's time limit ends.

The alternative: hikari runs the suite in a runner container and records the report itself. Then no agent is in the path, but the tester cannot debug. That belongs with the validation agent. I recommend F3 now. Do you agree?

**F4. Limits.** One environment per hub at a time. Each scenario has a time limit, from the plan, with a default of 30 minutes. The topology can set CPU and memory limits for a hub's environments. Do you agree?

**F5. The tester reads the logs of each service.** hikari writes each service's logs to `/hikari/private/environment/logs/<service>.log` in the tester's container. A log can hold a stack trace with file paths of a system's code. I recommend that the tester reads them anyway: they are what the system prints, not its code, and without them the tester cannot say which system failed. Do you agree?

**F6. External services: substitutes only in this phase.** An environment gets no credentials and calls no real cloud service. The plan uses substitutes, and names the real service in a gap. A later phase can let the topology pass named secrets to an environment for a test against a real service. Do you agree?

**F7. The UI shows each environment.** The hub view shows the environment's scenario, its services, their state, and a stop button. Do you agree?

## G. When a test fails

**G1. The tester reports to the orchestrator, with the suspects computed.** The tester posts the story, the test, the step that failed, and the log lines. hikari adds the suspects: the systems whose features the story links. The orchestrator decides which owner to ask. The tester describes behavior to owners (the request, what it expected, what it got), because an owner cannot read E2E code. Do you agree?

## H. The project repository

**H1. A project repository, with knowledge in git and products out of it.** I recommend this layout:

```
myproject/                         git repository
  compose.yaml
  *.Dockerfile
  .env                             ignored
  store/
    shared/                        versioned
    hubs/<hub>/stories/            versioned
    systems/<s>/map.yaml           versioned
    systems/<s>/features/          versioned
    systems/<s>/staging/           ignored
    systems/<s>/releases/          ignored
  e2e/<hub>/                       the tester's scope: plan, lock, environment, tests; results ignored
```

The rule: version what people and agents write as knowledge, and ignore products and results. Do you agree?

**H2. Releases stay out of git.** A clone of the project has no releases, but its lock still names them by digest. The durable copies are the targets, such as `github-release` and `dockerhub`. Large fixtures, such as recorded sequences, come from releases where a system makes them; small fixtures that the tester makes live in its scope. Do you agree?

**H3. hikari commits the store.** In a project repository, the server writes maps, features, stories, and vocabularies through tools. I recommend that hikari commits each change of those tools, with the agent as author and you as committer, as it does for a scope. The wiki then has a history. `docs/design.md` says the store is a plain directory; that section would change. Do you agree?

**H4. The project repository follows the same branch rules as every repo.** hikari finds it as the git repository that holds the topology file, under the reserved repo name `project`. `hikari up` requires it to be clean, checks out its hikari branch, and commits agent changes there. Your own edits to `store/shared` during a run stay uncommitted, and you commit them before the next start. The alternative keeps the project repository on your branch, which breaks the rule that agents commit only to a hikari branch that you merge. I recommend one rule for every repo. Which do you want?

**H5. A system's code can live in the project repository.** A small project can keep its systems in the project repository. Separate system repos stay the normal case, as in P3. Do you agree?

**H6. `hikari init`.** A command, `hikari init <dir>`, writes a new project repository with the layout of H1, a starter topology, and the ignore file. `myprojects/yami/` then moves to its own repository. Do you want both in this phase?

## I. Order of work and examples

**I1. Four steps, each with its own spec.**

1. **The project repository** (H). It decides where every new file lives.
2. **Stories, the tester, the plan, and the lock** (A to D, G). Tests run in the tester's own container, against mounted releases: libraries, binaries, and data. This proves the documents, the checks, and story health without an environment.
3. **Environments** (E, F). Mounted releases in any image, image releases built by hikari, actors, and scenarios.
4. **Bench results** (C7).

Do you agree with the order?

**I2. A small example shaped like P3, for hikari's own tests.** I recommend a new example with four systems, each small enough to build in seconds:

- `edge`: a compiled Go binary, released as a tarball and run from a mount in a Debian image;
- `cloud`: an HTTP API with Postgres, released as an image that hikari builds;
- `console`: a web client, released as a static build and tested in a browser;
- `models`: a data release that the edge loads.

Mosquitto is a substitute, a sensor that replays fixture files is a simulator, and five edges run in parallel. A key story: "A reading over its threshold opens an incident that the console shows." A gap: the edge's hardware watchdog. Every mechanism of this phase runs in it. The shop stays as the smallest smoke test. Do you agree?

**I3. A live run on a slice of P3.** As the last check of step 3, I recommend a hikari project for a slice of P3: p3-cloud, p3-edge, one itl-cv model folder, and DataCapS3 sequences, with the hotspot story as the key story. It would run against copies or hikari branches of the real repositories. Do you want this run in this phase, or after it?
