import type { Message } from './types'

export type HubTab = 'messages' | 'vocabulary' | 'wiki' | 'tests'

export type TopologyTab = 'structure' | 'yaml'

export type View =
  | { kind: 'all' }
  | { kind: 'shared' }
  // topology is the compose file: its Structure tab, or its YAML tab, where
  // line moves the editor to a line.
  // select is the entity that the Structure tab shows, as kind:id.
  | { kind: 'topology'; tab?: TopologyTab; line?: number; select?: string }
  | { kind: 'hub'; name: string; tab?: HubTab }
  | { kind: 'dm'; name: string }
  | { kind: 'system'; name: string }
  | { kind: 'release'; name: string; deliverable: string; version: string }
  // map is the topology map, or with system or hub the wiki map of one
  // system or hub; doc selects one of its documents.
  | { kind: 'map'; system?: string; hub?: string; doc?: string }

export const allView: View = { kind: 'all' }

// viewKey names a conversation for unread counts. The tabs of a hub share
// one key.
export function viewKey(v: View): string {
  if (v.kind === 'all' || v.kind === 'shared' || v.kind === 'topology') return v.kind
  if (v.kind === 'map') return v.system ? `map:${v.system}` : v.hub ? `map:hub:${v.hub}` : 'map'
  if (v.kind === 'release') return `release:${v.name}:${v.deliverable}:${v.version}`
  return `${v.kind}:${v.name}`
}

export function viewHash(v: View): string {
  if (v.kind === 'all') return '#/'
  if (v.kind === 'shared') return '#/shared'
  if (v.kind === 'topology') return v.tab === 'yaml' ? '#/topology/yaml' : v.select ? `#/topology?select=${encodeURIComponent(v.select)}` : '#/topology'
  if (v.kind === 'map') {
    const doc = v.doc ? `/${encodeURIComponent(v.doc)}` : ''
    if (v.system) return `#/map/${v.system}${doc}`
    if (v.hub) return `#/hub/${v.hub}/map${doc}`
    return '#/map'
  }
  if (v.kind === 'hub' && v.tab) return `#/hub/${v.name}/${v.tab}`
  if (v.kind === 'release') return `#/system/${v.name}/release/${v.deliverable}/${v.version}`
  return `#/${v.kind}/${v.name}`
}

export function parseHash(hash: string): View {
  if (hash === '#/shared') return { kind: 'shared' }
  if (hash === '#/topology') return { kind: 'topology' }
  const select = hash.match(/^#\/topology\?select=([^&]+)$/)
  if (select) return { kind: 'topology', select: decodeURIComponent(select[1]) }
  if (hash === '#/topology/yaml') return { kind: 'topology', tab: 'yaml' }
  if (hash === '#/map') return { kind: 'map' }
  const map = hash.match(/^#\/map\/([\w-]+)(?:\/([^/]+))?$/)
  if (map) return map[2] ? { kind: 'map', system: map[1], doc: decodeURIComponent(map[2]) } : { kind: 'map', system: map[1] }
  const hubMap = hash.match(/^#\/hub\/([\w-]+)\/map(?:\/([^/]+))?$/)
  if (hubMap) return hubMap[2] ? { kind: 'map', hub: hubMap[1], doc: decodeURIComponent(hubMap[2]) } : { kind: 'map', hub: hubMap[1] }
  const release = hash.match(/^#\/system\/([\w-]+)\/release\/([\w-]+)\/([0-9A-Za-z.-]+)$/)
  if (release) return { kind: 'release', name: release[1], deliverable: release[2], version: release[3] }
  const hub = hash.match(/^#\/hub\/([\w-]+)\/(vocabulary|wiki|tests)$/)
  if (hub) return { kind: 'hub', name: hub[1], tab: hub[2] as HubTab }
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
