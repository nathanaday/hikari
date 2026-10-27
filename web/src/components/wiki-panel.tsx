import { useState } from 'react'
import { HealthBadge } from '@/components/health'
import { Markdown } from '@/components/markdown'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { asText, vaultFileURL, useFetch } from '@/hooks/use-vault-file'
import type { DocKind, WikiDoc, WikiView } from '@/lib/types'
import { cn } from '@/lib/utils'
import { documentBody } from '@/lib/wiki'
import { agentIDProblem, isUserKind, kindsOf, removeDoc, template, userDocProblem, wikiName, writeDoc, type WikiIndex } from '@/lib/wikis'

const noNames: ReadonlySet<string> = new Set()

export function KindBadge({ kind }: { kind: DocKind }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground" data-testid="kind">
      <span className="inline-block size-2 shrink-0 rounded-full" style={{ background: kind === 'note' ? 'var(--lens-none)' : `var(--kind-${kind})` }} />
      {kind}
    </span>
  )
}

// DocumentPanel shows one document of a wiki: its kind, its health or
// support, its text, and the documents that link it (rule U5 of the wikis
// spec), and lets the user change it (rule U6).
export function DocumentPanel({ wiki, doc, index, onLink }: { wiki: WikiView; doc: WikiDoc; index: WikiIndex; onLink: (id: string) => void }) {
  const text = useFetch(`${vaultFileURL(doc.file)}&v=${doc.hash}`, asText)
  const [editing, setEditing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const backlinks = index.backlinks.get(doc.id) ?? []
  const user = isUserKind(doc.kind)

  if (editing && text.data !== undefined) {
    return <DocumentEditor wiki={wiki} id={doc.id} initial={text.data} onDone={() => setEditing(false)} />
  }
  const remove = async () => {
    const err = await removeDoc(wiki.ref, doc.id)
    setError(err)
    setConfirm(false)
  }
  return (
    <div className="space-y-2 pr-6" data-testid="document-panel">
      <div>
        <h2 className="flex items-center gap-1.5 text-sm font-semibold">
          {doc.name || doc.id}
        </h2>
        <code className="text-xs text-muted-foreground">
          {doc.id} · {wikiName(wiki.ref)}
        </code>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <KindBadge kind={doc.kind} />
        {doc.health && <HealthBadge health={doc.health} />}
        {doc.support && (
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground" title="The health of the features it links">
            support <HealthBadge health={doc.support} />
          </span>
        )}
        {doc.status && <span className="text-xs text-muted-foreground">status: {doc.status}</span>}
      </div>
      {doc.description && <p className="text-sm">{doc.description}</p>}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="outline" onClick={() => setEditing(true)} disabled={text.data === undefined}>
          Edit
        </Button>
        {user &&
          (confirm ? (
            <>
              <Button size="sm" variant="destructive" onClick={remove}>
                Remove it
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setConfirm(false)}>
                Keep it
              </Button>
            </>
          ) : (
            <Button size="sm" variant="ghost" onClick={() => setConfirm(true)}>
              Remove
            </Button>
          ))}
      </div>
      {error && <p className="text-xs text-red-700">{error}</p>}
      {text.loading && <p className="text-xs text-muted-foreground">Loading the document…</p>}
      {text.error && <p className="text-xs text-red-700">{text.error}</p>}
      {text.data !== undefined && <Markdown text={documentBody(text.data)} names={noNames} onLocalLink={onLink} />}
      {backlinks.length > 0 && (
        <div className="border-t pt-2" data-testid="backlinks">
          <div className="text-xs text-muted-foreground">Linked from</div>
          <ul className="mt-1 space-y-0.5">
            {backlinks.map((b) => (
              <li key={`${b.wiki.ref}/${b.doc.id}`} className="text-sm">
                <button type="button" className="underline underline-offset-2" onClick={() => onLink(b.doc.id)}>
                  {b.doc.name || b.doc.id}
                </button>{' '}
                <span className="text-xs text-muted-foreground">
                  {b.doc.kind} · {wikiName(b.wiki.ref)}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

// DocumentEditor edits a document's markdown, header included. The server
// checks it as a hand edit that must be valid (rule U8).
export function DocumentEditor({ wiki, id, initial, onDone }: { wiki: WikiView; id: string; initial: string; onDone: (saved: boolean) => void }) {
  const [text, setText] = useState(initial)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const save = async () => {
    setSaving(true)
    const err = await writeDoc(wiki.ref, id, text)
    setSaving(false)
    setError(err)
    if (!err) onDone(true)
  }
  return (
    <div className="space-y-2 pr-6" data-testid="document-editor">
      <h2 className="text-sm font-semibold">
        {id} <span className="font-normal text-muted-foreground">· {wikiName(wiki.ref)}</span>
      </h2>
      <Textarea value={text} onChange={(e) => setText(e.target.value)} className="min-h-80 font-mono text-xs" aria-label="Document text" />
      {error && <p className="text-xs whitespace-pre-wrap text-red-700">{error}</p>}
      <div className="flex gap-2">
        <Button size="sm" onClick={save} disabled={saving}>
          Save
        </Button>
        <Button size="sm" variant="ghost" onClick={() => onDone(false)}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

// NewDocument asks for a kind and a name, then opens the editor with a
// template (rule U6).
export function NewDocument({ wiki, onDone }: { wiki: WikiView; onDone: (id: string | null) => void }) {
  const [kind, setKind] = useState<DocKind>('note')
  const [name, setName] = useState('')
  const [editing, setEditing] = useState(false)
  const problem = name === '' ? null : isUserKind(kind) ? userDocProblem(name) : agentIDProblem(name)
  const taken = wiki.documents.some((d) => d.id.toLowerCase() === name.toLowerCase())
  if (editing) {
    return <DocumentEditor wiki={wiki} id={name} initial={template(kind, name)} onDone={(saved) => onDone(saved ? name : null)} />
  }
  return (
    <div className="space-y-3 pr-6" data-testid="new-document">
      <h2 className="text-sm font-semibold">New document in {wikiName(wiki.ref)}</h2>
      <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Kind">
        {kindsOf(wiki.scope).map((k) => (
          <button
            key={k}
            type="button"
            role="radio"
            aria-checked={kind === k}
            onClick={() => setKind(k)}
            className={cn('rounded-md border px-2 py-1 text-xs', kind === k ? 'border-foreground font-medium' : 'text-muted-foreground hover:text-foreground')}
          >
            {k}
          </button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {isUserKind(kind)
          ? 'A plan or a note is yours: agents read it and never change it. Its name is its file name.'
          : `A ${kind} keeps the rules of its kind, and its author can change it later. Its name is its id: lowercase letters, digits, and dashes.`}
      </p>
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder={isUserKind(kind) ? 'Roadmap' : 'customer-orders-items'}
        aria-label="Name"
        className="w-full rounded-md border px-2 py-1.5 text-sm"
      />
      {(problem || taken) && <p className="text-xs text-red-700">{problem ?? 'A document of this wiki has this name.'}</p>}
      <div className="flex gap-2">
        <Button size="sm" onClick={() => setEditing(true)} disabled={name === '' || !!problem || taken}>
          Write it
        </Button>
        <Button size="sm" variant="ghost" onClick={() => onDone(null)}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
