import { ChevronRight, Lock, TriangleAlert } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { HealthBadge } from '@/components/health'
import { releaseCommits } from '@/lib/labels'
import type { State } from '@/lib/store'
import type { Feature, Interface, Private, Release, PartitionView, TaskView } from '@/lib/types'
import { StatusChip } from '@/components/work-view'

function ago(iso?: string): string {
  if (!iso) return ''
  const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  return new Date(iso).toLocaleDateString()
}

// groupByDeliverable keeps the server's order: by deliverable, newest first.
function groupByDeliverable(releases: Release[]): [string, Release[]][] {
  const groups: [string, Release[]][] = []
  for (const r of releases) {
    const last = groups[groups.length - 1]
    if (last && last[0] === r.deliverable) last[1].push(r)
    else groups.push([r.deliverable, [r]])
  }
  return groups
}

export function Section({ title, children, private: isPrivate }: { title: string; children: React.ReactNode; private?: boolean }) {
  return (
    <section className="space-y-2">
      <h2 className="flex items-center gap-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
        {isPrivate && <PrivateLabel />}
      </h2>
      {children}
    </section>
  )
}

// PrivateLabel marks what only the partition agent and the user see. Other agents see
// the public map alone.
function PrivateLabel() {
  return (
    <span className="inline-flex items-center gap-1 font-normal tracking-normal normal-case" title="Only the partition agent and you see this. Other agents see the feature map.">
      <Lock className="size-3" />
      private
    </span>
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

function FeatureRow({ f, priv }: { f: Feature; priv: Private }) {
  const codeMap = priv.code_map.features?.[f.id]
  const link = `[[${f.id}]]`
  const keys = Object.entries(priv.tests.tests ?? {})
    .filter(([, links]) => links?.includes(link))
    .map(([key]) => key)
    .sort()
  const gaps = priv.tests.gaps?.[f.id] ?? undefined
  const pending = f.tests.pending ? ` · ${f.tests.pending} pending` : ''
  const tests = f.tests.ran
    ? `${f.tests.passed} passed · ${f.tests.failed} failed${pending} · ${ago(f.tests.ran)}`
    : keys.length
      ? 'no results yet'
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
        {f.state && f.state !== 'current' ? (
          <span className="w-24 shrink-0 text-xs text-amber-700" data-testid="feature-state">
            {f.state}
          </span>
        ) : (
          <span className="w-24 shrink-0" />
        )}
      </summary>
      <div className="space-y-2 pb-3 pl-6.5 text-sm">
        {f.description && <p>{f.description}</p>}
        <p className="text-xs text-muted-foreground sm:hidden">{tests}</p>
        <Paths label="interfaces" paths={f.interfaces} />
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
        {(codeMap || keys.length > 0 || gaps) && (
          <div className="mt-1 space-y-1.5 border-l-2 pl-3" data-testid="feature-private">
            <div className="text-xs text-muted-foreground">
              <PrivateLabel />
            </div>
            <Paths label="code" paths={codeMap?.code ?? undefined} />
            {codeMap?.notes && <p className="text-xs text-muted-foreground">{codeMap.notes}</p>}
            <Paths label="tests" paths={keys.length ? keys : undefined} />
            <List label="Not tested" items={gaps} />
          </div>
        )}
      </div>
    </details>
  )
}

// openFeature opens and shows a related feature without changing the view in
// the URL.
// Workspace shows the folders of a partition by place, and the branch of
// each repo it commits to, with its uncommitted changes (rule F15).
function Workspace({ v }: { v: PartitionView }) {
  return (
    <Section title="Workspace">
      <ul className="space-y-1 text-sm" data-testid="workspace">
        {v.folders.map((f) => (
          <li key={f.place} className="flex flex-wrap items-baseline gap-x-2">
            <code className="text-xs">/workspace/{f.place}</code>
            <span className="text-muted-foreground">
              {f.path || 'the whole repo'} of {f.repo}
              {f.read_only ? ', read-only' : ''}
            </span>
          </li>
        ))}
      </ul>
      <ul className="mt-2 space-y-0.5 text-sm text-muted-foreground" data-testid="workspace-repos">
        {v.repos.map((r) => (
          <li key={r.name}>
            {r.name} on <code className="text-xs">{r.branch}</code>
            {r.dirty ? ', with uncommitted changes' : ', clean'}
          </li>
        ))}
      </ul>
    </Section>
  )
}

function openFeature(e: React.MouseEvent, id: string) {
  e.preventDefault()
  const el = document.getElementById(`feature-${id}`) as HTMLDetailsElement | null
  if (!el) return
  el.open = true
  el.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
}

// scopesOf lists the scopes of the task pipeline that name a partition
// (rule TK29 of the task pipeline spec).
function scopesOf(state: State, partition: string): TaskView[] {
  const work = state.modules.tasks
  if (!work) return []
  return [...work.queues.flatMap((q) => q.scopes), ...work.drafts].filter((t) => t.partitions.includes(partition))
}

export function PartitionDetail({ state, partition: v }: { state: State; partition: PartitionView }) {
  const partitionAgent = state.agents.find((a) => a.name === v.partition_agent)
  const flags = v.flags ?? []
  const providers = v.providers ?? {}
  const flagsFor = (name: string) => flags.filter((f) => f.interface === name).map((f) => f.problem)
  const s = v.partition
  const priv = v.private
  const parts = priv.code_map.parts ?? []
  const suites = Object.entries(priv.tests.suites ?? {})

  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="partition-detail">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        {v.error && (
          <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            The map file is invalid, so this view shows the last valid map: {v.error}
          </p>
        )}
        {v.private_errors?.length ? (
          <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" data-testid="private-errors">
            <p className="font-medium">The partition agent's private documents have problems:</p>
            <ul className="ml-4 list-disc">
              {v.private_errors.map((e) => (
                <li key={e}>{e}</li>
              ))}
            </ul>
          </div>
        ) : null}
        {v.state === 'stale' && (
          <p className="rounded-md border px-3 py-2 text-sm" data-testid="changed">
            <span className="font-medium">Changed since the map was marked current:</span>{' '}
            <span className="text-muted-foreground">{v.changed?.join(', ')}</span>
          </p>
        )}
        <Workspace v={v} />
        {v.state === 'unbuilt' && !s.summary && v.features.length === 0 ? (
          <p className="py-12 text-center text-sm text-muted-foreground">
            {partitionAgent?.status.state === 'indexing'
              ? `${v.partition_agent} is reading its partition and building the map.`
              : `${v.partition_agent} has not described this partition yet.`}
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
            <Section title="Releases">
              {v.releases?.length ? (
                <div className="space-y-3" data-testid="releases">
                  {groupByDeliverable(v.releases).map(([deliverable, releases]) => (
                    <div key={deliverable}>
                      <h3 className="mb-1 text-sm font-medium">{deliverable}</h3>
                      <ul className="divide-y border-y">
                        {releases.map((r) => (
                          <li key={r.version} className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 py-2 text-sm">
                            <a href={`#/partition/${v.partition_agent}/release/${r.deliverable}/${r.version}`} className="font-medium underline underline-offset-2">
                              {r.version}
                            </a>
                            {r.tags?.map((tag) => (
                              <code key={tag} className="rounded bg-neutral-100 px-1 text-xs" data-testid="tag">
                                {tag}
                              </code>
                            ))}
                            <span className="min-w-0 flex-1">{r.notes}</span>
                            <span className="text-xs text-muted-foreground">
                              {r.product.files} file{r.product.files === 1 ? '' : 's'}
                              {r.product_present === false ? ' · product absent' : ''}
                              {r.remote ? ` · remote ${r.remote}` : ''}
                              {r.certificates?.length ? ` · ${r.certificates.map((c) => c.type).join(', ')}` : ''} · {releaseCommits(r)} ·{' '}
                              {ago(r.released)}
                              {r.pinned_by?.length ? ` · pinned by ${r.pinned_by.join(', ')}` : ''}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No releases yet.</p>
              )}
            </Section>
            <Section title="Pins">
              {v.pins?.length ? (
                <ul className="divide-y border-y" data-testid="pins">
                  {v.pins.map((p) => (
                    <li key={`${p.partition}/${p.deliverable}`} className="flex flex-wrap items-baseline gap-x-3 py-2 text-sm">
                      <span className="font-medium">
                        {p.partition} {p.deliverable}
                      </span>
                      <span className="text-muted-foreground">{p.tag ? `follows the tag ${p.tag}` : `version ${p.version}`}</span>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">No pins: the partition uses no release of another partition.</p>
              )}
            </Section>
            {scopesOf(state, v.partition_agent).length > 0 && (
              <Section title="Scopes">
                <ul className="space-y-1" data-testid="partition-scopes">
                  {scopesOf(state, v.partition_agent).map((t) => (
                    <li key={t.id} className="flex flex-wrap items-center gap-2 text-sm">
                      <a href={`#/work/${encodeURIComponent(t.id)}`} className="hover:underline">
                        {t.name}
                      </a>
                      <StatusChip status={t.status} />
                      {t.parts.find((p) => p.partition === v.partition_agent) && <StatusChip status={t.parts.find((p) => p.partition === v.partition_agent)!.status} label="part" />}
                      {t.queue && <span className="text-xs text-muted-foreground">in {t.queue}</span>}
                    </li>
                  ))}
                </ul>
              </Section>
            )}
            <Section title={`Features (${v.features.length})`}>
              {v.features.length ? (
                <div className="border-t">
                  {v.features.map((f) => (
                    <FeatureRow key={f.id} f={f} priv={priv} />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No features yet.</p>
              )}
            </Section>
            {parts.length > 0 && (
              <Section title="Code map" private>
                <ul className="space-y-1 text-sm">
                  {parts.map((l) => (
                    <li key={l.path} className="flex gap-3">
                      <code className="shrink-0 text-xs leading-5">{l.path}</code>
                      <span className="text-muted-foreground">{l.description}</span>
                    </li>
                  ))}
                </ul>
              </Section>
            )}
            {suites.length > 0 && (
              <Section title="Test suites" private>
                <ul className="space-y-1.5 text-sm">
                  {suites.map(([name, suite]) => (
                    <li key={name}>
                      <span className="font-medium">{name}</span>{' '}
                      <code className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs">{suite.command}</code>
                      {suite.description && <p className="text-muted-foreground">{suite.description}</p>}
                    </li>
                  ))}
                </ul>
              </Section>
            )}
          </>
        )}
      </div>
    </div>
  )
}
