# Repositories

This guide is about the Repositories section of the web UI: where each repository's work stands in git and on its forge, and the three actions that are yours, push, open a pull request, and merge.

## What hikari knows about a repository

Agents never run git. hikari commits each agent's changes to your branch of each repository, the one that the topology names, as `branch: ${HIKARI_BRANCH:-hikari}` or a name of its own. Everything after that is yours: pushing the branch, opening a pull request, watching its checks, and merging.

While `hikari up` runs, the server keeps one view of each repository of the topology:

- **Your branch** against the default branch: how many commits ahead, how many behind, how many are not pushed yet, and whether it is merged.
- **Every branch**, local and on `origin`, with its last commit, its distance from the default branch, and its open pull request when it has one.
- **The pull requests** of the forge: open ones, and the ones closed in the last two weeks, each with its checks (the check runs and the commit statuses of its head, summed up as passing, failing, pending, or none), its review decision, whether it can merge, its size, and the issues it closes.
- **The forge** itself: the host, the owner and name, when it was last read, and the last error.
- **The code no partition owns**: the tracked paths of your branch outside every writable folder, each at the highest directory that holds no partition folder. No agent can change them. The repository's page lists them, the map's panel counts them, and `hikari check` warns about them.
- **The drift of the default branch**: for each partition with a folder in the repository, the paths of its folders that the default branch changed since your branch diverged. The partition agent works on your branch, so it does not see these changes until you bring them in. The repository's page and the partition's page show them; the orchestration agent reads them through `list_repositories`.

The git side refreshes every few seconds, the forge every minute, and `origin` is fetched every few minutes with git's prompts off, so that a passphrase never hangs the run. Refresh does all three at once.

## Forges

A forge is the service that hosts a repository's remote. hikari reads it from the `origin` URL: `https://github.com/you/app.git`, `git@github.com:you/app.git`, and a GitHub Enterprise host such as `github.example.com` are GitHub. A repository whose origin hikari does not know, or that has no origin, still shows its branches, with no pull requests.

The credential is your machine's. hikari uses `GITHUB_TOKEN`, else `GH_TOKEN`, else the token of the GitHub CLI when you have run `gh auth login`. It stores none of them, and the agents' containers never see one: every tool that reaches the forge goes through the server on the host. Without a credential, the repository's page says so, and the branches still show.

## The pages

The sidebar lists each repository with a dot: green when all is well, amber when your branch has unpushed commits or a pull request's checks are pending, red when a pull request's checks fail, it conflicts with its base, or the forge could not be read. The Repositories page shows every repository as a card, and a repository's page shows:

- **Your branch**, with Push and Open pull request. Push sends your branch to `origin`, never forced. Open pull request takes a title and a body, from your branch to the default branch; write `Closes #12` in the body to close an issue when it merges. Both refuse with git's or the forge's reason when they cannot go on, such as a branch that is not pushed yet.
- **Pull requests**, each with its checks, its review, and whether it can merge. Merge, with merge, squash, or rebase, appears on an open pull request whose checks do not fail and that does not conflict.
- **Branches**, every one, so that you see a teammate's branch and an agent's branch side by side.
- **Partitions** in the repository, linking to each partition's page.

On the map, hover a repository for the same summary, and open its panel for the page.

## In the room

The orchestration agent has `list_repositories`: your branch of each repository, how far it is ahead, whether it is pushed, and each open pull request with its checks. Ask it where the work stands, and it answers from that and not from memory. Pushing, opening a pull request, and merging stay yours; the agent may say that they are the next step.

## In a terminal

`hikari push` pushes your branch of every linked repository and of the project repository, and `hikari sync` fetches and reports, as the [onboarding guide](onboarding.md) says. The web UI adds the pull requests and the merge; the terminal does not.
