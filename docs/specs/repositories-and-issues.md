# Spec: Repositories, Forges, and Issues

Status: built. Checked with `go test -race ./...`, `npm test`, and `npm run build`; the GitHub client is checked against a fake GitHub in the tests, since the build machine had no GitHub credential. A live check against a real repository is the first thing to do on a machine that has one: `hikari up` on a project whose repository's `origin` is on GitHub, with `GITHUB_TOKEN` set or `gh auth login` done.

Sources: the request of 2026-11-04, in two parts. First, the UI gives no view of where the work stands in git: which branch each agent's work landed on, what is pushed, what has a pull request, whether its checks pass, and what is merged; the agents recap this in conversation, and the recap is lost. Second, the task pipeline's stubs, scopes, and queues are hard to use: it is unclear how a stub reaches an agent, and the pipeline lives only on one machine. The request asks for GitHub first, with the door open to other hosts and other trackers.

## Summary

Hikari learns to read the service that hosts each repository's remote, which this spec calls the repository's **forge**. GitHub is the first forge. For each repository of the topology, hikari shows your branch against the default branch, every branch with how far it is ahead or behind, the pull requests with their checks and reviews, and what is merged, on a page of its own and on the map; and it lets you push, open a pull request, and merge from there.

The task pipeline becomes issue-centered. An **issue** is the unit of work. When a repository's forge has an issue tracker, its issues are the project's, read and written through the forge. A project with no such repository keeps its issues in the vault, as markdown, as before. You triage in the Work tab or on GitHub, drag issues into a queue, start and pause the queue, or hand one issue to the agents with one click. The orchestration agent scopes an issue when the queue starts it, so there is no separate scoping step to drive; hikari posts the scope, each block, and the finish back to the issue as comments and labels, so the plan and the progress are public.

## Decisions

1. **A forge is detected, not configured.** The `origin` URL of a repository says where it is hosted. `github.com` and a GitHub Enterprise host are GitHub. A repository whose origin hikari does not know has no forge, and the UI says so. A key in the topology is not needed; one can come later for a host that cannot be told from its URL.
2. **Two interfaces, one implementation.** `internal/forge` defines `Forge`, which reads and changes the code side: the default branch, the pull requests, the checks of a commit, and merges; and `Tracker`, which reads and changes the issues: list, read with comments, create, update, comment, and label. GitHub implements both. A later Linear or Jira implements only `Tracker`; a later GitLab implements both. The vault's local issues are the tasks module's own, and its view joins them with the trackers' issues, so the Work tab draws one list.
3. **The credential is the machine's.** The GitHub token comes from `GITHUB_TOKEN` or `GH_TOKEN`, or from `gh auth token` when the GitHub CLI is signed in. Hikari stores no credential, and the agents' containers never see it: every tool that reaches the forge goes through the server on the host.
4. **The server reads the forge; the agents read the server.** The orchestration agent gets `list_repositories` and the issue tools, which call the server's routes. Agents never run git and never hold a token, as before.
5. **Hikari fetches, and never pushes or merges on its own.** The repository view fetches `origin` on a slow interval and on Refresh, with git's prompts off and a timeout, so that an SSH passphrase never hangs a run. Push and merge are your clicks.
6. **One view per repository, in the snapshot.** `Snapshot.repos` keeps `name`, `branch`, and `partitions`, which the map draws, and gains the branches, the pull requests, the forge, and the sync time. A `repo` event carries one changed view.
7. **An issue is the unit of work, and a task is its record.** A queue lists tasks, as before; a task that stands for a tracker issue has an `issue` property, `<repo>#<number>`, and the id `<repo>-<number>`. A local issue is a task without `issue`. The UI shows issues and joins each with its task.
8. **Any issue can enter a queue, and the orchestration agent scopes it when the queue starts it.** The stub stage stays in the file format, with the name `stub`, for a task that is not scoped yet. While a started task is not scoped, no queue starts another, so the scoping happens one at a time and the partitions it names are known before the next start.
9. **"Work on this" is one click.** It puts the issue at the front of the project's work queue, which it makes when missing, and starts the queue.
10. **Hikari writes to the issue at the moments that matter.** On start, a comment that names the queue; when the scope is written, a comment with the goal and each partition's part; on a block, a comment with the reason; on done, a comment with the features that pass and the branch to merge. Labels `hikari:queued`, `hikari:working`, `hikari:blocked`, and `hikari:done` follow the status. Each write is best effort: a forge error reaches the feed and never stops the pipeline.
11. **`plant_stub` becomes `create_issue`.** An agent that finds work outside its part creates an issue, on the tracker of the project's repository when it has one, and in the vault otherwise. The word stub leaves the UI, the skills, and the prompts.
12. **Reordering has buttons, and drag and drop on top.** The Work tab keeps Move up, Move down, Move to queue, and Take out, which tests and keyboards reach, and adds dragging a card into a queue or between places.

## Terms

- **Forge**: the service that hosts a repository's remote: GitHub, and later others. It is read from the `origin` URL.
- **Tracker**: where a project's issues live: the forge of a repository with issues, or the vault.
- **Issue**: one piece of work as the tracker holds it. On GitHub, a GitHub issue; in the vault, a task document without an `issue` property.
- **Pull request**: the forge's proposal to merge a branch.
- **Your branch**: the hikari branch of a repository, as the linked repositories spec names it.

## 1. Forges

- **FG1.** `forge.Parse(url)` reads an `https://`, `ssh://`, or `git@host:` URL into a remote: its kind, host, owner, name, web URL, and API base. `github.com` and any host whose URL is read as GitHub by this rule are kind `github`; a GitHub Enterprise host gets `https://<host>/api/v3`. Any other URL has no forge.
- **FG2.** `Forge` gives: `Remote()`, `DefaultBranch`, `PullRequests` (open ones, and closed ones by recent update, with head, base, head commit, draft, author, merged time, mergeable state, additions, deletions, files, and review decision), `Checks(sha)` (check runs and commit statuses, each with name, state, and link), `CreatePullRequest`, and `Merge(number, method)`.
- **FG3.** `Tracker` gives: `Remote()`, `Issues` (open ones, and closed ones by recent update, without pull requests), `Issue(number)` with its comments, `CreateIssue`, `UpdateIssue` (title, body, state, labels), `Comment`, `AddLabels`, and `RemoveLabel`. A label that does not exist is created on first use.
- **FG4.** The GitHub token is `GITHUB_TOKEN`, else `GH_TOKEN`, else the output of `gh auth token -h <host>`. Without one, the forge is known but not reachable, and the view says why.
- **FG5.** Every call has a timeout, and a GitHub error answers with its message, not a status code alone.

## 2. Repositories

- **RP1.** `internal/repos` keeps one `RepoView` for each repository of the topology: name, branch (your branch), path, origin URL, forge, default branch, partitions, dirty, head, branches, pull requests, when the forge was last read, and the last error of git or the forge.
- **RP2.** A branch of the view is a local branch or a branch of `origin`, by name once: head, subject, author, and time of its last commit; ahead and behind the default branch; unpushed commits; whether it is yours; whether it is merged into the default branch; and the number of its open pull request when one has it as head.
- **RP3.** A pull request of the view carries the forge's fields of FG2 and the checks of its head: a summary state (`passing`, `failing`, `pending`, or `none`) with the counts, and each run.
- **RP4.** The git side refreshes every few seconds; the forge side every minute, and at once on Refresh. Refresh also fetches `origin`, with `GIT_TERMINAL_PROMPT=0` and a timeout.
- **RP5.** Actions, yours alone: Refresh, Push your branch (never forced), Open a pull request from your branch to the default branch (title and body yours; the body may name issues to close), and Merge a pull request (merge, squash, or rebase). Each refuses with the forge's or git's reason.
- **RP6.** The snapshot's `repos` holds the views; a `repo` event carries one; `GET /api/ui/repos` lists them, and `POST /api/ui/repos/{name}/refresh`, `/push`, `/pulls`, and `/pulls/{n}/merge` are the actions.
- **RP7.** The orchestration agent gets `list_repositories`: each repository's branch, how far it is ahead of the default branch, what is unpushed, and each open pull request with its checks, so that it reports where the work stands from the facts and not from memory.

## 3. Issues

- **IS1.** A task may have `issue: <repo>#<number>`. Its id is `<repo>-<number>`, its name the issue's title, and its Request the issue's body. Hikari makes it when the issue enters a queue, or when the orchestration agent scopes the issue.
- **IS2.** The module reads each tracker's issues every minute, and on Sync. The view's `issues` are every tracker issue, open or recently closed, and every local task, each with its task when one exists.
- **IS3.** Any task may enter a queue (TK11 is relaxed). A task whose stage is `stub` when the queue starts it is `started` with no parts, and hikari tells the orchestration agent to scope it with `scope_task` and carry it out. While such a task exists, every other queue's next task waits, with the reason.
- **IS4.** `scope_task` is allowed on a started task that is not scoped yet, and sets its parts `queued`. It stays refused for a scope in a queue.
- **IS5.** A started task without an orchestration agent cannot be scoped, so it is `blocked` with that reason.
- **IS6.** Tracker writes, as decision 10, run outside the store's lock and report errors to the feed.
- **IS7.** Tools: `create_issue(repo, title, body)` for every agent; `get_issue(ref)`, `list_issues`, and `comment_issue(ref, body)` for the orchestration agent; `plant_stub` is gone. `list_tasks`, `get_task`, `set_task_status`, `scope_task`, and `change_queue` stay, and `change_queue` takes an `issue` as well as a `scope`.
- **IS8.** UI routes below `/api/ui/tasks`: `GET /view`, `POST /sync`, `POST /issues`, `GET /issues/{ref}`, `PATCH /issues/{ref}`, `POST /issues/{ref}/comments`, `POST /work`, and the routes of the task pipeline spec.
- **IS9.** `hikari issue list|new|work|sync` join `hikari task` and `hikari queue`; `hikari task stub` is gone.

## 4. The UI

- **UI1.** The sidebar gets Repositories: one entry per repository, with a dot for its state, and the Repositories page lists them all.
- **UI2.** A repository's page shows your branch against the default branch with Push and Open pull request, the pull requests with checks, review, mergeable state, and Merge, the branches, and the partitions in it.
- **UI3.** On the map, a repository's hover card and panel say the branch, how far ahead, the open pull requests, and the checks, and link to the page.
- **UI4.** The Work tab shows the issues of every tracker and the local ones, with filters; a form that creates an issue; each queue as a column with Start and Pause; drag and drop into and within a queue; and Work on this on each issue.
- **UI5.** An issue's panel shows its body, its comments, its labels, its task's scope and log when it has one, Set status for you, Plan with the agent (a direct message), Work on this, and Open on GitHub. The title and body are editable.

## 5. Docs and checks

- **DC1.** The glossary gains forge, tracker, issue, and pull request, and retires stub from the UI and the skills. `docs/guides/tasks.md` follows an issue from GitHub to done. The README names the repositories view and the issues.
- **DC2.** `go vet ./...`, `go test -race ./...`, `npm test`, and `npm run build` pass.

## Build plan

1. git queries, the forge package with GitHub, and the repository views (FG1 to FG5, RP1 to RP5).
2. The server: snapshot, events, routes, the tool (RP6, RP7).
3. The tasks module: issues, the scheduler's gate, the tracker hooks, the tools, the routes, the CLI (IS1 to IS9).
4. The UI (UI1 to UI5).
5. Docs and checks (DC1, DC2).
