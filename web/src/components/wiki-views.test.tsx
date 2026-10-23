import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { HubWiki } from '@/components/hub-wiki'
import { DocumentPanel, NewDocument } from '@/components/wiki-panel'
import { doc, wikiOf } from '@/lib/fixtures'
import { wikiIndex } from '@/lib/wikis'

const hub = { name: 'main', participants: [] }
const wiki = wikiOf('hub:main', [
  doc('Roadmap', 'plan', { status: 'idea', description: 'Ranked choice.' }),
  doc('ballots-decide-validity', 'principle', { name: 'Ballots decides validity' }),
  doc('ballots-decide-winner', 'story', { name: 'Raw ballots to a winner', key: true, support: 'passing', links: ['count-votes'] }),
])
const tally = wikiOf('system:tally', [doc('count-votes', 'feature', { name: 'Count votes', health: 'passing' })])

describe('HubWiki', () => {
  it('lists the documents by kind with key stories first (U7)', () => {
    const html = renderToStaticMarkup(<HubWiki hub={hub} wiki={wiki} show={() => {}} />)
    const order = ['Raw ballots to a winner', 'Ballots decides validity', 'Roadmap'].map((t) => html.indexOf(t))
    expect(order.every((i, n) => i > 0 && (n === 0 || i > order[n - 1]))).toBe(true)
    expect(html).toContain('aria-label="key story"')
    expect(html).toContain('· idea')
    expect(html).toContain('passing')
  })

  it('says when the wiki is empty', () => {
    expect(renderToStaticMarkup(<HubWiki hub={hub} wiki={undefined} show={() => {}} />)).toContain('has no documents yet')
  })
})

describe('DocumentPanel', () => {
  it('shows the kind, the key, the support, the actions, and the backlinks (U5, U6)', () => {
    const index = wikiIndex([wiki, tally])
    const story = renderToStaticMarkup(<DocumentPanel wiki={wiki} doc={wiki.documents[2]} index={index} onLink={() => {}} />)
    for (const want of ['Raw ballots to a winner', 'ballots-decide-winner · #main', 'story', 'support', 'Edit', 'Unmark key']) expect(story).toContain(want)
    expect(story).not.toContain('>Remove<')
    const feature = renderToStaticMarkup(<DocumentPanel wiki={tally} doc={tally.documents[0]} index={index} onLink={() => {}} />)
    expect(feature).toContain('Linked from')
    expect(feature).toContain('Raw ballots to a winner')
    const plan = renderToStaticMarkup(<DocumentPanel wiki={wiki} doc={wiki.documents[0]} index={index} onLink={() => {}} />)
    expect(plan).toContain('>Remove<')
    expect(plan).toContain('status: idea')
  })
})

describe('NewDocument', () => {
  it('offers the kinds of the wiki (U6)', () => {
    const html = renderToStaticMarkup(<NewDocument wiki={wiki} onDone={() => {}} />)
    for (const k of ['story', 'rule', 'principle', 'plan', 'note']) expect(html).toContain(`>${k}<`)
    expect(html).not.toContain('>feature<')
  })
})

describe('HubTests', () => {
  it('shows the stories with their health, the gaps, the plan, and the release set (UT1)', async () => {
    const { HubTests } = await import('@/components/hub-tests')
    const html = renderToStaticMarkup(
      <HubTests
        show={() => {}}
        plan={{
          hub: 'main',
          tester: 'vote-e2e',
          plan: {
            components: { tally: { release: 'tally/python-library' }, machine: { simulates: 'a voting machine' } },
            actors: { operator: { runs: 'tests', count: 1 } },
            scenarios: { smoke: { stories: ['ballots-decide-winner'], actors: { operator: 1 } } },
            gaps: null,
          },
          body: '## The test system\n\nThe libraries, installed from their releases.',
          lock: { version: 1, releases: [{ system: 'tally', deliverable: 'python-library', version: '0.1.0', digest: 'sha256:0123456789abcdef0123' }] },
          warnings: ['the key story "x" has no test and no gap'],
          bench: [{ id: 'npu', title: 'Runs on the NPU', touches: ['[[x]]'], procedure: '1. Flash the image.', result: { passed: true, note: 'Fast.', at: '2026-10-23T08:00:00Z', current: false } }],
          gaps: [{ id: 'real-machine', what: 'Ballots from a real machine', why: 'no machine', touches: ['[[accept-ballot]]'], approved: false }],
          environment: {
            state: 'running',
            scenario: 'smoke',
            connected: true,
            stops: '2026-10-23T08:00:00Z',
            services: [{ name: 'cloud', state: 'running', image: 'hikari/p-cloud-http-service:1.0.0', platform: 'linux/amd64', replicas: 1, emulated: true }],
          },
          stories: [
            { id: 'ballots-decide-winner', kind: 'story', name: 'Raw ballots to a winner', key: true, health: 'failing', passed: 0, failed: 1, pending: 0, support: 'passing', suspects: ['ballots'], gaps: ['real-machine'] },
          ],
        }}
      />,
    )
    for (const want of ['vote-e2e proves', 'Raw ballots to a winner', 'failing', 'suspects: ballots', 'gap proposed: Ballots from a real machine', 'Approve', 'release tally/python-library', 'a voting machine', '1 operator', 'tally/python-library', '0.1.0', 'has no test and no gap', 'The test system', 'scenario smoke', 'Stop', 'hikari/p-cloud-http-service:1.0.0 · linux/amd64 · emulated', 'Runs on the NPU', 'Flash the image', 'pending: the release set or the procedure changed', 'Note: Fast.', 'Passed', 'Failed']) {
      expect(html).toContain(want)
    }
  })
})
