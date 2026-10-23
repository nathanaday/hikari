# E2E orchestration: round 1

## What I take from your proposal

- **Hub stories.** Each hub gets a wiki of business rules and user stories: what the ecosystem must do well. A story links the features of the systems that carry it out, and it gets health from tests, as a feature does.
- **An orchestration plan.** A description of how the systems fit together to do the ecosystem's tasks, and a runtime plan that starts the released products together for a test.
- **E2E tests against releases.** The tests use only released deliverables, never code or staging areas.
- **The orchestrator does the E2E work.** It designs the scaffold, writes the tests, and runs them.
- **A project repository.** Each hikari project is a git repository that holds the topology, the shared area, and the E2E scaffold.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

- **Story**: one document of a hub wiki. B2 asks whether a story and a business rule are two kinds.
- **Hub wiki**: the stories of one hub.
- **Tester**: the agent that writes and runs the E2E suite of a hub (A1).
- **E2E suite**: tests that check stories against released products.
- **Release set**: the one version of each release that an E2E run uses.
- **Environment**: the containers that run a release set while the E2E suite runs.
- **Project repository**: the git repository that holds a hikari project.

## What exists today

- An orchestrator cannot own a system. `internal/topology/topology.go:480` refuses it. Orchestrators never see code and have no scope.
- A feature link is `[[id]]` inside one system's wiki. Links never cross systems.
- The test map links tests to the owner's own features. The owner runs its suites and records each JUnit report with `record_test_run`.
- A release is a directory of files. The two deliverables in use are libraries: `python-library` in the shop and `npm-package` in yami. No deliverable makes a container image, and no example has a service that runs.
- Containers cannot reach each other, and no container gets the Docker socket. Only hikari, on the host, starts containers.
- The store is a plain directory, not a git repository. hikari commits only the scopes of owners.
- `myprojects/yami/` is almost a project repository: a topology, a Dockerfile, a store, and an Obsidian vault. It lives inside the hikari repo.
- `docs/design.md` lists "an E2E test agent that runs tests across systems" as open work. `phase-2.md` plants a validation agent.

---

## A. Who does the work

**A1. The orchestrator writes the stories; a tester proves them.** I recommend that you split "what must be true" from "the proof that it is true":

- The orchestrator writes the stories. It already decides what the ecosystem must do.
- A separate agent, the tester, owns the E2E scaffold and writes and runs the tests.

My reasons:

1. **The agent that sets the bar should not also write the check.** When one agent writes the story and the test, and a test fails, that agent can change either one to make the failure go away. A test written by an agent that cannot change the story is a stronger signal.
2. **The orchestrator's context is the plan.** The orchestrator wakes on every hub post. A scaffold, service logs, and a flaky wait would fill its context with work that is not planning.
3. **The tester is an ordinary owner.** It owns one scope, the scaffold. It has release grants and never sees system code. The rule that an orchestrator cannot own a system stays.

Your proposal, where the orchestrator does all of it, needs one agent fewer and no hand-off between story and test. It makes the orchestrator a new kind of agent that owns a scope. Which do you want?

**A2. The topology names the hub that a tester checks.** I recommend a key on the tester's service:

```yaml
  shop-e2e:
    x-hikari:
      description: Proves the shop's stories against its releases.
      system: {repo: project, scope: e2e/shop}
      tests: shop                 # the hub whose stories this agent checks
```

With `tests: shop`, hikari grants the tester the releases of every system in the hub and the hub's stories, read-only. The tester must be a member of the hub. A hub has at most one tester. Do you agree?

## B. The hub wiki

**B1. Stories use the format of the feature wiki.** Each story is a markdown document in `store/hubs/<hub>/stories/<id>.md`, with the same header, body rules, and `[[ ]]` links as a feature. The orchestrator changes stories through one tool, `change_stories`, which applies a set of writes as a whole. You edit the `## Notes` section in Obsidian. The code reuses the `featurewiki` package. Do you agree?

**B2. Two kinds: story and rule.** I recommend a `kind` property with two values:

- A **story** has an actor, a goal, a flow across systems, and an outcome. Example: "A customer orders two items, and the order reserves stock for both."
- A **rule** is a statement that every flow must keep. Example: "Stock never goes below zero."

A story links the rules that it must keep. The difference matters for the tester: it checks a story by walking the flow once, and it checks a rule in every story that links it. The alternative is one document type with no kind, as the feature wiki has no groups. Do you want two kinds or one?

**B3. A story links features across systems.** A story can link a feature of each system that the orchestrator sees, as `[[inventory/reserve-stock]]`. A rename in a system's wiki also rewrites the links in stories, and a link to a removed feature is flagged in the orchestrator view. The link form must also resolve in Obsidian. I will check which form does before the spec. Do you agree?

**B4. The orchestration layout is computed, not written.** The "layout that describes how each system fits together" already exists in two parts: the links from stories to features, and the interfaces that each map provides and consumes. I recommend no separate layout document. A story's body tells the flow in prose, with its links in order. The UI draws the hub's layout from the links. This keeps the rule of the feature wiki: links are the only structure. Do you agree?

**B5. Story health comes from E2E tests only.** The tester's test map links its tests to stories, and a story gets one of the four health values, as a feature does. A failing unit test in a system does not change a story's health. The UI shows a second signal beside it: the health of the features that the story links. I recommend that the two signals stay separate. A story can pass while one of its features has a failing unit test, and that fact is useful only when you can see both. Do you agree?

**B6. Owners read the stories that link their features.** Today an owner reads no other system's map. A story names features of other systems, but it holds no code. I recommend that an owner reads each story that links one of its features, so that it knows why the feature matters and which flows use it. The alternative keeps owners blind to stories, and the orchestrator passes on what they need. Which do you want?

**B7. One level of hubs in this phase.** A parent hub's orchestrator sees only the systems of its direct members, so a parent story cannot link the features of a child hub. An E2E run for a parent hub would also need the releases of systems that the parent cannot see. I recommend that this phase supports E2E only for a hub whose members own systems. A parent hub's E2E is planted. Do you agree?

## C. Release sets

**C1. The tester pins a release set in a lock file.** `e2e/<hub>/releases.lock` names one version of each release that the suite uses, with its digest. The tester changes it through a tool, `pin_releases`, which reads the versions and digests from the store, so the tester never types a digest. hikari checks the digests when an environment starts. The alternative is to always use `latest`. That needs no lock, but a run cannot be repeated, and a result does not say what it tested. Do you agree with the lock?

**C2. A change to the lock makes every E2E result pending.** A result counts only for the release set it ran on. I considered making pending only the stories that link features of the changed system. But an E2E test can depend on any service in the environment, so that rule would guess. Do you agree?

**C3. hikari tells the tester about new releases.** When a system publishes a version newer than the pin, the tester's next prompt names it, and the orchestrator view shows the hub as behind. The tester decides when to pin and run. hikari does not pin or run by itself. Do you agree?

## D. Products that run

**D1. Libraries need no environment.** The tester installs libraries from their releases into its own container, as yami's plugin owner does today. The shop's E2E can check a checkout story with both of its libraries in one process. So A, B, and C can be built and tested without any of section E. Do you agree?

**D2. A service is released as a container image.** For a service, I recommend a deliverable whose product is an OCI image archive, `image.tar`, written by `docker save`. hikari loads it into Docker on the host before the environment starts. The deliverable declares `runs: image`, so that hikari knows which products are images. The two alternatives:

| Option | For | Against |
|---|---|---|
| Image archive in the release (recommended) | The test runs the exact image that ships. The digest proves it. | Large files in the store. |
| Dockerfile and build context in the release, built by hikari | Small releases. | The build can differ from what ships. The context can carry source code to anyone who reads the release. |
| Image from a registry by digest, through the `dockerhub` target | Nothing large in the store. | Every version must be pushed. hikari needs registry credentials to pull. |

Which do you want?

## E. The environment

**E1. hikari runs the environment on the host.** A container cannot start containers, and the Docker socket stays refused. So the tester describes the environment, and hikari starts it. Do you agree?

**E2. The description is a compose file in the tester's scope.** `e2e/<hub>/environment.yaml` is a compose file. A service names a release instead of an image, and the lock gives the version:

```yaml
services:
  inventory:
    x-hikari: {release: inventory/http-service}
    healthcheck: {test: [CMD, curl, -f, http://localhost:8000/health]}
  orders:
    x-hikari: {release: orders/http-service}
    environment: {INVENTORY_URL: http://inventory:8000}
  db:
    image: postgres:17
```

Other services can use any image. hikari applies the boundary checks of the topology: no bind mount of a repo or the store, no Docker socket, no host network, and no privileges. Do you agree?

**E3. The tester joins the environment and runs its suite itself.** Two tools, `start_environment` and `stop_environment`:

1. `start_environment` returns at once. hikari loads the images, runs compose up, and waits for the health checks.
2. hikari connects the tester's container to the environment's network, then wakes the tester with a message: ready, or failed with the reason.
3. The tester runs its suite with its shell and records each report with `record_test_run`, as owners do today. It can also call the services by hand to find a fault.
4. `stop_environment` disconnects the tester and removes the containers, networks, and volumes, so that each start is fresh. hikari also stops the environment when hikari stops.

The alternative: hikari runs the suite in a runner container and records the report itself. Then no agent is in the path, which is harder to fake, but the tester cannot debug. That is the "hikari runs the suites" item planted with the validation agent. I recommend E3 now, and the runner with the validation agent. Do you agree?

**E4. The tester reads the logs of each service.** hikari writes each service's logs to `/hikari/private/environment/logs/<service>.log` in the tester's container. A log can hold a stack trace with file paths of a system's code. I recommend that the tester reads them anyway: they are what the system prints, not its code, and without them the tester cannot say which system failed. Do you agree?

**E5. The UI shows each environment.** The hub view shows the state of the hub's environment, its services, and a stop button. Do you agree?

## F. When a test fails

**F1. The tester reports to the orchestrator, with the suspects computed.** The tester posts the story, the test, the step that failed, and the log lines. hikari adds the suspects: the systems whose features the story links. The orchestrator decides which owner to ask. The tester describes behavior to owners (the request, what it expected, what it got), because an owner cannot read E2E code. Do you agree?

## G. The project repository

**G1. A project repository, with knowledge in git and products out of it.** I recommend this layout:

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
  e2e/<hub>/                       the tester's scope; results ignored
```

The rule: version what people and agents write as knowledge, and ignore products and results. Do you agree?

**G2. Releases stay out of git.** A clone of the project has no releases, but its lock still names them by digest. The durable copies are the targets, such as `github-release` and `dockerhub`. The alternative is to version small releases. That fills the history with binaries and needs a size limit. Do you agree that releases stay out?

**G3. hikari commits the store.** In a project repository, the server writes maps, features, stories, and vocabularies through tools. I recommend that hikari commits each change of those tools, with the agent as author and you as committer, as it does for a scope. The wiki then has a history, and `git log` on a feature shows who changed it and when. `docs/design.md` says the store is a plain directory; that section would change. Do you agree?

**G4. The project repository follows the same branch rules as every repo.** hikari finds it as the git repository that holds the topology file, under the reserved repo name `project`. `hikari up` requires it to be clean, checks out its hikari branch, and commits agent changes there. Your own edits to `store/shared` during a run stay uncommitted, and you commit them before the next start.

The alternative: the project repository stays on your branch, and hikari commits only the paths that its tools wrote. That is easier for hand edits, but it breaks the rule that agents commit only to a hikari branch that you merge. I recommend the same rule for every repo. Which do you want?

**G5. A system's code can live in the project repository.** A small project, like the shop, can keep its systems in the project repository too. Separate system repos stay the normal case. Do you agree?

**G6. `hikari init`, and yami moves out.** I recommend a command, `hikari init <dir>`, that writes a new project repository with the layout of G1, a starter topology, and the ignore file. `myprojects/yami/` then moves to its own repository as the first real project. Do you want both in this phase?

## H. Order of work

**H1. Three steps, each with its own spec.**

1. **The project repository** (G). It is small, and it decides where every new file lives, so nothing must move later.
2. **The hub wiki and the tester, against libraries** (A, B, C, D1, F). The shop's tester checks a checkout story with both libraries.
3. **Environments for services** (D2, E). This needs an example with services.

Do you agree with the order?

**H2. The shop becomes two services.** Step 3 needs an example where systems talk over the network. I recommend that the shop's orders and inventory become two small HTTP services, each releasing an image, because the shop already has the stories to test. The alternative is a third example, which leaves the shop as the library case. Which do you want?
