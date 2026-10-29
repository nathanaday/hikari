import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Activity as ActivityIcon, AlertCircle, Check } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { StatusDot } from '@/components/status-dot'
import type { State } from '@/lib/store'
import { entryAnchor, entryView } from '@/lib/feed'
import { groupTurns, stepLabel } from '@/lib/turns'
import type { Entry } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { View } from '@/lib/view'

export function FeedView({ state, show }: { state: State; show: (v: View) => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const [open, setOpen] = useState<number | null>(null)
  const names = useMemo(() => new Set(state.participants.map((p) => p.name)), [state.participants])

  useLayoutEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight })
  }, [])
  const count = state.feed.length
  useEffect(() => {
    if (ref.current && atBottom.current) ref.current.scrollTo({ top: ref.current.scrollHeight })
  }, [count])
  const onScroll = () => {
    const el = ref.current!
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
  }

  const follow = (e: Entry) => {
    const v = entryView(e, state)
    if (!v) return
    show(v)
    const anchor = entryAnchor(e)
    if (anchor) {
      requestAnimationFrame(() => requestAnimationFrame(() => document.getElementById(anchor)?.scrollIntoView({ block: 'center' })))
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <Strip state={state} />
      <div ref={ref} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto font-mono text-[13px]" data-testid="feed">
        {state.feed.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-6 py-16 text-center font-sans text-sm text-muted-foreground">
            <ActivityIcon className="size-6 text-neutral-300" />
            <p className="font-medium text-foreground">Nothing yet</p>
            <p>The feed fills as the agents connect and work.</p>
          </div>
        ) : (
          <div className="py-2">
            {state.feed.map((e) => (
              <EntryRow key={e.id} entry={e} open={open === e.id} toggle={() => setOpen(open === e.id ? null : e.id)} follow={entryView(e, state) ? () => follow(e) : undefined} me={state.me} names={names} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// Strip shows each agent's state and current step, and the run's cost
// (rule FD9).
function Strip({ state }: { state: State }) {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-1 border-b bg-neutral-50/60 px-4 py-2 text-xs md:px-6" data-testid="strip">
      {state.agents.map((a) => {
        const turn = groupTurns(a.name, state.activity[a.name] ?? []).at(-1)
        const current = turn?.live ? turn.steps.findLast((s) => !s.done) : undefined
        const working = a.status.state === 'working' || a.status.state === 'indexing'
        return (
          <span key={a.name} className="inline-flex items-center gap-1.5" data-testid="strip-agent">
            <StatusDot state={a.status.state} />
            <span className="font-medium">{a.name}</span>
            {working && <span className="max-w-48 truncate text-muted-foreground">· {current ? stepLabel(current) : a.status.state}</span>}
            {a.status.state === 'error' && <span className="max-w-48 truncate text-red-600">· error</span>}
          </span>
        )
      })}
      <span className="ml-auto text-muted-foreground" data-testid="cost">
        ${state.cost.toFixed(2)} spent
      </span>
    </div>
  )
}

function EntryRow({ entry: e, open, toggle, follow, me, names }: { entry: Entry; open: boolean; toggle: () => void; follow?: () => void; me: string; names: ReadonlySet<string> }) {
  const time = new Date(e.time)
  const who = e.who === me ? 'you' : e.who
  const to = e.kind === 'message' ? (e.to ? (e.to === me ? 'you' : e.to) : 'room') : undefined
  const name = who && (
    <span className="shrink-0 font-semibold">
      {follow ? (
        <button type="button" className="hover:underline" onClick={(ev) => { ev.stopPropagation(); follow() }} title="Open">
          {who}
        </button>
      ) : (
        who
      )}
      {to && <span className="font-normal text-muted-foreground"> → {to}</span>}
    </span>
  )
  return (
    <div data-testid="entry" data-kind={e.kind} data-open={open || undefined} className={cn('cursor-pointer px-4 py-0.5 hover:bg-neutral-50 md:px-6', open && 'bg-neutral-50')} onClick={toggle}>
      <div className="flex items-baseline gap-2 leading-6">
        <time dateTime={e.time} title={time.toLocaleString()} className="shrink-0 text-muted-foreground">
          {time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
        </time>
        {e.kind === 'step' ? (
          <span className="shrink-0">{e.error ? <AlertCircle className="inline size-3.5 text-red-600" /> : <Check className="inline size-3.5 text-emerald-600" />}</span>
        ) : (
          name
        )}
        <div className={cn('min-w-0 flex-1', open ? 'whitespace-pre-wrap break-words' : 'truncate', e.error && 'text-red-600')} data-testid="entry-text">
          {open && e.kind === 'message' ? <Markdown text={e.text} names={names} /> : open ? e.text : e.text.replace(/\s+/g, ' ')}
        </div>
      </div>
    </div>
  )
}
