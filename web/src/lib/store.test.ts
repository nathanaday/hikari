import { describe, expect, it } from 'vitest'
import { initialState, reducer, type State } from './store'
import type { Message, Snapshot } from './types'
import { allView } from './view'

const snapshot: Snapshot = {
  me: 'user',
  agents: [
    { name: 'lead', hubs: ['eng'], status: { state: 'idle' } },
    { name: 'dev', hubs: ['eng'], status: { state: 'idle' } },
  ],
  hubs: [
    {
      name: 'eng',
      participants: [
        { name: 'lead', role: 'orchestrator' },
        { name: 'dev', role: 'member' },
        { name: 'user', role: 'operator' },
      ],
    },
  ],
  messages: [],
}

let nextID = 1
const msg = (m: Partial<Message>): Message => ({ id: nextID++, from: 'dev', body: 'x', time: '2026-01-01T00:00:00Z', ...m })

function loaded(view = allView): State {
  return reducer(initialState(view), { type: 'event', event: { type: 'snapshot', snapshot } })
}

const receive = (s: State, m: Message) => reducer(s, { type: 'event', event: { type: 'message', message: m } })

describe('reducer', () => {
  it('keeps a view from the URL that names a known hub', () => {
    expect(loaded({ kind: 'hub', name: 'eng' }).view).toEqual({ kind: 'hub', name: 'eng' })
  })

  it('falls back to All activity for an unknown view', () => {
    expect(loaded({ kind: 'dm', name: 'ghost' }).view).toEqual(allView)
  })

  it('accepts a null message list in the snapshot', () => {
    const s = reducer(initialState(allView), {
      type: 'event',
      event: { type: 'snapshot', snapshot: { ...snapshot, messages: null as unknown as Message[] } },
    })
    expect(s.messages).toEqual([])
  })

  it('counts unread messages per conversation, not for the open view', () => {
    let s = loaded({ kind: 'dm', name: 'dev' })
    s = receive(s, msg({ hub: 'eng' }))
    s = receive(s, msg({ to: 'user' }))
    s = receive(s, msg({ from: 'lead', to: 'user' }))
    s = receive(s, msg({ from: 'lead', to: 'user' }))
    expect(s.unread).toEqual({ 'hub:eng': 1, 'dm:lead': 2 })
  })

  it('does not count messages from the user or between agents', () => {
    let s = loaded()
    s = receive(s, msg({ from: 'user', hub: 'eng' }))
    s = receive(s, msg({ from: 'dev', to: 'lead' }))
    expect(s.unread).toEqual({})
    expect(s.messages).toHaveLength(2)
  })

  it('clears the count of a view when it opens', () => {
    let s = receive(loaded(), msg({ hub: 'eng' }))
    s = reducer(s, { type: 'view', view: { kind: 'hub', name: 'eng' } })
    expect(s.unread).toEqual({})
  })

  it('updates the status of one agent', () => {
    const s = reducer(loaded(), { type: 'event', event: { type: 'status', node: 'dev', status: { state: 'working' } } })
    expect(s.agents.map((a) => a.status.state)).toEqual(['idle', 'working'])
  })
})
