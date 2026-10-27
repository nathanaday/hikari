# Restructure, execute, verify

The plan is approved. You now carry it out: you do the steps whose actor is `session`, hikari's agents do the rest, and you check the result.

The restructure commits go on the hikari branch of each repository, the branch that is checked out, with the user's identity. That branch is the one that the agents work on and that the user reviews and merges into their own. Do not make another branch for the restructure.

## Stage 5: Restructure

Set the stage to `hikify: restructure`. hikari must be stopped: ask the user to stop it if `hikari status` or the user says that it runs, and confirm with the user.

Do the steps whose actor is `session`, in the order of the plan, up to the first step of another actor. After each step, check its "Done when". Record the result in `state.md`, and commit. When a check fails, stop and tell the user what failed.

- **A new repository.** Make it in a temporary folder outside `repos/`, from the moved code with its history, as the step says. Add a README and an ignore file. Do not move the folder into `repos/`. Give the user the commands that create the remote and push `main` to it. Run them only when the user approves, in the conversation. Then run `hikari sync topology.yaml`, which clones it into `repos/` with the right origin, once `topology.yaml` names it.
- **Drafts.** `onboarding/vault/` mirrors the vault, so copy the tree into it as it is, and check that the copy changed no file that the vault already had:

  ```sh
  cp -R onboarding/vault/. vault/
  git status --short vault/
  ```

  `git status` lists only new files: the public wiki's rules, principles, and plan in `vault/public/wiki/`, the deliverables in `vault/public/deliverables/`, the orchestration features in `vault/orchestration/read-only/features/`, and the partition plans in `vault/partitions/<id>/read-only/features/`. A line that starts with `M` is a draft that replaced a file of the vault: restore the file with `git checkout -- <path>`, and tell the user.
- **Features that change partitions.** An id is unique in the whole vault, so move the document itself, from the old partition's feature map, `vault/partitions/<old id>/read-only/features/`, to the new partition's, as the step says. Remove its entries from the old partition agent's `code-map.yaml` and `tests.yaml`, in `.hikari/private/` of the old partition's first writable folder. Their paths are workspace paths; write them as the new partition's workspace places the code.
- **The topology.** Copy `topology.target.yaml` over `topology.yaml`, and remove `topology.target.yaml`.

When the restructure steps are done, commit in each repository that you changed, and in the project. Run `hikari check topology.yaml`. It must pass, and `git status` must be clean in the project and in each clone.

## Stage 6: Execute

Set the stage to `hikify: execute`, and commit. Ask the user to start hikari in their own terminal:

```sh
hikari up topology.yaml
```

When the agents have connected and finished their first turns (`hikari status` shows each new partition described), ask the user to post this in the room, with the orchestration agent's name and the plan's first step that is not yours:

```
@<orchestration agent> Please carry out the plan [[hikify-plan]], from step S<n>. Follow the Carry out a plan procedure of the hikari-plan-work skill.
```

The orchestration agent then sends each step to its actor when the step's needs are met, and posts the state of the steps in the room. Follow the run from files. Do not talk to an agent.

- `hikari status topology.yaml` shows each partition's map and recorded tests.
- `.hikari/runs/<newest>/messages.jsonl` holds every message of the run, one JSON object to a line: `from`, then `to` for a direct message (a post to the room has none), then `body` and `time`. It appears with the first message. Read the newest lines to see the orchestration agent's posts.
- `git -C repos/<name> log hikari` shows each partition agent's commits.

Check every few minutes; a step takes a partition agent many minutes. When the orchestration agent posts that a step waits for the user or for you, do it, or tell the user what to do. When a partition agent reports that a step cannot be done as written, propose a change to the plan, and ask the user. With approval, change the plan documents in the vault, as the user's edit, and commit.

The stage ends when the orchestration agent reports every step done. Check each "Done when" that a file can show.

## Stage 7: Verify

Set the stage to `hikify: verify`.

1. Run `hikari status -tests topology.yaml`. For each row of `onboarding/verification.md`, write the result: the test's outcome in its planned place, or `missing`.
2. Ask the user to approve each row that is `dropped`.
3. Check that every partition agent's suite passes, and that the health of each orchestration feature is passing. Ask the user to read it on the orchestration agent's Feature map tab.
4. Run `hikari check topology.yaml`.
5. **The dry run.** It proves that a teammate gets the same project from one clone.
   1. Ask the user to stop hikari.
   2. Push the hikari branches with `hikari push topology.yaml`, once the user approves.
   3. Clone the project into a temporary folder: from its origin, or else from its own folder.
   4. In the clone, run `hikari sync topology.yaml` and `hikari check topology.yaml`. Copy the project's `.env` into the clone.
   5. Ask the user to run `hikari up topology.yaml` in the clone until every agent connects, and then to stop it. Or, with approval, run it in the background yourself, and stop it with an interrupt when the log shows `Connected`.
   6. Remove the temporary folder.

   A failure is a finding of the verification. Fix it in the project, and run the dry run again.

When every check holds, set the stage to `hikify: done`, with the date, and commit. Sum up for the user: the partitions and their releases, the suites left for integration, the orchestration features with their health, and the hikari branches that wait for the user's merge.
