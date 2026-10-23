import type { DeliverableInfo, DocKind, Hub, RepoInfo, SystemView, WikiDoc, WikiView } from './types'
import { wikiName, type WikiIndex } from './wikis'

export type NodeKind = 'system' | 'repo' | 'deliverable' | 'hub' | 'doc'
export type Shape = 'circle' | 'square' | 'diamond' | 'hexagon'

// GraphNode is one node as the canvas draws it. ref names what it stands
// for: a system, a repository, a deliverable, a hub, or a document id.
export type GraphNode = {
  id: string
  kind: NodeKind
  ref: string
  label: string
  shape: Shape
  radius: number
  // A document node names its wiki and kind. foreign marks a document of
  // another wiki on a wiki map (rule U2 of the wikis spec).
  wiki?: string
  docKind?: DocKind
  foreign?: boolean
}

export type GraphEdge = { source: string; target: string; dashed: boolean }

export type Graph = { nodes: GraphNode[]; edges: GraphEdge[] }

export const nodeID = (kind: NodeKind, ref: string) => `${kind}:${ref}`

// systemRadius grows with the square root of the feature count, from 6 to
// 18 pixels (rule U30).
export function systemRadius(features: number): number {
  return Math.min(18, 6 + 2 * Math.sqrt(features))
}

// featureRadius grows with the number of neighbors, from 5 to 14 pixels
// (rule U38).
export function featureRadius(neighbors: number): number {
  return Math.min(14, 5 + 1.5 * neighbors)
}

type TopologyInput = { systems: SystemView[]; repos: RepoInfo[]; deliverables: DeliverableInfo[]; hubs?: Hub[] }

// topologyGraph builds the topology map: systems, repositories, held
// deliverables, and hubs, with their edges (rules U30 and U31, and U1 of the
// wikis spec).
export function topologyGraph({ systems, repos, deliverables, hubs = [] }: TopologyInput): Graph {
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  const names = new Set(systems.map((s) => s.owner))
  for (const s of [...systems].sort(byOwner)) {
    nodes.push({ id: nodeID('system', s.owner), kind: 'system', ref: s.owner, label: s.owner, shape: 'circle', radius: systemRadius(s.features.length) })
  }
  for (const r of [...repos].sort(byName)) {
    const members = r.systems.filter((s) => names.has(s))
    nodes.push({ id: nodeID('repo', r.name), kind: 'repo', ref: r.name, label: r.name, shape: 'square', radius: 10 })
    for (const s of members) edges.push({ source: nodeID('system', s), target: nodeID('repo', r.name), dashed: false })
  }
  for (const d of [...deliverables].sort(byName)) {
    const holders = d.holders.filter((h) => names.has(h)).sort()
    if (holders.length === 0) continue
    nodes.push({ id: nodeID('deliverable', d.name), kind: 'deliverable', ref: d.name, label: d.name, shape: 'diamond', radius: 10 })
    for (const h of holders) edges.push({ source: nodeID('deliverable', d.name), target: nodeID('system', h), dashed: false })
  }
  const hubNames = new Set(hubs.map((h) => h.name))
  for (const h of [...hubs].sort(byName)) {
    nodes.push({ id: nodeID('hub', h.name), kind: 'hub', ref: h.name, label: `#${h.name}`, shape: 'hexagon', radius: 11 })
    const members = h.participants.filter((p) => p.role === 'member').map((p) => p.name)
    for (const m of [...members].sort()) {
      if (names.has(m)) edges.push({ source: nodeID('hub', h.name), target: nodeID('system', m), dashed: false })
    }
    for (const child of [...hubs].sort(byName)) {
      const orchestrator = child.participants.find((p) => p.role === 'orchestrator')?.name
      if (child.name !== h.name && orchestrator && members.includes(orchestrator) && hubNames.has(child.name)) {
        edges.push({ source: nodeID('hub', h.name), target: nodeID('hub', child.name), dashed: false })
      }
    }
  }
  const pairs = new Set<string>()
  const dashed = (a: string, b: string) => {
    if (a === b || !names.has(a) || !names.has(b)) return
    const key = [a, b].sort().join('\n')
    if (pairs.has(key)) return
    pairs.add(key)
    edges.push({ source: nodeID('system', a), target: nodeID('system', b), dashed: true })
  }
  for (const s of [...systems].sort(byOwner)) {
    for (const r of s.grants?.releases ?? []) dashed(s.owner, r)
    for (const providers of Object.values(s.providers ?? {})) {
      for (const p of providers) dashed(s.owner, p)
    }
  }
  return { nodes, edges }
}

// docShape gives each kind its shape (rule U3).
export const docShape: Record<DocKind, Shape> = {
  feature: 'circle',
  story: 'circle',
  principle: 'diamond',
  rule: 'square',
  plan: 'circle',
  note: 'circle',
}

export const docNodeID = (wiki: string, id: string) => `doc:${wiki}/${id}`

// wikiGraph builds the wiki map of one system or hub: a node per document,
// and a smaller node for each document of another wiki that links one of
// them or that one of them links, with an edge when either links the other
// (rules U2 and U3).
export function wikiGraph(wiki: WikiView, index: WikiIndex): Graph {
  const own = new Map(wiki.documents.map((d) => [d.id, d]))
  const nodes = new Map<string, GraphNode>()
  const edges: GraphEdge[] = []
  const pairs = new Set<string>()
  const degree = new Map<string, number>()
  const edge = (a: string, b: string) => {
    if (a === b) return
    const key = [a, b].sort().join('\n')
    if (pairs.has(key)) return
    pairs.add(key)
    edges.push({ source: a, target: b, dashed: false })
    degree.set(a, (degree.get(a) ?? 0) + 1)
    degree.set(b, (degree.get(b) ?? 0) + 1)
  }
  const foreign = (ref: string, d: WikiDoc) => {
    const id = docNodeID(ref, d.id)
    if (!nodes.has(id)) {
      nodes.set(id, { id, kind: 'doc', ref: d.id, wiki: ref, docKind: d.kind, foreign: true, label: `${d.name || d.id} · ${wikiName(ref)}`, shape: docShape[d.kind], radius: 4 })
    }
    return id
  }
  for (const d of [...wiki.documents].sort((a, b) => a.id.localeCompare(b.id))) {
    const id = docNodeID(wiki.ref, d.id)
    nodes.set(id, { id, kind: 'doc', ref: d.id, wiki: wiki.ref, docKind: d.kind, label: d.name || d.id, shape: docShape[d.kind], radius: 0 })
  }
  for (const d of wiki.documents) {
    const from = docNodeID(wiki.ref, d.id)
    for (const target of d.links) {
      if (own.has(target)) {
        edge(from, docNodeID(wiki.ref, target))
        continue
      }
      const found = index.byID.get(target)
      if (found) edge(from, foreign(found.wiki.ref, found.doc))
    }
    for (const back of index.backlinks.get(d.id) ?? []) {
      if (back.wiki.ref !== wiki.ref && index.byID.get(d.id)?.wiki.ref === wiki.ref) edge(from, foreign(back.wiki.ref, back.doc))
    }
  }
  for (const n of nodes.values()) {
    if (!n.foreign) n.radius = n.docKind === 'plan' || n.docKind === 'note' ? 5 : featureRadius(degree.get(n.id) ?? 0)
  }
  return { nodes: [...nodes.values()], edges }
}

// neighborsOf returns the ids of the nodes that share an edge with id.
export function neighborsOf(g: Graph, id: string): Set<string> {
  const out = new Set<string>()
  for (const e of g.edges) {
    if (e.source === id) out.add(e.target)
    if (e.target === id) out.add(e.source)
  }
  return out
}

const byOwner = (a: SystemView, b: SystemView) => a.owner.localeCompare(b.owner)
const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name)
