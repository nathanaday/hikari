import { describe, expect, it } from 'vitest'
import { feature, system } from './fixtures'
import { featureGraph, featureRadius, neighborsOf, systemRadius, topologyGraph } from './graph'

const shop = () => ({
  systems: [
    system('orders', [feature('a'), feature('b'), feature('c'), feature('d')], {
      grants: { releases: ['inventory'], deliverables: ['python-library'] },
      providers: { 'Stock.reserve': ['inventory'] },
    }),
    system('inventory', [feature('x')], { grants: { releases: [], deliverables: ['python-library'] }, providers: {} }),
  ],
  repos: [{ name: 'shop', branch: 'hikari/demo', systems: ['inventory', 'orders'] }],
  deliverables: [
    { name: 'python-library', holders: ['inventory', 'orders'] },
    { name: 'unheld', holders: [] },
  ],
})

describe('topologyGraph', () => {
  it('has a node for each system, repository, and held deliverable (U30)', () => {
    const g = topologyGraph(shop())
    expect(g.nodes.map((n) => n.id)).toEqual(['system:inventory', 'system:orders', 'repo:shop', 'deliverable:python-library'])
    expect(g.nodes.map((n) => n.shape)).toEqual(['circle', 'circle', 'square', 'diamond'])
    const orders = g.nodes.find((n) => n.id === 'system:orders')!
    expect(orders.radius).toBe(systemRadius(4))
    expect(systemRadius(0)).toBe(6)
    expect(systemRadius(1000)).toBe(18)
  })

  it('joins systems to repositories and holders, and systems to each other once (U31)', () => {
    const g = topologyGraph(shop())
    const edges = g.edges.map((e) => `${e.source} ${e.target}${e.dashed ? ' dashed' : ''}`)
    expect(edges).toEqual([
      'system:inventory repo:shop',
      'system:orders repo:shop',
      'deliverable:python-library system:inventory',
      'deliverable:python-library system:orders',
      'system:orders system:inventory dashed',
    ])
  })

  it('leaves out edges to systems that are not in the graph', () => {
    const input = shop()
    input.systems[0].grants!.releases = ['gone']
    input.systems[0].providers = { x: ['gone', 'orders'] }
    input.repos[0].systems.push('gone')
    expect(topologyGraph(input).edges.filter((e) => e.dashed || e.source.includes('gone') || e.target.includes('gone'))).toEqual([])
  })
})

describe('featureGraph', () => {
  it('has one node per feature and one edge per pair of neighbors (U38)', () => {
    const sys = system('inventory', [
      feature('reserve', { related: ['check', 'restock'] }),
      feature('check', { related: ['reserve'] }),
      feature('restock', { related: ['reserve', 'ghost', 'restock'] }),
      feature('lonely', { name: 'Lonely one' }),
    ])
    const g = featureGraph(sys)
    expect(g.nodes.map((n) => n.ref)).toEqual(['check', 'lonely', 'reserve', 'restock'])
    expect(g.nodes.find((n) => n.ref === 'lonely')!.label).toBe('Lonely one')
    expect(g.edges.map((e) => `${e.source} ${e.target}`)).toEqual(['feature:reserve feature:check', 'feature:reserve feature:restock'])
    expect(g.nodes.find((n) => n.ref === 'reserve')!.radius).toBe(featureRadius(2))
    expect(g.nodes.find((n) => n.ref === 'lonely')!.radius).toBe(5)
    expect(featureRadius(100)).toBe(14)
    expect([...neighborsOf(g, 'feature:reserve')].sort()).toEqual(['feature:check', 'feature:restock'])
  })
})
