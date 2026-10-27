import { describe, expect, it } from 'vitest'
import { doc, wikiOf } from './fixtures'
import { linkTarget } from './wiki'
import { orderDocs, template, userDocProblem, wikiIndex, wikiName } from './wikis'

describe('wikis', () => {
  it('finds documents and their backlinks across wikis (N6)', () => {
    const pub = wikiOf('public', [doc('order-items', 'story', { links: ['create-order', 'ghost', 'order-items'] })])
    const sys = wikiOf('partition:orders', [doc('create-order'), doc('Ideas', 'note', { links: ['create-order', 'create-order'] })])
    const index = wikiIndex([pub, sys])
    expect(index.byID.get('create-order')!.wiki.ref).toBe('partition:orders')
    expect(index.backlinks.get('create-order')!.map((b) => b.doc.id)).toEqual(['Ideas', 'order-items'])
    expect(index.backlinks.get('order-items')).toBeUndefined()
  })

  it('orders a list by kind, then by name (U7)', () => {
    const docs = [doc('n', 'note'), doc('b', 'story'), doc('a', 'story'), doc('r', 'rule'), doc('f')]
    expect(orderDocs(docs).map((d) => d.id)).toEqual(['a', 'b', 'r', 'f', 'n'])
  })

  it('names wikis, checks names, and makes templates (D2, U6)', () => {
    expect(wikiName('public')).toBe('the public wiki')
    expect(wikiName('partition:orders')).toBe('orders')
    expect(userDocProblem('Ideas for Q1')).toBeNull()
    for (const bad of ['', ' x', '.x', 'a/b', 'a:b', 'a?b', 'x'.repeat(201)]) expect(userDocProblem(bad)).not.toBeNull()
    expect(template('plan', 'Roadmap')).toBe('---\nkind: plan\nstatus: idea\n---\n\n')
    expect(template('story', 'order-items')).toContain('id: order-items\nkind: story\n')
    expect(linkTarget('systems/orders/wiki/create-order.md')).toBe('create-order')
  })
})
