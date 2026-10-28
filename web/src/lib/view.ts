import type { Message } from './types'

export type RoomTab = 'messages' | 'vocabulary' | 'wiki'

export type TopologyTab = 'structure' | 'yaml'

export type View =
  | { kind: 'all' }
  | { kind: 'shared' }
  // topology is the topology file: its Structure tab, or its YAML tab, where
  // line moves the editor to a line.
  // select is the entity that the Structure tab shows, as kind:id.
  | { kind: 'topology'; tab?: TopologyTab; line?: number; select?: string }
  | { kind: 'room'; tab?: RoomTab }
  | { kind: 'dm'; name: string }
  | { kind: 'partition'; name: string }
  | { kind: 'release'; name: string; deliverable: string; version: string }
  // map is the topology map, or the wiki map of one partition, or with public
  // the map of the public wiki, or with orchestration the orchestration
  // agent's feature map; doc selects one of its documents.
  | { kind: 'map'; partition?: string; public?: boolean; orchestration?: boolean; doc?: string }
  // work is the task pipeline's Work tab, with task open.
  | { kind: 'work'; task?: string }

export const allView: View = { kind: 'all' }

// viewKey names a conversation for unread counts. The tabs of the room
// share one key.
export function viewKey(v: View): string {
  if (v.kind === 'all' || v.kind === 'shared' || v.kind === 'topology' || v.kind === 'room' || v.kind === 'work') return v.kind
  if (v.kind === 'map') return v.partition ? `map:${v.partition}` : v.public ? 'map:public' : v.orchestration ? 'map:@orchestration' : 'map'
  if (v.kind === 'release') return `release:${v.name}:${v.deliverable}:${v.version}`
  return `${v.kind}:${v.name}`
}

export function viewHash(v: View): string {
  if (v.kind === 'all') return '#/'
  if (v.kind === 'shared') return '#/shared'
  if (v.kind === 'topology') return v.tab === 'yaml' ? '#/topology/yaml' : v.select ? `#/topology?select=${encodeURIComponent(v.select)}` : '#/topology'
  if (v.kind === 'map') {
    const doc = v.doc ? `/${encodeURIComponent(v.doc)}` : ''
    if (v.partition) return `#/map/${v.partition}${doc}`
    if (v.public) return `#/room/map${doc}`
    if (v.orchestration) return `#/orchestration/map${doc}`
    return '#/map'
  }
  if (v.kind === 'work') return v.task ? `#/work/${encodeURIComponent(v.task)}` : '#/work'
  if (v.kind === 'room') return v.tab && v.tab !== 'messages' ? `#/room/${v.tab}` : '#/room'
  if (v.kind === 'release') return `#/partition/${v.name}/release/${v.deliverable}/${v.version}`
  return `#/${v.kind}/${v.name}`
}

export function parseHash(hash: string): View {
  if (hash === '#/shared') return { kind: 'shared' }
  if (hash === '#/work') return { kind: 'work' }
  const work = hash.match(/^#\/work\/([^/]+)$/)
  if (work) return { kind: 'work', task: decodeURIComponent(work[1]) }
  if (hash === '#/topology') return { kind: 'topology' }
  const select = hash.match(/^#\/topology\?select=([^&]+)$/)
  if (select) return { kind: 'topology', select: decodeURIComponent(select[1]) }
  if (hash === '#/topology/yaml') return { kind: 'topology', tab: 'yaml' }
  if (hash === '#/map') return { kind: 'map' }
  const map = hash.match(/^#\/map\/([\w-]+)(?:\/([^/]+))?$/)
  if (map) return map[2] ? { kind: 'map', partition: map[1], doc: decodeURIComponent(map[2]) } : { kind: 'map', partition: map[1] }
  const publicMap = hash.match(/^#\/room\/map(?:\/([^/]+))?$/)
  if (publicMap) return publicMap[1] ? { kind: 'map', public: true, doc: decodeURIComponent(publicMap[1]) } : { kind: 'map', public: true }
  const orchestrationMap = hash.match(/^#\/orchestration\/map(?:\/([^/]+))?$/)
  if (orchestrationMap) return orchestrationMap[1] ? { kind: 'map', orchestration: true, doc: decodeURIComponent(orchestrationMap[1]) } : { kind: 'map', orchestration: true }
  const release = hash.match(/^#\/partition\/([\w-]+)\/release\/([\w-]+)\/([0-9A-Za-z.-]+)$/)
  if (release) return { kind: 'release', name: release[1], deliverable: release[2], version: release[3] }
  if (hash === '#/room') return { kind: 'room' }
  const room = hash.match(/^#\/room\/(vocabulary|wiki)$/)
  if (room) return { kind: 'room', tab: room[1] as RoomTab }
  const m = hash.match(/^#\/(dm|partition)\/([\w-]+)$/)
  return m ? { kind: m[1] as 'dm' | 'partition', name: m[2] } : allView
}

// conversationKeys returns the views other than All activity that show m. A
// direct message between two agents is in no conversation of the user.
export function conversationKeys(m: Message, me: string): string[] {
  if (!m.to) return ['room']
  if (m.from === me && m.to) return [`dm:${m.to}`]
  if (m.to === me) return [`dm:${m.from}`]
  return []
}

export function inView(m: Message, v: View, me: string): boolean {
  return v.kind === 'all' || conversationKeys(m, me).includes(viewKey(v))
}
