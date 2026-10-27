import type { Finding, TopologyAgent, TopologyFolder, TopologyModel } from './types'

// An entity of the Structure tab: its kind and its id. The project has no
// id.
export type Selection = { kind: 'project' } | { kind: 'repo' | 'agent' | 'sidecar'; id: string } | { kind: 'add'; what: AddKind }
export type AddKind = 'repo' | 'agent' | 'sidecar'

// Op is one operation of the topology API (section 4 of its spec).
export type Op = { op: string } & Record<string, unknown>

export function selectionKey(s: Selection): string {
  if (s.kind === 'project') return 'project'
  if (s.kind === 'add') return `add:${s.what}`
  return `${s.kind}:${s.id}`
}

export function parseSelection(key: string | undefined): Selection | undefined {
  if (!key) return undefined
  if (key === 'project') return { kind: 'project' }
  const [kind, id] = key.split(':')
  if (kind === 'add' && ['repo', 'agent', 'sidecar'].includes(id)) return { kind: 'add', what: id as AddKind }
  if (['repo', 'agent', 'sidecar'].includes(kind) && id) return { kind: kind as 'repo', id }
  return undefined
}

// exists reports whether the model still has the selected entity.
export function exists(s: Selection, m: TopologyModel): boolean {
  switch (s.kind) {
    case 'repo':
      return m.repos.some((r) => r.id === s.id)
    case 'agent':
      return m.agents.some((a) => a.id === s.id)
    case 'sidecar':
      return m.sidecars.some((x) => x.id === s.id)
  }
  return true
}

// entityPaths returns the YAML paths that hold an entity's keys (rule SL3).
export function entityPaths(s: Selection): string[] {
  switch (s.kind) {
    case 'project':
      return ['name', 'x-hikari.name', 'x-hikari.project', 'x-hikari.vault', 'x-hikari.orchestration']
    case 'repo':
      return [`x-hikari.repos.${s.id}`]
    case 'agent':
    case 'sidecar':
      return [`services.${s.id}`]
  }
  return []
}

const under = (path: string, p: string) => path === p || path.startsWith(p + '.') || path.startsWith(p + '[')

// findingsOf returns the findings whose path belongs to an entity.
export function findingsOf(fs: Finding[], s: Selection): Finding[] {
  const paths = entityPaths(s)
  return fs.filter((f) => f.path && paths.some((p) => under(f.path!, p)))
}

// worst is the most severe of some findings, or null.
export function worst(fs: Finding[]): Finding['severity'] | null {
  if (fs.some((f) => f.severity === 'error')) return 'error'
  return fs.length ? 'warning' : null
}

// roles says what an agent does (decision 2 of the structure view spec).
export function roles(a: TopologyAgent, m: TopologyModel): string {
  const parts: string[] = []
  if (m.project.orchestration === a.id) parts.push('orchestration agent')
  if (a.partition) {
    const writable = a.partition.folders.filter((f) => !f.read_only)
    parts.push(`owns ${writable.map((f) => placeOf(f, m)).join(', ') || 'no writable folder'}`)
  }
  if (!parts.length) parts.push('has no partition')
  return parts.join(', ')
}

// partitionsIn returns the agents with a folder in a repo.
export const partitionsIn = (repo: string, m: TopologyModel) => m.agents.filter((a) => a.partition?.folders.some((f) => f.repo === repo))

// repoName returns the name of the repo with an id: project for the project
// repository.
export const repoName = (id: string, m: TopologyModel) => (id === 'project' ? 'project' : (m.repos.find((r) => r.id === id)?.name ?? id))

// placeOf returns where a folder mounts, relative to /workspace: its at,
// or the repo's name and its path (decision 1 of the partition folders
// spec).
export function placeOf(f: TopologyFolder, m: TopologyModel): string {
  if (f.at) return f.at
  return f.path ? `${repoName(f.repo, m)}/${f.path}` : repoName(f.repo, m)
}

// partitionAgents returns the agents that have a partition.
export const partitionAgents = (m: TopologyModel) => m.agents.filter((a) => a.partition)
