# Topology editor: round 1 answers

Answers to `topology-editor.md`, 2026-10-25. Questions not listed here are still open.

| Question | Answer |
|---|---|
| B2 | No new command. "Use `hikari up` without a topology, or pointed at the malformed topology, and it still serves the web UI", so that you can create a new topology in the directory where you ran `hikari up`, or fix the broken one in the UI. |
| B3 | "Use an internal unique slug or id to specify entities rather than rely on their name, so renames can just be a simple change to a display-name parameter. The UI should ask the user if they also want to propagate the name through the knowledge base, releases, etc." |

## What the answers set

**`hikari up` serves the UI before it has a topology.** It listens and prints the URL first. With a valid file, it starts as it does today. With no file, a missing one, or one that fails its checks, it stays up with the editor open instead of exiting. The server can no longer be built from one topology at start, so `up` becomes a loop of two states: editing, with only the editor and the store views, and running, with everything. Questions R1 to R4 of round 2 settle the details.

**Every entity has an id that never changes, and a name that a rename changes.** The id keys everything that lives outside the file: store directories, the vocabulary file, release manifests, `releases.lock`, containers, and volumes. The name is what people and agents use: `@name`, `#hub`, the sidebar. A rename changes one field, needs no store move, and so is no longer refused during a run (B3 falls away). An owner with releases can be renamed, because its releases record the id (D2 falls away). Questions I1 to I6 settle the details.

B5 changes too: once `up` is a loop, "Stop" and "Start" from the UI are its two transitions, so a restart with new changes costs little. R4 asks whether to build it now.
