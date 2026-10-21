import { describe, expect, it } from 'vitest'
import { conversationKeys, parseHash, viewHash } from './view'

describe('view', () => {
  it('round-trips through the URL hash', () => {
    for (const v of [{ kind: 'all' }, { kind: 'hub', name: 'eng' }, { kind: 'dm', name: 'eng-lead' }] as const) {
      expect(parseHash(viewHash(v))).toEqual(v)
    }
    expect(parseHash('#/nonsense')).toEqual({ kind: 'all' })
  })

  it('files a message under its conversation', () => {
    const base = { id: 1, body: '', time: '' }
    expect(conversationKeys({ ...base, from: 'dev', hub: 'eng' }, 'user')).toEqual(['hub:eng'])
    expect(conversationKeys({ ...base, from: 'user', to: 'dev' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'user' }, 'user')).toEqual(['dm:dev'])
    expect(conversationKeys({ ...base, from: 'dev', to: 'lead' }, 'user')).toEqual([])
  })
})
