import { describe, expect, it } from 'vitest'
import { agent, doc, feature, partition, wikiOf, withHealth } from './fixtures'
import { topologyGraph, wikiGraph, type GraphNode } from './graph'
import {
  activityPaint,
  age,
  docDetail,
  docPaint,
  healthCounts,
  legend,
  minAge,
  neutral,
  partitionHealth,
  topologyContext,
  topologyDetail,
  topologyPaint,
  wikiContext,
} from './lens'
import { wikiIndex } from './wikis'

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
  it('sums a partition from its features', () => {
    expect(partitionHealth([withHealth('a', 'passing'), withHealth('b', 'failing'), withHealth('c', 'pending')])).toBe('failing')
    expect(partitionHealth([withHealth('a', 'passing'), withHealth('c', 'pending')])).toBe('pending')
    expect(partitionHealth([withHealth('a', 'passing'), withHealth('b', 'untested')])).toBe('passing')
    expect(partitionHealth([withHealth('b', 'untested')])).toBe('untested')
    expect(partitionHealth([])).toBe('untested')
  })

  it('counts the features of each health', () => {
    expect(healthCounts([withHealth('a', 'passing'), withHealth('b', 'untested'), withHealth('c', 'passing')])).toBe('2 passing, 1 untested')
    expect(healthCounts([])).toBe('no features')
  })
})

describe('topology lenses', () => {
  const partitions = [
    partition('orders', [withHealth('a', 'failing')], { dirty: true, repos: [{ name: 'shop', branch: 'h', dirty: true }] }),
    partition('inventory', [withHealth('b', 'passing')], { repos: [{ name: 'shop', branch: 'h', dirty: false }] }),
    partition('docs', [], { repos: [{ name: 'site', branch: 'h', dirty: false }] }),
  ]
  const g = topologyGraph({
    partitions,
    repos: [
      { name: 'shop', branch: 'h', partitions: ['orders', 'inventory'] },
      { name: 'site', branch: 'h', partitions: ['docs'] },
    ],
    deliverables: [{ name: 'lib', holders: ['orders'] }],
  })
  const ctx = topologyContext(partitions, [agent('orders', 'working'), agent('inventory', 'idle'), agent('docs', 'exited')])
  const node = (id: string) => g.nodes.find((n) => n.id === id)!

  it('colors partitions under each lens', () => {
    expect(topologyPaint('activity', node('partition:orders'), ctx).fill).toBe('active')
    expect(topologyPaint('activity', node('partition:inventory'), ctx).fill).toBe('waiting')
    expect(topologyPaint('tests', node('partition:orders'), ctx).fill).toBe('fail')
    expect(topologyPaint('tests', node('partition:inventory'), ctx).fill).toBe('pass')
    expect(topologyPaint('tests', node('partition:docs'), ctx).fill).toBe('none')
    expect(topologyPaint('git', node('partition:orders'), ctx).fill).toBe('dirty')
    expect(topologyPaint('git', node('partition:inventory'), ctx)).toEqual(neutral)
  })

  it('colors a repository only under git, and a deliverable never (U35)', () => {
    expect(topologyPaint('git', node('repo:shop'), ctx).fill).toBe('dirty')
    expect(topologyPaint('git', node('repo:site'), ctx)).toEqual(neutral)
    for (const lens of ['activity', 'tests'] as const) expect(topologyPaint(lens, node('repo:shop'), ctx)).toEqual(neutral)
    for (const lens of ['activity', 'tests', 'git'] as const) expect(topologyPaint(lens, node('deliverable:lib'), ctx)).toEqual(neutral)
  })

  it('says the lens detail in the hover card (U36)', () => {
    expect(topologyDetail('activity', node('partition:inventory'), ctx)).toBe('waiting')
    expect(topologyDetail('tests', node('partition:orders'), ctx)).toBe('1 failing')
    expect(topologyDetail('git', node('repo:shop'), ctx)).toBe('uncommitted changes')
    expect(topologyDetail('activity', node('deliverable:lib'), ctx)).toBe('')
  })
})

describe('wiki lenses', () => {
  const docs = [
    doc('old', 'feature', { created: '2026-10-01T00:00:00Z', health: 'passing' }),
    doc('mid', 'feature', { created: '2026-10-11T00:00:00Z' }),
    doc('new', 'rule', { created: '2026-10-21T00:00:00Z' }),
    doc('Roadmap', 'plan', { status: 'idea' }),
  ]
  const features = [feature('old'), feature('mid', { state: 'stale' })]
  const wiki = wikiOf('partition:inventory', docs)
  const g = wikiGraph(wiki, wikiIndex([wiki]))
  const ctx = wikiContext(g.nodes, (n) => docs.find((d) => d.id === n.ref), features)
  const node = (ref: string) => g.nodes.find((n) => n.ref === ref)!

  it('scales age between the oldest and the newest, and a user document is the oldest (U39, U4)', () => {
    expect(age(docs[0].created, ctx)).toBe(minAge)
    expect(age(docs[2].created, ctx)).toBe(1)
    expect(age(docs[1].created, ctx)).toBeCloseTo(minAge + (1 - minAge) / 2)
    expect(age(undefined, ctx)).toBe(minAge)
    expect(age('2026-10-01T00:00:00Z', { oldest: 5, newest: 5 })).toBe(1)
    expect(docPaint('age', node('new'), ctx)).toMatchObject({ fill: 'active', opacity: 1 })
  })

  it('colors each kind, and a user document lighter (U4)', () => {
    expect(docPaint('kind', node('old'), ctx)).toMatchObject({ fill: 'feature', opacity: 1 })
    expect(docPaint('kind', node('new'), ctx)).toMatchObject({ fill: 'rule' })
    expect(docPaint('kind', node('Roadmap'), ctx)).toMatchObject({ fill: 'plan', opacity: 0.45 })
    expect(docDetail('kind', node('Roadmap'), ctx)).toBe('plan · idea')
  })

  it('colors health or support, and dashes a stale feature under every lens (U40, U40a)', () => {
    expect(docPaint('tests', node('old'), ctx)).toMatchObject({ fill: 'pass', dashed: false })
    expect(docPaint('tests', node('mid'), ctx)).toMatchObject({ dashed: true })
    expect(docPaint('kind', node('mid'), ctx).dashed).toBe(true)
    expect(docPaint('tests', node('Roadmap'), ctx)).toEqual(neutral)
    expect(docDetail('tests', node('old'), ctx)).toBe('passing')
    expect(docDetail('age', node('mid'), ctx)).toBe('created 2026-10-11 · document stale')
    expect(docDetail('tests', node('Roadmap'), ctx)).toBe('no tests')
    const story: GraphNode = { id: 's', kind: 'doc', ref: 's', label: 's', shape: 'circle', radius: 5, docKind: 'story' }
    const withStory = wikiContext([story], () => doc('s', 'story', { support: 'failing' }), [])
    expect(docPaint('tests', story, withStory)).toMatchObject({ fill: 'fail' })
    expect(docDetail('tests', story, withStory)).toBe('support failing')
  })

  it('has a legend for every lens (U37)', () => {
    for (const lens of ['activity', 'tests', 'git', 'age', 'kind'] as const) expect(legend(lens).length).toBeGreaterThan(1)
  })
})
