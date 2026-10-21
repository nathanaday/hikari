import type { Hub } from './types'

// These functions repeat the wake rules of internal/server, so that the
// composer can show who a draft will wake before it is sent.

const mentionPattern = /(?<![\w@])@([a-z][a-z0-9-]*)/g

export function mentionsIn(body: string): string[] {
  return [...body.matchAll(mentionPattern)].map((m) => m[1])
}

export function orchestratorOf(hub: Hub): string {
  return hub.participants.find((p) => p.role === 'orchestrator')!.name
}

// wakeTargets returns the participants that a post from `from` will start a
// turn for.
export function wakeTargets(hub: Hub, from: string, body: string): string[] {
  const names = new Set(hub.participants.map((p) => p.name))
  const mentioned = new Set(mentionsIn(body).filter((n) => names.has(n)))
  const orchestrator = orchestratorOf(hub)
  return hub.participants
    .filter((p) => p.role !== 'operator' && p.name !== from)
    .filter(
      (p) =>
        p.name === orchestrator ||
        mentioned.has(p.name) ||
        (from === orchestrator && mentioned.size === 0),
    )
    .map((p) => p.name)
}
