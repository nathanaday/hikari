import { Activity, Hash } from 'lucide-react'
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
          <span className="text-[15px] font-semibold tracking-tight">hikari</span>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className={state.connection === 'live' ? 'size-1.5 rounded-full bg-emerald-500' : 'size-1.5 rounded-full bg-neutral-300'} />
            {connectionLabel[state.connection]}
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>{item({ kind: 'all' }, 'All activity', <Activity />)}</SidebarMenu>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Hubs</SidebarGroupLabel>
          <SidebarMenu>
            {state.hubs.map((h) => item({ kind: 'hub', name: h.name }, h.name, <Hash />))}
          </SidebarMenu>
        </SidebarGroup>
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
      </SidebarContent>
    </Sidebar>
  )
}
