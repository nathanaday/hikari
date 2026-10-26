# Spec: the Structure tab

Status: built. Checked in a headless Chrome on 2026-10-26 against `hikari up` on the project of the topology API's live run, while its agents ran. In one review of six changes, the tab added the hub `#ops`, added the agent `reviewer` from the template `orchestrator` into `#core` and `#ops`, changed its id to `critic`, set its model, removed it from `#core`, and renamed `#core`, whose id `main` is locked, to `#platform`. The review listed the note "Ask @app in #platform before the release." under a checkbox, and the save committed the compose file and the note as `user: add hub #ops; add agent reviewer in #core, #ops; ...`. After Stop, a pending member was saved with no "applies at the next start". A system with a scope and a read path that do not exist, and a release grant, showed four findings at line 41 and an error dot on the agent's row. A file that is not YAML showed "Fix it in the YAML tab". The browser logged no errors.

Changes made during the build:

1. **Summaries use names.** A commit message named hubs and agents by id, as `add agent reviewer in #main`, while you know the hub as `#core`. Each summary now uses the names of the moment. Found in the browser check.
2. **You sit in the corner of the hub diagram.** Beside the orchestrator, your node pushed it off the center, so its line missed the members' rail. Found in the browser check.
3. **Edit in YAML with pending changes opens their review**, because the YAML tab edits the saved file (SP4).
4. **The tab guard's message goes when its reason does**: after a save or a discard.
5. **A file that is not YAML shows no sentence about commits**, because whether a save commits depends on the text you save.
6. **A finding of a key inside a flow mapping points at the mapping's line.** The checks give the path of the key, and the source writes the whole `x-hikari` of a service on one line, so its findings share that line.

Sources: `docs/brainstorm/topology-editor.md` (C2, C3, D1, E3, F2, F3, G3), `topology-editor-round-2.md` (I1 to I5), and the built `topology-api.md` and `yaml-view.md`. This is the last of four steps of the topology editor.

## Summary

The Structure tab is a form of the topology. A list on the left holds the project, the repos, the hubs, the agents, and the sidecars; the inspector on the right edits the one you select. Each edit becomes an operation of the topology API and waits in a list of pending changes. The form shows the topology as the pending changes would leave it, because every change is planned on the server as you make it. Review opens the plan: the diff of the file, what it leaves outside the file, the places a rename can propagate to, each with a checkbox, and the findings. Save applies it.

## Decisions

From the rounds and their answers:

1. A list on the left and an inspector on the right (F2).
2. Every save shows its plan first, with the diff (F3).
3. The form edits `x-hikari` and the identity of each service; it shows the other compose keys and sends you to the YAML tab for them (C2).
4. A field whose source is an expression shows the expression (C3).
5. The UI checks fields as you type, with rules and cases shared with Go (E3).
6. A rename asks where to propagate, place by place (I5); an id can change only while nothing is keyed by it (I4).

## Decisions made in this spec

1. **The form shows the plan's model.** The server already turns operations into text and the text into a model, so a plan returns the model of its new text, and the form renders that model. The pending changes and the form can never disagree, and no rule of an operation is written twice.
2. **Agents are one flat list, each with its roles.** An agent can be in several hubs, so a tree of hubs would show it more than once. Each row says what it does: "orchestrates #eng", "member of #eng, owns app".
3. **A field commits its change when you leave it or press Enter.** A change per keystroke would plan per keystroke. A text field keeps its draft until then, with Esc to drop it.
4. **The hub diagram is the one drawing of the tab.** The hub inspector draws its orchestrator above its members, with the node avatars of the Map, and you beside them, so that the shape of a hub reads at a glance; the other inspectors are plain forms.
5. **The shared cases are one JSON file.** `internal/topology/testdata/rules.json` lists values of names, branches, and urls with whether each is valid, and the Go tests and the TypeScript tests both run it.

## Terms

- **Pending changes**: the operations you made since the last save, in order.
- **Preview**: the plan of the pending changes: its model, its findings, and its text.

## 1. The List

- **SL1.** The list has the groups Project, Repos, Hubs, Agents, and Sidecars; an empty group except Project shows only its Add button. Each row shows the entity's name, and its id in muted text when the two differ.
- **SL2.** A hub's row shows its member count; an agent's row shows its roles (decision 2); a repo's row shows its url or path.
- **SL3.** A row with a finding shows the finding's severity dot. A finding belongs to an entity when its path starts with the entity's path, such as `x-hikari.hubs.eng`.
- **SL4.** Selecting a row opens its inspector. The selection is in the address: `#/topology?select=hub:eng`.

## 2. The Inspectors

Every inspector shows the entity's name, its id, and its findings, and ends with Remove, except the project's.

- **SI1. Project.** Name. The project repository on or off; its branch when on. The store directory.
- **SI2. Repo.** Url or path, one of the two. Branch. The systems in it. Remove is disabled while a system uses it, and says which.
- **SI3. Hub.** The diagram (decision 4). Description. Orchestrator, from the agents. Members, as chips with Add member. The tester, when an agent tests the hub, with a link to it.
- **SI4. Agent.** Description and instructions. Model: the default, `sonnet`, `opus`, `haiku`, or another value you type. Its hubs, with its role in each. Its container: the image or the build, the template it merges, its other compose keys, and Edit in YAML, which opens the YAML tab at the agent's line. System: off, or a repo, a scope, read paths, and a test command. Grants: releases from the other owners, and deliverables from the store. Tests: a hub it is a member of, and the e2e limits of a tester.
- **SI5. Sidecar.** The agent it serves. Its image, with Edit in YAML.
- **SI6. Names and ids.** Name has Rename. Id has Change id while the entity has no state outside the file, and else says "The store is keyed by this id". A renamed agent or hub adds a rename operation; the review offers its propagation.
- **SI7. Add.** Add repo, Add hub, Add agent, and Add sidecar open a short form in the inspector: the id and the fields an operation needs (`add_repo`: url or path, branch; `add_hub`: orchestrator, description; `add_agent`: template, description, hubs; `add_sidecar`: serves, image).
- **SI8. Expressions.** A field whose value holds `${` shows it as written, with a note that compose fills it in, and stays read-only in the form; Edit in YAML changes it.

## 3. Pending Changes

- **SP1.** Each committed field adds an operation (decision 3). The UI plans the pending changes at once, and the form shows the plan's model (decision 1).
- **SP2.** When a plan refuses an operation, the UI drops it, keeps the others, and shows the refusal at the field.
- **SP3.** A bar at the bottom shows "1 change" or "n changes" with Discard and Review and save, as the YAML tab's bar does.
- **SP4.** Switching to the YAML tab with pending changes asks to save or discard them first.

## 4. The Review

- **SR1.** The review shows the summary of the operations, the diff, the effects, the findings, and, in the running mode, "Applies at the next start".
- **SR2.** For each rename, the review lists its occurrences, each with its file, its line, and its new text, under a checkbox, all checked. An occurrence without a key shows "Its owner updates it", with no checkbox.
- **SR3.** Save applies the operations with the checked keys. It then clears the pending changes, and shows the commit or the note of the save, and the skipped keys.
- **SR4.** On a conflict, the review says the file changed, and Plan again plans the same operations on the new file.

## 5. Checks as You Type

- **SV1.** A name or id field checks the rule of ids (a lowercase letter, then lowercase letters, digits, and dashes, up to 32), the reserved `user` and `project`, and that no other entity of its namespace uses it as a name or an id.
- **SV2.** A branch field checks the rule of branch names, unless it is an expression. A url field checks the rule of urls. The project's name is one line of 1 to 64 characters.
- **SV3.** A field with a problem shows it under the field, and does not commit until it is fixed.
- **SV4.** The rules repeat `internal/topology` and run the cases of `rules.json` in both languages (decision 5).

## 6. Out of Scope

- Editing a service's compose keys in the form.
- Undo of a single pending change; Discard drops them all.

## 7. Tests

1. **Rules.** The cases of `rules.json` in Go and in TypeScript.
2. **List.** Groups, rows, roles, and the findings of SL3, from a model fixture.
3. **Inspectors.** The hub inspector's diagram and members; an agent's container and Edit in YAML; an id that is locked.
4. **Pending changes and review.** The bar's count; the review renders the summary, the diff, the effects, and the occurrences with their checkboxes, from a plan fixture with a rename.
5. **API.** The plan's model, in Go, and in its fixture.

## 8. Build Plan

1. **Shared rules.** `rules.json`, its Go test, and `topology-rules.ts` with its test.
2. **The plan's model.** The API change and its fixture.
3. **The tab.** The list, the inspectors, pending changes, and the review, with tests.
4. **Browser check and docs.** Rename a hub with propagation, add an agent from a template into two hubs, give it a system and a grant, change an id, remove a member, and save, during a run and when stopped. Then update the README, `docs/design.md`, and this spec's status.
