import { NodeAvatar } from '@/components/node-avatar'
import type { TopologyHub, TopologyModel } from '@/lib/types'

function Node({ name, role, me, onClick }: { name: string; role?: string; me?: boolean; onClick?: () => void }) {
  const body = (
    <>
      <NodeAvatar name={name} me={!!me} className="size-9" />
      <span className="max-w-24 truncate text-xs font-medium">{name}</span>
      {role && <span className="text-[11px] text-muted-foreground">{role}</span>}
    </>
  )
  return onClick ? (
    <button type="button" onClick={onClick} className="flex flex-col items-center gap-1 rounded-md px-2 py-1 hover:bg-neutral-50" data-testid="diagram-node">
      {body}
    </button>
  ) : (
    <div className="flex flex-col items-center gap-1 px-2 py-1">{body}</div>
  )
}

// HubDiagram draws a hub: its orchestrator above its members, and you,
// who are in every hub, beside them (decision 4 of the structure view
// spec). The lines are the hub: everyone in it reads every post.
export function HubDiagram({ hub, model, onAgent }: { hub: TopologyHub; model: TopologyModel; onAgent: (id: string) => void }) {
  const nameOf = (id: string) => model.agents.find((a) => a.id === id)?.name ?? id
  const tester = model.agents.find((a) => a.tests === hub.id)?.id
  const member = (m: string) => (m === tester ? 'tester' : model.agents.find((a) => a.id === m)?.system ? 'owner' : 'member')
  const slot = 7 // rem: the width of each member's column, so that the rail meets their centers
  return (
    <div className="relative rounded-md border bg-neutral-50/60 px-4 pt-4 pb-3" data-testid="hub-diagram">
      <div className="absolute top-3 right-3 opacity-80">
        <Node name="you" me role="in every hub" />
      </div>
      <div className="flex flex-col items-center">
        <Node name={nameOf(hub.orchestrator)} role="orchestrator" onClick={() => onAgent(hub.orchestrator)} />
        {hub.members.length > 0 && (
          <>
            <span className="h-4 w-px bg-[var(--lens-outline)]" />
            <span className="h-px bg-[var(--lens-outline)]" style={{ width: `${(hub.members.length - 1) * slot}rem` }} />
            <div className="flex">
              {hub.members.map((m) => (
                <div key={m} className="flex flex-col items-center" style={{ width: `${slot}rem` }}>
                  <span className="h-3 w-px bg-[var(--lens-outline)]" />
                  <Node name={nameOf(m)} role={member(m)} onClick={() => onAgent(m)} />
                </div>
              ))}
            </div>
          </>
        )}
        {hub.members.length === 0 && <p className="mt-2 text-xs text-muted-foreground">No members yet. The orchestrator works alone, with you.</p>}
      </div>
    </div>
  )
}
