# Analysis brief

Send this brief to one fresh subagent for each repository, with `<repo>`, its system `<system>`, and the paths below. The subagent reads, and writes one file.

---

Analyze the repository `<repo>` of a hikari project, for a refactor into isolated systems. Read:

- the code in `repos/<repo>`;
- its owner's map, `store/systems/<system>/map.yaml`, and wiki, `store/systems/<system>/wiki/`;
- the baseline, `onboarding/baseline.md`, and the survey in `onboarding/state.md`;
- the names and purposes of the other repositories, in `onboarding/state.md`. Read the code of another repository only to compare a piece of code that this one copies.

Change no file except the one you write. Write `onboarding/analysis/<repo>.md` with exactly these sections, each with the files that show each fact:

```markdown
# <repo>

## Releases
What this repository makes for others: a library, a service, a binary, a client, data. Who uses each.

## Consumes
What it takes from the other repositories, and how: copied code, a package, an API, files, a database.

## Interfaces
The interfaces it provides and consumes: their names, and what crosses each one.

## Duplicated code
Each piece of code that another repository also holds, with the files on both sides, and each difference in behavior between the copies. Quote the lines that differ. Do not judge which copy is right.

## E2E tests
- Tests that need a display, a browser, hardware, or the whole application, with what each needs.
- Flows that cross repositories and that no test covers, with the steps of each.

## Run files
Compose files, Dockerfiles, CI workflows, and what each starts or runs.

## Open questions
What only the user can decide.

## Your answers
(Left for the user.)
```

Name no secret. Keep the report under 700 words.
