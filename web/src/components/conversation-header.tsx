import { BookOpen, Boxes, Hash, Package } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { MapStateDot } from '@/components/health'
import { NodeAvatar } from '@/components/node-avatar'
import { StatusDot } from '@/components/status-dot'
import { mapStateLabel } from '@/lib/labels'
import { orchestratorOf } from '@/lib/rules'
import type { State } from '@/lib/store'
import { cn } from '@/lib/utils'
import type { HubTab, View } from '@/lib/view'

type Props = { state: State; show: (v: View) => void; onMention: (name: string) => void }

export function ConversationHeader({ state, show, onMention }: Props) {
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
  } else if (v.kind === 'shared') {
    body = (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <BookOpen className="size-4 text-muted-foreground" />
          Shared
        </h1>
        <p className="text-sm text-muted-foreground">
          What every agent can read in store/shared: your policies, skills, and agent definitions. The hub vocabularies are in each hub.
        </p>
      </>
    )
  } else if (v.kind === 'release') {
    body = (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <Package className="size-4 text-muted-foreground" />
          {v.name} {v.version}
        </h1>
        <p className="text-sm text-muted-foreground">
          A release of{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'system', name: v.name })}>
            {v.name}
          </button>
          . It never changes. Agents with access read it at{' '}
          <code className="text-xs">
            /hikari/public/readonly/systems/{v.name}/releases/{v.version}
          </code>
          .
        </p>
      </>
    )
  } else if (v.kind === 'hub') {
    const hub = state.hubs.find((h) => h.name === v.name)!
    const vocab = state.vocabularies.find((x) => x.hub === hub.name)
    const entries = (vocab?.terms.length ?? 0) + (vocab?.interfaces.length ?? 0)
    const tab = v.tab ?? 'messages'
    const tabButton = (t: HubTab, label: string) => (
      <button
        type="button"
        onClick={() => show({ kind: 'hub', name: hub.name, tab: t === 'messages' ? undefined : t })}
        aria-pressed={tab === t}
        className={cn(
          '-mb-px border-b-2 px-1 pb-2 text-sm transition-colors',
          tab === t ? 'border-foreground font-medium' : 'border-transparent text-muted-foreground hover:text-foreground',
        )}
      >
        {label}
      </button>
    )
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
        <div className="mt-3 -mb-3 flex gap-4">
          {tabButton('messages', 'Messages')}
          {tabButton('vocabulary', entries ? `Vocabulary (${entries})` : 'Vocabulary')}
        </div>
      </>
    )
  } else if (v.kind === 'system') {
    const sys = state.systems.find((x) => x.owner === v.name)!
    const owner = state.agents.find((a) => a.name === sys.owner)
    const described = sys.system.described_state
    body = (
      <>
        <h1 className="flex items-center gap-2 text-base font-semibold">
          <Boxes className="size-4 text-muted-foreground" />
          {sys.owner}
          <Badge variant="secondary" className="gap-1.5 font-normal" data-testid="map-state">
            <MapStateDot state={sys.state} busy={owner?.status.state === 'indexing'} error={!!sys.error} />
            {owner?.status.state === 'indexing' ? 'indexing' : mapStateLabel[sys.state]}
          </Badge>
        </h1>
        <p className="text-sm text-muted-foreground">
          Owned by{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'dm', name: sys.owner })}>
            {sys.owner}
          </button>{' '}
          · <code className="text-xs">{sys.scope ? `${sys.repo}/${sys.scope}` : sys.repo}</code> · branch{' '}
          <code className="text-xs">{sys.branch}</code>
          {described.commit && (
            <>
              {' '}
              · describes <code className="text-xs">{described.commit.slice(0, 7)}</code>
            </>
          )}
        </p>
      </>
    )
  } else {
    const agent = state.agents.find((a) => a.name === v.name)!
    const roles = agent.hubs.map((h) => {
      const hub = state.hubs.find((x) => x.name === h)!
      return `${orchestratorOf(hub) === agent.name ? 'orchestrator' : 'member'} of #${h}`
    })
    const owns = state.systems.some((x) => x.owner === agent.name)
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
            {owns && (
              <>
                {' · '}
                <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'system', name: agent.name })}>
                  system map
                </button>
              </>
            )}
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
