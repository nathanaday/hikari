import { useCallback, useState } from 'react'
import { FileText } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { asJSON, asText, storeFileURL, useFetch } from '@/hooks/use-store-file'
import type { Certificate, ReleaseDetail } from '@/lib/types'
import { cn } from '@/lib/utils'

const noNames = new Set<string>()

// resolve returns the store path that href, a link in the file at path,
// points to.
function resolve(path: string, href: string): string {
  const parts = path.split('/').slice(0, -1)
  for (const part of href.split('#')[0].split('/')) {
    if (part === '..') parts.pop()
    else if (part !== '.' && part !== '') parts.push(part)
  }
  return parts.join('/')
}

function FileContent({ path, open }: { path: string; open: (path: string) => void }) {
  const { data, error, loading } = useFetch(storeFileURL(path), asText)
  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>
  if (error) return <p className="text-sm text-muted-foreground">{error}</p>
  return path.endsWith('.md') ? (
    <Markdown text={data ?? ''} names={noNames} onLocalLink={(href) => open(resolve(path, href))} />
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

// where returns the place a certificate names, for its type.
function where(c: Certificate): string {
  const parts = [c.repo ?? c.repository, c.kind, c.tag, c.revision?.slice(0, 7)].filter(Boolean)
  return parts.join(' · ')
}

function CertificateRow({ c }: { c: Certificate }) {
  return (
    <li className="text-xs" data-testid="certificate">
      <span className="font-medium">{c.type}</span>{' '}
      {c.url ? (
        <a href={c.url} target="_blank" rel="noreferrer" className="underline underline-offset-2">
          {where(c)}
        </a>
      ) : (
        where(c)
      )}
      {c.digest && <code className="block truncate text-muted-foreground">{c.digest}</code>}
      {c.assets?.map((a) => (
        <code key={a.name} className="block truncate text-muted-foreground">
          {a.name} {a.sha256.slice(0, 12)}…
        </code>
      ))}
    </li>
  )
}

export function ReleaseView({ system, deliverable, version }: { system: string; deliverable: string; version: string }) {
  const parse = useCallback((r: Response) => asJSON<ReleaseDetail>(r), [])
  const { data, error } = useFetch(`/api/ui/store/releases/${system}/${deliverable}/${version}`, parse)
  if (error) return <p className="px-6 py-10 text-sm text-muted-foreground">{error}</p>
  if (!data) return <p className="px-6 py-10 text-sm text-muted-foreground">Loading…</p>
  return <ReleaseBody system={system} deliverable={deliverable} version={version} data={data} />
}

// ReleaseBody shows a loaded release: its manifest, certificates, and files,
// with the product's README as the main page when there is one.
export function ReleaseBody({ system, deliverable, version, data }: { system: string; deliverable: string; version: string; data: ReleaseDetail }) {
  const [open, setOpen] = useState<string | null>(null)
  const m = data.manifest
  const base = `systems/${system}/releases/${deliverable}/${version}/`
  const readme = base + 'product/README.md'
  const shown = open ?? (data.files.includes(readme) ? readme : base + 'release.yaml')
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="release-view">
      <div className="grid max-w-6xl gap-6 px-4 py-5 md:grid-cols-[18rem_1fr] md:px-6">
        <aside className="space-y-4">
          <div className="space-y-1.5">
            {m.notes && <p className="text-sm">{m.notes}</p>}
            <Field label="released">{m.released ? new Date(m.released).toLocaleString() : '—'}</Field>
            <Field label="commit">
              <code className="text-xs">{m.source_commit?.slice(0, 7)}</code>
            </Field>
            <Field label="product">
              {m.product.files} file{m.product.files === 1 ? '' : 's'}, {m.product.bytes} bytes
            </Field>
            <Field label="digest">
              <code className="text-xs break-all">{m.product.digest}</code>
            </Field>
            <Field label="hikari">{m.hikari}</Field>
            {m.hubs?.length ? <Field label="announced">{m.hubs.map((h) => `#${h}`).join(', ')}</Field> : null}
          </div>
          {m.certificates?.length ? (
            <div>
              <div className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">Certificates</div>
              <ul className="space-y-1.5">
                {m.certificates.map((c) => (
                  <CertificateRow key={c.type} c={c} />
                ))}
              </ul>
            </div>
          ) : null}
          <div>
            <div className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">Files</div>
            <ul className="space-y-0.5" data-testid="release-files">
              {data.files.map((f) => {
                const rel = f.slice(base.length)
                const digest = data.digests[rel]
                return (
                  <li key={f}>
                    <button
                      type="button"
                      onClick={() => setOpen(f)}
                      title={digest ? `sha256 ${digest}` : undefined}
                      className={cn(
                        'flex w-full items-center gap-1.5 rounded px-1.5 py-0.5 text-left text-xs hover:bg-neutral-50',
                        shown === f && 'bg-neutral-100 font-medium',
                      )}
                    >
                      <FileText className="size-3 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{rel}</span>
                      {digest && <code className="shrink-0 text-muted-foreground">{digest.slice(0, 8)}</code>}
                    </button>
                  </li>
                )
              })}
            </ul>
          </div>
        </aside>
        <article className="min-w-0" data-testid="release-doc">
          <FileContent key={shown} path={shown} open={setOpen} />
        </article>
      </div>
    </div>
  )
}
