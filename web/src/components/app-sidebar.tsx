import { Activity, BookOpen, FileCode2, MessagesSquare, Network } from 'lucide-react'
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
  const item = (v: View, label: React.ReactNode, icon: React.ReactNode) => (
    <SidebarMenuItem key={viewKey(v)}>
      <SidebarMenuButton isActive={active === viewKey(v)} onClick={() => show(v)}>
        {icon}
        <span className="truncate">{label}</span>
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
            <SidebarGroup>
              <SidebarMenu>
                {item({ kind: 'all' }, 'All activity', <Activity />)}
                {state.partitions.length > 0 && item({ kind: 'map' }, 'Map', <Network />)}
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
