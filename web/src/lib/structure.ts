import type { Finding, TopologyAgent, TopologyModel } from './types'

// An entity of the Structure tab: its kind and its id. The project has no
// id.
export type Selection = { kind: 'project' } | { kind: 'repo' | 'hub' | 'agent' | 'sidecar'; id: string } | { kind: 'add'; what: AddKind }
export type AddKind = 'repo' | 'hub' | 'agent' | 'sidecar'

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
  if (kind === 'add' && ['repo', 'hub', 'agent', 'sidecar'].includes(id)) return { kind: 'add', what: id as AddKind }
  if (['repo', 'hub', 'agent', 'sidecar'].includes(kind) && id) return { kind: kind as 'repo', id }
  return undefined
}

// exists reports whether the model still has the selected entity.
export function exists(s: Selection, m: TopologyModel): boolean {
  switch (s.kind) {
    case 'repo':
      return m.repos.some((r) => r.id === s.id)
    case 'hub':
      return m.hubs.some((h) => h.id === s.id)
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
      return ['name', 'x-hikari.name', 'x-hikari.project', 'x-hikari.store']
    case 'repo':
      return [`x-hikari.repos.${s.id}`]
    case 'hub':
      return [`x-hikari.hubs.${s.id}`]
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

// roles says what an agent does, by the names of its hubs (decision 2 of
// the structure view spec).
export function roles(a: TopologyAgent, m: TopologyModel): string {
  const hubName = (id: string) => m.hubs.find((h) => h.id === id)?.name ?? id
  const parts: string[] = []
  const orchestrates = m.hubs.filter((h) => h.orchestrator === a.id).map((h) => `#${h.name}`)
  const member = m.hubs.filter((h) => h.members.includes(a.id)).map((h) => `#${h.name}`)
  if (orchestrates.length) parts.push(`orchestrates ${orchestrates.join(', ')}`)
  if (member.length) parts.push(`member of ${member.join(', ')}`)
  if (a.system) parts.push(`owns ${m.repos.find((r) => r.id === a.system!.repo)?.name ?? a.system.repo}${a.system.scope ? '/' + a.system.scope : ''}`)
  if (a.tests) parts.push(`tests #${hubName(a.tests)}`)
  if (!parts.length) parts.push('in no hub')
  return parts.join(', ')
}

// systemsIn returns the agents whose system is in a repo.
export const systemsIn = (repo: string, m: TopologyModel) => m.agents.filter((a) => a.system?.repo === repo)

// owners returns the agents that own a system.
export const owners = (m: TopologyModel) => m.agents.filter((a) => a.system)
