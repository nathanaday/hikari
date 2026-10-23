import { cn } from '@/lib/utils'
import type { AgentState } from '@/lib/types'

// The dots use the colors of the map's activity lens (rule U47): cyan while
// the agent works, muted cyan while it waits, an outline when it does not
// run, and a red outline for an error.
const styles: Record<AgentState, string> = {
  starting: 'border border-[var(--lens-outline)] bg-white',
  idle: 'bg-[var(--lens-waiting)]',
  working: 'bg-[var(--lens-active)] animate-pulse',
  indexing: 'bg-[var(--lens-active)] animate-pulse',
  error: 'border-[1.5px] border-[var(--lens-fail)] bg-white',
  exited: 'border border-[var(--lens-outline)] bg-white',
}

export function StatusDot({ state, className }: { state: AgentState; className?: string }) {
  return <span aria-label={state} className={cn('inline-block size-2 shrink-0 rounded-full', styles[state], className)} />
}
