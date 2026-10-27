# Shared language: answers

Answers to `shared-language.md`, 2026-10-26.

| Question | Answer |
|---|---|
| B2 | "Zero or one is fair." |
| B6 | "Vocabulary today is just the one file but for letting future flexibility, I like a directory so we can eventually have more 'vocabulary-like' files in vocabulary/." |
| E2 | "Yes I think that's right." |
| E5 | "Yes." |
| E7 | "Yes env is the structure for now, an established secret system is held out for future work." |

## What the answers set

**A project has zero or one orchestration agent.** `x-hikari.orchestration` is optional. With no orchestration agent, a post without mentions starts a turn for every agent (B4).

**The vocabulary is a directory, `vault/public/vocabulary/`.** Today's file moves into it as `terms.yaml`, in today's format without the `hub:` field. It holds the terms and the interfaces, and `define_term` and `define_interface` both write it. Hikari reads only `terms.yaml` and ignores any other file in the directory, so a later vocabulary-like file can be added there without a change to the vault layout. The D1 layout reads `vocabulary/terms.yaml` in place of `vocabulary.yaml`.

**Every folder mounts at `/workspace/<repo>/<path>`, unless it sets `at:`.** The migration of C4 moves each test command and code map once.

**Hikari applies at the end of every turn and at stop.** An agent's `apply` names the commit; the end of a turn makes one when the agent did not.

**Secrets stay in compose's `env_file` and `environment` on the partition's service.** A secret system of Hikari's own is planted in `next-phase.md`.
