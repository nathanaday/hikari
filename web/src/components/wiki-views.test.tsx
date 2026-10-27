import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { PublicWiki } from '@/components/public-wiki'
import { DocumentPanel, NewDocument } from '@/components/wiki-panel'
import { doc, wikiOf } from '@/lib/fixtures'
import { wikiIndex } from '@/lib/wikis'

const wiki = wikiOf('public', [
  doc('Roadmap', 'plan', { status: 'idea', description: 'Ranked choice.' }),
  doc('ballots-decide-validity', 'principle', { name: 'Ballots decides validity' }),
  doc('ballots-decide-winner', 'rule', { name: 'Raw ballots decide a winner', support: 'passing', links: ['count-votes'] }),
])
const tally = wikiOf('partition:tally', [doc('count-votes', 'feature', { name: 'Count votes', health: 'passing' })])

describe('PublicWiki', () => {
  it('lists the documents by kind (U7)', () => {
    const html = renderToStaticMarkup(<PublicWiki orchestration="lead" wiki={wiki} show={() => {}} />)
    const order = ['Raw ballots decide a winner', 'Ballots decides validity', 'Roadmap'].map((t) => html.indexOf(t))
    expect(order.every((i, n) => i > 0 && (n === 0 || i > order[n - 1]))).toBe(true)
    expect(html).toContain('which lead writes')
    expect(html).not.toContain('stor')
    expect(html).toContain('· idea')
    expect(html).toContain('passing')
  })

  it('says when the wiki is empty', () => {
    expect(renderToStaticMarkup(<PublicWiki wiki={undefined} show={() => {}} />)).toContain('The public wiki has no documents yet')
  })
})

describe('DocumentPanel', () => {
  it('shows the kind, the support, the actions, and the backlinks (U5, U6)', () => {
    const index = wikiIndex([wiki, tally])
    const rule = renderToStaticMarkup(<DocumentPanel wiki={wiki} doc={wiki.documents[2]} index={index} onLink={() => {}} />)
    for (const want of ['Raw ballots decide a winner', 'ballots-decide-winner · the public wiki', 'rule', 'support', 'Edit']) expect(rule).toContain(want)
    expect(rule).not.toContain('key')
    expect(rule).not.toContain('>Remove<')
    const feature = renderToStaticMarkup(<DocumentPanel wiki={tally} doc={tally.documents[0]} index={index} onLink={() => {}} />)
    expect(feature).toContain('Linked from')
    expect(feature).toContain('Raw ballots decide a winner')
    const plan = renderToStaticMarkup(<DocumentPanel wiki={wiki} doc={wiki.documents[0]} index={index} onLink={() => {}} />)
    expect(plan).toContain('>Remove<')
    expect(plan).toContain('status: idea')
  })
})

describe('NewDocument', () => {
  it('offers the kinds of the wiki (U6)', () => {
    const html = renderToStaticMarkup(<NewDocument wiki={wiki} onDone={() => {}} />)
    for (const k of ['rule', 'principle', 'plan', 'note']) expect(html).toContain(`>${k}<`)
    expect(html).not.toContain('>feature<')
    expect(html).not.toContain('>story<')
    const orchestration = renderToStaticMarkup(<NewDocument wiki={wikiOf('orchestration', [])} onDone={() => {}} />)
    for (const k of ['feature', 'rule', 'principle', 'plan', 'note']) expect(orchestration).toContain(`>${k}<`)
  })
})
