import type { Peer } from './types'

// These functions repeat the wake rules of internal/server, so that the
// composer can show who a draft will wake before it is sent. The tests of
// both run the cases of fixtures/wake.json.

const mentionPattern = /(?<![\w@])@([a-z][a-z0-9-]*)/g

export function mentionsIn(body: string): string[] {
  return [...body.matchAll(mentionPattern)].map((m) => m[1])
}

// orchestrationOf returns the orchestration agent of the room, or undefined.
export function orchestrationOf(participants: Peer[]): string | undefined {
  return participants.find((p) => p.role === 'orchestration')?.name
}

// wakeTargets returns the agents that a message from `from` will start a
// turn for: a direct message wakes its recipient, and a post wakes the
// orchestration agent, every agent it mentions, and every agent when the
// orchestration agent, or anyone in a room without one, posts without mentions.
export function wakeTargets(participants: Peer[], from: string, body: string, to?: string): string[] {
  const agents = participants.filter((p) => p.role !== 'operator' && p.role !== 'hikari')
  if (to) return agents.some((p) => p.name === to) ? [to] : []
  const names = new Set(participants.map((p) => p.name))
  const mentioned = new Set(mentionsIn(body).filter((n) => names.has(n)))
  const orchestration = orchestrationOf(participants)
  return agents
    .filter((p) => p.name !== from)
    .filter((p) => mentioned.has(p.name) || p.name === orchestration || (mentioned.size === 0 && (!orchestration || from === orchestration)))
    .map((p) => p.name)
}
