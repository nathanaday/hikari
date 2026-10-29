import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { FeedView } from '@/components/feed-view'
import { entryView } from '@/lib/feed'
import { agent } from '@/lib/fixtures'
import { initialState, reducer, type State } from '@/lib/store'
import type { Activity, Entry } from '@/lib/types'
import { allView } from '@/lib/view'

let nextID = 1
const entry = (e: Partial<Entry> & { kind: Entry['kind']; text: string }): Entry => ({ id: nextID++, time: '2026-10-28T14:02:00Z', ...e })

const feed: Entry[] = [
  entry({ kind: 'step', text: 'Connected 3 agents' }),
  entry({ kind: 'message', who: 'user', text: '@lead plan the **coupons**\nplease', message: 1 }),
  entry({ kind: 'message', who: 'orders', to: 'inventory', text: 'a private word', message: 2 }),
  entry({ kind: 'status', who: 'orders', text: 'connected' }),
  entry({ kind: 'turn', who: 'orders', text: 'turn done · 3m · 2 files changed', turn: 1 }),
  entry({ kind: 'status', who: 'inventory', text: 'error: claude: boom', error: true }),
  entry({ kind: 'module', who: 'hikari', text: '[[cancel]] orders set the part of orders to done', task: 'cancel' }),
]

const turnOfOrders: Activity[] = [
  { id: 1, turn: 1, time: '2026-10-28T14:00:00Z', kind: 'turn', text: 'woken by user in the room', done: true },
  { id: 2, turn: 1, time: '2026-10-28T14:00:01Z', kind: 'done', done: true },
]

function stateWith(extra: Partial<State> = {}): State {
  return {
    ...initialState(allView),
    loaded: true,
    me: 'user',
    agents: [agent('lead', 'idle', 'orchestration'), agent('orders', 'working'), agent('inventory', 'error')],
    participants: [{ name: 'lead', role: 'orchestration' }, { name: 'orders', role: 'partition' }, { name: 'user', role: 'operator' }],
    feed,
    cost: 1.5,
    activity: {
      orders: [...turnOfOrders, { id: 3, turn: 2, time: '2026-10-28T14:03:00Z', kind: 'turn', done: true }, { id: 4, turn: 2, time: '2026-10-28T14:03:01Z', kind: 'tool', tool: 'Read', input: { file_path: 'a/b.go' }, done: false }],
    },
    ...extra,
  }
}

const noop = () => {}

describe('FeedView', () => {
  it('lists each entry as one line with its time, name, and text (FD8)', () => {
    const html = renderToStaticMarkup(<FeedView state={stateWith()} show={noop} />)
    expect(html.match(/data-testid="entry"/g)).toHaveLength(feed.length)
    for (const want of ['Connected 3 agents', '>you<', '→ room', '>orders<', '→ inventory', 'a private word', 'turn done · 3m · 2 files changed', 'error: claude: boom', '>hikari<', '[[cancel]] orders set the part of orders to done']) {
      expect(html).toContain(want)
    }
    // A message's text is one line until it opens.
    expect(html).toContain('@lead plan the **coupons** please')
    expect(html).not.toContain('<strong>coupons</strong>')
  })

  it('shows the strip: each agent with its step, and the cost (FD9)', () => {
    const html = renderToStaticMarkup(<FeedView state={stateWith()} show={noop} />)
    expect(html.match(/data-testid="strip-agent"/g)).toHaveLength(3)
    expect(html).toContain('Reading b.go')
    expect(html).toContain('$1.50 spent')
  })

  it('shows nothing yet on an empty feed', () => {
    const html = renderToStaticMarkup(<FeedView state={stateWith({ feed: [] })} show={noop} />)
    expect(html).toContain('Nothing yet')
  })
})

describe('entryView', () => {
  const s = stateWith()
  it('opens the conversation of a message, or nothing for a private one between agents (FD10)', () => {
    expect(entryView(feed[1], s)).toEqual({ kind: 'room' })
    expect(entryView(feed[2], s)).toBeUndefined()
    expect(entryView(entry({ kind: 'message', who: 'lead', to: 'user', text: 'x' }), s)).toEqual({ kind: 'dm', name: 'lead' })
    expect(entryView(entry({ kind: 'message', who: 'user', to: 'lead', text: 'x' }), s)).toEqual({ kind: 'dm', name: 'lead' })
  })

  it('opens the conversation that shows a turn, or the direct messages when the turn is gone', () => {
    expect(entryView(feed[4], s)).toEqual({ kind: 'room' })
    expect(entryView(entry({ kind: 'turn', who: 'orders', text: 'x', turn: 9 }), s)).toEqual({ kind: 'dm', name: 'orders' })
    expect(entryView(entry({ kind: 'turn', who: 'orders', text: 'x', turn: 1 }), stateWith({ activity: { orders: [{ ...turnOfOrders[0], text: 'woken by user directly' }, turnOfOrders[1]] } }))).toEqual({ kind: 'dm', name: 'orders' })
  })

  it('opens a task, an agent, and nothing for a step', () => {
    expect(entryView(feed[6], s)).toEqual({ kind: 'work', task: 'cancel' })
    expect(entryView(feed[3], s)).toEqual({ kind: 'dm', name: 'orders' })
    expect(entryView(feed[0], s)).toBeUndefined()
  })
})

describe('the store and the feed', () => {
  it('takes the feed and the cost from the snapshot, appends each entry, and adds each status cost (FD2, FD6)', () => {
    let s = reducer(initialState(allView), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [agent('orders')], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] }, feed: [feed[0]], cost: 0.5 } } })
    expect(s.feed).toHaveLength(1)
    expect(s.cost).toBe(0.5)
    s = reducer(s, { type: 'event', event: { type: 'feed', entry: feed[1] } })
    expect(s.feed.map((e) => e.id)).toEqual([feed[0].id, feed[1].id])
    s = reducer(s, { type: 'event', event: { type: 'status', agent: 'orders', status: { state: 'idle', cost: 0.25 } } })
    expect(s.cost).toBe(0.75)
    expect(s.unread).toEqual({})
  })
})
