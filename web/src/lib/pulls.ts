import type { IssueView, PartitionView, RepoView, TaskView } from './types'
import { openPulls, yourBranch } from './repos'

// A pull request from a scope (rule E1 of the action list): for each
// repository that a done scope's partitions commit to, what to do next and
// the title and body of the pull request, prefilled from the scope, with
// the line that closes its issue.

export type ScopePull = {
  repo: string
  branch: string
  base?: string
  // state says what is next: push the branch, open the pull request, or
  // nothing, because a pull request is open or merged, or the repository
  // has no forge.
  state: 'push' | 'open' | 'opened' | 'merged' | 'no-forge' | 'no-branch'
  pull?: number
  url?: string
  title: string
  body: string
}

// closesLine writes the line that makes the forge close the issue when the
// pull request merges: #n in the issue's own repository, owner/name#n from
// another repository of the same forge, and nothing for a local issue.
export function closesLine(issue: IssueView, repo: RepoView, repos: RepoView[]): string {
  if (issue.local || !issue.repo || !issue.number) return ''
  if (issue.repo === repo.name) return `Closes #${issue.number}`
  const home = repos.find((r) => r.name === issue.repo)
  if (home?.forge && repo.forge && home.forge.host === repo.forge.host) return `Closes ${home.forge.owner}/${home.forge.name}#${issue.number}`
  return ''
}

// scopePulls lists, for a scope, the pull request of each repository its
// partitions write to.
export function scopePulls(task: TaskView, issue: IssueView, repos: RepoView[], partitions: PartitionView[]): ScopePull[] {
  const names = new Set<string>()
  for (const p of task.partitions) {
    const v = partitions.find((x) => x.partition_agent === p)
    for (const f of v?.folders ?? []) if (!f.read_only) names.add(f.repo)
  }
  const out: ScopePull[] = []
  for (const name of [...names].sort()) {
    const repo = repos.find((r) => r.name === name)
    if (!repo) continue
    const yours = yourBranch(repo)
    const closes = closesLine(issue, repo, repos)
    const goal = issue.title || task.name
    const partsOf = task.partitions.filter((p) => partitions.find((x) => x.partition_agent === p)?.folders.some((f) => !f.read_only && f.repo === name))
    const body = [closes, `${goal}: the work of ${partsOf.join(', ')} on the scope \`${task.id}\`, done in hikari.`, task.features.length ? `Features: ${task.features.join(', ')}.` : ''].filter(Boolean).join('\n\n')
    const pull: ScopePull = { repo: name, branch: repo.branch, base: repo.default_branch, state: 'open', title: issue.title || task.name, body }
    const pr = yours?.pull_request ? repo.pull_requests.find((p) => p.number === yours.pull_request) : undefined
    const merged = repo.pull_requests.find((p) => p.head === repo.branch && p.state === 'merged')
    if (!yours) pull.state = 'no-branch'
    else if (pr) Object.assign(pull, { state: 'opened', pull: pr.number, url: pr.url })
    else if (yours.merged && merged) Object.assign(pull, { state: 'merged', pull: merged.number, url: merged.url })
    else if (!repo.forge) pull.state = 'no-forge'
    else if (!yours.remote || yours.unpushed > 0) pull.state = 'push'
    else if (openPulls(repo).some((p) => p.head === repo.branch)) {
      const open = openPulls(repo).find((p) => p.head === repo.branch)!
      Object.assign(pull, { state: 'opened', pull: open.number, url: open.url })
    }
    out.push(pull)
  }
  return out
}
