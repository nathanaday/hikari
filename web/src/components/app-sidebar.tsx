import { Activity, BookOpen, CircleAlert, FileCode2, GitBranch, LayoutGrid, ListChecks, MessagesSquare, Network } from 'lucide-react'
import { useMemo } from 'react'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { MapStateDot } from '@/components/health'
import { StatusDot } from '@/components/status-dot'
import { attentionFor, attentionItems, type AttentionItem } from '@/lib/attention'
import { repoHealth, type RepoHealth } from '@/lib/repos'
import type { Connection, State } from '@/lib/store'
import { viewKey, type View } from '@/lib/view'

const connectionLabel: Record<Connection, string> = {
  connecting: 'connecting',
  live: 'live',
  reconnecting: 'reconnecting',
  'signed-out': 'signed out',
}

export function AppSidebar({ state, show }: { state: State; show: (v: View) => void }) {
  const active = viewKey(state.view)
  const items = useMemo(() => attentionItems(state), [state])
  const item = (v: View, label: React.ReactNode, icon: React.ReactNode, needs = 0) => (
    <SidebarMenuItem key={viewKey(v)}>
      <SidebarMenuButton isActive={active === viewKey(v)} onClick={() => show(v)}>
        {icon}
        <span className="truncate">{label}</span>
        {needs > 0 && <Needs count={needs} />}
      </SidebarMenuButton>
      {state.unread[viewKey(v)] ? <SidebarMenuBadge>{state.unread[viewKey(v)]}</SidebarMenuBadge> : null}
    </SidebarMenuItem>
  )

  return (
    <Sidebar>
      <SidebarHeader className="px-4 pt-4 pb-2">
        <div className="flex items-baseline justify-between">
          <span className="text-[15px] font-semibold tracking-tight">
            hikari
            {state.version && (
              <span className="ml-1.5 text-xs font-normal text-muted-foreground" data-testid="version">
                v{state.version}
              </span>
            )}
          </span>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className={state.connection === 'live' ? 'size-1.5 rounded-full bg-emerald-500' : 'size-1.5 rounded-full bg-neutral-300'} />
            {connectionLabel[state.connection]}
          </span>
        </div>
        {state.project && (
          <div className="truncate text-xs text-muted-foreground" data-testid="project">
            {state.project}
          </div>
        )}
      </SidebarHeader>
      <SidebarContent>
        {state.mode !== 'running' ? (
          // Without agents, the topology is the whole UI (rule YV5).
          <SidebarGroup>
            <SidebarMenu>{item({ kind: 'topology' }, 'Topology', <FileCode2 />)}</SidebarMenu>
          </SidebarGroup>
        ) : (
          <>
            {items.length > 0 && <NeedsYou items={items} show={show} />}
            <SidebarGroup>
              <SidebarMenu>
                {item({ kind: 'all' }, 'All activity', <Activity />)}
                {item({ kind: 'grid' }, 'Grid', <LayoutGrid />)}
                {state.modules.tasks && item({ kind: 'work' }, 'Work', <ListChecks />, items.filter((i) => i.kind === 'blocked' || i.kind === 'tracker').length)}
                {state.partitions.length > 0 && item({ kind: 'map' }, 'Map', <Network />)}
                {state.repos.length > 0 && item({ kind: 'repos' }, 'Repositories', <GitBranch />)}
                {item({ kind: 'shared' }, 'Vault', <BookOpen />)}
                {item({ kind: 'topology' }, 'Topology', <FileCode2 />)}
              </SidebarMenu>
            </SidebarGroup>
            <SidebarGroup>
              <SidebarMenu>{item({ kind: 'room' }, 'Room', <MessagesSquare />)}</SidebarMenu>
            </SidebarGroup>
            {state.partitions.length > 0 && (
              <SidebarGroup>
                <SidebarGroupLabel>Partitions</SidebarGroupLabel>
                <SidebarMenu>
                  {state.partitions.map((sys) => {
                    const partitionAgent = state.agents.find((a) => a.name === sys.partition_agent)
                    const busy = partitionAgent?.status.state === 'working' || partitionAgent?.status.state === 'indexing'
                    return item(
                      { kind: 'partition', name: sys.partition_agent },
                      sys.partition_agent,
                      <span className="flex size-4 items-center justify-center">
                        <MapStateDot state={sys.state} busy={busy} error={!!sys.error} />
                      </span>,
                    )
                  })}
                </SidebarMenu>
              </SidebarGroup>
            )}
            {state.repos.length > 0 && (
              // One entry per repository, with the dot of its state (rule UI1).
              <SidebarGroup>
                <SidebarGroupLabel>Repositories</SidebarGroupLabel>
                <SidebarMenu>
                  {[...state.repos]
                    .sort((a, b) => Number(!!a.project) - Number(!!b.project) || a.name.localeCompare(b.name))
                    .map((r) =>
                      item(
                        { kind: 'repo', name: r.name },
                        r.name,
                        <span className="flex size-4 items-center justify-center">
                          <RepoDot health={repoHealth(r)} />
                        </span>,
                        attentionFor(items, r.name),
                      ),
                    )}
                </SidebarMenu>
              </SidebarGroup>
            )}
            <SidebarGroup>
              <SidebarGroupLabel>Direct messages</SidebarGroupLabel>
              <SidebarMenu>
                {state.agents.map((a) =>
                  item(
                    { kind: 'dm', name: a.name },
                    a.name,
                    <span className="flex size-4 items-center justify-center">
                      <StatusDot state={a.status.state} />
                    </span>,
                    attentionFor(items, a.name),
                  ),
                )}
              </SidebarMenu>
            </SidebarGroup>
          </>
        )}
      </SidebarContent>
    </Sidebar>
  )
}

// Needs marks an entry that waits for you, with how many items do (rule
// AT3).
function Needs({ count }: { count: number }) {
  return (
    <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 rounded bg-amber-100 px-1 text-[11px] font-medium text-amber-800" data-testid="needs" aria-label={`${count} waiting for you`}>
      <CircleAlert className="size-3" />
      {count > 1 ? count : ''}
    </span>
  )
}

const kindLabel: Record<AttentionItem['kind'], string> = {
  decision: 'decide',
  message: 'answer',
  blocked: 'blocked',
  prompt: 'permission',
  pull: 'pull request',
  repo: 'repository',
  tracker: 'tracker',
}

const maxShown = 6

// NeedsYou lists everything blocked on you, at the top of the sidebar
// (rules AT1 to AT3).
function NeedsYou({ items, show }: { items: AttentionItem[]; show: (v: View) => void }) {
  return (
    <SidebarGroup data-testid="needs-you">
      <SidebarGroupLabel className="text-amber-800">
        <CircleAlert className="mr-1 size-3.5" />
        Needs you ({items.length})
      </SidebarGroupLabel>
      <SidebarMenu>
        {items.slice(0, maxShown).map((i) => (
          <SidebarMenuItem key={i.key}>
            <SidebarMenuButton onClick={() => show(i.view)} className="h-auto items-start py-1" title={i.text}>
              <span className="mt-0.5 w-16 shrink-0 text-[11px] text-muted-foreground">{kindLabel[i.kind]}</span>
              <span className="line-clamp-2 text-xs whitespace-normal">{i.text}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
        {items.length > maxShown && <p className="px-2 text-xs text-muted-foreground">and {items.length - maxShown} more</p>}
      </SidebarMenu>
    </SidebarGroup>
  )
}

const repoDot: Record<RepoHealth, string> = { good: 'bg-emerald-500', warn: 'bg-amber-500', bad: 'bg-red-500' }

// RepoDot is the state of a repository: green, amber for work to push or
// checks to wait for, red for failing checks, a conflict, or an error.
export function RepoDot({ health }: { health: RepoHealth }) {
  return <span aria-label={`repository ${health}`} className={`inline-block size-2 shrink-0 rounded-full ${repoDot[health]}`} data-health={health} />
}
