# Onboarding: round 1 answers

Answers to `onboarding.md`, 2026-10-23.

| Question | Answer |
|---|---|
| A1 to I3 | "Agree." Every recommendation stands. |

Where a question offered a choice, the recommendation is the answer:

- **A1.** The onboarding skills run in your Claude Code session on the host; hikari's agents make the code changes.
- **A2.** The binary carries the skills and writes them into the project's `.claude/skills/`, ignored by git.
- **B5.** Each teammate's agents commit to their own branch, set in `.env`.
- **G5.** The orchestrator tracks the plan in the hub.

## What the answers set

Four steps, each with its own spec, built in order (I1):

1. Linked repositories: `docs/specs/linked-repos.md`.
2. Containerize.
3. Hikify design: analysis, topology, and change plan.
4. Hikify execution and verification.

Each spec is written when the step before it is built, so that it starts from what the build learned. The last check of step 4 is an onboarding of P3 with you (I3), which replaces the P3 slice of the environments spec.
