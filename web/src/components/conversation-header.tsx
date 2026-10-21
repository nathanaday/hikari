import { Hash } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { NodeAvatar } from '@/components/node-avatar'
import { StatusDot } from '@/components/status-dot'
import { orchestratorOf } from '@/lib/rules'
import type { State } from '@/lib/store'

export function ConversationHeader({ state, onMention }: { state: State; onMention: (name: string) => void }) {
  const v = state.view
  const status = (name: string) => state.agents.find((a) => a.name === name)?.status.state ?? 'idle'

  let body: React.ReactNode
  if (v.kind === 'all') {
    body = (
      <>
        <h1 className="text-base font-semibold">All activity</h1>
        <p className="text-sm text-muted-foreground">
          Every message in the network, including private messages between agents.
        </p>
      </>
    )
  } else if (v.kind === 'hub') {
    const hub = state.hubs.find((h) => h.name === v.name)!
    body = (
      <>
        <h1 className="flex items-center gap-1 text-base font-semibold">
          <Hash className="size-4 text-muted-foreground" />
          {hub.name}
        </h1>
        {hub.description && <p className="text-sm text-muted-foreground">{hub.description}</p>}
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {hub.participants.map((p) =>
            p.name === state.me ? (
              <Badge key={p.name} variant="outline" className="h-6 gap-1.5 font-normal text-muted-foreground">
                you
              </Badge>
            ) : (
              <button
                key={p.name}
                type="button"
                title={`Mention @${p.name}`}
                onClick={() => onMention(p.name)}
                className="inline-flex h-6 items-center gap-1.5 rounded-md border px-2 text-xs transition-colors hover:bg-neutral-50"
              >
                <StatusDot state={status(p.name)} />
                <span className="font-medium">{p.name}</span>
                {p.role === 'orchestrator' && <span className="text-muted-foreground">orchestrator</span>}
              </button>
            ),
          )}
        </div>
      </>
    )
  } else {
    const agent = state.agents.find((a) => a.name === v.name)!
    const roles = agent.hubs.map((h) => {
      const hub = state.hubs.find((x) => x.name === h)!
      return `${orchestratorOf(hub) === agent.name ? 'orchestrator' : 'member'} of #${h}`
    })
    body = (
      <div className="flex gap-3">
        <NodeAvatar name={agent.name} me={false} className="mt-0.5" />
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-base font-semibold">
            {agent.name}
            <Badge variant="secondary" className="gap-1.5 font-normal">
              <StatusDot state={agent.status.state} />
              {agent.status.state}
            </Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            {[agent.description, roles.join(', ')].filter(Boolean).join(' · ')}
          </p>
          {agent.status.detail && <p className="mt-1 text-sm break-words text-red-600">{agent.status.detail}</p>}
        </div>
      </div>
    )
  }

  return (
    <header className="flex shrink-0 items-start gap-2 border-b px-4 py-3 md:px-6">
      <SidebarTrigger className="-ml-2 md:hidden" />
      <div className="min-w-0 flex-1">{body}</div>
    </header>
  )
}
