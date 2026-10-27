# Spec: set the E2E work aside

Status: approved.

Sources: `docs/brainstorm/shared-language.md` (questions A1 to A5) and `shared-language-answers.md`. This is step 1 of the seven steps of the shared language.

## Summary

The tester, its plan, gaps, approvals, and release lock, its environments, bench procedures and results, and image releases leave the code. The tag `archive/hubs-and-e2e` (at `3524f6d`) keeps all of it. The health of a feature comes from unit tests alone again: a result counts while the test's file and the code of the features it touches are unchanged. Hubs stay until step 2, and this step uses today's words.

## Decisions

From the round and its answers:

1. The code is deleted on `main`; the tag keeps it, and no branch does (A1).
2. The specs and brainstorms of the removed features move to `docs/archive/` (A2).
3. Image releases leave with environments (A4).
4. A suite that needs the whole application stays in its repository, outside every test command, and hikify records it as left for integration (A5).

## Decisions made in this spec

1. **An old key fails with the decoder's own error.** The topology and `deliverable.yaml` are decoded strictly, so `tests:`, `e2e:`, and `image:` already fail with `field tests not found`. A special message would be code that names a removed feature forever.
2. **The `story` kind stays in this step.** Step 2 moves stories into the orchestration feature map. Here a story's health is its support, the health of the features it links, as for a rule or a principle of a hub.
3. **`plan.md`, `releases.lock`, and `environment.yaml` in a scope are no longer special.** They are ordinary files of the scope.

## Requirements

### Code that leaves whole

- **RE1.** `internal/e2e`, `internal/environments`, and `internal/images` are deleted, with their tests.
- **RE2.** `internal/systems/tester.go`, `environment.go`, `tester_test.go`, and `image_test.go` are deleted. No test of image releases remains, because image releases are gone.
- **RE3.** The `hikari-tester` skill and `web/src/components/hub-tests.tsx` are deleted.
- **RE4.** `examples/sensors` and the `example-sensors` Makefile target are deleted. The comment "Remove containers and networks left by runs..." sits above `clean`, where it belongs.

### Topology and editor

- **RE5.** `Agent.Tests`, `Agent.E2E`, `E2ELimits`, rules TS1 to TS4 and ST7, `TestedSystems`, and `Tester` are gone. `ReadableReleases` is the visible systems and the release grants, without the agent itself.
- **RE6.** The editor has no `set_e2e` operation and no `tests` field in `set_agent`. `remove_member`, `remove_hub`, and a hub's `change_id` no longer mention a tester. The model has no `tests` or `e2e` field. `docs/specs/topology-api.md` drops them.

### Health

- **RE7.** A test result stores the hash of the code of the features it touches, `sysmap.ContentHash`, as before the tester. `codeHash`, `lockHash`, `storyIDs`, `testerDocs`, `testerProblems`, `missingReleases`, `unrestoredPins`, `newerReleases`, and any helper left unused are gone.
- **RE8.** `sysmap.Validate` and `PrivateErrors` take no extra ids. The private `.gitignore` lists `reports/` and `results.yaml` only, and Hikari rewrites an older one that lists `environment/`. The X3 message of the test map does not mention a tester.
- **RE9.** `TurnContext` has no `NewerReleases`, `MissingReleases`, or `UnrestoredPins`. `NoLocalResults` and its notice stay.
- **RE10.** A hub wiki's view has no `Tests`. A story's, a rule's, and a principle's health is its support. A rename of a hub document no longer follows into a tester's test map (TT2).

### Deliverables

- **RE11.** `ImageSpec`, the `image:` section of `deliverable.yaml`, `Manifest.Image`, the IM rules, and `BuildImage` are gone. A manifest with an `image:` key still loads; the key is ignored.

### Tools, routes, and prompts

- **RE12.** The MCP tools `pin_releases`, `start_environment`, `stop_environment`, `environment_status`, and `get_test_plan` are gone, with their client methods, the routes `/api/ui/e2e/*` and `/api/node/e2e/*`, and the wire types `TestPlan`, `GapView`, `BenchView`, `BenchResultView`, and `StoryHealth`. `Self.Tests`, `Self.Testers`, `TesterSkill`, and `doLong` (if nothing else uses it) are gone.
- **RE13.** The prompts have no tester text: no `testerPrompt`, no `e2ePlan`, no pin or missing-release restore steps, no newer-release notice, and no tester line in the orchestrator's prompt. A restore turn still runs when `NoLocalResults` is set.
- **RE14.** `cmd/hikari` no longer calls `SetRun` or `StopEnvironments`.

### UI

- **RE15.** The UI has no Tests tab, no `tests` route, no e2e types or request helpers, no tester or e2e fields in the Structure tab, and no tester role in the hub diagram. The fixtures in `web/src/lib/fixtures/` are written again by the Go tests.

### Skills of the Hikari agent

- **RE16.** `hikify` and `containerize-repo` never move a suite. A suite that needs a display, a browser, the whole application, or another system is recorded in `onboarding/` as left for integration, and stays in its repository outside every test command. Their topology references have no tester. `hikari-system` has no image-release note, and `hikari-hub` does not mention `get_test_plan`.

### Docs

- **RE17.** `docs/specs/tester.md`, `environments.md`, and `bench.md`, and `docs/brainstorm/e2e.md` and `e2e-answers.md`, move to `docs/archive/` with `git mv`. `docs/archive/README.md` says that these describe removed features and names the tag.
- **RE18.** `README.md`, `docs/design.md`, and `docs/guides/onboarding.md` describe no tester, environment, bench result, or image release. The principle that each system tests only its own code stays, and says that a suite of the whole application is left for integration, which is planted.
- **RE19.** Each spec that still mentions the tester as built (`entity-ids.md`, `linked-repos.md`, `hikify-design.md`, `hikify-execution.md`, `project-repo.md`, `wikis.md`, `structure-view.md`, `shared-store.md`, `deliverables.md`) gets one line under its status: "The tester, environments, and image releases were removed in `remove-e2e.md`."

### Checks

- **RE20.** `go vet ./...`, `go test -race ./...`, and `npm test` in `web/` pass, and `npm run build` succeeds.
- **RE21.** Outside `docs/archive/`, `docs/brainstorm/`, and the status lines of RE19, no file in `internal/`, `cmd/`, `web/src/`, or `examples/` contains `tester`, `e2e`, `bench`, `releases.lock`, or `start_environment`, except where `environment` is compose's key and where `e2e` names a test fixture's directory.
- **RE22.** A live check: `hikari check examples/shop/compose.yaml` passes, and `hikari up -no-launch` on a copy of the shop serves the UI, where a hub shows no Tests tab.
