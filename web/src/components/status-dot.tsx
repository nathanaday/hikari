import { cn } from '@/lib/utils'
import type { AgentState } from '@/lib/types'

// Color appears only where it carries a signal: activity and failure.
const styles: Record<AgentState, string> = {
  starting: 'border border-neutral-300 bg-white',
  idle: 'bg-neutral-300',
  working: 'bg-emerald-500 animate-pulse',
  error: 'bg-red-500',
  exited: 'border border-neutral-300 bg-white',
}

export function StatusDot({ state, className }: { state: AgentState; className?: string }) {
  return <span aria-label={state} className={cn('inline-block size-2 shrink-0 rounded-full', styles[state], className)} />
}
