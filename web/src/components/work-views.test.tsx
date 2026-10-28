import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { WorkView } from '@/components/work-view'
import { PartitionDetail } from '@/components/partition-detail'
import { partition } from '@/lib/fixtures'
import { initialState, reducer, type State } from '@/lib/store'
import type { TaskView, TasksView } from '@/lib/types'
import { parseHash, viewHash, viewKey } from '@/lib/view'

function task(id: string, extra: Partial<TaskView> = {}): TaskView {
  return { id, name: id, stage: 'scope', partitions: [], features: [], needs: [], parts: [], file: `tasks/${id}.md`, hash: '1', ...extra }
}

const work: TasksView = {
  stubs: [task('gift-wrap', { stage: 'stub', name: 'Gift wrap', author: 'inventory' })],
  drafts: [task('coupons', { name: 'Coupons', partitions: ['orders'] })],
  queues: [
    {
      id: 'api',
      name: 'API',
      state: 'running',
      file: 'tasks/queues/api.md',
      scopes: [
        task('cancel', { name: 'Cancel orders', status: 'testing', partitions: ['orders', 'inventory'], parts: [{ partition: 'orders', status: 'testing' }, { partition: 'inventory', status: 'done' }] }),
        task('restock', { name: 'Restock', status: 'waiting', wait: 'inventory is busy with [[cancel]]', partitions: ['inventory'] }),
      ],
    },
    { id: 'ui', name: 'UI', state: 'paused', file: 'tasks/queues/ui.md', scopes: [] },
  ],
  problems: [{ file: 'tasks/Two Words.md', reason: 'the file’s name is not an id' }],
  orchestration: 'lead',
  partitions: ['inventory', 'orders'],
}

function stateWith(w: TasksView | undefined): State {
  return { ...initialState({ kind: 'work' }), loaded: true, modules: w ? { tasks: w } : {} }
}

const noop = () => {}
const send = async () => null

describe('WorkView', () => {
  it('shows the stubs, the drafts, and each queue with its scopes and statuses (TK27)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} send={send} />)
    for (const want of ['Gift wrap', 'by inventory', 'Scope', 'Coupons', 'Add to queue', 'API', 'running', 'Pause', 'UI', 'paused', 'Start', 'Cancel orders', 'Waits: inventory is busy with [[cancel]]', 'New queue', 'Two Words.md']) {
      expect(html).toContain(want)
    }
    expect(html).toContain('data-status="testing"')
    expect(html).toContain('>orders testing<')
    expect(html).toContain('>inventory done<')
  })

  it('lets you move only a scope that has not started (TK11)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} send={send} />)
    const cancel = html.slice(html.indexOf('data-task="cancel"'), html.indexOf('data-task="restock"'))
    const restock = html.slice(html.indexOf('data-task="restock"'))
    expect(cancel).not.toContain('Move down')
    expect(cancel).not.toContain('Take out')
    expect(restock).toContain('Move to queue')
    expect(restock).toContain('Take out')
  })

  it('opens a task in its panel (TK28)', () => {
    const html = renderToStaticMarkup(<WorkView state={stateWith(work)} show={noop} task="cancel" send={send} />)
    expect(html).toContain('data-testid="task-panel"')
    expect(html).toContain('Set status')
    expect(html).toContain('tasks/cancel.md')
  })
})

describe('the Work view and the module event', () => {
  it('routes #/work and #/work/<id>', () => {
    expect(parseHash('#/work')).toEqual({ kind: 'work' })
    expect(parseHash('#/work/cancel')).toEqual({ kind: 'work', task: 'cancel' })
    expect(viewHash({ kind: 'work', task: 'cancel' })).toBe('#/work/cancel')
    expect(viewKey({ kind: 'work', task: 'cancel' })).toBe('work')
  })

  it('keeps the module view current, and leaves the Work view without the module (MO5)', () => {
    let s = reducer(initialState({ kind: 'work' }), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] }, modules: { tasks: work } } } })
    expect(s.view).toEqual({ kind: 'work' })
    s = reducer(s, { type: 'event', event: { type: 'module', module: 'tasks', view: { ...work, stubs: [] } } })
    expect(s.modules.tasks?.stubs).toEqual([])
    const plain = reducer(initialState({ kind: 'work' }), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] } } } })
    expect(plain.view).toEqual({ kind: 'all' })
  })
})

describe('PartitionDetail', () => {
  it('lists the scopes that name the partition (TK29)', () => {
    const s = { ...stateWith(work), agents: [{ name: 'orders', role: 'partition' as const, status: { state: 'idle' as const } }] }
    const html = renderToStaticMarkup(<PartitionDetail state={s} partition={partition('orders')} />)
    expect(html).toContain('data-testid="partition-scopes"')
    expect(html).toContain('Cancel orders')
    expect(html).toContain('Coupons')
    expect(html).not.toContain('Restock')
  })
})
