import { describe, expect, it } from 'vitest'
import { mentionsIn, wakeTargets } from './rules'
import type { Hub } from './types'

const eng: Hub = {
  name: 'eng',
  participants: [
    { name: 'lead', role: 'orchestrator' },
    { name: 'critic', role: 'member' },
    { name: 'writer', role: 'member' },
    { name: 'user', role: 'operator' },
  ],
}

// The cases match TestWakeRules in internal/server/server_test.go.
describe('wakeTargets', () => {
  it.each([
    ['user post wakes only the orchestrator', 'user', 'hello team', ['lead']],
    ['a mention also wakes that member', 'user', '@writer draft it', ['lead', 'writer']],
    ['orchestrator post without mentions wakes all', 'lead', 'plan: write then review', ['critic', 'writer']],
    ['orchestrator post with a mention wakes that member', 'lead', '@critic review this', ['critic']],
    ['member post wakes the orchestrator', 'writer', 'draft done', ['lead']],
    ['member can wake another member', 'writer', '@critic please look', ['lead', 'critic']],
    ['mentions outside the hub do not count', 'lead', 'I report to @ceo', ['critic', 'writer']],
    ['an email address is not a mention', 'lead', 'mail a@writer.dev', ['critic', 'writer']],
  ])('%s', (_, from, body, want) => {
    expect(wakeTargets(eng, from, body).sort()).toEqual([...want].sort())
  })
})

describe('mentionsIn', () => {
  it('finds names after @ at a word start', () => {
    expect(mentionsIn('@a, (@b-2) x@c @@d')).toEqual(['a', 'b-2'])
  })
})
