# Planted for the next phase

Items planted during the feature wiki round 1 answers, 2026-10-22, in the user's words
with short notes. Each gets its own round before it is built. `phase-2.md` holds the
earlier plants: work orders, change logs, and the validation agent.

## The hikari files on the host

> Produce a clean, helpful document describing the hikari files as they exist on the
> host machine, and where they are bind mounted to a container's file system by path.

Notes: the facts are spread over the README, `docs/specs/shared-store.md` (the mount
table), and `docs/specs/system-maps.md` (section 3). One document, with the host path
of every file for a worked example such as the shop, and the container path it mounts
to, for an owner, an orchestrator, and a plain agent. Do it when the feature wiki has
decided where the wiki lives.

## Findings

> Findings are also docs with their own type; one finding doc may touch many features,
> so it's better to compute what findings are associated with a feature.

Notes: a finding document links the features it touches with the same `[[id]]` links
as the wiki. The findings of a feature are computed from those links. The feature
document never lists findings.

## Plans and work

> Plans are their own doc control, we have not gotten into the concept of planning and
> work orders yet, but it's the same idea: we would carefully build up these entities,
> they link to features they touch, then you can compute the relationship.

Notes: joins the work orders of `phase-2.md`. A plan links features; the plans of a
feature are computed.

## Testing and health

> The testing system has to live inside the container, so it needs its own design
> considerations. The test and health system is responsible for taking a feature map
> and tracking the health of each feature.
>
> - Each unit test will have its own entity that links to the features it touches (1 or
>   many).
> - For a feature to be healthy, it must have passing tests for all unit tests that
>   touch it.
> - When unit tests are run, the container agent publishes the unit test manifest,
>   score, and which features it touches; the server side can compute this against the
>   feature map to come up with the health and passing state of every feature.
> - A Hikari skill explains how this unit test map is constructed.
> - The test map is internal to the container agent as it is heavily defined by code
>   paths, functions, classes, which outside members are not exposed to.
> - As a future note to plant: docstrings on the unit test in a hikari format can be
>   automatically parsed and converted to the markdown set.

Notes: built, as `docs/specs/test-health.md`, from `test-health.md` and its answers.
Docstrings parsed into the test map stay planted, and so does hikari running the suites
itself, with the validation agent of `phase-2.md`.

## Working with remote repositories

> We need to do some more working with remote repos and it deserves its own design
> cycle.

Notes: planted from the system graphs round, question D5. The git lens of the topology
map was to show each repository's commits ahead of and behind its remote. That needs
answers first: whether hikari fetches, with whose credentials, which upstream the hikari
branch compares with before it is pushed, and whether hikari pushes.

## Secrets

> Yes env is the structure for now, an established secret system is held out for future work.

Notes: planted from question E7 of `shared-language.md`. The Phase 2 canvas draws secrets as a private file of a partition. Until that design, a partition's secrets are compose's `env_file` and `environment` on its service.
