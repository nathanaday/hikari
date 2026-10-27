import type { MapState, Role } from './types'

export const mapStateLabel: Record<MapState, string> = {
  current: 'map current',
  stale: 'map stale',
  unbuilt: 'map not built',
}

export const roleLabel: Record<Role, string> = {
  orchestration: 'orchestration agent',
  partition: 'partition agent',
  operator: 'operator',
}

// shortCommit writes <repo>@<commit> with seven characters of the commit,
// and a commit without a repo as seven characters.
export function shortCommit(c: string): string {
  const at = c.indexOf('@')
  return at < 0 ? c.slice(0, 7) : c.slice(0, at + 8)
}

// releaseCommits returns the commits of a release, in short form.
export function releaseCommits(m: { source_commits?: string[]; source_commit?: string }): string {
  if (m.source_commits?.length) return m.source_commits.map(shortCommit).join(', ')
  return m.source_commit ? shortCommit(m.source_commit) : ''
}
