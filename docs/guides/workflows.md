# Agent workflows: policies, styles, skills, and profiles

This guide is about the second half of a hikari project. The topology says what each agent sees. The workflow says how each agent works: what it must always do, how its work should look, how it does a kind of task, and which of those a given agent takes. You set all of it in files, not in prompts you type, and the agents read it on their next turn.

## The four pieces

| Piece | Answers | Lives in | Reaches an agent by |
|---|---|---|---|
| Policy | What must every agent do? | `vault/public/policies/<name>.md` | being there: every system prompt gets every policy |
| Style | How should this agent's work look? | `vault/styles/<name>.md` | `grants.styles` on the agent, or a profile; into its system prompt under `# Styles` |
| Skill | How is this kind of task done? | `vault/skills/<name>/SKILL.md` | `grants.skills` on the agent, or a profile; a Claude Code skill it loads when the task fits |
| Profile | Which of these does this agent take? | `x-hikari.profiles` in `topology.yaml` | `profile: <name>` on the agent |

A policy and a style are text in the prompt, so they are short and always apply. A skill is a package that loads when its description matches the task, so it can be long and specific. When a style grows into a procedure, make it a skill.

A profile is a named bundle: a model, an effort, whether the agent's work is reviewed, and the skills and styles it is granted. Several agents can take the same profile. What an agent sets itself wins over its profile, and its own skills and styles join the profile's.

```yaml
x-hikari:
  profiles:
    careful:
      description: Strong model, high effort, reviewed; terse and conventional.
      model: opus
      effort: high
      review: required
      skills: [tdd, refactor-safely]
      styles: [terse, conventional-commits]
    quick: {model: haiku, effort: medium, styles: [terse]}

services:
  billing:
    x-hikari:
      description: Invoices and payments.
      profile: careful
      grants: {skills: [debug-a-failure]}     # joins the profile's skills
      partition: {folders: [{repo: backend, path: services/billing}]}
  docs:
    x-hikari: {description: The handbook., profile: quick, partition: {folders: [{repo: handbook}]}}
```

Here `billing` runs on `opus` at high effort, needs a review before it asks for done, loads `tdd`, `refactor-safely`, and `debug-a-failure` when they fit, and follows `terse` and `conventional-commits`. `docs` runs on `haiku` and follows `terse`. The Structure tab of the web UI edits all of this per agent, and shows what each agent takes from its profile.

`effort` is one of `low`, `medium`, `high`, `xhigh`, and `max`, and `review` is `required` or `none`. hikari passes the effort to Claude Code with each turn.

## Reviews

`review: required` on an agent or its profile makes a review part of done. Before a partition agent asks for done on its part of a scope, it loads the built-in `hikari-review-work` skill: it reads its whole change with `get_changes`, the diff of its folders from the merge base with the default branch, and sends the `hikari-reviewer` subagent with the diff, the part of the scope, the policies in `/hikari/public/policies/`, the styles in `/hikari/styles/`, and the feature documents the change touches. The reviewer is a fresh context that did not write the change. It returns findings as `blocker`, `should`, or `nit`, each with a place and the rule it cites, which is how your policies and styles become checks and not only text. The agent fixes the blockers, runs its suites again, and passes a one-line verdict as `review` to `set_task_status`. hikari refuses done without it, and the verdict is a line of the task's log.

A review is a reading, not a test run and not approval: tests are `hikari-validate-work`, health is hikari's, and approval is yours when you merge. An agent whose review is not required can still load the skill when a change is large; the policies you install from the toolbox say when.

## The toolbox

hikari ships ready-made policies, styles, skills, and profile recipes. List them, read one, and install one:

```sh
hikari toolbox list
hikari toolbox show style/terse
hikari toolbox add policy/no-workarounds style/terse skill/tdd
hikari toolbox add profile/careful      # printed, to paste under x-hikari.profiles
```

An installed file is yours: change it, and commit it with the project. `hikari toolbox add` refuses to replace a file the vault already has unless you pass `-force`, so a change you made is never lost by accident. The Shared view of the web UI has the same list with an Add button on each item.

What is in it:

- **Policies**: `no-workarounds`, `tests-are-evidence`, `ask-when-it-costs`, `small-steps`, `say-what-you-did`, `no-secrets`. A good first set for any project is all six.
- **Styles**: `terse`, `conventional-commits`, `plain-docs`, and one per language: `python-idiomatic`, `go-idiomatic`, `typescript-strict`.
- **Skills**: `tdd`, `refactor-safely`, `debug-a-failure`, `record-a-decision`. Each is written for a hikari partition agent: it names the test map, `record_test_run`, `apply`, and `change_wiki` where they come in.
- **Profiles**: `careful`, `quick`, `library-maintainer`.

## Writing your own

A policy or a style is one Markdown file with no header: a few sentences that an agent can follow and that a reviewer can check. Say what to do, not what to value. "A commit subject is imperative and under 72 characters" is a style; "write good commits" is not.

A skill is a directory with `SKILL.md`, whose frontmatter has a `name` that matches the directory and a `description` that says when to load it. Claude Code shows the agent the description and loads the body when it fits, so the description does the work. The body is a procedure: numbered steps, each with what to run and what to check. Put long references in files beside `SKILL.md` and link them. The names `hikari-*` are hikari's own, so pick another prefix.

A profile is YAML under `x-hikari.profiles`. Name it for the kind of work, not the agent: `careful`, `frontend`, `library-maintainer`.

## How they reach the container

The driver rebuilds the agent's context before every turn, so a change of a policy, a style, or a skill applies from the next turn, without a restart. Every container mounts `vault/public/` read-only at `/hikari/public/`, so the policies are there. A granted style is mounted at `/hikari/styles/<name>.md` and a granted skill at `/hikari/skills/<name>/`, both read-only, and the driver links each skill into `~/.claude/skills/`. A profile is resolved when hikari reads the topology, so a change of a profile needs `hikari up` again. The agent's inspector in the web UI shows the mounts and the resolved skills and styles.

## Where things are

| Path | What it holds |
|---|---|
| `vault/public/policies/<name>.md` | a policy, for every agent |
| `vault/styles/<name>.md` | a style, granted |
| `vault/skills/<name>/SKILL.md` | a skill, granted |
| `topology.yaml`, `x-hikari.profiles` | the profiles |
| `/hikari/public/policies/`, `/hikari/styles/`, `/hikari/skills/` | the same, in a container |
