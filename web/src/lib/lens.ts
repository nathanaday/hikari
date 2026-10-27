import type { GraphNode } from './graph'
import type { Agent, AgentState, DocKind, Feature, FeatureState, Health, PartitionView, WikiDoc } from './types'
import { docHealth } from './wikis'

export type TopologyLens = 'activity' | 'tests' | 'git'
export type WikiLens = 'kind' | 'tests' | 'age'
export type Lens = TopologyLens | WikiLens

export const topologyLenses: TopologyLens[] = ['activity', 'tests', 'git']
export const wikiLenses: WikiLens[] = ['kind', 'tests', 'age']

// Color is the name of a lens color token in index.css (rule U46), or
// white. outline is the gray outline of a neutral node. The kind colors
// are the kind lens's (rule U4 of the wikis spec).
export type Color = 'active' | 'waiting' | 'pass' | 'pending' | 'fail' | 'dirty' | 'none' | 'outline' | 'white' | DocColor
export type DocColor = 'feature' | 'story' | 'rule' | 'principle' | 'plan'

// Paint is how a lens draws one node: its fill and outline, how strong the
// fill is (age), whether the outline pulses, and whether it is dashed.
export type Paint = { fill: Color; stroke: Color; opacity: number; pulse: boolean; dashed: boolean }

export const neutral: Paint = { fill: 'white', stroke: 'outline', opacity: 1, pulse: false, dashed: false }
const filled = (fill: Color, extra: Partial<Paint> = {}): Paint => ({ ...neutral, fill, stroke: fill, ...extra })

// activityPaint colors a partition from its partition agent's status (rule U33).
export function activityPaint(state: AgentState | undefined): Paint {
  switch (state) {
    case 'working':
    case 'indexing':
      return filled('active', { pulse: true })
    case 'idle':
      return filled('waiting')
    case 'error':
      return { ...neutral, stroke: 'fail' }
    default:
      return neutral
  }
}

const healthColor: Record<Health, Color> = { failing: 'fail', pending: 'pending', passing: 'pass', untested: 'none' }

// partitionHealth sums a partition's features into one health (rule U34).
export function partitionHealth(features: Feature[]): Health {
  const has = (h: Health) => features.some((f) => f.health === h)
  if (has('failing')) return 'failing'
  if (has('pending')) return 'pending'
  if (has('passing')) return 'passing'
  return 'untested'
}

export function healthPaint(h: Health): Paint {
  return filled(healthColor[h])
}

// TopologyContext is what the topology lenses read.
export type TopologyContext = { partitions: Map<string, PartitionView>; agents: Map<string, Agent> }

export function topologyContext(partitions: PartitionView[], agents: Agent[]): TopologyContext {
  return { partitions: new Map(partitions.map((s) => [s.partition_agent, s])), agents: new Map(agents.map((a) => [a.name, a])) }
}

// topologyPaint gives a topology node its paint under a lens. Nodes that a
// lens says nothing about are neutral (rule C4 of the brainstorm, U35).
export function topologyPaint(lens: TopologyLens, node: GraphNode, ctx: TopologyContext): Paint {
  if (node.kind === 'partition') {
    const sys = ctx.partitions.get(node.ref)
    switch (lens) {
      case 'activity':
        return activityPaint(ctx.agents.get(node.ref)?.status.state)
      case 'tests':
        return healthPaint(partitionHealth(sys?.features ?? []))
      case 'git':
        return sys?.dirty ? filled('dirty') : neutral
    }
  }
  if (node.kind === 'orchestration' && lens === 'activity') return activityPaint(ctx.agents.get(node.ref)?.status.state)
  if (node.kind === 'repo' && lens === 'git') {
    const dirty = [...ctx.partitions.values()].some((s) => s.repo === node.ref && s.dirty)
    return dirty ? filled('dirty') : neutral
  }
  return neutral
}

// topologyDetail is the lens line of a topology node's hover card (rule
// U36).
export function topologyDetail(lens: TopologyLens, node: GraphNode, ctx: TopologyContext): string {
  if (node.kind === 'repo') {
    if (lens !== 'git') return ''
    return topologyPaint('git', node, ctx).fill === 'dirty' ? 'uncommitted changes' : 'clean'
  }
  if (node.kind !== 'partition' && (node.kind !== 'orchestration' || lens !== 'activity')) return ''
  const sys = ctx.partitions.get(node.ref)
  switch (lens) {
    case 'activity': {
      const status = ctx.agents.get(node.ref)?.status
      if (!status) return ''
      const word = status.state === 'idle' ? 'waiting' : status.state
      return status.detail ? `${word}: ${status.detail}` : word
    }
    case 'tests':
      return healthCounts(sys?.features ?? [])
    case 'git':
      return sys?.dirty ? 'uncommitted changes' : 'clean'
  }
}

// healthCounts names how many features have each health, in the order of
// the health rules.
export function healthCounts(features: Feature[]): string {
  if (features.length === 0) return 'no features'
  const order: Health[] = ['failing', 'pending', 'passing', 'untested']
  return order
    .map((h) => [h, features.filter((f) => f.health === h).length] as const)
    .filter(([, n]) => n > 0)
    .map(([h, n]) => `${n} ${h}`)
    .join(', ')
}

// WikiContext is what the wiki lenses read: every document by node, the
// state of each feature, and the range of the creation times on the map.
export type WikiContext = {
  docs: Map<string, WikiDoc>
  states: Map<string, FeatureState | undefined>
  oldest: number
  newest: number
}

// wikiContext reads the documents of the nodes of a wiki map. docOf finds a
// node's document; features gives the state of each feature.
export function wikiContext(nodes: GraphNode[], docOf: (n: GraphNode) => WikiDoc | undefined, features: Feature[]): WikiContext {
  const docs = new Map<string, WikiDoc>()
  for (const n of nodes) {
    const d = docOf(n)
    if (d) docs.set(n.id, d)
  }
  const times = [...docs.values()].map((d) => (d.created ? Date.parse(d.created) : NaN)).filter((t) => !Number.isNaN(t))
  return {
    docs,
    states: new Map(features.map((f) => [f.id, f.state])),
    oldest: times.length ? Math.min(...times) : NaN,
    newest: times.length ? Math.max(...times) : NaN,
  }
}

// The dimmest age a document can have, so that it stays visible.
export const minAge = 0.2

// age scales a document's creation time between the oldest and the newest
// on its map, from minAge to 1 (rule U39). With one distinct time every
// document is 1; a document with no time, such as a user document, is
// minAge (rule U4 of the wikis spec).
export function age(created: string | undefined, ctx: { oldest: number; newest: number }): number {
  const t = created ? Date.parse(created) : NaN
  if (Number.isNaN(t)) return minAge
  if (!(ctx.newest > ctx.oldest)) return 1
  return minAge + (1 - minAge) * ((t - ctx.oldest) / (ctx.newest - ctx.oldest))
}

const kindColor: Record<DocKind, Color> = { feature: 'feature', story: 'story', rule: 'rule', principle: 'principle', plan: 'plan', note: 'none' }

// kindPaint colors a document by its kind. A user document is lighter, so
// that the agents' documents stand out.
export function kindPaint(kind: DocKind): Paint {
  const user = kind === 'plan' || kind === 'note'
  return filled(kindColor[kind], user ? { opacity: 0.45 } : {})
}

// docPaint gives a document its paint under a lens. A stale or undescribed
// feature is dashed under every lens (rule U40a).
export function docPaint(lens: WikiLens, node: GraphNode, ctx: WikiContext): Paint {
  const d = ctx.docs.get(node.id)
  if (!d) return neutral
  const state = d.kind === 'feature' && !node.foreign ? ctx.states.get(d.id) : undefined
  const dashed = state === 'stale' || state === 'undescribed'
  let paint: Paint
  switch (lens) {
    case 'kind':
      paint = kindPaint(d.kind)
      break
    case 'tests': {
      const h = docHealth(d)
      paint = h ? healthPaint(h) : neutral
      break
    }
    case 'age':
      paint = filled('active', { opacity: age(d.created, ctx) })
      break
  }
  return { ...paint, dashed }
}

// docDetail is the lens line of a document's hover card.
export function docDetail(lens: WikiLens, node: GraphNode, ctx: WikiContext): string {
  const d = ctx.docs.get(node.id)
  if (!d) return ''
  const state = d.kind === 'feature' && !node.foreign ? ctx.states.get(d.id) : undefined
  const stale = state === 'stale' || state === 'undescribed' ? ` · document ${state}` : ''
  switch (lens) {
    case 'kind':
      return [d.kind, d.status ?? ''].filter(Boolean).join(' · ') + stale
    case 'tests':
      if (d.health) return d.health + stale
      if (d.support) return `support ${d.support}`
      return 'no tests'
    case 'age':
      return (d.created ? `created ${d.created.slice(0, 10)}` : 'creation date unknown') + stale
  }
}

// Legend lists the colors of a lens and what each means (rule U37).
export type LegendEntry = { paint: Paint; label: string }

export function legend(lens: Lens): LegendEntry[] {
  switch (lens) {
    case 'activity':
      return [
        { paint: activityPaint('working'), label: 'working' },
        { paint: activityPaint('idle'), label: 'waiting' },
        { paint: activityPaint('exited'), label: 'not running' },
        { paint: activityPaint('error'), label: 'error' },
      ]
    case 'tests':
      return [
        { paint: healthPaint('failing'), label: 'failing' },
        { paint: healthPaint('pending'), label: 'pending' },
        { paint: healthPaint('passing'), label: 'passing' },
        { paint: healthPaint('untested'), label: 'untested' },
      ]
    case 'git':
      return [
        { paint: filled('dirty'), label: 'uncommitted changes' },
        { paint: neutral, label: 'clean' },
      ]
    case 'age':
      return [
        { paint: filled('active'), label: 'newest' },
        { paint: filled('active', { opacity: minAge }), label: 'oldest or unknown' },
      ]
    case 'kind':
      return (['story', 'rule', 'principle', 'feature', 'plan', 'note'] as DocKind[]).map((k) => ({ paint: kindPaint(k), label: k }))
  }
}
