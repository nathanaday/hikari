import { useEffect, useLayoutEffect, useMemo, useRef } from 'react'
import { MessagesSquare, Zap } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { NodeAvatar } from '@/components/node-avatar'
import { TurnCard } from '@/components/turn-card'
import { orchestratorOf } from '@/lib/rules'
import type { State } from '@/lib/store'
import { groupTurns, timeline, turnInView, type Turn } from '@/lib/turns'
import type { Message } from '@/lib/types'
import { inView, viewKey, type View } from '@/lib/view'

const groupWindowMs = 5 * 60 * 1000

function channel(m: Message): string {
  return m.to ? `${[m.from, m.to].sort().join('>')}` : 'room'
}

export function MessageList({ state }: { state: State }) {
  const ref = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const v = state.view
  const shown = useMemo(() => state.messages.filter((m) => inView(m, v, state.me)), [state.messages, v, state.me])
  const turns = useMemo(() => {
    const out: Turn[] = []
    for (const a of state.agents) {
      for (const t of groupTurns(a.name, state.activity[a.name] ?? [])) {
        if (turnInView(t, v, state.me)) out.push(t)
      }
    }
    return out
  }, [state.activity, state.agents, v, state.me])
  const items = useMemo(() => timeline(shown, turns), [shown, turns])
  // Anyone in the room can be mentioned.
  const names = useMemo(() => new Set(state.participants.map((p) => p.name)), [state.participants])

  const key = viewKey(v)
  useLayoutEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight })
    atBottom.current = true
  }, [key])

  // Follow new messages and live steps while the reader is at the bottom,
  // and always after the user sends a message.
  const count = shown.length
  const lastFrom = shown.at(-1)?.from
  const activityCount = turns.reduce((n, t) => n + t.steps.length + (t.end ? 1 : 0), 0)
  useEffect(() => {
    if (ref.current && (atBottom.current || lastFrom === state.me)) {
      ref.current.scrollTo({ top: ref.current.scrollHeight })
    }
  }, [count, lastFrom, activityCount, state.me])

  const onScroll = () => {
    const el = ref.current!
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
  }

  return (
    <div ref={ref} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto" data-testid="log">
      {items.length === 0 ? (
        <Empty view={v} />
      ) : (
        <div className="py-3">
          {items.map((item, i) => {
            if (item.kind === 'turn') {
              const t = item.turn
              return <TurnCard key={`${t.agent}:${t.id}`} turn={t} names={names} />
            }
            const m = item.message
            const prevItem = items[i - 1]
            const prev = prevItem?.kind === 'message' ? prevItem.message : undefined
            const compact =
              prev !== undefined &&
              prev.from === m.from &&
              channel(prev) === channel(m) &&
              Date.parse(m.time) - Date.parse(prev.time) < groupWindowMs
            return <MessageRow key={m.id} m={m} compact={compact} state={state} names={names} />
          })}
        </div>
      )}
    </div>
  )
}

function MessageRow({ m, compact, state, names }: { m: Message; compact: boolean; state: State; names: ReadonlySet<string> }) {
  const me = state.me
  const isOrchestrator = orchestratorOf(state.participants) === m.from
  const time = new Date(m.time)
  const privateDM = !!m.to && m.from !== me && m.to !== me

  return (
    <div data-testid="message" className={`flex gap-3 px-4 hover:bg-neutral-50 md:px-6 ${compact ? 'py-0.5' : 'pt-2.5 pb-0.5'}`}>
      <div className="w-8 shrink-0">{!compact && <NodeAvatar name={m.from} me={m.from === me} />}</div>
      <div className="min-w-0 flex-1">
        {!compact && (
          <div className="flex flex-wrap items-baseline gap-x-2 text-sm leading-5">
            <span className="font-medium">{m.from === me ? 'you' : m.from}</span>
            {isOrchestrator && <span className="text-xs text-muted-foreground">orchestrator</span>}
            {state.view.kind === 'all' && (
              <span className="text-xs text-muted-foreground" data-testid="channel">
                {m.to ? `to ${m.to === me ? 'you' : m.to}${privateDM ? ' · private' : ''}` : 'in the room'}
              </span>
            )}
            <time dateTime={m.time} title={time.toLocaleString()} className="text-xs text-muted-foreground">
              {time.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
            </time>
          </div>
        )}
        <Markdown text={m.body} names={names} />
        {!m.to && (
          <div className="mt-0.5 mb-1 flex items-center gap-1 text-xs text-muted-foreground" data-testid="woke">
            <Zap className="size-3" />
            {m.woke?.length ? `woke ${m.woke.join(', ')}` : 'woke no agents'}
          </div>
        )}
      </div>
    </div>
  )
}

function Empty({ view }: { view: View }) {
  const hint =
    view.kind === 'room'
      ? 'Post to the room to start.'
      : view.kind === 'dm'
        ? `Send ${view.name} a direct message.`
        : 'Open the room or a direct message to send the first message.'
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-16 text-center text-sm text-muted-foreground">
      <MessagesSquare className="size-6 text-neutral-300" />
      <p className="font-medium text-foreground">No messages yet</p>
      <p>{hint}</p>
    </div>
  )
}
