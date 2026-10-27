import { BookOpen, Boxes, MessagesSquare, Network, Package } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { MapStateDot } from '@/components/health'
import { ParticipantAvatar } from '@/components/participant-avatar'
import { StatusDot } from '@/components/status-dot'
import { mapStateLabel, roleLabel } from '@/lib/labels'
import type { State } from '@/lib/store'
import { cn } from '@/lib/utils'
import type { RoomTab, View } from '@/lib/view'

type Props = { state: State; show: (v: View) => void; onMention: (name: string) => void }

export function ConversationHeader({ state, show, onMention }: Props) {
  const v = state.view
  if (v.kind === 'topology') return null
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
  } else if (v.kind === 'map') {
    const crumb = (
      <button type="button" className="font-normal text-muted-foreground hover:text-foreground" onClick={() => show({ kind: 'map' })}>
        Map
      </button>
    )
    body = v.partition ? (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <Network className="size-4 text-muted-foreground" />
          {crumb}
          <span className="font-normal text-muted-foreground">/</span>
          {v.partition}
        </h1>
        <p className="text-sm text-muted-foreground">
          The wiki of {v.partition}: its features, principles, and rules, your plans and notes, and the documents of the public wiki that link them.{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'partition', name: v.partition! })}>
            Open the partition page
          </button>
          .
        </p>
      </>
    ) : v.public ? (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <Network className="size-4 text-muted-foreground" />
          {crumb}
          <span className="font-normal text-muted-foreground">/</span>
          public wiki
        </h1>
        <p className="text-sm text-muted-foreground">
          The public wiki: its stories, rules, and principles, your plans and notes, and the features of the partitions that they link.{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'room', tab: 'wiki' })}>
            Open the list
          </button>
          .
        </p>
      </>
    ) : (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <Network className="size-4 text-muted-foreground" />
          Map
        </h1>
        <p className="text-sm text-muted-foreground">
          The orchestration agent and each partition, with its repository and the deliverables it holds. Click a partition to open its wiki, or the orchestration agent to open the public wiki.
        </p>
      </>
    )
  } else if (v.kind === 'shared') {
    body = (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <BookOpen className="size-4 text-muted-foreground" />
          Vault
        </h1>
        <p className="text-sm text-muted-foreground">
          Your files in the vault: the policies and deliverables of vault/public, which every agent reads, and the skills of vault/skills, which the topology grants to agents. The vocabulary and the public wiki are in the room.
        </p>
      </>
    )
  } else if (v.kind === 'release') {
    body = (
      <>
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <Package className="size-4 text-muted-foreground" />
          {v.name} {v.deliverable} {v.version}
        </h1>
        <p className="text-sm text-muted-foreground">
          A release of the deliverable {v.deliverable} by{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'partition', name: v.name })}>
            {v.name}
          </button>
          . It never changes. {v.name} reads it at{' '}
          <code className="text-xs">
            /hikari/self/releases/{v.deliverable}/{v.version}
          </code>
          , the orchestration agent at{' '}
          <code className="text-xs">
            /hikari/partitions/{v.name}/releases/{v.deliverable}/{v.version}
          </code>
          , and each agent granted its releases at{' '}
          <code className="text-xs">
            /hikari/grants/{v.name}/releases/{v.deliverable}/{v.version}
          </code>
          .
        </p>
      </>
    )
  } else if (v.kind === 'room') {
    const entries = state.vocabulary.terms.length + state.vocabulary.interfaces.length
    const docs = state.wikis.find((w) => w.ref === 'public')?.documents.length ?? 0
    const tab = v.tab ?? 'messages'
    const tabButton = (t: RoomTab, label: string) => (
      <button
        type="button"
        onClick={() => show({ kind: 'room', tab: t === 'messages' ? undefined : t })}
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
        <h1 className="flex items-center gap-1.5 text-base font-semibold">
          <MessagesSquare className="size-4 text-muted-foreground" />
          Room
        </h1>
        <p className="text-sm text-muted-foreground">Every agent and you. A post reaches everyone.</p>
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {state.participants.map((p) =>
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
                {p.role === 'orchestration' && <span className="text-muted-foreground">orchestration agent</span>}
              </button>
            ),
          )}
        </div>
        <div className="mt-3 -mb-3 flex gap-4">
          {tabButton('messages', 'Messages')}
          {tabButton('vocabulary', entries ? `Vocabulary (${entries})` : 'Vocabulary')}
          {tabButton('wiki', docs ? `Wiki (${docs})` : 'Wiki')}
        </div>
      </>
    )
  } else if (v.kind === 'partition') {
    const sys = state.partitions.find((x) => x.partition_agent === v.name)!
    const partitionAgent = state.agents.find((a) => a.name === sys.partition_agent)
    const described = sys.partition.described_state
    body = (
      <>
        <h1 className="flex items-center gap-2 text-base font-semibold">
          <Boxes className="size-4 text-muted-foreground" />
          {sys.partition_agent}
          <Badge variant="secondary" className="gap-1.5 font-normal" data-testid="map-state">
            <MapStateDot state={sys.state} busy={partitionAgent?.status.state === 'indexing'} error={!!sys.error} />
            {partitionAgent?.status.state === 'indexing' ? 'indexing' : mapStateLabel[sys.state]}
          </Badge>
        </h1>
        <p className="text-sm text-muted-foreground">
          Owned by{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'dm', name: sys.partition_agent })}>
            {sys.partition_agent}
          </button>{' '}
          · <code className="text-xs">{sys.scope ? `${sys.repo}/${sys.scope}` : sys.repo}</code> · branch{' '}
          <code className="text-xs">{sys.branch}</code>
          {described.commit && (
            <>
              {' '}
              · describes <code className="text-xs">{described.commit.slice(0, 7)}</code>
            </>
          )}{' '}
          ·{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'map', partition: sys.partition_agent })}>
            Feature map
          </button>
        </p>
      </>
    )
  } else {
    const agent = state.agents.find((a) => a.name === v.name)!
    const owns = state.partitions.some((x) => x.partition_agent === agent.name)
    body = (
      <div className="flex gap-3">
        <ParticipantAvatar name={agent.name} me={false} className="mt-0.5" />
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-base font-semibold">
            {agent.name}
            <Badge variant="secondary" className="gap-1.5 font-normal">
              <StatusDot state={agent.status.state} />
              {agent.status.state}
            </Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            {[roleLabel[agent.role], agent.description].filter(Boolean).join(' · ')}
            {owns && (
              <>
                {' · '}
                <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'partition', name: agent.name })}>
                  feature map
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
