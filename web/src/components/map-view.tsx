import { useCallback, useMemo, useState } from 'react'
import { X } from 'lucide-react'
import { GraphCanvas } from '@/components/graph-canvas'
import type { Handoff } from '@/components/graph-engine'
import { HealthBadge } from '@/components/health'
import { Markdown } from '@/components/markdown'
import { asText, storeFileURL, useFetch } from '@/hooks/use-store-file'
import { featureGraph, nodeID, topologyGraph, type GraphNode } from '@/lib/graph'
import {
  featureContext,
  featureDetail,
  featureLenses,
  featurePaint,
  legend,
  topologyContext,
  topologyDetail,
  topologyLenses,
  topologyPaint,
  type Color,
  type FeatureLens,
  type Lens,
  type Paint,
  type TopologyLens,
} from '@/lib/lens'
import type { State } from '@/lib/store'
import type { SystemView } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { View } from '@/lib/view'
import { documentBody } from '@/lib/wiki'

const lensKey = { topology: 'hikari.map.lens', feature: 'hikari.features.lens' }

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

// handoff says how the canvas moves between two maps: a system opens into
// its feature map, and a feature map folds back into its system (rule U41).
function handoff(from: string, to: string): Handoff | undefined {
  if (from === 'topology' && to.startsWith('feature:')) return { kind: 'in', from: nodeID('system', to.slice('feature:'.length)) }
  if (from.startsWith('feature:') && to === 'topology') return { kind: 'out', to: nodeID('system', from.slice('feature:'.length)) }
  return undefined
}

// MapView draws the topology map, and the feature map of one system, on one
// canvas (rules U30 to U45).
export function MapView({ state, show }: { state: State; show: (v: View) => void }) {
  const system = state.view.kind === 'map' ? state.view.system : undefined
  const sys = system ? state.systems.find((s) => s.owner === system) : undefined
  const [topoLens, setTopoLens] = useState<TopologyLens>(() => remembered(lensKey.topology, topologyLenses, 'activity'))
  const [featLens, setFeatLens] = useState<FeatureLens>(() => remembered(lensKey.feature, featureLenses, 'tests'))
  const graphKey = sys ? `feature:${sys.owner}` : 'topology'
  // The selection belongs to one visit of a map, so it clears whenever the
  // map changes.
  const [selected, setSelected] = useState<string | null>(null)
  const [selectedKey, setSelectedKey] = useState(graphKey)
  if (selectedKey !== graphKey) {
    setSelectedKey(graphKey)
    setSelected(null)
  }

  const topology = useMemo(
    () => topologyGraph({ systems: state.systems, repos: state.repos, deliverables: state.deliverables }),
    [state.systems, state.repos, state.deliverables],
  )
  const features = useMemo(() => (sys ? featureGraph(sys) : null), [sys])
  const graph = features ?? topology

  const topoCtx = useMemo(() => topologyContext(state.systems, state.agents), [state.systems, state.agents])
  const featCtx = useMemo(() => featureContext(sys?.features ?? []), [sys])
  const paint = useCallback(
    (n: GraphNode): Paint => (n.kind === 'feature' ? featurePaint(featLens, n, featCtx) : topologyPaint(topoLens, n, topoCtx)),
    [featLens, featCtx, topoLens, topoCtx],
  )
  const card = useCallback(
    (n: GraphNode) => {
      if (n.kind === 'feature') {
        return { title: n.label, lines: [n.ref, featureDetail(featLens, n, featCtx)] }
      }
      return { title: n.label, lines: [kindLine(n, state), topologyDetail(topoLens, n, topoCtx)] }
    },
    [featLens, featCtx, topoLens, topoCtx, state],
  )

  // The tests lens carries over between the maps (rule U45).
  const open = (name: string) => {
    if (topoLens === 'tests') pickFeatureLens('tests')
    show({ kind: 'map', system: name })
  }
  const back = () => {
    if (featLens === 'tests') pickTopologyLens('tests')
    show({ kind: 'map' })
  }
  const pickTopologyLens = (l: TopologyLens) => {
    setTopoLens(l)
    remember(lensKey.topology, l)
  }
  const pickFeatureLens = (l: FeatureLens) => {
    setFeatLens(l)
    remember(lensKey.feature, l)
  }

  const node = selected ? graph.nodes.find((n) => n.id === selected) : undefined
  const onClick = (id: string | null) => {
    const n = id ? graph.nodes.find((x) => x.id === id) : undefined
    if (n?.kind === 'system') open(n.ref)
    else setSelected(id)
  }
  const onOpen = (id: string) => {
    const n = graph.nodes.find((x) => x.id === id)
    if (n?.kind === 'system') open(n.ref)
  }

  if (state.systems.length === 0) {
    return <p className="flex-1 py-12 text-center text-sm text-muted-foreground">No agent owns a system, so there is nothing to map.</p>
  }

  const lens: Lens = sys ? featLens : topoLens
  const panel = node && (
    <Panel onClose={() => setSelected(null)}>
      {node.kind === 'feature' && sys ? (
        <FeaturePanel sys={sys} id={node.ref} onLink={(id) => setSelected(nodeID('feature', id))} />
      ) : node.kind === 'system' ? (
        <SystemPanel name={node.ref} state={state} open={open} show={show} />
      ) : node.kind === 'repo' ? (
        <RepoPanel name={node.ref} state={state} open={open} />
      ) : node.kind === 'deliverable' ? (
        <DeliverablePanel name={node.ref} state={state} show={show} />
      ) : null}
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
      onSelect={setSelected}
      onOpen={onOpen}
      onEscape={() => (sys ? back() : setSelected(null))}
      aside={panel}
    >
      <div className="absolute top-3 left-3 flex items-center gap-2">
        <LensControl
          lenses={sys ? featureLenses : topologyLenses}
          value={lens}
          onChange={(l) => (sys ? pickFeatureLens(l as FeatureLens) : pickTopologyLens(l as TopologyLens))}
        />
      </div>
      <Legend lens={lens} dashed={!!sys} />
      {sys && sys.features.length === 0 && (
        <p className="pointer-events-none absolute inset-x-0 top-1/3 text-center text-sm text-muted-foreground">{sys.owner} has no features yet.</p>
      )}
    </GraphCanvas>
  )
}

function kindLine(n: GraphNode, state: State): string {
  if (n.kind === 'system') {
    const count = state.systems.find((s) => s.owner === n.ref)?.features.length ?? 0
    return `system · ${count} feature${count === 1 ? '' : 's'}`
  }
  if (n.kind === 'repo') return `repository · branch ${state.repos.find((r) => r.name === n.ref)?.branch ?? ''}`
  const holders = state.deliverables.find((d) => d.name === n.ref)?.holders ?? []
  return `deliverable · held by ${holders.join(', ')}`
}

const lensLabel: Record<Lens, string> = { activity: 'Activity', tests: 'Tests', git: 'Git', age: 'Age' }

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
          document stale or undescribed
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

const noNames: ReadonlySet<string> = new Set()

function FeaturePanel({ sys, id, onLink }: { sys: SystemView; id: string; onLink: (id: string) => void }) {
  const f = sys.features.find((x) => x.id === id)
  const doc = useFetch(storeFileURL(`systems/${sys.owner}/features/${id}.md`), asText)
  if (!f) return null
  return (
    <div className="space-y-2 pr-6" data-testid="feature-panel">
      <div>
        <h2 className="text-sm font-semibold">{f.name}</h2>
        <code className="text-xs text-muted-foreground">{f.id}</code>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <HealthBadge health={f.health} />
        {f.state && f.state !== 'current' && <span className="text-xs text-amber-700">document {f.state}</span>}
      </div>
      {f.description && <p className="text-sm">{f.description}</p>}
      {doc.loading && <p className="text-xs text-muted-foreground">Loading the document…</p>}
      {doc.error && <p className="text-xs text-red-700">{doc.error}</p>}
      {doc.data && <Markdown text={documentBody(doc.data)} names={noNames} onLocalLink={onLink} />}
    </div>
  )
}

function SystemPanel({ name, state, open, show }: { name: string; state: State; open: (s: string) => void; show: (v: View) => void }) {
  const sys = state.systems.find((s) => s.owner === name)
  if (!sys) return null
  return (
    <div className="space-y-2 pr-6 text-sm">
      <h2 className="font-semibold">{sys.owner}</h2>
      {sys.system.summary && <p>{sys.system.summary}</p>}
      <p className="text-muted-foreground">
        {sys.features.length} feature{sys.features.length === 1 ? '' : 's'} · {sys.repo}/{sys.scope}
      </p>
      <div className="flex gap-3">
        <button type="button" className="underline underline-offset-2" onClick={() => open(sys.owner)}>
          Open its features
        </button>
        <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'system', name: sys.owner })}>
          Open the system page
        </button>
      </div>
    </div>
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
