import { ChevronRight, TriangleAlert } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { HealthBadge } from '@/components/health'
import type { State } from '@/lib/store'
import type { Feature, Interface, SystemView } from '@/lib/types'

function ago(iso?: string): string {
  if (!iso) return ''
  const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  return new Date(iso).toLocaleDateString()
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h2>
      {children}
    </section>
  )
}

function Paths({ label, paths }: { label: string; paths?: string[] }) {
  if (!paths?.length) return null
  return (
    <div className="flex flex-wrap items-baseline gap-1.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      {paths.map((p) => (
        <code key={p} className="rounded bg-neutral-100 px-1 py-0.5 text-xs">
          {p}
        </code>
      ))}
    </div>
  )
}

function List({ label, items }: { label: string; items?: string[] }) {
  if (!items?.length) return null
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <ul className="ml-4 list-disc text-sm">
        {items.map((x) => (
          <li key={x}>{x}</li>
        ))}
      </ul>
    </div>
  )
}

function InterfaceRow({ iface, note, warning }: { iface: Interface; note?: string; warning?: string[] }) {
  return (
    <li className="space-y-0.5 py-1.5" data-testid="interface">
      <div className="flex flex-wrap items-center gap-2">
        <code className="text-sm font-medium">{iface.name}</code>
        <Badge variant="outline" className="h-5 font-normal text-muted-foreground">
          {iface.kind}
        </Badge>
        {note && <span className="text-xs text-muted-foreground">{note}</span>}
      </div>
      {iface.description && <p className="text-sm text-muted-foreground">{iface.description}</p>}
      {warning?.map((w) => (
        <p key={w} className="flex items-center gap-1 text-xs text-amber-700" data-testid="flag">
          <TriangleAlert className="size-3" />
          {w}
        </p>
      ))}
    </li>
  )
}

function FeatureRow({ f }: { f: Feature }) {
  const tests = f.tests.refs?.length
    ? `${f.tests.passed} passed · ${f.tests.failed} failed${f.tests.ran ? ` · ${ago(f.tests.ran)}` : ''}`
    : 'no tests'
  return (
    <details id={`feature-${f.id}`} className="group border-b last:border-b-0" data-testid="feature">
      <summary className="flex cursor-pointer list-none items-center gap-3 py-2.5 hover:bg-neutral-50 [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
        <span className="min-w-0 flex-1">
          <span className="text-sm font-medium">{f.name}</span>{' '}
          <code className="text-xs text-muted-foreground">{f.id}</code>
        </span>
        <span className="hidden text-xs text-muted-foreground sm:inline">{tests}</span>
        <span className="w-20 shrink-0">
          <HealthBadge health={f.health} />
        </span>
      </summary>
      <div className="space-y-2 pb-3 pl-6.5 text-sm">
        {f.description && <p>{f.description}</p>}
        <p className="text-xs text-muted-foreground sm:hidden">{tests}</p>
        <Paths label="interfaces" paths={f.interfaces} />
        <Paths label="code" paths={f.code} />
        <Paths label="tests" paths={f.tests.refs} />
        <List label="Findings" items={f.findings} />
        <List label="Plans" items={f.plans} />
        {f.related?.length ? (
          <div className="flex flex-wrap items-baseline gap-1.5">
            <span className="text-xs text-muted-foreground">related</span>
            {f.related.map((r) => (
              <a key={r} href={`#feature-${r}`} onClick={(e) => openFeature(e, r)} className="text-xs underline underline-offset-2">
                {r}
              </a>
            ))}
          </div>
        ) : null}
      </div>
    </details>
  )
}

// openFeature opens and shows a related feature without changing the view in
// the URL.
function openFeature(e: React.MouseEvent, id: string) {
  e.preventDefault()
  const el = document.getElementById(`feature-${id}`) as HTMLDetailsElement | null
  if (!el) return
  el.open = true
  el.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
}

export function SystemDetail({ state, system: v }: { state: State; system: SystemView }) {
  const owner = state.agents.find((a) => a.name === v.owner)
  const flags = v.flags ?? []
  const providers = v.providers ?? {}
  const flagsFor = (name: string) => flags.filter((f) => f.interface === name).map((f) => f.problem)
  const s = v.system

  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="system-detail">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        {v.error && (
          <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            The map file is invalid, so this view shows the last valid map: {v.error}
          </p>
        )}
        {v.state === 'stale' && (
          <p className="rounded-md border px-3 py-2 text-sm" data-testid="changed">
            <span className="font-medium">Changed since the map was marked current:</span>{' '}
            <span className="text-muted-foreground">{v.changed?.join(', ')}</span>
          </p>
        )}
        {v.state === 'unbuilt' && !s.summary && v.features.length === 0 ? (
          <p className="py-12 text-center text-sm text-muted-foreground">
            {owner?.status.state === 'indexing'
              ? `${v.owner} is reading its system and building the map.`
              : `${v.owner} has not described this system yet.`}
          </p>
        ) : (
          <>
            {(s.summary || s.responsibilities?.length) && (
              <Section title="Purpose">
                {s.summary && <p className="text-sm">{s.summary}</p>}
                {s.responsibilities?.length ? (
                  <ul className="ml-4 list-disc text-sm text-muted-foreground">
                    {s.responsibilities.map((r) => (
                      <li key={r}>{r}</li>
                    ))}
                  </ul>
                ) : null}
              </Section>
            )}
            <div className="grid gap-6 md:grid-cols-2">
              <Section title="Provides">
                {s.interfaces.provides.length ? (
                  <ul className="divide-y">
                    {s.interfaces.provides.map((i) => (
                      <InterfaceRow key={i.name} iface={i} warning={flagsFor(i.name)} />
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-muted-foreground">Nothing.</p>
                )}
              </Section>
              <Section title="Consumes">
                {s.interfaces.consumes.length ? (
                  <ul className="divide-y">
                    {s.interfaces.consumes.map((i) => (
                      <InterfaceRow
                        key={i.name}
                        iface={i}
                        note={providers[i.name]?.length ? `from ${providers[i.name].join(', ')}` : undefined}
                        warning={flagsFor(i.name)}
                      />
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-muted-foreground">Nothing.</p>
                )}
              </Section>
            </div>
            <Section title={`Features (${v.features.length})`}>
              {v.features.length ? (
                <div className="border-t">
                  {v.features.map((f) => (
                    <FeatureRow key={f.id} f={f} />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No features yet.</p>
              )}
            </Section>
            {s.layout?.length ? (
              <Section title="Layout">
                <ul className="space-y-1 text-sm">
                  {s.layout.map((l) => (
                    <li key={l.path} className="flex gap-3">
                      <code className="shrink-0 text-xs leading-5">{l.path}</code>
                      <span className="text-muted-foreground">{l.description}</span>
                    </li>
                  ))}
                </ul>
              </Section>
            ) : null}
            {s.test_command && (
              <Section title="Tests">
                <code className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs">{s.test_command}</code>
              </Section>
            )}
          </>
        )}
      </div>
    </div>
  )
}
