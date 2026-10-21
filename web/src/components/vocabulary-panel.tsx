import { BookOpen } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { orchestratorOf } from '@/lib/rules'
import type { Hub, Vocabulary } from '@/lib/types'

export function VocabularyPanel({ hub, vocabulary }: { hub: Hub; vocabulary?: Vocabulary }) {
  const terms = vocabulary?.terms ?? []
  const interfaces = vocabulary?.interfaces ?? []
  if (terms.length === 0 && interfaces.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center gap-2 px-6 py-16 text-center text-sm text-muted-foreground" data-testid="vocabulary">
        <BookOpen className="size-6 text-neutral-300" />
        <p className="font-medium text-foreground">No vocabulary yet</p>
        <p>{orchestratorOf(hub)} defines the shared names of #{hub.name} as the work needs them.</p>
      </div>
    )
  }
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="vocabulary">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        <p className="text-sm text-muted-foreground">
          The names that everyone in #{hub.name} uses. {orchestratorOf(hub)} keeps this vocabulary.
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
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  )
}
