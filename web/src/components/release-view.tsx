import { useCallback, useState } from 'react'
import { FileText } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { asJSON, asText, storeFileURL, useFetch } from '@/hooks/use-store-file'
import type { ReleaseDetail } from '@/lib/types'
import { cn } from '@/lib/utils'

const noNames = new Set<string>()

function FileContent({ path }: { path: string }) {
  const { data, error, loading } = useFetch(storeFileURL(path), asText)
  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>
  if (error) return <p className="text-sm text-muted-foreground">{error}</p>
  return path.endsWith('.md') ? (
    <Markdown text={data ?? ''} names={noNames} />
  ) : (
    <pre className="overflow-x-auto rounded-md border bg-neutral-50 p-3 text-xs">{data}</pre>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3 text-sm">
      <span className="w-24 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words">{children}</span>
    </div>
  )
}

export function ReleaseView({ system, version }: { system: string; version: string }) {
  const parse = useCallback((r: Response) => asJSON<ReleaseDetail>(r), [])
  const { data, error } = useFetch(`/api/ui/store/releases/${system}/${version}`, parse)
  const [open, setOpen] = useState<string | null>(null)

  if (error) return <p className="px-6 py-10 text-sm text-muted-foreground">{error}</p>
  if (!data) return <p className="px-6 py-10 text-sm text-muted-foreground">Loading…</p>

  const m = data.manifest
  const base = `systems/${system}/releases/${version}/`
  const shown = open ?? base + m.docs.index
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="release-view">
      <div className="grid max-w-6xl gap-6 px-4 py-5 md:grid-cols-[16rem_1fr] md:px-6">
        <aside className="space-y-4">
          <div className="space-y-1.5">
            {m.notes && <p className="text-sm">{m.notes}</p>}
            <Field label="released">{m.released ? new Date(m.released).toLocaleString() : '—'}</Field>
            <Field label="commit">
              <code className="text-xs">{m.source_commit?.slice(0, 7)}</code>
            </Field>
            <Field label="product">
              {m.product.kind === 'remote' ? (
                <code className="text-xs">
                  {m.product.location?.type}: {m.product.location?.ref}
                </code>
              ) : (
                <code className="text-xs">{m.product.path}</code>
              )}
            </Field>
            {m.runtime.requires?.length ? <Field label="requires">{m.runtime.requires.join(', ')}</Field> : null}
            {m.runtime.install && (
              <Field label="install">
                <code className="text-xs">{m.runtime.install}</code>
              </Field>
            )}
            {m.runtime.run && (
              <Field label="run">
                <code className="text-xs">{m.runtime.run}</code>
              </Field>
            )}
            {m.interfaces?.length ? <Field label="provides">{m.interfaces.join(', ')}</Field> : null}
          </div>
          <div>
            <div className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">Files</div>
            <ul className="space-y-0.5" data-testid="release-files">
              {data.files.map((f) => (
                <li key={f}>
                  <button
                    type="button"
                    onClick={() => setOpen(f)}
                    className={cn(
                      'flex w-full items-center gap-1.5 truncate rounded px-1.5 py-0.5 text-left text-xs hover:bg-neutral-50',
                      shown === f && 'bg-neutral-100 font-medium',
                    )}
                  >
                    <FileText className="size-3 shrink-0 text-muted-foreground" />
                    {f.slice(base.length)}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </aside>
        <article className="min-w-0" data-testid="release-doc">
          <FileContent key={shown} path={shown} />
        </article>
      </div>
    </div>
  )
}
