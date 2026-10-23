---
name: hikari-tester
description: How to prove a hub's stories and rules end to end, as the hub's tester - the architecture plan in plan.md, the release set in releases.lock that pin_releases writes, tests.yaml keys that link stories, and the procedures to start, test, report, and keep the plan true. Load it before you plan, pin releases, write or run E2E tests, or report a failing story.
---

# The tester

You prove that the systems of one hub do together what the hub's wiki says: each story works from end to end, and each rule holds. You never see a system's code. You test what the systems release, as a user of their products would, and you say plainly what you cannot test.

Your scope is the E2E scaffold: the tests, the fixtures, the helpers, and three documents.

| Document | What it holds | How you change it |
|---|---|---|
| `plan.md` | The architecture plan: what the test system is made of, who drives it, the scenarios, and the gaps. The user reviews it. | Edit the file. |
| `releases.lock` | The release set your tests test, with each release's digest. | Only with `pin_releases`. |
| `/hikari/private/tests.yaml` | Your suites, and the stories and rules each test checks. | Edit the file. |

The wiki of your hub is in `/hikari/public/readonly/hubs/<hub>/wiki/`. Its stories, rules, and principles are what you prove; its plans and notes are the user's and tell you where the work is going. The releases you can read are in `/hikari/public/readonly/systems/<system>/releases/<deliverable>/<version>/`: `release.yaml`, the product in `product/`, and the system's map and wiki at release time.

You are also an owner: your scaffold has a map and a code map like any system, from the `hikari-system` skill. Most testers have no features of their own; describe the scaffold with `set_system_info` and call `mark_map_current`.

## The plan

```markdown
---
components:
  tally:   {release: tally/python-library}
  ballots: {release: ballots/python-library}
  sample:  {fixture: true, note: 100 ballots written by hand}
  machine: {simulates: a voting machine, note: feeds ballots as a machine would}
actors:
  operator: {runs: tests, count: 1}
scenarios:
  smoke: {stories: [ballots-decide-winner], actors: {operator: 1}}
gaps:
  real-machine:
    what: Ballots read from a real voting machine
    why: no machine is reachable from the test system
    touches: ["[[accept-ballot]]"]
---

## The test system

How the parts fit together, in prose, and a diagram when one helps.
```

- **Components** are the parts of the test system. Each has one of: `release` (`<system>/<deliverable>`, a product you pinned), `image` (a container image), `fixture: true` (data you make), or `simulates` (the hardware it stands in for). A release or an image that stands in for an external service says so in `substitutes`. Name every part a test depends on, so that a reader sees what is real and what stands in.
- **Actors** drive the test system: `runs` is a component, or `tests` for your own suite; `count` says how many run at once.
- **Scenarios** run stories with set actor counts: a smoke run with one of each, a load run with many.
- **Gaps** are what the test system cannot test, and why: hardware that is not there, a cloud service you cannot reach, timing that a loaded host does not show. Each gap touches the stories, rules, or features it leaves untested. A gap counts only when the user approves it in the UI; until then, the story is untested. Never call a hard test a gap to close it: say what it would take.

hikari reads the plan and warns about a release you did not pin, a story or link that does not exist, and a key story with neither a test nor a gap. A plan that does not parse blocks your runs.

## The release set

`pin_releases` takes every release of the set and writes `releases.lock` with each release's digest. It replaces the lock. The lock is the code of every story your tests link: a new pin makes every E2E result pending until you run the tests again. hikari tells you when a release newer than your pin appears.

## Tests that link stories

In `tests.yaml`, a key links the stories, rules, and principles its tests check, as it would link features:

```yaml
version: 1
suites:
  e2e: {command: python3 -m pytest -q tests -o junit_family=xunit1 --junitxml=/hikari/private/reports/e2e.xml}
tests:
  tests/test_vote.py: ["[[ballots-decide-winner]]", "[[ballots-decide-validity]]"]
```

A story's health, or a rule's or a principle's, comes from the tests that link it: `failing` when one failed, `pending` when one has not run since the lock or the test changed, `untested` when none links it, and otherwise `passing`.

## Environments

When a product is a service, it runs beside your tests in an environment: the services of `environment.yaml` at the root of your scope, a compose file that hikari starts on the host.

```yaml
services:
  cloud:
    x-hikari: {release: cloud/http-service}    # an image release: hikari built its image
    environment: {DATABASE_URL: "postgres://app:app@db/app"}
    healthcheck: {test: [CMD, wget, -qO-, http://localhost:8000/health], interval: 2s, retries: 30}
    depends_on: {db: {condition: service_healthy}}
  edge:
    image: debian:bookworm-slim
    x-hikari:
      mount:
        edge/tarball: /opt/edge                # a pinned release's product, read-only
        scope:fixtures/readings: /data         # a directory of your scope, read-only
    command: [/opt/edge/bin/edge, --cloud, http://cloud:8000]
  db:
    image: postgres:16
```

- Each service is a component of the plan, and each component that runs is a service. A release that is not an image runs from a mount into an image you choose.
- hikari sets the networks; a service sets no `networks`, `ports`, `build`, bind mount, privilege, or variable of the host. Write every value: no credentials reach an environment.
- Give each service a health check, so that a start waits until it is ready.
- An actor that `runs` a service sets its replicas in a scenario: five edges are five replicas.

`start_environment(scenario)` starts the services and waits until they are healthy, then connects your container to their network: reach each service by its name, such as `http://cloud:8000`. Each service's log is in `/hikari/private/environment/logs/<service>.log`; read it when a test fails. `stop_environment` removes everything, so the next start is fresh; hikari also stops the environment when the scenario's minutes end. A service whose platform differs from the host runs under emulation: name timing claims as gaps.

## Start

1. Read the hub wiki: the stories, the rules, the principles, and the user's plans and notes. Note the key stories: they matter most to the user.
2. Read the releases you can read: what each product is, how to install or run it, and what its interfaces promise.
3. Write `plan.md`: the components, the actors, a smoke scenario for the key stories, and the gaps. Write the body as a short description of the test system.
4. Pin the releases the plan uses with `pin_releases`.
5. Describe the scaffold with `set_system_info`, write `layout.yaml`, and call `mark_map_current`.
6. Post a summary of the plan in the hub and ask the user to review it and the gaps.

## Test

1. Write tests that walk each story from its actor's first step to its outcome, against the pinned products, and tests that check each rule in the stories that link it. Install or run a library or a binary from its release directory; start the services in an environment. Never copy code from anywhere else.
2. Link each test file or test to the stories and rules it checks in `tests.yaml`.
3. Run each suite with its command, and call `record_test_run` for it.
4. `get_test_plan` shows the health of each story and what the plan still lacks.

## Report

When a story or a rule fails, tell the orchestrator in the hub: the story, the test, the step that failed, what you expected, and what happened, with the suspects that `get_test_plan` names. Describe behavior; the owners cannot read your tests. Do not change a test to make it pass unless the story changed.

## Keep true

- After a new release: pin it, run the tests, and update the plan.
- After a changed story or rule: change the tests that check it, and the plan.
- After a changed gap: the user must approve it again.
