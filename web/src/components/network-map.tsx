import { Hash } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { StatusDot } from '@/components/status-dot'
import type { State } from '@/lib/store'
import type { View } from '@/lib/view'

// NetworkMap shows each hub with its orchestrator and members. An agent in
// more than one hub is the path between them.
export function NetworkMap({ state, show }: { state: State; show: (v: View) => void }) {
  return (
    <div className="grid gap-3 px-4 pt-4 pb-2 sm:grid-cols-2 lg:grid-cols-3 md:px-6">
      {state.hubs.map((hub) => (
        <Card
          key={hub.name}
          role="button"
          tabIndex={0}
          onClick={() => show({ kind: 'hub', name: hub.name })}
          onKeyDown={(e) => e.key === 'Enter' && show({ kind: 'hub', name: hub.name })}
          className="cursor-pointer gap-2 py-3 shadow-none transition-colors hover:bg-neutral-50"
        >
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-1 text-sm">
              <Hash className="size-3.5 text-muted-foreground" />
              {hub.name}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 px-4 text-sm">
            {hub.participants
              .filter((p) => p.name !== state.me)
              .map((p) => {
                const agent = state.agents.find((a) => a.name === p.name)!
                const elsewhere = agent.hubs.filter((h) => h !== hub.name)
                return (
                  <div key={p.name} className="flex items-center gap-2">
                    <StatusDot state={agent.status.state} />
                    <span className={p.role === 'orchestrator' ? 'font-medium' : ''}>{p.name}</span>
                    {p.role === 'orchestrator' && <span className="text-xs text-muted-foreground">orchestrator</span>}
                    {elsewhere.length > 0 && (
                      <span className="ml-auto text-xs text-muted-foreground">
                        also in {elsewhere.map((h) => `#${h}`).join(', ')}
                      </span>
                    )}
                  </div>
                )
              })}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
