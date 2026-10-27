import { describe, expect, it } from 'vitest'
import { doc, feature, partition, wikiOf } from './fixtures'
import { docNodeID, featureRadius, neighborsOf, partitionRadius, topologyGraph, wikiGraph } from './graph'
import { wikiIndex } from './wikis'

const shop = () => ({
  partitions: [
    partition('orders', [feature('a'), feature('b'), feature('c'), feature('d')], {
      grants: { releases: ['inventory'], deliverables: ['python-library'], skills: [] },
      providers: { 'Stock.reserve': ['inventory'] },
    }),
    partition('inventory', [feature('x')], { grants: { releases: [], deliverables: ['python-library'], skills: [] }, providers: {} }),
  ],
  repos: [{ name: 'shop', branch: 'hikari/demo', partitions: ['inventory', 'orders'] }],
  deliverables: [
    { name: 'python-library', holders: ['inventory', 'orders'] },
    { name: 'unheld', holders: [] },
  ],
})

describe('topologyGraph', () => {
  it('has a node for each partition, repository, and held deliverable (U30)', () => {
    const g = topologyGraph(shop())
    expect(g.nodes.map((n) => n.id)).toEqual(['partition:inventory', 'partition:orders', 'repo:shop', 'deliverable:python-library'])
    expect(g.nodes.map((n) => n.shape)).toEqual(['circle', 'circle', 'square', 'diamond'])
    const orders = g.nodes.find((n) => n.id === 'partition:orders')!
    expect(orders.radius).toBe(partitionRadius(4))
    expect(partitionRadius(0)).toBe(6)
    expect(partitionRadius(1000)).toBe(18)
  })

  it('joins partitions to repositories and holders, and partitions to each other once (U31)', () => {
    const g = topologyGraph(shop())
    const edges = g.edges.map((e) => `${e.source} ${e.target}${e.dashed ? ' dashed' : ''}`)
    expect(edges).toEqual([
      'partition:inventory repo:shop',
      'partition:orders repo:shop',
      'deliverable:python-library partition:inventory',
      'deliverable:python-library partition:orders',
      'partition:orders partition:inventory dashed',
    ])
  })

  it('leaves out edges to partitions that are not in the graph', () => {
    const input = shop()
    input.partitions[0].grants!.releases = ['gone']
    input.partitions[0].providers = { x: ['gone', 'orders'] }
    input.repos[0].partitions.push('gone')
    expect(topologyGraph(input).edges.filter((e) => e.dashed || e.source.includes('gone') || e.target.includes('gone'))).toEqual([])
  })
})

describe('topologyGraph with an orchestration agent', () => {
  it('draws the orchestration agent as a hexagon joined to every partition (decision 5)', () => {
    const g = topologyGraph({ ...shop(), orchestration: 'lead' })
    const lead = g.nodes.filter((n) => n.kind === 'orchestration')
    expect(lead.map((n) => [n.id, n.shape, n.label])).toEqual([['orchestration:lead', 'hexagon', 'lead']])
    const edges = g.edges.filter((e) => e.source.startsWith('orchestration:')).map((e) => `${e.source} ${e.target}`)
    expect(edges).toEqual(['orchestration:lead partition:inventory', 'orchestration:lead partition:orders'])
    expect(g.edges.filter((e) => e.dashed)).toHaveLength(1)
  })
})

describe('wikiGraph', () => {
  const orders = wikiOf('partition:orders', [
    doc('create-order', 'feature', { links: ['validate-order'] }),
    doc('validate-order', 'feature'),
    doc('never-oversell', 'rule', { links: ['create-order', 'stock-never-negative'] }),
    doc('Ideas', 'note', { links: ['create-order'] }),
  ])
  const pub = wikiOf('public', [
    doc('order-items', 'story', { name: 'Order items', links: ['create-order', 'reserve-stock'] }),
    doc('stock-never-negative', 'rule'),
  ])
  const inventory = wikiOf('partition:inventory', [doc('reserve-stock', 'feature')])
  const index = wikiIndex([pub, orders, inventory])

  it('has a node per document with the shape of its kind (U2, U3)', () => {
    const g = wikiGraph(orders, index)
    const own = g.nodes.filter((n) => !n.foreign)
    expect(own.map((n) => [n.ref, n.shape])).toEqual([
      ['create-order', 'circle'],
      ['Ideas', 'circle'],
      ['never-oversell', 'square'],
      ['validate-order', 'circle'],
    ])
    expect(own.find((n) => n.ref === 'Ideas')!.radius).toBe(5)
    expect(own.find((n) => n.ref === 'create-order')!.radius).toBe(featureRadius(4))
  })

  it('adds the documents of other wikis that link it or that it links (U2)', () => {
    const g = wikiGraph(orders, index)
    const foreign = g.nodes.filter((n) => n.foreign).map((n) => [n.id, n.label, n.radius])
    expect(foreign).toEqual([
      [docNodeID('public', 'order-items'), 'Order items · the public wiki', 4],
      [docNodeID('public', 'stock-never-negative'), 'stock-never-negative · the public wiki', 4],
    ])
    expect([...neighborsOf(g, docNodeID('partition:orders', 'create-order'))].sort()).toEqual([
      docNodeID('partition:orders', 'Ideas'),
      docNodeID('partition:orders', 'never-oversell'),
      docNodeID('partition:orders', 'validate-order'),
      docNodeID('public', 'order-items'),
    ])
  })

  it('draws the stories of the public wiki with the features of the partitions', () => {
    const g = wikiGraph(pub, index)
    expect(g.nodes.filter((n) => n.foreign).map((n) => n.ref).sort()).toEqual(['create-order', 'never-oversell', 'reserve-stock'])
    expect(g.edges.length).toBe(3)
  })
})
