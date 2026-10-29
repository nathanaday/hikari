import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { ArrowUp, Loader2, MessageSquarePlus, Square, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Markdown } from '@/components/markdown'
import { ParticipantAvatar } from '@/components/participant-avatar'
import { TurnCard } from '@/components/turn-card'
import { api, completions, efforts, models, permissionModes } from '@/lib/assistant'
import type { State } from '@/lib/store'
import { groupTurns, timeline } from '@/lib/turns'
import type { AssistantPrompt, Message } from '@/lib/types'
import { cn } from '@/lib/utils'

// agentName is how the turns of the hikari agent are named in the panel.
const agentName = 'hikari'

const minWidth = 320
const maxWidth = 800

function stored(key: string, fallback: number): number {
  try {
    const v = Number(localStorage.getItem(key))
    return v >= minWidth && v <= maxWidth ? v : fallback
  } catch {
    return fallback
  }
}

// AssistantPanel is the chat with the hikari agent (rules HA14 to HA17 of
// the hikari agent spec).
export function AssistantPanel({ state, close }: { state: State; close: () => void }) {
  const a = state.assistant
  const [width, setWidth] = useState(() => stored('hikari.panel.width', 440))
  const dragging = useRef(false)
  useEffect(() => {
    const move = (e: MouseEvent) => {
      if (!dragging.current) return
      setWidth(Math.min(maxWidth, Math.max(minWidth, window.innerWidth - e.clientX)))
    }
    const up = () => {
      if (!dragging.current) return
      dragging.current = false
      document.body.style.cursor = ''
      try {
        localStorage.setItem('hikari.panel.width', String(width))
      } catch {
        // The width is a convenience.
      }
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
    return () => {
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', up)
    }
  }, [width])

  return (
    <aside data-testid="assistant-panel" className="fixed inset-y-0 right-0 z-40 flex w-full flex-col border-l bg-background md:static md:w-[var(--panel-width)] md:shrink-0" style={{ '--panel-width': `${width}px` } as React.CSSProperties}>
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the panel"
        onMouseDown={(e) => {
          e.preventDefault()
          dragging.current = true
          document.body.style.cursor = 'col-resize'
        }}
        className="absolute inset-y-0 -left-1 hidden w-2 cursor-col-resize md:block"
      />
      <Header state={state} close={close} />
      {a.enabled ? (
        <>
          <Timeline state={state} />
          <Composer state={state} />
        </>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center text-sm text-muted-foreground" data-testid="assistant-off">
          <p className="font-medium text-foreground">The hikari agent is off</p>
          <p>{a.reason ?? 'hikari is not running agents.'}</p>
        </div>
      )}
    </aside>
  )
}

function Header({ state, close }: { state: State; close: () => void }) {
  const a = state.assistant
  const working = a.state === 'working'
  const [error, setError] = useState<string | null>(null)
  const option = (model: string, effort: string, mode: string) => api.options(model, effort, mode).then(setError)
  const select = (label: string, value: string, options: { value: string; label: string }[], onChange: (v: string) => void, testid: string) => (
    <select
      aria-label={label}
      data-testid={testid}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="h-7 max-w-36 truncate rounded-md border bg-background px-1.5 text-xs"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  )
  return (
    <header className="shrink-0 border-b px-3 py-2">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-semibold">Hikari</h2>
        <span className="flex items-center gap-1 text-xs text-muted-foreground" data-testid="assistant-state">
          {working && <Loader2 className="size-3 animate-spin text-emerald-600" />}
          {working ? 'working' : 'idle'}
        </span>
        <span className="text-xs text-muted-foreground" data-testid="assistant-cost">
          ${a.cost.toFixed(2)}
        </span>
        <span className="flex-1" />
        {working && (
          <Button variant="outline" size="xs" onClick={() => api.stop().then(setError)} aria-label="Stop the turn" data-testid="assistant-stop">
            <Square className="size-3" /> Stop
          </Button>
        )}
        <Button variant="ghost" size="icon-xs" onClick={() => api.newChat().then(setError)} disabled={working || !a.enabled} aria-label="New chat" title="New chat" data-testid="assistant-new">
          <MessageSquarePlus />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={close} aria-label="Close the panel">
          <X />
        </Button>
      </div>
      {a.enabled && (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {select('Model', a.model, models, (v) => option(v, a.effort, a.permission_mode), 'assistant-model')}
          {select('Effort', a.effort, efforts, (v) => option(a.model, v, a.permission_mode), 'assistant-effort')}
          {select('Permission mode', a.permission_mode, permissionModes, (v) => option(a.model, a.effort, v), 'assistant-mode')}
        </div>
      )}
      {error && <p className="mt-1 text-xs text-red-600">{error}</p>}
    </header>
  )
}

function Timeline({ state }: { state: State }) {
  const a = state.assistant
  const ref = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const names = useMemo(() => new Set(state.participants.map((p) => p.name)), [state.participants])
  const items = useMemo(() => {
    const messages: Message[] = a.messages.map((m) => ({ id: m.id, from: state.me, body: m.text, time: m.time }))
    return timeline(messages, groupTurns(agentName, a.activity))
  }, [a.messages, a.activity, state.me])
  const count = a.activity.length + a.messages.length + a.prompts.length
  useLayoutEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight })
  }, [])
  useEffect(() => {
    if (ref.current && atBottom.current) ref.current.scrollTo({ top: ref.current.scrollHeight })
  }, [count])
  const onScroll = () => {
    const el = ref.current!
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
  }
  return (
    <div ref={ref} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto" data-testid="assistant-timeline">
      {items.length === 0 ? (
        <div className="px-6 py-16 text-center text-sm text-muted-foreground">
          <p className="font-medium text-foreground">Ask Hikari anything about the project</p>
          <p className="mt-1">Your own Claude Code, in the project root, with the Hikari plugin. It is not in the room; it posts there as you when you ask.</p>
        </div>
      ) : (
        <div className="py-3">
          {items.map((item) => {
            if (item.kind === 'message') {
              const m = item.message
              return (
                <div key={`m${m.id}`} className="flex gap-3 px-3 pt-2.5 pb-1" data-testid="assistant-message">
                  <ParticipantAvatar name={state.me} me className="size-6" />
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium">you</div>
                    <Markdown text={m.body} names={names} />
                  </div>
                </div>
              )
            }
            const t = item.turn
            const prompts = a.prompts.filter((p) => p.turn === t.id)
            return (
              <div key={`t${t.id}`} className="-mx-1">
                <TurnCard turn={t} names={names} reply>
                  {prompts.map((p) => (
                    <PromptCard key={p.id} prompt={p} />
                  ))}
                </TurnCard>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// promptInput shows what a tool asks to do: a command, a file, or its
// input as JSON.
function promptInput(input: unknown): string {
  const o = (input ?? {}) as Record<string, unknown>
  if (typeof o.command === 'string') return o.command
  if (typeof o.file_path === 'string') return o.file_path
  return JSON.stringify(input ?? {}, null, 2)
}

function PromptCard({ prompt: p }: { prompt: AssistantPrompt }) {
  const [error, setError] = useState<string | null>(null)
  return (
    <div data-testid="assistant-prompt" data-answer={p.answer} className={cn('my-1.5 rounded-md border px-3 py-2 text-sm', !p.answer ? 'border-amber-300 bg-amber-50' : 'bg-neutral-50')}>
      <div className="flex items-baseline gap-2">
        <span className="font-medium">{p.tool}</span>
        {p.description && <span className="min-w-0 truncate text-xs text-muted-foreground">{p.description}</span>}
      </div>
      <pre className="mt-1 max-h-40 overflow-auto rounded bg-white/70 p-2 text-xs whitespace-pre-wrap text-neutral-800">{promptInput(p.input)}</pre>
      {p.answer ? (
        <p className="mt-1 text-xs text-muted-foreground">{p.answer === 'allow' ? 'Allowed' : `Denied${p.message ? `: ${p.message}` : ''}`}</p>
      ) : (
        <div className="mt-2 flex gap-2">
          <Button size="xs" onClick={() => api.answer(p.id, true).then(setError)} data-testid="assistant-allow">
            Allow
          </Button>
          <Button size="xs" variant="outline" onClick={() => api.answer(p.id, false).then(setError)} data-testid="assistant-deny">
            Deny
          </Button>
        </div>
      )}
      {error && <p className="mt-1 text-xs text-red-600">{error}</p>}
    </div>
  )
}

function Composer({ state }: { state: State }) {
  const a = state.assistant
  const working = a.state === 'working'
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState(0)
  const ref = useRef<HTMLTextAreaElement>(null)
  const options = completions(draft, a.commands)
  const pick = (cmd: string) => {
    setDraft(`/${cmd} `)
    setSelected(0)
    ref.current?.focus()
  }
  const submit = async () => {
    if (!draft.trim() || working) return
    const err = await api.send(draft)
    setError(err)
    if (!err) setDraft('')
    ref.current?.focus()
  }
  return (
    <div className="shrink-0 px-3 pb-3">
      {error && <p className="mb-2 text-sm text-red-600">{error}</p>}
      {options.length > 0 && (
        <ul role="listbox" aria-label="Slash commands" data-testid="assistant-completions" className="mb-1 max-h-48 overflow-y-auto rounded-md border bg-background py-1 text-sm shadow-sm">
          {options.map((c, i) => (
            <li key={c} role="option" aria-selected={i === selected}>
              <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => pick(c)} className={cn('w-full px-3 py-1 text-left font-mono text-xs', i === selected ? 'bg-neutral-100' : 'hover:bg-neutral-50')}>
                /{c}
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="rounded-lg border bg-background focus-within:border-neutral-400"
      >
        <Textarea
          ref={ref}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            setSelected(0)
          }}
          onKeyDown={(e) => {
            if (options.length > 0) {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setSelected((selected + 1) % options.length)
                return
              }
              if (e.key === 'ArrowUp') {
                e.preventDefault()
                setSelected((selected + options.length - 1) % options.length)
                return
              }
              if (e.key === 'Tab' || e.key === 'Enter') {
                e.preventDefault()
                pick(options[selected])
                return
              }
            }
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault()
              submit()
            }
          }}
          rows={1}
          disabled={working}
          placeholder={working ? 'Hikari is working…' : 'Ask Hikari, or type / for a skill'}
          aria-label="Message Hikari"
          className="max-h-48 min-h-11 resize-none border-0 px-3 py-2.5 shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center justify-between gap-2 px-3 pb-2">
          <p className="truncate text-xs text-muted-foreground">{working ? 'Stop the turn to send another message.' : 'Enter sends · Shift-Enter for a line'}</p>
          <Button type="submit" size="icon" className="size-7 shrink-0" disabled={!draft.trim() || working} aria-label="Send">
            <ArrowUp className="size-4" />
          </Button>
        </div>
      </form>
    </div>
  )
}
