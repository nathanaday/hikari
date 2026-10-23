# System graphs: round 1 answers

Answers to `system-graphs.md`, 2026-10-22.

| Question | Answer |
|---|---|
| A1 to H1 | Agree with every recommendation, with the two clarifications below. |
| D2 | "Waiting just means idle." |
| D5 | Hold off on ahead and behind against the remote: "we need to do some more working with remote repos and it deserves its own design cycle." |

## What the answers change

**D1 and D2.** An idle agent is waiting, so `idle` is muted cyan. Gray means the agent is not running: `starting` and `exited`. The server needs no signal for queued messages, so G1 loses that item.

**D5.** The git lens keeps D4: magenta for uncommitted changes in a scope, white with a gray outline for a clean one. The repository node gets no ahead or behind badge, and G1 loses the remote counts. Remote tracking is planted in `next-phase.md`.
