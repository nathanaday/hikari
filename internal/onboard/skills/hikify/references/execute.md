# Restructure, execute, verify

The plan is approved. You now carry it out: you do the steps whose actor is `session`, hikari's agents do the rest, and you check the result.

The restructure commits go on the hikari branch of each repository, the branch that is checked out, with the user's identity. That branch is the one that the agents work on and that the user reviews and merges into their own. Do not make another branch for the restructure.

## Stage 5: Restructure

Set the stage to `hikify: restructure`. hikari must be stopped: ask the user to stop it if `hikari status` or the user says that it runs, and confirm with the user.

Do the steps whose actor is `session`, in the order of the plan, up to the first step of another actor. After each step, check its "Done when". Record the result in `state.md`, and commit. When a check fails, stop and tell the user what failed.

- **A new repository.** Make it in a temporary folder outside `repos/`, from the moved code with its history, as the step says. Add a README and an ignore file. Do not move the folder into `repos/`. Give the user the commands that create the remote and push `main` to it. Run them only when the user approves, in the conversation. Then run `hikari sync compose.yaml`, which clones it into `repos/` with the right origin, once `compose.yaml` names it.
- **Drafts.** Copy each draft of `onboarding/wiki/hubs/<hub>/` to `store/hubs/<hub>/wiki/`, each draft of `onboarding/wiki/systems/<system>/` to `store/systems/<system>/wiki/`, and each deliverable of `onboarding/shared/deliverables/` to `store/shared/deliverables/`.
- **Features that change systems.** An id is unique in the whole store, so move the document itself, from the old system's wiki to the new system's, as the step says. Remove its entries from the old owner's `layout.yaml` and `tests.yaml`, in `<scope>/.hikari/private/` of the old repository.
- **The topology.** Copy `compose.target.yaml` over `compose.yaml`, and remove `compose.target.yaml`.

When the restructure steps are done, commit in each repository that you changed, and in the project. Run `hikari check compose.yaml`. It must pass, and `git status` must be clean in the project and in each clone.

## Stage 6: Execute

Set the stage to `hikify: execute`, and commit. Ask the user to start hikari in their own terminal:

```sh
hikari up compose.yaml
```

When the agents have connected and finished their first turns (`hikari status` shows each new system described), ask the user to post this in the hub of the plan, with the hub's orchestrator and the plan's first step that is not yours:

```
@<orchestrator> Please carry out the plan [[hikify-plan]], from step S<n>. Follow the Carry out a plan procedure of the hikari-hub skill.
```

The orchestrator then sends each step to its actor when the step's needs are met, and posts the state of the steps in the hub. Follow the run from files. Do not talk to an agent.

- `hikari status compose.yaml` shows each system's map and recorded tests.
- `.hikari/runs/<newest>/messages.jsonl` holds every message of the run, one JSON object to a line: `from`, then `hub` for a post or `to` for a direct message, then `body` and `time`. It appears with the first message. Read the newest lines to see the orchestrator's posts.
- `git -C repos/<name> log hikari` shows each owner's commits.

Check every few minutes; a step takes an owner many minutes. When the orchestrator posts that a step waits for the user or for you, do it, or tell the user what to do. When an owner reports that a step cannot be done as written, propose a change to the plan, and ask the user. With approval, change the plan documents in the store, as the user's edit, and commit.

The stage ends when the orchestrator reports every step done. Check each "Done when" that a file can show.

## Stage 7: Verify

Set the stage to `hikify: verify`.

1. Run `hikari status -tests compose.yaml`. For each row of `onboarding/verification.md`, write the result: the test's outcome in its planned place, or `missing`.
2. Ask the user to approve each row that is `dropped`.
3. Check that every owner's suite passes, and that the support of each key story is passing. Ask the user to read it on the hub's Wiki tab.
4. Run `hikari check compose.yaml`.
5. **The dry run.** It proves that a teammate gets the same project from one clone.
   1. Ask the user to stop hikari.
   2. Push the hikari branches with `hikari push compose.yaml`, once the user approves.
   3. Clone the project into a temporary folder: from its origin, or else from its own folder.
   4. In the clone, run `hikari sync compose.yaml` and `hikari check compose.yaml`. Copy the project's `.env` into the clone.
   5. Ask the user to run `hikari up compose.yaml` in the clone until every agent connects, and then to stop it. Or, with approval, run it in the background yourself, and stop it with an interrupt when the log shows `Connected`.
   6. Remove the temporary folder.

   A failure is a finding of the verification. Fix it in the project, and run the dry run again.

When every check holds, set the stage to `hikify: done`, with the date, and commit. Sum up for the user: the systems and their releases, the suites left for integration, the stories with their support, and the hikari branches that wait for the user's merge.
