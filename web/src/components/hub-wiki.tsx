import { Star } from 'lucide-react'
import { HealthBadge } from '@/components/health'
import { KindBadge } from '@/components/wiki-panel'
import type { Hub, WikiView } from '@/lib/types'
import type { View } from '@/lib/view'
import { orderDocs } from '@/lib/wikis'

// HubWiki lists the documents of a hub's wiki by kind, key stories first,
// and opens each one on the map (rule U7 of the wikis spec).
export function HubWiki({ hub, wiki, show }: { hub: Hub; wiki: WikiView | undefined; show: (v: View) => void }) {
  const docs = orderDocs(wiki?.documents ?? [])
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4" data-testid="hub-wiki">
      <div className="mx-auto max-w-3xl space-y-3">
        <p className="text-sm text-muted-foreground">
          The stories, rules, and principles of #{hub.name}, which its orchestrator writes, and your plans and notes.{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'map', hub: hub.name })}>
            Open the wiki map
          </button>{' '}
          to see how they link the features of its systems, or to write a document.
        </p>
        {docs.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">The wiki of #{hub.name} has no documents yet.</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {docs.map((d) => (
              <li key={d.id}>
                <button
                  type="button"
                  onClick={() => show({ kind: 'map', hub: hub.name, doc: d.id })}
                  className="flex w-full items-start gap-3 px-3 py-2 text-left hover:bg-neutral-50"
                >
                  <span className="w-20 shrink-0 pt-0.5">
                    <KindBadge kind={d.kind} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-1.5 text-sm font-medium">
                      {d.key && <Star className="size-3.5 fill-current text-amber-500" aria-label="key story" />}
                      {d.name || d.id}
                      {d.status && <span className="text-xs font-normal text-muted-foreground">· {d.status}</span>}
                    </span>
                    {d.description && <span className="block truncate text-xs text-muted-foreground">{d.description}</span>}
                  </span>
                  {d.support && (
                    <span className="shrink-0 pt-0.5" title="The health of the features it links">
                      <HealthBadge health={d.support} />
                    </span>
                  )}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
