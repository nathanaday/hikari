import type { Message, Scorecard, Turn } from './types'

// The scorecard of a scope (the scorecard of the task pipeline): what the
// task's log says of how the work went, with the turns and the cost of each
// partition agent inside its part's window, summed from the run's turns.

export type PartCost = { partition: string; turns: number; cost: number; open: boolean }

// partCosts sums, for each part of a scope, the turns of its partition agent
// from when the part started to when it was done, or to now while it is
// open. A part that has not started has no turns.
export function partCosts(sc: Scorecard | undefined, turns: Turn[], now = Date.now()): PartCost[] {
  if (!sc) return []
  return sc.parts.map((w) => {
    if (!w.started) return { partition: w.partition, turns: 0, cost: 0, open: false }
    const from = Date.parse(w.started)
    const to = w.finished ? Date.parse(w.finished) : now
    let n = 0
    let cost = 0
    for (const t of turns) {
      if (t.agent !== w.partition) continue
      const at = Date.parse(t.time)
      if (at >= from && at <= to) {
        n++
        cost += t.cost
      }
    }
    return { partition: w.partition, turns: n, cost, open: !w.finished }
  })
}

export function totalCost(parts: PartCost[]): number {
  return parts.reduce((sum, p) => sum + p.cost, 0)
}

export function totalTurns(parts: PartCost[]): number {
  return parts.reduce((sum, p) => sum + p.turns, 0)
}

// duration writes the time from a scope's start to its finish, or to now.
export function duration(sc: Scorecard | undefined, now = Date.now()): string {
  if (!sc?.started) return ''
  const ms = (sc.finished ? Date.parse(sc.finished) : now) - Date.parse(sc.started)
  if (ms < 0) return ''
  const m = Math.floor(ms / 60000)
  if (m < 1) return 'under a minute'
  if (m < 60) return `${m} min`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h} h ${m % 60} min`
  return `${Math.floor(h / 24)} d`
}

export type TimelineEntry = { time: string; kind: 'log' | 'decision' | 'answer'; text: string; message?: Message }

// timeline merges a scope's log with the decisions that named it and their
// answers, oldest first.
export function timeline(sc: Scorecard | undefined, messages: Message[], taskID: string): TimelineEntry[] {
  const out: TimelineEntry[] = []
  for (const l of sc?.log ?? []) if (l.time) out.push({ time: l.time, kind: 'log', text: l.text })
  const asked = new Map<number, Message>()
  for (const m of messages) {
    if (m.decision?.task === taskID) {
      asked.set(m.id, m)
      out.push({ time: m.time, kind: 'decision', text: `${m.from} asked: ${m.decision.question}`, message: m })
    }
  }
  for (const m of messages) {
    if (m.answers && asked.has(m.answers)) out.push({ time: m.time, kind: 'answer', text: `${m.from} answered: ${m.body.split('\n')[0]}`, message: m })
  }
  return out.sort((a, b) => Date.parse(a.time) - Date.parse(b.time))
}

export function money(n: number): string {
  return `$${n.toFixed(2)}`
}
