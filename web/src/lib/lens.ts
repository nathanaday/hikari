import type { GraphNode } from './graph'
import type { Agent, AgentState, Feature, Health, SystemView } from './types'

export type TopologyLens = 'activity' | 'tests' | 'git'
export type FeatureLens = 'age' | 'tests'
export type Lens = TopologyLens | FeatureLens

export const topologyLenses: TopologyLens[] = ['activity', 'tests', 'git']
export const featureLenses: FeatureLens[] = ['age', 'tests']

// Color is the name of a lens color token in index.css (rule U46), or
// white. outline is the gray outline of a neutral node.
export type Color = 'active' | 'waiting' | 'pass' | 'pending' | 'fail' | 'dirty' | 'none' | 'outline' | 'white'

// Paint is how a lens draws one node: its fill and outline, how strong the
// fill is (age), whether the outline pulses, and whether it is dashed.
export type Paint = { fill: Color; stroke: Color; opacity: number; pulse: boolean; dashed: boolean }

export const neutral: Paint = { fill: 'white', stroke: 'outline', opacity: 1, pulse: false, dashed: false }
const filled = (fill: Color, extra: Partial<Paint> = {}): Paint => ({ ...neutral, fill, stroke: fill, ...extra })

// activityPaint colors a system from its owner's status (rule U33).
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

// systemHealth sums a system's features into one health (rule U34).
export function systemHealth(features: Feature[]): Health {
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
export type TopologyContext = { systems: Map<string, SystemView>; agents: Map<string, Agent> }

export function topologyContext(systems: SystemView[], agents: Agent[]): TopologyContext {
  return { systems: new Map(systems.map((s) => [s.owner, s])), agents: new Map(agents.map((a) => [a.name, a])) }
}

// topologyPaint gives a topology node its paint under a lens. Nodes that a
// lens says nothing about are neutral (rule C4 of the brainstorm, U35).
export function topologyPaint(lens: TopologyLens, node: GraphNode, ctx: TopologyContext): Paint {
  if (node.kind === 'system') {
    const sys = ctx.systems.get(node.ref)
    switch (lens) {
      case 'activity':
        return activityPaint(ctx.agents.get(node.ref)?.status.state)
      case 'tests':
        return healthPaint(systemHealth(sys?.features ?? []))
      case 'git':
        return sys?.dirty ? filled('dirty') : neutral
    }
  }
  if (node.kind === 'repo' && lens === 'git') {
    const dirty = [...ctx.systems.values()].some((s) => s.repo === node.ref && s.dirty)
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
  if (node.kind !== 'system') return ''
  const sys = ctx.systems.get(node.ref)
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

// FeatureContext is what the feature lenses read: the features by id, and
// the range of their creation times.
export type FeatureContext = { features: Map<string, Feature>; oldest: number; newest: number }

export function featureContext(features: Feature[]): FeatureContext {
  const times = features.map((f) => (f.created ? Date.parse(f.created) : NaN)).filter((t) => !Number.isNaN(t))
  return {
    features: new Map(features.map((f) => [f.id, f])),
    oldest: times.length ? Math.min(...times) : NaN,
    newest: times.length ? Math.max(...times) : NaN,
  }
}

// The dimmest age a feature can have, so that it stays visible.
export const minAge = 0.2

// age scales a feature's creation time between the oldest and the newest
// of its system, from minAge to 1 (rule U39). With one distinct time every
// feature is 1; a feature with no time is minAge.
export function age(f: Feature | undefined, ctx: FeatureContext): number {
  const t = f?.created ? Date.parse(f.created) : NaN
  if (Number.isNaN(t)) return minAge
  if (!(ctx.newest > ctx.oldest)) return 1
  return minAge + (1 - minAge) * ((t - ctx.oldest) / (ctx.newest - ctx.oldest))
}

// featurePaint gives a feature its paint under a lens. A stale or
// undescribed document is dashed under every lens (rule U40a).
export function featurePaint(lens: FeatureLens, node: GraphNode, ctx: FeatureContext): Paint {
  const f = ctx.features.get(node.ref)
  const dashed = f?.state === 'stale' || f?.state === 'undescribed'
  const paint = lens === 'age' ? filled('active', { opacity: age(f, ctx) }) : healthPaint(f?.health ?? 'untested')
  return { ...paint, dashed }
}

// featureDetail is the lens line of a feature's hover card.
export function featureDetail(lens: FeatureLens, node: GraphNode, ctx: FeatureContext): string {
  const f = ctx.features.get(node.ref)
  if (!f) return ''
  const state = f.state === 'stale' || f.state === 'undescribed' ? ` · document ${f.state}` : ''
  if (lens === 'age') {
    return (f.created ? `created ${f.created.slice(0, 10)}` : 'creation date unknown') + state
  }
  const t = f.tests
  return `${f.health} · ${t.passed} passed, ${t.failed} failed, ${t.pending} pending${state}`
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
  }
}
