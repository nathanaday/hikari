import { describe, expect, it } from 'vitest'
import { agent, feature, system, withHealth } from './fixtures'
import { featureGraph, topologyGraph } from './graph'
import {
  activityPaint,
  age,
  featureContext,
  featureDetail,
  featurePaint,
  healthCounts,
  legend,
  minAge,
  neutral,
  systemHealth,
  topologyContext,
  topologyDetail,
  topologyPaint,
} from './lens'

describe('activity lens (U33)', () => {
  it('maps each status', () => {
    expect(activityPaint('working')).toMatchObject({ fill: 'active', pulse: true })
    expect(activityPaint('indexing')).toMatchObject({ fill: 'active', pulse: true })
    expect(activityPaint('idle')).toMatchObject({ fill: 'waiting', pulse: false })
    expect(activityPaint('starting')).toEqual(neutral)
    expect(activityPaint('exited')).toEqual(neutral)
    expect(activityPaint('error')).toMatchObject({ fill: 'white', stroke: 'fail' })
  })
})

describe('tests lens (U34, U40)', () => {
  it('sums a system from its features', () => {
    expect(systemHealth([withHealth('a', 'passing'), withHealth('b', 'failing'), withHealth('c', 'pending')])).toBe('failing')
    expect(systemHealth([withHealth('a', 'passing'), withHealth('c', 'pending')])).toBe('pending')
    expect(systemHealth([withHealth('a', 'passing'), withHealth('b', 'untested')])).toBe('passing')
    expect(systemHealth([withHealth('b', 'untested')])).toBe('untested')
    expect(systemHealth([])).toBe('untested')
  })

  it('counts the features of each health', () => {
    expect(healthCounts([withHealth('a', 'passing'), withHealth('b', 'untested'), withHealth('c', 'passing')])).toBe('2 passing, 1 untested')
    expect(healthCounts([])).toBe('no features')
  })
})

describe('topology lenses', () => {
  const systems = [
    system('orders', [withHealth('a', 'failing')], { dirty: true, repo: 'shop' }),
    system('inventory', [withHealth('b', 'passing')], { repo: 'shop' }),
    system('docs', [], { repo: 'site' }),
  ]
  const g = topologyGraph({
    systems,
    repos: [
      { name: 'shop', branch: 'h', systems: ['orders', 'inventory'] },
      { name: 'site', branch: 'h', systems: ['docs'] },
    ],
    deliverables: [{ name: 'lib', holders: ['orders'] }],
  })
  const ctx = topologyContext(systems, [agent('orders', 'working'), agent('inventory', 'idle'), agent('docs', 'exited')])
  const node = (id: string) => g.nodes.find((n) => n.id === id)!

  it('colors systems under each lens', () => {
    expect(topologyPaint('activity', node('system:orders'), ctx).fill).toBe('active')
    expect(topologyPaint('activity', node('system:inventory'), ctx).fill).toBe('waiting')
    expect(topologyPaint('tests', node('system:orders'), ctx).fill).toBe('fail')
    expect(topologyPaint('tests', node('system:inventory'), ctx).fill).toBe('pass')
    expect(topologyPaint('tests', node('system:docs'), ctx).fill).toBe('none')
    expect(topologyPaint('git', node('system:orders'), ctx).fill).toBe('dirty')
    expect(topologyPaint('git', node('system:inventory'), ctx)).toEqual(neutral)
  })

  it('colors a repository only under git, and a deliverable never (U35)', () => {
    expect(topologyPaint('git', node('repo:shop'), ctx).fill).toBe('dirty')
    expect(topologyPaint('git', node('repo:site'), ctx)).toEqual(neutral)
    for (const lens of ['activity', 'tests'] as const) expect(topologyPaint(lens, node('repo:shop'), ctx)).toEqual(neutral)
    for (const lens of ['activity', 'tests', 'git'] as const) expect(topologyPaint(lens, node('deliverable:lib'), ctx)).toEqual(neutral)
  })

  it('says the lens detail in the hover card (U36)', () => {
    expect(topologyDetail('activity', node('system:inventory'), ctx)).toBe('waiting')
    expect(topologyDetail('tests', node('system:orders'), ctx)).toBe('1 failing')
    expect(topologyDetail('git', node('repo:shop'), ctx)).toBe('uncommitted changes')
    expect(topologyDetail('activity', node('deliverable:lib'), ctx)).toBe('')
  })
})

describe('feature lenses', () => {
  const features = [
    feature('old', { created: '2026-10-01T00:00:00Z', health: 'passing', tests: { passed: 2, failed: 0, pending: 0 } }),
    feature('mid', { created: '2026-10-11T00:00:00Z', state: 'stale' }),
    feature('new', { created: '2026-10-21T00:00:00Z', state: 'undescribed' }),
    feature('unknown'),
  ]
  const ctx = featureContext(features)
  const g = featureGraph(system('inventory', features))
  const node = (ref: string) => g.nodes.find((n) => n.ref === ref)!

  it('scales age between the oldest and the newest (U39)', () => {
    expect(age(ctx.features.get('old'), ctx)).toBe(minAge)
    expect(age(ctx.features.get('new'), ctx)).toBe(1)
    expect(age(ctx.features.get('mid'), ctx)).toBeCloseTo(minAge + (1 - minAge) / 2)
    expect(age(ctx.features.get('unknown'), ctx)).toBe(minAge)
    const one = featureContext([feature('a', { created: '2026-10-01T00:00:00Z' }), feature('b', { created: '2026-10-01T00:00:00Z' })])
    expect(age(one.features.get('a'), one)).toBe(1)
    expect(featurePaint('age', node('new'), ctx)).toMatchObject({ fill: 'active', opacity: 1 })
  })

  it('colors health and dashes stale or undescribed documents under every lens (U40, U40a)', () => {
    expect(featurePaint('tests', node('old'), ctx)).toMatchObject({ fill: 'pass', dashed: false })
    expect(featurePaint('tests', node('mid'), ctx)).toMatchObject({ fill: 'none', dashed: true })
    expect(featurePaint('age', node('new'), ctx).dashed).toBe(true)
    expect(featureDetail('tests', node('old'), ctx)).toBe('passing · 2 passed, 0 failed, 0 pending')
    expect(featureDetail('age', node('mid'), ctx)).toBe('created 2026-10-11 · document stale')
    expect(featureDetail('age', node('unknown'), ctx)).toBe('creation date unknown')
  })

  it('has a legend for every lens (U37)', () => {
    for (const lens of ['activity', 'tests', 'git', 'age'] as const) expect(legend(lens).length).toBeGreaterThan(1)
  })
})
