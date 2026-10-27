import { describe, expect, it } from 'vitest'
import { conversationKeys, parseHash, viewHash, viewKey } from './view'

describe('view', () => {
  it('round-trips through the URL hash', () => {
    for (const v of [
      { kind: 'all' },
      { kind: 'room' },
      { kind: 'room', tab: 'vocabulary' },
      { kind: 'dm', name: 'eng-lead' },
      { kind: 'partition', name: 'orders' },
      { kind: 'release', name: 'orders', deliverable: 'python-library', version: '1.2.0-rc.1' },
      { kind: 'shared' },
      { kind: 'map' },
      { kind: 'map', partition: 'orders' },
      { kind: 'map', partition: 'orders', doc: 'reserve-stock' },
      { kind: 'map', public: true },
      { kind: 'map', public: true, doc: 'Ideas for Q1' },
      { kind: 'map', orchestration: true },
      { kind: 'map', orchestration: true, doc: 'customer-orders' },
      { kind: 'room', tab: 'wiki' },
    ] as const) {
      expect(parseHash(viewHash(v))).toEqual(v)
    }
    expect(parseHash('#/nonsense')).toEqual({ kind: 'all' })
    expect(parseHash('#/team/eng')).toEqual({ kind: 'all' })
    expect(viewKey({ kind: 'room', tab: 'vocabulary' })).toBe(viewKey({ kind: 'room' }))
  })

  it('files a message under its conversation', () => {
    const base = { id: 1, body: '', time: '' }
    expect(conversationKeys({ ...base, from: 'dev' }, 'user')).toEqual(['room'])
    expect(conversationKeys({ ...base, from: 'user', to: 'dev' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'user' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'lead' }, 'user')).toEqual([])
  })
})
