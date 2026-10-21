import { useEffect, useLayoutEffect, useMemo, useRef } from 'react'
import { MessagesSquare, Zap } from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { NetworkMap } from '@/components/network-map'
import { NodeAvatar } from '@/components/node-avatar'
import { orchestratorOf } from '@/lib/rules'
import type { State } from '@/lib/store'
import type { Message } from '@/lib/types'
import { inView, viewKey, type View } from '@/lib/view'

const groupWindowMs = 5 * 60 * 1000

function channel(m: Message): string {
  return m.hub ? `#${m.hub}` : `${[m.from, m.to].sort().join('>')}`
}

export function MessageList({ state, show }: { state: State; show: (v: View) => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const v = state.view
  const shown = useMemo(() => state.messages.filter((m) => inView(m, v, state.me)), [state.messages, v, state.me])
  const hubNames = useMemo(
    () => new Map(state.hubs.map((h) => [h.name, new Set(h.participants.map((p) => p.name))])),
    [state.hubs],
  )
  const noNames = useMemo(() => new Set<string>(), [])

  const key = viewKey(v)
  useLayoutEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight })
    atBottom.current = true
  }, [key])

  // Follow new messages while the reader is at the bottom, and always after
  // the user sends one.
  const count = shown.length
  const lastFrom = shown.at(-1)?.from
  useEffect(() => {
    if (ref.current && (atBottom.current || lastFrom === state.me)) {
      ref.current.scrollTo({ top: ref.current.scrollHeight })
    }
  }, [count, lastFrom, state.me])

  const onScroll = () => {
    const el = ref.current!
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
  }

  return (
    <div ref={ref} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto" data-testid="log">
      {v.kind === 'all' && <NetworkMap state={state} show={show} />}
      {shown.length === 0 ? (
        <Empty view={v} />
      ) : (
        <div className="py-3">
          {shown.map((m, i) => {
            const prev = shown[i - 1]
            const compact =
              prev !== undefined &&
              prev.from === m.from &&
              channel(prev) === channel(m) &&
              Date.parse(m.time) - Date.parse(prev.time) < groupWindowMs
            return (
              <MessageRow
                key={m.id}
                m={m}
                compact={compact}
                state={state}
                names={m.hub ? hubNames.get(m.hub) ?? noNames : noNames}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

function MessageRow({ m, compact, state, names }: { m: Message; compact: boolean; state: State; names: ReadonlySet<string> }) {
  const me = state.me
  const hub = m.hub ? state.hubs.find((h) => h.name === m.hub) : undefined
  const isOrchestrator = hub !== undefined && orchestratorOf(hub) === m.from
  const time = new Date(m.time)
  const privateDM = !m.hub && m.from !== me && m.to !== me

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
                {m.hub ? `in #${m.hub}` : `to ${m.to === me ? 'you' : m.to}${privateDM ? ' · private' : ''}`}
              </span>
            )}
            <time dateTime={m.time} title={time.toLocaleString()} className="text-xs text-muted-foreground">
              {time.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
            </time>
          </div>
        )}
        <Markdown text={m.body} names={names} />
        {m.hub && (
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
    view.kind === 'hub'
      ? `Post to #${view.name} to start.`
      : view.kind === 'dm'
        ? `Send ${view.name} a direct message.`
        : 'Open a hub or a direct message to send the first message.'
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-16 text-center text-sm text-muted-foreground">
      <MessagesSquare className="size-6 text-neutral-300" />
      <p className="font-medium text-foreground">No messages yet</p>
      <p>{hint}</p>
    </div>
  )
}
