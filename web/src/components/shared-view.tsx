import { useCallback, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { asJSON, asText, storeFileURL, useFetch } from '@/hooks/use-store-file'
import type { SharedListing } from '@/lib/types'

const noNames = new Set<string>()

function Entry({ name, path }: { name: string; path: string }) {
  const [open, setOpen] = useState(false)
  const { data, error } = useFetch(open ? storeFileURL(path) : null, asText)
  return (
    <details className="group border-b last:border-b-0" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} data-testid="shared-entry">
      <summary className="flex cursor-pointer list-none items-center gap-2 py-2 text-sm hover:bg-neutral-50 [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
        <span className="font-medium">{name}</span>
        <code className="text-xs text-muted-foreground">{path}</code>
      </summary>
      <div className="pb-3 pl-5.5">
        {error ? <p className="text-sm text-muted-foreground">{error}</p> : data !== undefined && <Markdown text={data} names={noNames} />}
      </div>
    </details>
  )
}

function Group({ title, hint, entries }: { title: string; hint: string; entries: { name: string; path: string }[] }) {
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

export function SharedView() {
  const parse = useCallback((r: Response) => asJSON<SharedListing>(r), [])
  const { data, error } = useFetch('/api/ui/store/shared', parse)
  if (error) return <p className="px-6 py-10 text-sm text-muted-foreground">{error}</p>
  if (!data) return <p className="px-6 py-10 text-sm text-muted-foreground">Loading…</p>
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="shared-view">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        <Group title="Policies" hint="Add rules for every agent as Markdown files in store/shared/policies/." entries={data.policies} />
        <Group title="Skills" hint="Add Claude Code skills in store/shared/skills/<name>/SKILL.md." entries={data.skills} />
        <Group title="Agents" hint="Add Claude Code subagent definitions in store/shared/agents/<name>.md." entries={data.agents} />
      </div>
    </div>
  )
}
