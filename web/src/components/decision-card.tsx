import { useEffect, useState } from 'react'
import { CircleHelp, Gavel } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { decide } from '@/lib/decisions'
import type { Message } from '@/lib/types'

// DecisionCard shows a decision as numbered choices (rule DC5 of the
// attention spec). While it is the one waiting, the digits 1 to 9 pick a
// choice; words in the field go with it. Once answered, it shows the pick.
export function DecisionCard({ m, answer, hot, onAnswered }: { m: Message; answer?: Message; hot?: boolean; onAnswered?: () => void }) {
  const d = m.decision!
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const pick = async (n: number) => {
    const err = await decide(m.id, n, note)
    setError(err)
    if (!err) {
      setNote('')
      onAnswered?.()
    }
  }
  useEffect(() => {
    if (!hot || answer) return
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null
      if (el && (el.tagName === 'TEXTAREA' || (el.tagName === 'INPUT' && !el.dataset.decisionNote))) return
      const n = Number(e.key)
      if (n >= 1 && n <= d.options.length && !e.metaKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault()
        pick(n)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hot, answer, note, d.options.length])
  const chosen = answer ? Number(answer.body.match(/^(\d+)\./)?.[1]) : undefined
  const p = d.proposal
  return (
    <div data-testid="decision" data-answered={!!answer} data-proposal={!!p} className={`my-1.5 rounded-md border px-3 py-2 text-sm ${answer ? 'bg-neutral-50' : 'border-violet-300 bg-violet-50/40'}`}>
      <div className="flex items-start gap-2">
        {p ? <Gavel className="mt-0.5 size-4 shrink-0 text-violet-700" /> : <CircleHelp className="mt-0.5 size-4 shrink-0 text-violet-700" />}
        <div className="min-w-0 flex-1">
          {p ? (
            <div className="space-y-1">
              <p className="text-xs font-medium tracking-wide text-violet-700 uppercase">Proposed rule · {p.name}</p>
              <p className="font-medium">{p.rule}</p>
              <p className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">Why:</span> {p.reason}
              </p>
              <p className="text-xs text-muted-foreground">
                Accept writes it into the public wiki as <code>{p.id}</code>, which every agent reads from its next turn; you can edit it there.
              </p>
            </div>
          ) : (
            <p className="font-medium">{d.question}</p>
          )}
          <ol className="mt-1.5 space-y-1">
            {d.options.map((o, i) => {
              const n = i + 1
              const picked = chosen === n
              return (
                <li key={n}>
                  {answer ? (
                    <span className={picked ? 'font-medium' : 'text-muted-foreground'}>
                      {n}. {o}
                      {picked && <span className="ml-1.5 text-xs text-violet-700">your pick</span>}
                    </span>
                  ) : (
                    <Button size="sm" variant="outline" className="h-7 justify-start font-normal" onClick={() => pick(n)} data-testid="decision-option">
                      <span className="w-4 text-muted-foreground">{n}</span>
                      {o}
                    </Button>
                  )}
                </li>
              )
            })}
          </ol>
          {answer ? (
            answer.body.includes('\n') && <p className="mt-1 text-xs text-muted-foreground">{answer.body.slice(answer.body.indexOf('\n') + 1)}</p>
          ) : (
            <div className="mt-2 flex items-center gap-2">
              <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="Words to go with your pick, optional" aria-label="Words with the pick" className="h-7 text-xs" data-decision-note="1" />
              <span className="shrink-0 text-xs text-muted-foreground">{hot ? 'Press a number' : ''}</span>
            </div>
          )}
          {error && (
            <p className="mt-1 text-xs text-red-600" role="alert">
              {error}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
