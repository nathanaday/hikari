import { useCallback, useMemo, useState } from 'react'
import { Plus, X } from 'lucide-react'
import { GraphCanvas } from '@/components/graph-canvas'
import type { Handoff } from '@/components/graph-engine'
import { Button } from '@/components/ui/button'
import { DocumentPanel, NewDocument } from '@/components/wiki-panel'
import { docNodeID, nodeID, topologyGraph, wikiGraph, type GraphNode } from '@/lib/graph'
import {
  docDetail,
  docPaint,
  legend,
  topologyContext,
  topologyDetail,
  topologyLenses,
  topologyPaint,
  wikiContext,
  wikiLenses,
  type Color,
  type Lens,
  type Paint,
  type TopologyLens,
  type WikiLens,
} from '@/lib/lens'
import type { State } from '@/lib/store'
import type { WikiView } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { View } from '@/lib/view'
import { linkTarget } from '@/lib/wiki'
import { hubRef, systemRef, wikiIndex, wikiName } from '@/lib/wikis'

const lensKey = { topology: 'hikari.map.lens', wiki: 'hikari.wiki.lens' }

// remembered reads a lens from local storage; a failed read gives the
// default (rule U45).
function remembered<T extends Lens>(key: string, allowed: T[], fallback: T): T {
  try {
    const v = localStorage.getItem(key) as T | null
    return v && allowed.includes(v) ? v : fallback
  } catch {
    return fallback
  }
}

function remember(key: string, lens: Lens) {
  try {
    localStorage.setItem(key, lens)
  } catch {
    // The lens is a convenience; losing it is fine.
  }
}

// handoff says how the canvas moves between maps: a system or a hub opens
// into its wiki map, and a wiki map folds back into its node (rule U41, and
// U1 of the wikis spec).
function handoff(from: string, to: string): Handoff | undefined {
  const node = (key: string) => {
    const ref = key.slice('wiki:'.length)
    const [scope, name] = ref.split(':')
    return nodeID(scope === 'hub' ? 'hub' : 'system', name)
  }
  if (from === 'topology' && to.startsWith('wiki:')) return { kind: 'in', from: node(to) }
  if (from.startsWith('wiki:') && to === 'topology') return { kind: 'out', to: node(from) }
  return undefined
}

// MapView draws the topology map, and the wiki map of one system or hub, on
// one canvas (rules U30 to U45, and section 10 of the wikis spec).
export function MapView({ state, show }: { state: State; show: (v: View) => void }) {
  const view = state.view.kind === 'map' ? state.view : { kind: 'map' as const }
  const ref = view.system ? systemRef(view.system) : view.hub ? hubRef(view.hub) : null
  const wiki = ref ? state.wikis.find((w) => w.ref === ref) : undefined
  const [topoLens, setTopoLens] = useState<TopologyLens>(() => remembered(lensKey.topology, topologyLenses, 'activity'))
  const [wikiLens, setWikiLens] = useState<WikiLens>(() => remembered(lensKey.wiki, wikiLenses, 'kind'))
  const [creating, setCreating] = useState(false)
  const graphKey = ref ? `wiki:${ref}` : 'topology'

  const index = useMemo(() => wikiIndex(state.wikis), [state.wikis])
  const topology = useMemo(
    () => topologyGraph({ systems: state.systems, repos: state.repos, deliverables: state.deliverables, hubs: state.hubs }),
    [state.systems, state.repos, state.deliverables, state.hubs],
  )
  const docs = useMemo(() => (wiki ? wikiGraph(wiki, index) : null), [wiki, index])
  const graph = docs ?? topology

  // The selection is the view's document on a wiki map, and a local choice
  // on the topology map.
  const [topoSelected, setTopoSelected] = useState<string | null>(null)
  const selected = wiki ? (view.doc ? docNodeID(wiki.ref, view.doc) : null) : topoSelected

  const topoCtx = useMemo(() => topologyContext(state.systems, state.agents), [state.systems, state.agents])
  const features = useMemo(() => (view.system ? (state.systems.find((s) => s.owner === view.system)?.features ?? []) : []), [view.system, state.systems])
  const wikiCtx = useMemo(
    () => wikiContext(graph.nodes, (n) => (n.kind === 'doc' && n.wiki ? state.wikis.find((w) => w.ref === n.wiki)?.documents.find((d) => d.id === n.ref) : undefined), features),
    [graph, state.wikis, features],
  )
  const paint = useCallback(
    (n: GraphNode): Paint => (n.kind === 'doc' ? docPaint(wikiLens, n, wikiCtx) : topologyPaint(topoLens, n, topoCtx)),
    [wikiLens, wikiCtx, topoLens, topoCtx],
  )
  const card = useCallback(
    (n: GraphNode) => {
      if (n.kind === 'doc') return { title: n.label, lines: [n.foreign ? `${n.docKind} in ${wikiName(n.wiki!)}` : n.ref, docDetail(wikiLens, n, wikiCtx)] }
      return { title: n.label, lines: [kindLine(n, state), topologyDetail(topoLens, n, topoCtx)] }
    },
    [wikiLens, wikiCtx, topoLens, topoCtx, state],
  )

  // The tests lens carries over between the maps (rule U45).
  const openWiki = (scope: 'system' | 'hub', name: string, doc?: string) => {
    if (topoLens === 'tests' && !ref) pickWikiLens('tests')
    setCreating(false)
    show(scope === 'system' ? { kind: 'map', system: name, doc } : { kind: 'map', hub: name, doc })
  }
  const back = () => {
    if (wikiLens === 'tests') pickTopologyLens('tests')
    setCreating(false)
    show({ kind: 'map' })
  }
  const pickTopologyLens = (l: TopologyLens) => {
    setTopoLens(l)
    remember(lensKey.topology, l)
  }
  const pickWikiLens = (l: WikiLens) => {
    setWikiLens(l)
    remember(lensKey.wiki, l)
  }
  // select selects a document of the open wiki, or opens the wiki of a
  // document of another one (rule U5).
  const selectDoc = (id: string | null) => {
    if (!wiki) return
    setCreating(false)
    if (id === null || wiki.documents.some((d) => d.id === id)) {
      show({ ...view, doc: id ?? undefined })
      return
    }
    const found = index.byID.get(id)
    if (found) {
      const [scope, name] = found.wiki.ref.split(':')
      openWiki(scope as 'system' | 'hub', name, id)
    }
  }

  const node = selected ? graph.nodes.find((n) => n.id === selected) : undefined
  const onClick = (id: string | null) => {
    const n = id ? graph.nodes.find((x) => x.id === id) : undefined
    if (n?.kind === 'system') openWiki('system', n.ref)
    else if (n?.kind === 'hub') openWiki('hub', n.ref)
    else if (n?.kind === 'doc') selectDoc(n.ref)
    else if (wiki) selectDoc(null)
    else setTopoSelected(id)
  }
  const onSelect = (id: string | null) => {
    const n = id ? graph.nodes.find((x) => x.id === id) : undefined
    if (wiki) selectDoc(n?.ref ?? null)
    else setTopoSelected(id)
  }
  const onOpen = (id: string) => {
    const n = graph.nodes.find((x) => x.id === id)
    if (n?.kind === 'system') openWiki('system', n.ref)
    if (n?.kind === 'hub') openWiki('hub', n.ref)
  }

  if (state.systems.length === 0 && state.hubs.length === 0) {
    return <p className="flex-1 py-12 text-center text-sm text-muted-foreground">Nothing to map yet.</p>
  }

  const lens: Lens = wiki ? wikiLens : topoLens
  const doc = node?.kind === 'doc' && wiki ? wiki.documents.find((d) => d.id === node.ref && !node.foreign) : undefined
  let panelBody: React.ReactNode = null
  if (creating && wiki) {
    panelBody = (
      <NewDocument
        wiki={wiki}
        onDone={(id) => {
          setCreating(false)
          if (id) selectDoc(id)
        }}
      />
    )
  } else if (doc && wiki) {
    panelBody = <DocumentPanel key={`${wiki.ref}/${doc.id}`} wiki={wiki} doc={doc} index={index} onLink={(href) => selectDoc(linkTarget(decodeURI(href)))} />
  } else if (node?.kind === 'repo') {
    panelBody = <RepoPanel name={node.ref} state={state} open={(s) => openWiki('system', s)} />
  } else if (node?.kind === 'deliverable') {
    panelBody = <DeliverablePanel name={node.ref} state={state} show={show} />
  }
  const panel = panelBody && (
    <Panel
      onClose={() => {
        setCreating(false)
        onSelect(null)
      }}
    >
      {panelBody}
    </Panel>
  )
  return (
    <GraphCanvas
      graph={graph}
      graphKey={graphKey}
      handoff={handoff}
      paint={paint}
      card={card}
      selected={selected}
      onClick={onClick}
      onSelect={onSelect}
      onOpen={onOpen}
      onEscape={() => (wiki ? (selected ? selectDoc(null) : back()) : setTopoSelected(null))}
      aside={panel}
    >
      <div className="absolute top-3 left-3 flex items-center gap-2">
        <LensControl
          lenses={wiki ? wikiLenses : topologyLenses}
          value={lens}
          onChange={(l) => (wiki ? pickWikiLens(l as WikiLens) : pickTopologyLens(l as TopologyLens))}
        />
        {wiki && (
          <Button size="sm" variant="outline" className="h-7 bg-white" onClick={() => setCreating(true)}>
            <Plus className="size-3.5" /> New
          </Button>
        )}
      </div>
      {wiki && <WikiProblems wiki={wiki} />}
      <Legend lens={lens} dashed={!!view.system} />
      {wiki && wiki.documents.length === 0 && (
        <p className="pointer-events-none absolute inset-x-0 top-1/3 text-center text-sm text-muted-foreground">{wikiName(wiki.ref)} has no documents yet.</p>
      )}
    </GraphCanvas>
  )
}

// WikiProblems lists the files that a hand edit broke, and the warnings of
// the wiki, folded (rule D4 of the wikis spec).
function WikiProblems({ wiki }: { wiki: WikiView }) {
  const items = [...(wiki.problems ?? []), ...(wiki.warnings ?? [])]
  if (items.length === 0) return null
  return (
    <details className="absolute top-3 right-3 max-w-80 rounded-md border bg-white/95 px-2.5 py-1.5 text-xs shadow-sm" data-testid="wiki-problems">
      <summary className={cn('cursor-pointer', wiki.problems?.length ? 'text-red-700' : 'text-amber-700')}>
        {wiki.problems?.length ? `${wiki.problems.length} broken file${wiki.problems.length === 1 ? '' : 's'}` : `${items.length} warning${items.length === 1 ? '' : 's'}`}
      </summary>
      <ul className="mt-1 max-h-60 list-disc space-y-0.5 overflow-y-auto pl-4 text-muted-foreground">
        {items.map((p) => (
          <li key={p}>{p}</li>
        ))}
      </ul>
    </details>
  )
}

function kindLine(n: GraphNode, state: State): string {
  if (n.kind === 'system') {
    const count = state.systems.find((s) => s.owner === n.ref)?.features.length ?? 0
    return `system · ${count} feature${count === 1 ? '' : 's'}`
  }
  if (n.kind === 'hub') {
    const count = state.wikis.find((w) => w.ref === hubRef(n.ref))?.documents.length ?? 0
    return `hub · ${count} document${count === 1 ? '' : 's'} in its wiki`
  }
  if (n.kind === 'repo') return `repository · branch ${state.repos.find((r) => r.name === n.ref)?.branch ?? ''}`
  const holders = state.deliverables.find((d) => d.name === n.ref)?.holders ?? []
  return `deliverable · held by ${holders.join(', ')}`
}

const lensLabel: Record<Lens, string> = { activity: 'Activity', tests: 'Tests', git: 'Git', age: 'Age', kind: 'Kind' }

function LensControl({ lenses, value, onChange }: { lenses: Lens[]; value: Lens; onChange: (l: Lens) => void }) {
  return (
    <div role="radiogroup" aria-label="Lens" className="flex rounded-md border bg-white p-0.5 text-xs shadow-sm" data-testid="lens-control">
      {lenses.map((l) => (
        <button
          key={l}
          type="button"
          role="radio"
          aria-checked={value === l}
          onClick={() => onChange(l)}
          className={cn('rounded px-2.5 py-1 transition-colors', value === l ? 'bg-neutral-900 text-white' : 'text-muted-foreground hover:text-foreground')}
        >
          {lensLabel[l]}
        </button>
      ))}
    </div>
  )
}

const swatch: Record<Color, string> = {
  active: 'var(--lens-active)',
  waiting: 'var(--lens-waiting)',
  pass: 'var(--lens-pass)',
  pending: 'var(--lens-pending)',
  fail: 'var(--lens-fail)',
  dirty: 'var(--lens-dirty)',
  none: 'var(--lens-none)',
  outline: 'var(--lens-outline)',
  white: '#fff',
  feature: 'var(--kind-feature)',
  story: 'var(--kind-story)',
  rule: 'var(--kind-rule)',
  principle: 'var(--kind-principle)',
  plan: 'var(--kind-plan)',
}

function Swatch({ paint }: { paint: Paint }) {
  return (
    <span
      className="inline-block size-2.5 shrink-0 rounded-full border-[1.5px]"
      style={{
        borderColor: swatch[paint.stroke],
        borderStyle: paint.dashed ? 'dashed' : 'solid',
        background: `color-mix(in oklab, ${swatch[paint.fill]} ${Math.round(paint.opacity * 100)}%, white)`,
      }}
    />
  )
}

// Legend shows the colors of the current lens (rule U37).
function Legend({ lens, dashed }: { lens: Lens; dashed: boolean }) {
  return (
    <div className="pointer-events-none absolute bottom-3 left-3 space-y-1 rounded-md border bg-white/90 px-2.5 py-2 text-xs text-muted-foreground shadow-sm" data-testid="legend">
      {legend(lens).map((e) => (
        <div key={e.label} className="flex items-center gap-2">
          <Swatch paint={e.paint} />
          {e.label}
        </div>
      ))}
      {dashed && (
        <div className="flex items-center gap-2">
          <Swatch paint={{ fill: 'white', stroke: 'outline', opacity: 1, pulse: false, dashed: true }} />
          feature stale or undescribed
        </div>
      )}
    </div>
  )
}

// Panel sits beside the canvas, or over its bottom on a narrow screen.
function Panel({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <aside
      className="absolute inset-x-0 bottom-0 z-20 max-h-[55%] overflow-y-auto border-t bg-white px-4 py-3 shadow-lg sm:relative sm:inset-auto sm:z-auto sm:max-h-none sm:w-96 sm:shrink-0 sm:border-t-0 sm:border-l sm:shadow-none"
      data-testid="map-panel"
    >
      <button type="button" onClick={onClose} aria-label="Close" className="absolute top-2.5 right-2.5 rounded p-1 text-muted-foreground hover:text-foreground">
        <X className="size-4" />
      </button>
      {children}
    </aside>
  )
}

function RepoPanel({ name, state, open }: { name: string; state: State; open: (s: string) => void }) {
  const repo = state.repos.find((r) => r.name === name)
  if (!repo) return null
  return (
    <div className="space-y-2 pr-6 text-sm">
      <h2 className="font-semibold">{repo.name}</h2>
      <p className="text-muted-foreground">
        Repository · branch <code className="text-xs">{repo.branch}</code>
      </p>
      <div>
        <div className="text-xs text-muted-foreground">Systems</div>
        <ul className="ml-4 list-disc">
          {repo.systems.map((s) => (
            <li key={s}>
              <button type="button" className="underline underline-offset-2" onClick={() => open(s)}>
                {s}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

function DeliverablePanel({ name, state, show }: { name: string; state: State; show: (v: View) => void }) {
  const d = state.deliverables.find((x) => x.name === name)
  if (!d) return null
  return (
    <div className="space-y-2 pr-6 text-sm">
      <h2 className="font-semibold">{d.name}</h2>
      {d.description && <p>{d.description}</p>}
      {d.error && <p className="text-xs text-red-700">{d.error}</p>}
      <p className="text-muted-foreground">Held by {d.holders.join(', ')}.</p>
      <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'shared' })}>
        Open the shared area
      </button>
    </div>
  )
}
