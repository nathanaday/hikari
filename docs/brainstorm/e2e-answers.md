# E2E orchestration: round 1 answers

Answers to `e2e.md` (the revised round 1), 2026-10-22.

| Question | Answer |
|---|---|
| A1 to I3 | "I agree with this." Every recommendation stands. |

Where a question offered a choice, the recommendation is the answer:

- **B2.** Two kinds, story and rule.
- **B6.** Owners read the stories that link their features.
- **C7.** Bench results by hand, in the fourth step.
- **E2.** hikari builds image releases on the host, from the commit that the map describes.
- **H4.** The project repository follows the branch rules of every repo.
- **H6.** `hikari init`, and yami moves to its own repository.
- **I3.** A live run on a slice of P3, as the last check of the third step.

The user added: "no doubt we will have to drive this in practice for a bit and do a few rounds of feedback, but get this spec'd first and built."

## What the answers set

Four steps, each with its own spec, built in order (I1):

1. The project repository: `docs/specs/project-repo.md`.
2. Stories, the tester, the plan, and the lock.
3. Environments.
4. Bench results.

Each spec is written when the step before it is built, so that it starts from what the build learned.

The user's note of 2026-10-22, during the first step, added the hub and system wikis as a step of their own, before the tester: principles, features, rules, and stories, and a place for the user's plans, roadmaps, and notes. The steps are now:

1. The project repository: `docs/specs/project-repo.md`.
2. The hub and system wikis: `docs/specs/wikis.md`.
3. The tester, the plan, and the lock.
4. Environments.
5. Bench results.
