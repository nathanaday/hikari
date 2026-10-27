import { describe, expect, it } from 'vitest'
import wake from './fixtures/wake.json'
import { mentionsIn, wakeTargets } from './rules'
import type { Peer } from './types'

// The cases are those of TestWakeRules in internal/server/server_test.go.
function room(name: string): Peer[] {
  const r = wake.rooms[name as keyof typeof wake.rooms]
  return [
    ...(r.orchestrator ? [{ name: r.orchestrator, role: 'orchestrator' as const }] : []),
    ...r.owners.map((n) => ({ name: n, role: 'owner' as const })),
    { name: 'user', role: 'operator' as const },
  ]
}

describe('wakeTargets', () => {
  it.each(wake.cases.map((c) => [c.name, c] as const))('%s', (_, c) => {
    const to = 'to' in c ? c.to : undefined
    expect(wakeTargets(room(c.room), c.from, c.body, to).sort()).toEqual([...c.woke].sort())
  })
})

describe('mentionsIn', () => {
  it('finds names after @ at a word start', () => {
    expect(mentionsIn('@a, (@b-2) x@c @@d')).toEqual(['a', 'b-2'])
  })
})
