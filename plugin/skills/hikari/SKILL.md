---
name: hikari
description: Hikari's front door. Use for any request that names hikari or its agents and containers - take these repositories and build a hikari topology, bring my repos into hikari, start working with hikari containers, set up or install hikari, start hikari, where does my hikari project stand, what comes next - and for /hikari:hikari. It installs and checks hikari, finds where the user stands, and hands the request to containerize-repo, hikify, propose-topology, change-topology, or room.
---

# Hikari

You work for the user in their own Claude Code session. Hikari runs a team of Claude Code agents, each in its own Docker container. A partition agent works in one partition, a set of folders of the user's repositories, and sees no other code. The orchestration agent plans across the partitions from their feature maps, and sees no code at all. A project is a git repository whose `topology.yaml` names the partitions, the repositories they take their folders from, and the orchestration agent. This skill finds where the user stands and hands the request to the skill of that stage.

The user is the authority on the project. Propose, show, and ask. Never push, create a remote, or merge a branch without the user's approval in the conversation, for that one action.

## 1. Install and check

Tell the user in one line that the first run installs hikari for this plugin's version. Then run the plugin's own launcher, in the project root when there is a project:

```sh
"${CLAUDE_PLUGIN_ROOT}/bin/hikari" doctor
```

Fix each problem with the user, as `references/setup.md` says, and run doctor again. Go on when it reports no problem. A warning can wait, except the PATH line when the user will run `hikari up` in their own terminal.

## 2. Find where the user stands

**A project** is a folder whose `topology.yaml` has an `x-hikari` key: the working folder, a folder that the user names, or one folder down from the working folder. Its folder is the project root. Read its stage in `onboarding/state.md`, and the open topology changes in `changes/*/state.md`.

**Repositories** are the ones that the user names by URL or path, or else the git repositories in the working folder and one folder down. For each one, read where it comes from and whether it is clean:

```sh
git -C <path> remote get-url origin
git -C <path> branch --show-current
git -C <path> status --short
```

Show the user the list in a few lines, each with its origin, and ask which ones the project takes, unless the request says.

## 3. Route

| Where the user stands | Next |
|---|---|
| Repositories, and the user wants a sketch first, or asks what a topology would look like | `propose-topology` |
| Repositories, and the user wants them in hikari | make the project (section 4), then `containerize-repo` |
| A project with no partition agent yet, or a stage of `containerize` before `containerize: done` | `containerize-repo` |
| `containerize: done`, or a stage of `hikify` before `hikify: done` | `hikify` |
| An open change in `changes/` | `change-topology`, to continue it |
| An onboarded project (`hikify: done`, or partitions and no `onboarding/`), with a request to add a repository, split, merge, or move partitions, or extract a library | `change-topology` |
| An onboarded project, with a request for work, or to start hikari | `room` |

When the request spans several stages, such as "take these repos, build a hikari topology, and start working", start at the first, and tell the user the path ahead (section 5) before the first step that costs agent time. A user who wants to work on a containerized project as it is can go to `room` from `containerize: done`; say that hikify is where the partitions become cleaner than one for each repository.

Then load the next skill, and follow its procedure in this conversation.

## 4. Make the project

1. Agree a name with the user: lowercase letters, digits, and dashes. Propose one from the repositories or the folder.
2. The project goes in a new folder inside the working folder, `<working folder>/<name>`. When the working folder is inside a git repository, make the project beside that repository instead, and say so: a project inside another repository's tree would be part of that tree.
3. Run `hikari init <path>`. When git has no identity, it commits nothing and says how to commit; set the identity with the user first.
4. The project root is now `<path>`. Run `cd <path>` once, as a command of its own, so that later commands run there, and give file tools and subagents absolute paths. When the project lies outside the session's folders, give each command the root instead, as `git -C <path> …` and `hikari <command> <path>/topology.yaml`.
5. Ask the user to set a credential for the agents: copy `.env.example` to `.env` in the project, and set `CLAUDE_CODE_OAUTH_TOKEN` to a token from `claude setup-token`, run in their own terminal, or set `ANTHROPIC_API_KEY`. Never read, write, or print a credential yourself. `hikari doctor <path>/topology.yaml` says when one is set.
6. When `propose-topology` ran in this conversation, write its proposal to `onboarding/proposal.md`, and commit it.
7. Go to `containerize-repo` with the repositories: for each one, its origin URL, or its path when it has no origin, and one sentence on what it does, drawn from its README, which the user confirms.

## 5. The path ahead

Before the first step that costs agent time, tell the user in a few lines:

1. **Containerize.** Each repository becomes one partition, with no change to its code. hikari's agents then read each repository and run its tests, in containers that `hikari up` starts in the user's own terminal. This takes minutes, and agent time, for each repository.
2. **Design.** hikify proposes cleaner partitions, libraries, and the flows across them; fresh reviewers check the plan; the user approves it.
3. **Carry out.** This session restructures the repositories, and hikari's agents carry out the plan, each in its own partition.
4. **Verify.** Every test of the baseline is kept, moved, or dropped with the user's approval, and a fresh clone of the project starts.
5. **Work.** The user, or this session with `room`, posts requests to the orchestration agent.

Each stage records its progress in the project's `onboarding/state.md`, so the user can stop after any stage and continue in a new session.

## References

- `references/setup.md`: what to do about each line of `hikari doctor`.
