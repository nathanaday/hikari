import { describe, expect, it } from 'vitest'
import { duration, partCosts, timeline, totalCost, totalTurns } from '@/lib/scorecard'
import type { Message, Scorecard, Turn } from '@/lib/types'

const sc: Scorecard = {
  started: '2026-11-04T09:00:00Z',
  finished: '2026-11-04T10:30:00Z',
  blocks: 1,
  refusals: 2,
  reviews: 1,
  parts: [
    { partition: 'orders', started: '2026-11-04T09:00:00Z', finished: '2026-11-04T10:00:00Z' },
    { partition: 'inventory', started: '2026-11-04T09:30:00Z' },
    { partition: 'web' },
  ],
  log: [
    { time: '2026-11-04T09:00:00Z', text: 'orders set the part of orders to started' },
    { time: '2026-11-04T10:00:00Z', text: 'orders set the part of orders to done: reviewed: clean' },
  ],
}

const turn = (agent: string, n: number, time: string, cost: number): Turn => ({ agent, turn: n, time, cost, duration: 1000 })

describe('the scorecard', () => {
  it('sums the turns of each partition agent inside its part, and to now while it is open', () => {
    const turns = [
      turn('orders', 1, '2026-11-04T08:50:00Z', 1), // before the part
      turn('orders', 2, '2026-11-04T09:10:00Z', 0.5),
      turn('orders', 3, '2026-11-04T09:50:00Z', 0.25),
      turn('orders', 4, '2026-11-04T10:10:00Z', 9), // after done
      turn('inventory', 1, '2026-11-04T09:40:00Z', 0.2),
      turn('inventory', 2, '2026-11-04T11:00:00Z', 0.3), // still open: counts up to now
      turn('lead', 1, '2026-11-04T09:20:00Z', 2), // the orchestration agent has no part
    ]
    const now = Date.parse('2026-11-04T12:00:00Z')
    const parts = partCosts(sc, turns, now)
    expect(parts).toEqual([
      { partition: 'orders', turns: 2, cost: 0.75, open: false },
      { partition: 'inventory', turns: 2, cost: 0.5, open: true },
      { partition: 'web', turns: 0, cost: 0, open: false },
    ])
    expect(totalCost(parts)).toBeCloseTo(1.25)
    expect(totalTurns(parts)).toBe(4)
    expect(partCosts(undefined, turns)).toEqual([])
  })

  it('writes the duration from start to finish, or to now', () => {
    expect(duration(sc)).toBe('1 h 30 min')
    expect(duration({ ...sc, finished: undefined }, Date.parse('2026-11-04T09:20:00Z'))).toBe('20 min')
    expect(duration({ ...sc, started: undefined })).toBe('')
    expect(duration({ ...sc, finished: '2026-11-04T09:00:30Z' })).toBe('under a minute')
  })

  it('merges the log with the decisions of the task and their answers, in time order', () => {
    const messages: Message[] = [
      { id: 1, from: 'hikari', to: 'user', body: 'q', time: '2026-11-04T09:05:00Z', decision: { question: 'Scoped with assumptions. Proceed?', options: ['Proceed', 'Change'], task: 'cancel', reply_to: 'lead' } },
      { id: 2, from: 'user', to: 'lead', body: '1. Proceed\nfine', time: '2026-11-04T09:06:00Z', answers: 1 },
      { id: 3, from: 'hikari', to: 'user', body: 'other', time: '2026-11-04T09:07:00Z', decision: { question: 'Other task?', options: ['a', 'b'], task: 'other' } },
    ]
    const entries = timeline(sc, messages, 'cancel')
    expect(entries.map((e) => e.kind)).toEqual(['log', 'decision', 'answer', 'log'])
    expect(entries[1].text).toBe('hikari asked: Scoped with assumptions. Proceed?')
    expect(entries[2].text).toBe('user answered: 1. Proceed')
  })
})
