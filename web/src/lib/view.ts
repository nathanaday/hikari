import type { Message } from './types'

export type View = { kind: 'all' } | { kind: 'hub'; name: string } | { kind: 'dm'; name: string }

export const allView: View = { kind: 'all' }

export function viewKey(v: View): string {
  return v.kind === 'all' ? 'all' : `${v.kind}:${v.name}`
}

export function viewHash(v: View): string {
  return v.kind === 'all' ? '#/' : `#/${v.kind}/${v.name}`
}

export function parseHash(hash: string): View {
  const m = hash.match(/^#\/(hub|dm)\/([\w-]+)$/)
  return m ? { kind: m[1] as 'hub' | 'dm', name: m[2] } : allView
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
