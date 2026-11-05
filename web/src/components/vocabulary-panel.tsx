import { useState } from 'react'
import { BookOpen, ChevronRight, FileText } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { asText, useFetch, vaultFileURL } from '@/hooks/use-vault-file'
import type { Interface, Vocabulary } from '@/lib/types'

// Contract shows the contract an interface names, and opens to its text.
export function Contract({ iface }: { iface: Interface }) {
  const [open, setOpen] = useState(false)
  const path = iface.contract ? `public/contracts/${iface.contract}` : null
  const { data, error } = useFetch(open && path ? vaultFileURL(path) : null, asText)
  if (!iface.contract) return null
  return (
    <details className="group text-xs" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)} data-testid="contract">
      <summary className="flex cursor-pointer list-none items-center gap-1 text-muted-foreground hover:text-foreground [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-3 transition-transform group-open:rotate-90" />
        <FileText className="size-3" />
        contract <code>{iface.contract}</code>
      </summary>
      <div className="mt-1 ml-4">
        {error ? <p className="text-amber-700">{error}</p> : data !== undefined && <pre className="max-h-64 overflow-auto rounded bg-neutral-50 p-2 text-xs whitespace-pre-wrap">{data}</pre>}
      </div>
    </details>
  )
}

// VocabularyPanel shows the project's vocabulary, which the orchestration agent
// keeps.
export function VocabularyPanel({ orchestration, vocabulary }: { orchestration?: string; vocabulary?: Vocabulary }) {
  const terms = vocabulary?.terms ?? []
  const interfaces = vocabulary?.interfaces ?? []
  if (terms.length === 0 && interfaces.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center gap-2 px-6 py-16 text-center text-sm text-muted-foreground" data-testid="vocabulary">
        <BookOpen className="size-6 text-neutral-300" />
        <p className="font-medium text-foreground">No vocabulary yet</p>
        <p>{orchestration ? `${orchestration} defines the shared names of the project as the work needs them.` : 'The project has no orchestration agent, so nobody writes the vocabulary.'}</p>
      </div>
    )
  }
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="vocabulary">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        <p className="text-sm text-muted-foreground">
          The names that everyone in the project uses. {orchestration ? `${orchestration} keeps this vocabulary.` : ''} An interface may name a contract, a file in the vault's public contracts/ that says its shape, which the provider and each consumer build against.
        </p>
        <section className="space-y-2">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Terms ({terms.length})</h2>
          <ul className="divide-y border-y">
            {terms.map((t) => (
              <li key={t.term} className="py-2">
                <div className="text-sm font-medium">{t.term}</div>
                <p className="text-sm text-muted-foreground">{t.definition}</p>
                {t.avoid?.length ? <p className="text-xs text-muted-foreground">Not: {t.avoid.join(', ')}</p> : null}
              </li>
            ))}
          </ul>
        </section>
        <section className="space-y-2">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Interfaces ({interfaces.length})</h2>
          <ul className="divide-y border-y">
            {interfaces.map((i) => (
              <li key={i.name} className="py-2">
                <div className="flex items-center gap-2">
                  <code className="text-sm font-medium">{i.name}</code>
                  <Badge variant="outline" className="h-5 font-normal text-muted-foreground">
                    {i.kind}
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground">{i.description}</p>
                <Contract iface={i} />
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  )
}
