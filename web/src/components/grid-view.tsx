import { useMemo } from 'react'
import { CircleAlert } from 'lucide-react'
import { ParticipantAvatar } from '@/components/participant-avatar'
import { StatusDot } from '@/components/status-dot'
import { StatusChip } from '@/components/work-view'
import { attentionFor, attentionItems } from '@/lib/attention'
import { currentStep, held } from '@/lib/grid'
import { roleLabel } from '@/lib/labels'
import type { State } from '@/lib/store'
import type { Message } from '@/lib/types'
import type { View } from '@/lib/view'

// lastMessage is the latest message the agent sent or received.
function lastMessage(state: State, agent: string): Message | undefined {
  return [...state.messages].reverse().find((m) => m.from === agent || m.to === agent)
}

// GridView shows every agent as a tile: its status, the step it is on, the
// tasks it holds, its last message, and whether it waits for you (rule
// GR1). A tile opens the agent's direct messages.
export function GridView({ state, show }: { state: State; show: (v: View) => void }) {
  const items = useMemo(() => attentionItems(state), [state])
  if (state.agents.length === 0) return <p className="flex-1 py-12 text-center text-sm text-muted-foreground">No agents are running.</p>
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 md:px-6" data-testid="grid-view">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {state.agents.map((a) => {
          const step = currentStep(state, a.name)
          const tasks = held(state, a)
          const last = lastMessage(state, a.name)
          const needs = attentionFor(items, a.name)
          return (
            <button
              key={a.name}
              type="button"
              onClick={() => show({ kind: 'dm', name: a.name })}
              className={`flex min-h-36 flex-col gap-2 rounded-lg border p-3 text-left transition-colors hover:bg-neutral-50 ${needs ? 'border-amber-400' : ''}`}
              data-testid="agent-tile"
              data-agent={a.name}
            >
              <div className="flex items-center gap-2">
                <ParticipantAvatar name={a.name} me={false} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 text-sm font-medium">
                    <span className="truncate">{a.name}</span>
                    {needs > 0 && (
                      <span className="inline-flex items-center gap-0.5 rounded bg-amber-100 px-1 text-xs text-amber-800" data-testid="tile-attention">
                        <CircleAlert className="size-3" />
                        needs you
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <StatusDot state={a.status.state} />
                    {a.status.state}
                    <span>· {roleLabel[a.role]}</span>
                  </div>
                </div>
              </div>
              {a.status.detail && <p className="text-xs break-words text-red-600">{a.status.detail}</p>}
              <p className={`truncate text-sm ${step?.live ? '' : 'text-muted-foreground'}`} data-testid="tile-step">
                {step ? step.text : 'no turn yet'}
              </p>
              {tasks.length > 0 && (
                <div className="flex flex-wrap items-center gap-1" data-testid="tile-tasks">
                  {tasks.map((t) => (
                    <span key={t.id} className="flex items-center gap-1 text-xs">
                      <span className="truncate">{t.name}</span>
                      <StatusChip status={a.role === 'orchestration' ? t.status : (t.parts.find((p) => p.partition === a.name)?.status ?? t.status)} />
                    </span>
                  ))}
                </div>
              )}
              {last && (
                <p className="mt-auto truncate text-xs text-muted-foreground" data-testid="tile-message">
                  {last.from === state.me ? 'you' : last.from}
                  {last.to ? ` → ${last.to === state.me ? 'you' : last.to}` : ' in the room'}: {last.body.split('\n')[0]}
                </p>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
