import { describe, expect, it } from 'vitest'
import { doc, wikiOf } from './fixtures'
import { linkTarget } from './wiki'
import { orderDocs, template, userDocProblem, wikiIndex, wikiName } from './wikis'

describe('wikis', () => {
  it('finds documents and their backlinks across wikis (N6)', () => {
    const hub = wikiOf('hub:shop', [doc('order-items', 'story', { links: ['create-order', 'ghost', 'order-items'] })])
    const sys = wikiOf('system:orders', [doc('create-order'), doc('Ideas', 'note', { links: ['create-order', 'create-order'] })])
    const index = wikiIndex([hub, sys])
    expect(index.byID.get('create-order')!.wiki.ref).toBe('system:orders')
    expect(index.backlinks.get('create-order')!.map((b) => b.doc.id)).toEqual(['Ideas', 'order-items'])
    expect(index.backlinks.get('order-items')).toBeUndefined()
  })

  it('orders a list with key stories first (U7)', () => {
    const docs = [doc('n', 'note'), doc('b', 'story'), doc('a', 'story', { key: true }), doc('r', 'rule'), doc('f')]
    expect(orderDocs(docs).map((d) => d.id)).toEqual(['a', 'b', 'r', 'f', 'n'])
  })

  it('names wikis, checks names, and makes templates (D2, U6)', () => {
    expect(wikiName('hub:shop')).toBe('#shop')
    expect(wikiName('system:orders')).toBe('orders')
    expect(userDocProblem('Ideas for Q1')).toBeNull()
    for (const bad of ['', ' x', '.x', 'a/b', 'a:b', 'a?b', 'x'.repeat(201)]) expect(userDocProblem(bad)).not.toBeNull()
    expect(template('plan', 'Roadmap')).toBe('---\nkind: plan\nstatus: idea\n---\n\n')
    expect(template('story', 'order-items')).toContain('id: order-items\nkind: story\n')
    expect(linkTarget('systems/orders/wiki/create-order.md')).toBe('create-order')
  })
})
