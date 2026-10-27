import { describe, expect, it } from 'vitest'
import { initialState, reducer, type State } from './store'
import type { Message, Snapshot, SystemView } from './types'
import { allView } from './view'

const snapshot: Snapshot = {
  me: 'user',
  agents: [
    { name: 'lead', role: 'orchestrator', status: { state: 'idle' } },
    { name: 'dev', role: 'owner', status: { state: 'idle' } },
  ],
  participants: [
    { name: 'lead', role: 'orchestrator' },
    { name: 'dev', role: 'owner' },
    { name: 'user', role: 'operator' },
  ],
  messages: [],
  systems: [],
  vocabulary: { version: 1, terms: [], interfaces: [] },
}

let nextID = 1
const msg = (m: Partial<Message>): Message => ({ id: nextID++, from: 'dev', body: 'x', time: '2026-01-01T00:00:00Z', ...m })

function loaded(view = allView): State {
  return reducer(initialState(view), { type: 'event', event: { type: 'snapshot', snapshot } })
}

const receive = (s: State, m: Message) => reducer(s, { type: 'event', event: { type: 'message', message: m } })

describe('reducer', () => {
  it('keeps the version of the running hikari', () => {
    expect(reducer(initialState(allView), { type: 'event', event: { type: 'snapshot', snapshot: { ...snapshot, version: '0.0.1' } } }).version).toBe('0.0.1')
  })

  it('keeps the room view from the URL', () => {
    expect(loaded({ kind: 'room', tab: 'wiki' }).view).toEqual({ kind: 'room', tab: 'wiki' })
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
    s = receive(s, msg({}))
    s = receive(s, msg({ to: 'user' }))
    s = receive(s, msg({ from: 'lead', to: 'user' }))
    s = receive(s, msg({ from: 'lead', to: 'user' }))
    expect(s.unread).toEqual({ room: 1, 'dm:lead': 2 })
  })

  it('does not count messages from the user or between agents', () => {
    let s = loaded()
    s = receive(s, msg({ from: 'user' }))
    s = receive(s, msg({ from: 'dev', to: 'lead' }))
    expect(s.unread).toEqual({})
    expect(s.messages).toHaveLength(2)
  })

  it('clears the count of a view when it opens', () => {
    let s = receive(loaded(), msg({}))
    s = reducer(s, { type: 'view', view: { kind: 'room', tab: 'vocabulary' } })
    expect(s.unread).toEqual({})
  })

  it('updates the status of one agent', () => {
    const s = reducer(loaded(), { type: 'event', event: { type: 'status', node: 'dev', status: { state: 'working' } } })
    expect(s.agents.map((a) => a.status.state)).toEqual(['idle', 'working'])
  })
})

const systemView = (owner: string, state: 'unbuilt' | 'current' = 'unbuilt'): SystemView => ({
  version: 1,
  system: { name: owner, interfaces: { provides: [], consumes: [] }, described_state: {} },
  features: [],
  state,
  private: { layout: { version: 1, parts: [], features: {} }, tests: { version: 1, suites: {}, tests: {} } },
  releases: [],
  owner,
  repo: 'shop',
  scope: `services/${owner}`,
  branch: 'hikari/work',
  flags: null,
  providers: null,
})

describe('systems and the vocabulary', () => {
  const withSystems = () =>
    reducer(initialState({ kind: 'system', name: 'dev' }), {
      type: 'event',
      event: { type: 'snapshot', snapshot: { ...snapshot, systems: [systemView('dev')] } },
    })

  it('keeps a system view from the URL for a known system', () => {
    expect(withSystems().view).toEqual({ kind: 'system', name: 'dev' })
    const s = reducer(initialState({ kind: 'system', name: 'ghost' }), { type: 'event', event: { type: 'snapshot', snapshot } })
    expect(s.view).toEqual(allView)
  })

  it('keeps release and shared views for known systems only', () => {
    const release = { kind: 'release', name: 'dev', deliverable: 'lib', version: '1.0.0' } as const
    expect(reducer(withSystems(), { type: 'view', view: release }).view).toEqual(release)
    expect(reducer(withSystems(), { type: 'view', view: { ...release, name: 'ghost' } }).view).toEqual(allView)
    expect(reducer(withSystems(), { type: 'view', view: { kind: 'shared' } }).view).toEqual({ kind: 'shared' })
  })

  it('replaces a system map on a system event', () => {
    const s = reducer(withSystems(), { type: 'event', event: { type: 'system', system: systemView('dev', 'current') } })
    expect(s.systems).toHaveLength(1)
    expect(s.systems[0].state).toBe('current')
  })

  it('replaces the vocabulary on a vocabulary event', () => {
    const v = { version: 1, terms: [{ term: 'order', definition: 'x' }], interfaces: [] }
    const s = reducer(withSystems(), { type: 'event', event: { type: 'vocabulary', vocabulary: v } })
    expect(s.vocabulary).toEqual(v)
  })
})
