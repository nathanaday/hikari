import { describe, expect, it } from 'vitest'
import { conversationKeys, parseHash, viewHash, viewKey } from './view'

describe('view', () => {
  it('round-trips through the URL hash', () => {
    for (const v of [
      { kind: 'all' },
      { kind: 'hub', name: 'eng' },
      { kind: 'hub', name: 'eng', tab: 'vocabulary' },
      { kind: 'dm', name: 'eng-lead' },
      { kind: 'system', name: 'orders' },
      { kind: 'release', name: 'orders', version: '1.2.0-rc.1' },
      { kind: 'shared' },
    ] as const) {
      expect(parseHash(viewHash(v))).toEqual(v)
    }
    expect(parseHash('#/nonsense')).toEqual({ kind: 'all' })
    expect(viewKey({ kind: 'hub', name: 'eng', tab: 'vocabulary' })).toBe(viewKey({ kind: 'hub', name: 'eng' }))
  })

  it('files a message under its conversation', () => {
    const base = { id: 1, body: '', time: '' }
    expect(conversationKeys({ ...base, from: 'dev', hub: 'eng' }, 'user')).toEqual(['hub:eng'])
    expect(conversationKeys({ ...base, from: 'user', to: 'dev' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'user' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'lead' }, 'user')).toEqual([])
  })
})
