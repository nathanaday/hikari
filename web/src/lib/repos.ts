import type { BranchView, PullRequestView, RepoView } from './types'

// Helpers that read a repository's view (section 2 of the repositories,
// forges, and issues spec).

// yourBranch is the view of your branch: the one the agents commit to.
export function yourBranch(v: RepoView): BranchView | undefined {
  return v.branches.find((b) => b.yours) ?? v.branches.find((b) => b.name === v.branch)
}

export function openPulls(v: RepoView): PullRequestView[] {
  return v.pull_requests.filter((p) => p.state === 'open')
}

// RepoHealth is the dot of a repository in the sidebar (rule UI1): bad when
// an open pull request has failing checks or conflicts, or git or the forge
// errored; warn when your branch has unpushed work, or an open pull request
// has pending checks; good otherwise.
export type RepoHealth = 'good' | 'warn' | 'bad'

export function repoHealth(v: RepoView): RepoHealth {
  const open = openPulls(v)
  if (v.error || v.git_error) return 'bad'
  if (open.some((p) => p.checks.state === 'failing' || p.mergeable === 'conflicting')) return 'bad'
  const yours = yourBranch(v)
  if (yours && v.url && (!yours.remote || yours.unpushed > 0)) return 'warn'
  if (open.some((p) => p.checks.state === 'pending')) return 'warn'
  return 'good'
}

// RepoGit is a repository's git state for the map's git lens (rule UI3).
export type RepoGit = 'failing' | 'dirty' | 'pending' | 'passing' | 'clean'

export function repoGit(v: RepoView): RepoGit {
  const open = openPulls(v)
  if (open.some((p) => p.checks.state === 'failing' || p.mergeable === 'conflicting')) return 'failing'
  if (v.dirty) return 'dirty'
  const yours = yourBranch(v)
  if ((yours && v.url && (!yours.remote || yours.unpushed > 0)) || open.some((p) => p.checks.state === 'pending')) return 'pending'
  if (open.some((p) => p.checks.state === 'passing')) return 'passing'
  return 'clean'
}

// branchLine says how your branch stands against the default branch.
export function branchLine(v: RepoView): string {
  const yours = yourBranch(v)
  if (!yours) return `${v.branch} · not created yet`
  const parts: string[] = []
  if (v.default_branch && !yours.default) parts.push(`${yours.ahead} ahead of ${v.default_branch}`, `${yours.behind} behind`)
  if (v.url) {
    if (!yours.remote) parts.push('not pushed yet')
    else if (yours.unpushed > 0) parts.push(`${yours.unpushed} unpushed`)
    else parts.push('pushed')
  }
  if (yours.merged && v.default_branch && !yours.default) parts.push(`merged into ${v.default_branch}`)
  return parts.length ? parts.join(' · ') : 'up to date'
}

// ago writes a time as "just now", "n min ago", "n h ago", or a date.
export function ago(iso?: string): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  return new Date(t).toLocaleDateString()
}

// issueRef names a task's issue: its tracker ref, or its id when local.
export function issueRef(t: { id: string; issue?: string }): string {
  return t.issue ?? t.id
}
