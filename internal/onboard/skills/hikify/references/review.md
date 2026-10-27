# Review brief

Send this brief to each fresh reviewer, with the round number. The reviewer only reads.

---

You review the refactor plan of a hikari project before any code changes. Your job is to find what would go wrong. Do not praise; return findings. Read:

- `onboarding/analysis/` (the reports and the synthesis), `onboarding/state.md` (the decisions the user made), and `onboarding/baseline.md`;
- `compose.target.yaml` and `onboarding/topology.md`;
- the plans, `onboarding/wiki/public/hikify-plan.md` and `onboarding/wiki/systems/*/hikify-plan*.md`;
- the ledger, `onboarding/verification.md`;
- the drafts of the public wiki in `onboarding/wiki/public/`;
- the code in `repos/`, to check every claim the plan makes about it;
- the earlier rounds in `onboarding/review/`, so that you do not repeat a finding that was resolved.

Look for:

1. A step that breaks a behavior the baseline covers, or that the user's decisions fixed.
2. A step that needs a release, a repository, or a file that no earlier step makes.
3. A boundary without a reason of the three kinds (library; client and server; scope in a repository), or a system that holds two concerns.
4. A test that the ledger loses, places twice, or places in a system that will not hold its code.
5. A system's suite that would still need a display, a browser, or another system.
6. A step whose "Done when" nothing can show.
7. A draft that breaks the wiki rules: an id that is not unique, a link that does not resolve, a missing `## Purpose`.
8. A claim about the code that the code does not support.

For each finding, return:

```markdown
### <n>. <one line>
- Severity: blocks | should fix | minor
- Where: the file and the step or line
- Evidence: what you read that shows it
- Fix: what would resolve it
```

Return "No findings." when you find nothing that stands. Keep the answer under 800 words.
