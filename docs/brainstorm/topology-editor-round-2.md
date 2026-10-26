# Topology editor: round 2

Source: your answers to B2 and B3 (`topology-editor-answers.md`). The questions of round 1 that you did not answer are still open; "the rest of round 1 stands" is enough for them.

## I. Ids and Names

**I1. The id is the YAML key, and the name defaults to it.** An id could be generated, such as `svc-3f9a`, but then every compose file would read as a list of codes, and the YAML view is meant to teach the file. So the key that you write today is the id: the service key of an agent or sidecar, the key of a hub or a repo under `x-hikari`, and the compose `name` of the project. A new field, `name`, holds the display name, and it is absent until the first rename:

```yaml
x-hikari:
  hubs:
    eng: {name: platform, orchestrator: eng-orchestrator, members: [dev]}

services:
  orders:
    x-hikari: {name: intake, description: Owns order intake.}
```

Every compose file of today stays valid with no change, because each name equals its id. The cost is that, after a rename, the file shows the old word as the key. The Structure view shows the id under each name, in muted text, so the two are never a surprise. Do you agree?

**I2. Agents use names only; ids stay out of their messages.** `@intake`, `#platform`, `send_direct_message(to: intake)`, and the system prompt all use names. The server maps a name to an id when a message arrives. The paths inside a container are still keyed by id, such as `/hikari/public/readonly/hubs/eng/wiki`, so the prompt names each hub's path beside its name, as it does today. Do you agree?

**I3. Names are unique among agents, hubs, and `user`, and so are ids, and a name may not equal another entity's id.** Mentions and the `to:` of a direct message must name one entity. A name that is another entity's id would make a log line from before a rename ambiguous. Repos have their own namespace, as today. Do you agree?

**I4. An id can change only while nothing is keyed by it.** A new agent or hub that has never started has no store directory, no releases, and no container, so its id is free to change. The editor offers "Change id" until the first start that prepared its store, and after that only a rename of its name. Do you agree?

**I5. Propagation rewrites marked references only, and you choose each one.** When you rename, the plan asks whether to propagate, and lists each place the old name appears: `@orders` and `#eng` in the bodies of the wiki documents and in the vocabulary descriptions. A system map's summary is the owner's to write, so the plan lists it without a checkbox, and the owner is told of the rename at its next start. It rewrites only these marked forms, never the bare word: a hub called `main` would otherwise rewrite every "main" in the prose. Each occurrence has a checkbox, all checked by default. Releases are never rewritten: they are immutable, and they record the id, so they need nothing. Wiki edits are committed as you, as the wikis spec says for hand edits; propagation changes only body text, never a header. Do you agree?

**I6. The project's name follows the same rule.** The compose `name` is the project's id: it prefixes every container, network, and named volume, so changing it would orphan the volumes. A rename sets `x-hikari.name`, which the UI and the terminal show. Do you agree?

## R. `hikari up` Without a Topology

**R1. Which file `up` opens.** With an argument, that file. Without one, `compose.yaml` in the current directory. When the file does not exist, the editor offers to create it at that path. Do you agree?

**R2. What "create" writes.** When the directory is empty, the UI runs what `hikari init` runs: a project repository with `compose.yaml`, the store, `.env.example`, `.gitignore`, the host skills, and a first commit. When the directory is not empty, for example a repo of your own, it writes only `compose.yaml` from the same starter, without `x-hikari.project`, and commits nothing. `hikari init` stays for scripts. Do you agree?

**R3. A failed start returns to the editor, and you start again from the UI.** Today `up` exits when the file is invalid, a repo has uncommitted changes, a clone is missing, or the project's containers already run. Instead, each of these returns `up` to the editing state with the findings shown, and a Start button that is enabled when lint finds no error. `up` never starts by itself when a file becomes valid: a save while you type should not start containers. Ctrl-C still stops everything and exits. Do you agree?

**R4. Stop and Start from the UI are built in this round.** Stop does what Ctrl-C does today without exiting: it stops the containers, commits what the owners left, and returns to editing. Start runs the start sequence again, as a new run with its own folder in `.hikari/runs/`. The UI's message list then starts empty, as the terminal's does. The URL and its token stay valid for the whole process. This replaces B5. Do you agree?
