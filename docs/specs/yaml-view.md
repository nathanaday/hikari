# Spec: the Topology view and its YAML tab

Status: built. Checked in a headless Chrome on 2026-10-25 against `hikari up` in an empty directory: the Topology view opened alone in the sidebar with Create the project; create opened the YAML tab; a member `ghost` drew a red dot on line 18 and the finding "hub "main" references unknown agent "ghost"" within a second; Cmd-S opened the review with the diff, and the save committed with Start disabled; the fix enabled Start, which ran the agents and filled the sidebar; an edit on disk under an unsaved change showed "The file changed on disk while you edited it"; Stop returned to the editing state. The browser logged no errors.

Terms changed in `shared-language-names.md`; see `docs/glossary.md`.

Changes made during the build:

1. **The editing mode reads "Agents stopped".** "editing" names hikari's state, not what you see; beside Start agents, "stopped" says what Start changes.
2. **A missing file on the first start is not a start finding.** The view's Create the project already says that the file does not exist, and the banner stayed after create. Found in the browser check.
3. **A removed line comes before the line that replaces it**, in the server's diff and the UI's. The diff showed the new line first, which reads backward. Found in the browser check.
4. **CodeMirror loads with the YAML tab**, in its own chunk of about 430 KB, so that the rest of the UI loads as before.
5. **A start finding with a line opens the YAML tab at that line.**

Sources: `docs/brainstorm/topology-editor.md` (F1, F3 to F5, G3), `topology-editor-answers.md`, and the built API of `topology-api.md`. This is the third of four steps of the topology editor.

## Summary

The sidebar gets a Topology item. Its view shows the compose file that the project runs from, with Start and Stop, and two tabs: Structure, which step 4 builds, and YAML. The YAML tab is a code editor of the file with lint as you type, the findings beside it, and a save that shows its diff first. When hikari waits in the editing state, the Topology view is the UI: it opens there, and a missing file offers to create the project.

## Decisions

From the rounds and their answers:

1. A Topology item in the sidebar, under Shared, with the tabs Structure and YAML (F1).
2. Every save shows its plan first: the diff, the findings, and whether it applies at the next start (F3).
3. The YAML tab uses CodeMirror 6 (F4).
4. A broken file still opens in the YAML tab (F5).
5. The UI's tests render from the API's fixtures (A5, G3).

## Decisions made in this spec

1. **The review is a sheet over the view, not a page.** The diff of a save is short, and you return to the editor at once; a sheet from the right keeps the editor in sight, as the existing sheet component does on narrow screens.
2. **The colors of findings are the lens colors.** An error uses `--lens-fail` and a warning `--lens-pending`, the colors that already mean failing and pending on the map. The diff uses the same red for a removed line, and `--lens-pass` for an added one.
3. **The file's text is the editor's text until you change it.** When the file changes on disk and the editor holds no change of yours, the editor takes the new text at once. When it holds a change, a bar offers to load the file's text and drop yours, or to keep yours; a save of yours then gets the conflict of rule TA2 and offers the same choice.
4. **Start and Stop live in the Topology view's header.** The editing state shows no other view, and a start follows a change of the topology, so the button sits beside the file.
5. **In the editing state, the sidebar shows only Topology.** Every other view needs agents (decision 6 of the topology API spec).

## Terms

- **Buffer**: the text in the editor.
- **Saved text**: the text of the file at the revision the editor last loaded.

## 1. The View

- **YV1.** The sidebar lists Topology under Shared, with the file icon. Its view's address is `#/topology` for the Structure tab and `#/topology/yaml` for the YAML tab.
- **YV2.** The header shows the title Topology, the file's path, whether a save commits ("Each save is a commit in the project repository", or "Saves are not committed: the topology is not in a project repository"), the mode, and the tabs.
- **YV3.** The mode shows as a status dot and a word: `editing` (neutral), `starting` (active), `running` (pass), `stopping` (active). Beside it, the button Start agents in the editing mode, and Stop agents in the running mode. Start is disabled while the saved text has an error, and its title says why.
- **YV4.** When a start fails, the view shows the start's findings above the tabs, with "The agents did not start", until the next start.
- **YV5.** In the editing state, the UI opens the Topology view, and the sidebar lists only Topology. The composer is hidden on the Topology view.
- **YV6.** When the file does not exist, the view shows what `create` writes in its directory, and the button Create the project. After it, the YAML tab opens.

## 2. The YAML Tab

- **YY1.** The editor shows the buffer with YAML highlighting, line numbers, the current line, and search (Cmd-F or Ctrl-F).
- **YY2.** Half a second after the buffer changes, the UI lints it (`POST lint`). Each finding with a line marks its line in the gutter and underlines it, in the color of its severity; hovering shows the message.
- **YY3.** A panel beside the editor lists the findings, errors first, each with its line. Clicking one moves the cursor to its line. With no finding, the panel says "No problems". On a narrow screen, the panel goes below the editor.
- **YY4.** When the buffer differs from the saved text, a bar at the bottom says "Unsaved changes", with Revert and Review and save. Cmd-S or Ctrl-S opens the review.
- **YY5.** The review shows the diff of the saved text and the buffer, the findings of the buffer, and, in the running mode, "Applies at the next start". Save writes the buffer with `PUT source`; the review closes, and the view shows the commit or the note of the save.
- **YY6.** Save is allowed with errors, because the file is yours (TA5), and the review then says "The file has errors; hikari will not start until you fix them".
- **YY7.** On a `409`, the review says the file changed on disk, with Load the file's text and Keep mine; Keep mine saves the buffer over the new revision.
- **YY8.** A `topology` event with a new revision loads the file: into the buffer when it has no change of yours, and else into the bar of decision 3.

## 3. State

- **YS1.** The UI's state holds the mode, the file, the revision, and the start's findings from each snapshot, and the revision from each `topology` event.
- **YS2.** The Topology view loads `GET /api/ui/topology` when it opens and when the revision changes.

## 4. Out of Scope

- The Structure tab (step 4); until then it says that the form arrives in step 4 and links the YAML tab.
- Completion of keys in the editor.

## 5. Tests

1. **View.** The hashes of YV1; the mode label and the Start button's state of YV3, from the `state` fixture.
2. **Lint.** Findings to editor markers: severity, line, and a finding without a line, which marks nothing.
3. **Review.** The diff renders from the `plan` fixture: added and removed lines, and the line numbers.
4. **State.** The reducer of YS1 for a snapshot of each mode and a `topology` event; the editing state's sidebar of YV5.

## 6. Build Plan

1. **State and routing.** YS1, YV1, YV5, with tests.
2. **The view.** The header, the mode, Start and Stop, the start's findings, and create (YV2 to YV6).
3. **The YAML tab.** CodeMirror, lint, the findings panel, the review, conflicts (YY1 to YY8), with tests.
4. **Browser check and docs.** Open `hikari up` on a missing file in a browser, create the project, break the file and watch the findings, save with review, start, stop, and change the file on disk while the editor is open. Then update the README and this spec's status.
