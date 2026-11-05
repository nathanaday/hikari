import { useCallback, useEffect, useState } from 'react'
import { Check, ChevronRight, Plus } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { Button } from '@/components/ui/button'
import { asJSON, asText, vaultFileURL, useFetch } from '@/hooks/use-vault-file'
import type { Deliverable, SharedListing, ToolboxItem } from '@/lib/types'

const kindTitles: Record<ToolboxItem['kind'], { title: string; hint: string }> = {
  policy: { title: 'Policies', hint: 'Rules for every agent. Installed into vault/public/policies/; every agent reads them on its next turn.' },
  style: { title: 'Styles', hint: 'How an agent\'s work should look. Installed into vault/styles/; grant one with grants.styles or a profile.' },
  skill: { title: 'Skills', hint: 'How a kind of task is done. Installed into vault/skills/; grant one with grants.skills or a profile.' },
  profile: { title: 'Profiles', hint: 'Bundles of model, effort, review, skills, and styles. Paste one under x-hikari.profiles, and give an agent profile: <name>.' },
}

// addFromToolbox installs one item as you, and returns what the server said:
// the files it wrote or the snippet to paste, or an error.
async function addFromToolbox(item: ToolboxItem, force: boolean): Promise<{ written?: string[]; snippet?: string; grant?: string; error?: string }> {
  const res = await fetch('/api/ui/toolbox/add', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kind: item.kind, name: item.name, force }) }).catch(() => undefined)
  if (!res) return { error: 'The hikari server is not reachable.' }
  const body = (await res.json().catch(() => ({}))) as { written?: string[]; snippet?: string; grant?: string; error?: string }
  if (!res.ok) return { error: body.error ?? res.statusText }
  return body
}

// ToolboxList shows the built-in items by kind, each with Add, or a check
// when the vault has it; add installs one and reports the result.
export function ToolboxList({ items, add }: { items: ToolboxItem[]; add: (item: ToolboxItem, force: boolean) => Promise<{ written?: string[]; snippet?: string; grant?: string; error?: string }> }) {
  const [notes, setNotes] = useState<Record<string, { text: string; snippet?: string; error?: boolean }>>({})
  const [done, setDone] = useState<Set<string>>(new Set())
  const key = (it: ToolboxItem) => `${it.kind}/${it.name}`
  const install = async (it: ToolboxItem) => {
    const res = await add(it, false)
    if (res.error) setNotes((n) => ({ ...n, [key(it)]: { text: res.error!, error: true } }))
    else if (res.snippet) setNotes((n) => ({ ...n, [key(it)]: { text: res.grant ?? '', snippet: res.snippet } }))
    else {
      setDone((d) => new Set(d).add(key(it)))
      setNotes((n) => ({ ...n, [key(it)]: { text: `Wrote ${(res.written ?? []).join(', ')}; ${res.grant ?? ''}` } }))
    }
  }
  return (
    <div className="space-y-4" data-testid="toolbox">
      {(['policy', 'style', 'skill', 'profile'] as const).map((kind) => {
        const group = items.filter((it) => it.kind === kind)
        if (group.length === 0) return null
        return (
          <div key={kind} className="space-y-1">
            <h3 className="text-sm font-medium">{kindTitles[kind].title}</h3>
            <p className="text-xs text-muted-foreground">{kindTitles[kind].hint}</p>
            <ul className="divide-y border-y">
              {group.map((it) => {
                const installed = it.installed || done.has(key(it))
                const note = notes[key(it)]
                return (
                  <li key={key(it)} className="space-y-1 py-1.5" data-toolbox={key(it)}>
                    <div className="flex items-center gap-2 text-sm">
                      <code className="text-xs">{it.name}</code>
                      <span className="min-w-0 flex-1 text-muted-foreground">{it.description}</span>
                      {installed ? (
                        <span className="inline-flex items-center gap-1 text-xs text-emerald-700" data-installed="true">
                          <Check className="size-3" /> in the vault
                        </span>
                      ) : (
                        <Button size="sm" variant="outline" onClick={() => install(it)}>
                          <Plus className="size-3.5" />
                          {kind === 'profile' ? 'Show' : 'Add'}
                        </Button>
                      )}
                    </div>
                    {note && <p className={`text-xs ${note.error ? 'text-red-700' : 'text-muted-foreground'}`}>{note.text}</p>}
                    {note?.snippet && <pre className="overflow-auto rounded bg-neutral-50 p-2 text-xs">{note.snippet}</pre>}
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
    </div>
  )
}

// Toolbox loads the built-in items and lets you install them.
function Toolbox({ onChange }: { onChange: () => void }) {
  const [items, setItems] = useState<ToolboxItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    fetch('/api/ui/toolbox')
      .then(async (r) => (r.ok ? ((await r.json()) as ToolboxItem[]) : Promise.reject(new Error(r.statusText))))
      .then(setItems)
      .catch((e: Error) => setError(e.message))
  }, [])
  return (
    <section className="space-y-2">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Toolbox</h2>
      <p className="text-sm text-muted-foreground">Ready-made policies, styles, skills, and profiles that come with hikari. An installed file is yours to change. In a terminal: hikari toolbox list.</p>
      {error ? <p className="text-sm text-muted-foreground">{error}</p> : items ? <ToolboxList items={items} add={async (it, force) => {
        const res = await addFromToolbox(it, force)
        if (res.written) onChange()
        return res
      }} /> : <p className="text-sm text-muted-foreground">Loading…</p>}
    </section>
  )
}

const noNames = new Set<string>()

function Entry({ name, path, note }: { name: string; path: string; note?: string }) {
  const [open, setOpen] = useState(false)
  const { data, error } = useFetch(open ? vaultFileURL(path) : null, asText)
  return (
    <details className="group border-b last:border-b-0" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} data-testid="shared-entry">
      <summary className="flex cursor-pointer list-none items-center gap-2 py-2 text-sm hover:bg-neutral-50 [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
        <span className="font-medium">{name}</span>
        <code className="text-xs text-muted-foreground">{path}</code>
        {note && <span className="ml-auto text-xs text-muted-foreground">{note}</span>}
      </summary>
      <div className="pb-3 pl-5.5">
        {error ? <p className="text-sm text-muted-foreground">{error}</p> : data !== undefined && <Markdown text={data} names={noNames} />}
      </div>
    </details>
  )
}

function Group({ title, hint, entries }: { title: string; hint: string; entries: { name: string; path: string; note?: string }[] }) {
  return (
    <section className="space-y-2">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title} ({entries.length})
      </h2>
      {entries.length ? (
        <div className="border-t">
          {entries.map((e) => (
            <Entry key={e.path} {...e} />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{hint}</p>
      )}
    </section>
  )
}

function Patterns({ label, patterns }: { label: string; patterns?: string[] }) {
  if (!patterns?.length) return null
  return (
    <div className="flex gap-3 text-sm">
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span className="flex flex-wrap gap-1">
        {patterns.map((p) => (
          <code key={p} className="rounded bg-neutral-100 px-1 text-xs">
            {p}
          </code>
        ))}
      </span>
    </div>
  )
}

// DeliverableEntry shows what a deliverable asks for and who holds it, and
// opens to its skill.
export function DeliverableEntry({ d }: { d: Deliverable }) {
  const [open, setOpen] = useState(false)
  const { data, error } = useFetch(open ? vaultFileURL(d.skill) : null, asText)
  const hasRules = d.layout.require?.length || d.layout.allow?.length || d.layout.forbid?.length
  return (
    <details className="group border-b last:border-b-0" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} data-testid="deliverable">
      <summary className="flex cursor-pointer list-none items-baseline gap-2 py-2 text-sm hover:bg-neutral-50 [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-3.5 self-center text-muted-foreground transition-transform group-open:rotate-90" />
        <span className="font-medium">{d.name}</span>
        <span className="min-w-0 flex-1 truncate text-muted-foreground">{d.error ? 'invalid' : d.description}</span>
        <span className="text-xs text-muted-foreground">{d.holders.length ? `held by ${d.holders.join(', ')}` : 'no holder'}</span>
      </summary>
      <div className="space-y-2 pb-3 pl-5.5">
        {d.error && <p className="text-sm text-red-700">{d.error}</p>}
        <code className="block text-xs text-muted-foreground">{d.path}/</code>
        {hasRules ? (
          <div className="space-y-1">
            <Patterns label="require" patterns={d.layout.require} />
            <Patterns label="allow" patterns={d.layout.allow} />
            <Patterns label="forbid" patterns={d.layout.forbid} />
          </div>
        ) : (
          !d.error && <p className="text-sm text-muted-foreground">No layout rules: any files.</p>
        )}
        {d.targets.length ? (
          <div className="flex gap-3 text-sm">
            <span className="w-16 shrink-0 text-muted-foreground">targets</span>
            <span>
              {d.targets
                .map((t) => [t.type, t.repo ?? t.repository, t.kind].filter(Boolean).join(' '))
                .join(', ')}
            </span>
          </div>
        ) : null}
        {error ? <p className="text-sm text-muted-foreground">{error}</p> : data !== undefined && <Markdown text={data} names={noNames} />}
      </div>
    </details>
  )
}

export function SharedView() {
  const parse = useCallback((r: Response) => asJSON<SharedListing>(r), [])
  const [generation, setGeneration] = useState(0)
  const { data, error } = useFetch(`/api/ui/vault/public?g=${generation}`, parse)
  if (error) return <p className="px-6 py-10 text-sm text-muted-foreground">{error}</p>
  if (!data) return <p className="px-6 py-10 text-sm text-muted-foreground">Loading…</p>
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="shared-view">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        <section className="space-y-2">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Deliverables ({data.deliverables.length})</h2>
          {data.deliverables.length ? (
            <div className="border-t">
              {data.deliverables.map((d) => (
                <DeliverableEntry key={d.name} d={d} />
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Add a deliverable as vault/public/deliverables/&lt;name&gt;/ with deliverable.yaml and SKILL.md, and grant it to a partition agent in the topology.
            </p>
          )}
        </section>
        <Group
          title="Remotes"
          hint="Add a remote as vault/public/remotes/<name>/remote.yaml, with the push and pull commands that hikari runs on this machine, and name it in a deliverable."
          entries={(data.remotes ?? []).map((r) => ({
            name: r.name,
            path: r.path,
            note: r.problems ? 'invalid' : [r.push ? 'push' : '', r.pull ? 'pull' : ''].filter(Boolean).join(' and ') || 'no commands',
          }))}
        />
        <Group
          title="Contracts"
          hint="A contract says the shape of an interface of the vocabulary: an OpenAPI document, a proto file, a JSON schema. The orchestration agent writes them with write_contract into vault/public/contracts/, and you can add files there too."
          entries={(data.contracts ?? []).map((c) => ({
            name: c.name,
            path: c.path,
            note: c.missing ? `named by ${c.interfaces.join(', ')}, but the file is missing` : c.interfaces.length ? `names the shape of ${c.interfaces.join(', ')}` : 'named by no interface',
          }))}
        />
        <Group title="Policies" hint="Add rules for every agent as Markdown files in vault/public/policies/." entries={data.policies} />
        <Group
          title="Styles"
          hint="A style says how an agent's work should look: code, commits, documents, messages. Add one as vault/styles/<name>.md, or with hikari toolbox, and grant it with grants.styles or a profile."
          entries={(data.styles ?? []).map((s) => ({ name: s.name, path: s.path, note: s.holders.length ? `followed by ${s.holders.join(', ')}` : 'followed by no agent' }))}
        />
        <Group
          title="Skills"
          hint="Add Claude Code skills in vault/skills/<name>/SKILL.md, and grant each to agents with grants.skills in the topology."
          entries={data.skills.map((s) => ({ name: s.name, path: s.path, note: s.holders.length ? `granted to ${s.holders.join(', ')}` : 'granted to no agent' }))}
        />
        <Toolbox onChange={() => setGeneration((g) => g + 1)} />
      </div>
    </div>
  )
}
