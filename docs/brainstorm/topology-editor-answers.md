# Topology editor: answers

Answers to `topology-editor.md` and `topology-editor-round-2.md`, 2026-10-25.

| Question | Answer |
|---|---|
| B2 | No new command. "Use `hikari up` without a topology, or pointed at the malformed topology, and it still serves the web UI", so that you can create a new topology in the directory where you ran `hikari up`, or fix the broken one in the UI. |
| B3 | "Use an internal unique slug or id to specify entities rather than rely on their name, so renames can just be a simple change to a display-name parameter. The UI should ask the user if they also want to propagate the name through the knowledge base, releases, etc." |
| Round 1, the rest | "I agree with round 1." Every other recommendation stands, so F2 is the list and inspector, not the Map. B3, B5, and D2 are replaced by the answers above and by round 2. |
| Round 2, I1 to R4 | "I agree with round 2." Every recommendation stands. |

## What the answers set

**`hikari up` serves the UI before it has a topology.** It listens and prints the URL first. With a valid file, it starts as it does today. With no file, a missing one, or one that fails its checks, it stays up with the editor open instead of exiting. The server can no longer be built from one topology at start, so `up` becomes a loop of two states: editing, with only the editor and the store views, and running, with everything. Questions R1 to R4 of round 2 settle the details.

**Every entity has an id that never changes, and a name that a rename changes.** The id keys everything that lives outside the file: store directories, the vocabulary file, release manifests, `releases.lock`, containers, and volumes. The name is what people and agents use: `@name`, `#hub`, the sidebar. A rename changes one field, needs no store move, and so is no longer refused during a run (B3 falls away). An owner with releases can be renamed, because its releases record the id (D2 falls away). Questions I1 to I6 settle the details.

B5 changes too: once `up` is a loop, "Stop" and "Start" from the UI are its two transitions, so a restart with new changes costs little, and R4 builds it in this work.

## Steps

The answers add two pieces of work before the editor of H1: ids, and a `hikari up` that serves the UI without a topology. Four steps, each with its own spec, built in order:

1. Ids and names: `docs/specs/entity-ids.md`. A hand-written `name` works everywhere, with no editor yet.
2. The editor API and the `hikari up` loop: findings with paths, the operations, the endpoints, propagation, Start and Stop.
3. The YAML view.
4. The Structure view.

Each spec is written when the step before it is built, so that it starts from what the build learned.
