import type { Message } from './types'

export type HubTab = 'messages' | 'vocabulary'

export type View =
  | { kind: 'all' }
  | { kind: 'hub'; name: string; tab?: HubTab }
  | { kind: 'dm'; name: string }
  | { kind: 'system'; name: string }

export const allView: View = { kind: 'all' }

// viewKey names a conversation for unread counts. The tabs of a hub share
// one key.
export function viewKey(v: View): string {
  return v.kind === 'all' ? 'all' : `${v.kind}:${v.name}`
}

export function viewHash(v: View): string {
  if (v.kind === 'all') return '#/'
  if (v.kind === 'hub' && v.tab === 'vocabulary') return `#/hub/${v.name}/vocabulary`
  return `#/${v.kind}/${v.name}`
}

export function parseHash(hash: string): View {
  const hub = hash.match(/^#\/hub\/([\w-]+)\/vocabulary$/)
  if (hub) return { kind: 'hub', name: hub[1], tab: 'vocabulary' }
  const m = hash.match(/^#\/(hub|dm|system)\/([\w-]+)$/)
  return m ? { kind: m[1] as 'hub' | 'dm' | 'system', name: m[2] } : allView
}

// conversationKeys returns the views other than All activity that show m. A
// direct message between two agents is in no conversation of the user.
export function conversationKeys(m: Message, me: string): string[] {
  if (m.hub) return [`hub:${m.hub}`]
  if (m.from === me && m.to) return [`dm:${m.to}`]
  if (m.to === me) return [`dm:${m.from}`]
  return []
}

export function inView(m: Message, v: View, me: string): boolean {
  return v.kind === 'all' || conversationKeys(m, me).includes(viewKey(v))
}
