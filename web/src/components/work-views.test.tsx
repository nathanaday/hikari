import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { WorkView } from '@/components/work-view'
import { PartitionDetail } from '@/components/partition-detail'
import { issue, partition, task } from '@/lib/fixtures'
import { initialState, reducer, type State } from '@/lib/store'
import type { TasksView } from '@/lib/types'
import { parseHash, viewHash, viewKey } from '@/lib/view'
import { entryView } from '@/lib/feed'

const cancel = task('shop-12', { name: 'Cancel orders', issue: 'shop#12', status: 'testing', partitions: ['orders', 'inventory'], parts: [{ partition: 'orders', status: 'testing' }, { partition: 'inventory', status: 'done' }], queue: 'api' })
const restock = task('restock', { name: 'Restock', status: 'waiting', wait: 'inventory is busy with [[shop-12]]', partitions: ['inventory'], queue: 'api' })
const scoping = task('shop-7', { name: 'Gift wrap', stage: 'stub', issue: 'shop#7', status: 'started', queue: 'api' })
const coupons = task('coupons', { name: 'Coupons', partitions: ['orders'] })

const work: TasksView = {
  issues: [
    issue('shop#12', { title: 'Cancel orders', labels: ['feature', 'hikari:working'], author: 'alex', comments: 2, url: 'https://github.com/acme/shop/issues/12', task: cancel }),
    issue('shop#7', { title: 'Gift wrap', task: scoping }),
    issue('shop#3', { title: 'Old bug', state: 'closed' }),
    issue('coupons', { title: 'Coupons', task: coupons, body: 'Let customers use a coupon.' }),
    issue('restock', { title: 'Restock', task: restock }),
  ],
  trackers: [{ repo: 'shop', kind: 'github', url: 'https://github.com/acme/shop', synced: '2026-11-04T12:00:00Z' }],
  tasks: [cancel, restock, scoping, coupons],
  queues: [
    { id: 'api', name: 'API', state: 'running', file: 'tasks/queues/api.md', scopes: [cancel, scoping, restock] },
    { id: 'ui', name: 'UI', state: 'paused', file: 'tasks/queues/ui.md', scopes: [] },
  ],
  problems: [{ file: 'tasks/Two Words.md', reason: 'the file’s name is not an id' }],
  orchestration: 'conductor',
  partitions: ['inventory', 'orders'],
  work_queue: 'work',
}

function stateWith(w: TasksView | undefined): State {
  return { ...initialState({ kind: 'work' }), loaded: true, modules: w ? { tasks: w } : {} }
}

const noop = () => {}
const send = async () => null

describe('WorkView', () => {
  it('shows the issues with their refs, labels, and tasks, and each queue with its scopes (UI4)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} send={send} />)
    for (const want of ['shop#12', 'Cancel orders', 'data-label="hikari:working"', 'by alex', '2 comments', 'Work on this', 'Plan', 'Open issue', 'API', 'running', 'Pause', 'UI', 'paused', 'Start', 'Waits: inventory is busy with [[shop-12]]', 'New queue', 'Two Words.md', 'data-tracker="shop"', 'synced']) {
      expect(html).toContain(want)
    }
    expect(html).toContain('data-status="testing"')
    expect(html).toContain('>orders testing<')
    expect(html).toContain('>inventory done<')
    // A task that the orchestration agent is scoping says so (IS3).
    expect(html).toContain('>scoping started<')
    // A queue card shows the issue's title, by its task's issue.
    const api = html.slice(html.indexOf('data-queue="api"'), html.indexOf('data-queue="ui"'))
    expect(api).toContain('Gift wrap')
    expect(api).toContain('shop#7')
    // Closed issues are filtered out by default, and local issues say so.
    expect(html).not.toContain('Old bug')
    expect(html).toContain('<code>local</code>')
  })

  it('makes an open issue draggable, and a queue a drop target (decision 12)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} send={send} />)
    const card = html.slice(html.indexOf('data-issue="shop#12"'), html.indexOf('data-issue="shop#7"'))
    expect(card).toContain('draggable="true"')
    expect(html).toContain('data-queue="ui"')
    expect(html).toContain('Drag an issue here')
  })

  it('lets you move only a task that has not started (TK11)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} send={send} />)
    const cancel = html.slice(html.indexOf('data-task="shop-12"'), html.indexOf('data-task="shop-7"'))
    const restock = html.slice(html.indexOf('data-task="restock"'))
    expect(cancel).not.toContain('Move down')
    expect(cancel).not.toContain('Take out')
    expect(restock).toContain('Move to queue')
    expect(restock).toContain('Take out')
  })

  it('opens an issue in its panel, with its task (UI5)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} issue="shop#12" send={send} />)
    expect(html).toContain('data-testid="issue-panel"')
    expect(html).toContain('Open on GitHub')
    expect(html).toContain('Set status')
    expect(html).toContain('tasks/shop-12.md')
    expect(html).toContain('>Close<')
    const local = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} issue="coupons" send={send} />)
    expect(local).toContain('>Remove<')
    expect(local).toContain('Let customers use a coupon.')
    expect(local).not.toContain('data-testid="issue-comments"')
  })
})

describe('the Work view and the module event', () => {
  it('routes #/work and #/work/<ref>, with the ref encoded', () => {
    expect(parseHash('#/work')).toEqual({ kind: 'work' })
    expect(parseHash('#/work/shop%2312')).toEqual({ kind: 'work', issue: 'shop#12' })
    expect(viewHash({ kind: 'work', issue: 'shop#12' })).toBe('#/work/shop%2312')
    expect(viewKey({ kind: 'work', issue: 'shop#12' })).toBe('work')
  })

  it('keeps the module view current, and leaves the Work view without the module (MO5)', () => {
    let s = reducer(initialState({ kind: 'work' }), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] }, modules: { tasks: work } } } })
    expect(s.view).toEqual({ kind: 'work' })
    s = reducer(s, { type: 'event', event: { type: 'module', module: 'tasks', view: { ...work, issues: [] } } })
    expect(s.modules.tasks?.issues).toEqual([])
    const plain = reducer(initialState({ kind: 'work' }), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] } } } })
    expect(plain.view).toEqual({ kind: 'all' })
  })

  it("opens a task's issue from the feed", () => {
    const s = stateWith(work)
    expect(entryView({ id: 1, time: 't', kind: 'module', who: 'hikari', text: 'x', task: 'shop-12' }, s)).toEqual({ kind: 'work', issue: 'shop#12' })
    expect(entryView({ id: 1, time: 't', kind: 'module', who: 'hikari', text: 'x', task: 'coupons' }, s)).toEqual({ kind: 'work', issue: 'coupons' })
  })
})

describe('PartitionDetail', () => {
  it('lists the scopes that name the partition, linked by issue (TK29)', () => {
    const s = { ...stateWith(work), agents: [{ name: 'orders', role: 'partition' as const, status: { state: 'idle' as const } }] }
    const html = renderToStaticMarkup(<PartitionDetail state={s} partition={partition('orders')} />)
    expect(html).toContain('data-testid="partition-scopes"')
    expect(html).toContain('Cancel orders')
    expect(html).toContain('#/work/shop%2312')
    expect(html).toContain('Coupons')
    expect(html).not.toContain('Restock')
    expect(html).toContain('#/repos/shop')
  })
})
