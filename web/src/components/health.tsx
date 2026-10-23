import { cn } from '@/lib/utils'
import type { Health, MapState } from '@/lib/types'

// Health and map state are the only places besides agent status where the UI
// uses color.
const healthDot: Record<Health, string> = {
  passing: 'bg-emerald-500',
  failing: 'bg-red-500',
  pending: 'bg-amber-500',
  untested: 'border border-neutral-300 bg-white',
}

export function HealthBadge({ health }: { health: Health }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground" data-testid="health">
      <span className={cn('inline-block size-2 shrink-0 rounded-full', healthDot[health])} />
      {health}
    </span>
  )
}

const mapDot: Record<MapState, string> = {
  current: 'bg-emerald-500',
  stale: 'bg-amber-500',
  unbuilt: 'border border-neutral-300 bg-white',
}

export function MapStateDot({ state, busy, error, className }: { state: MapState; busy?: boolean; error?: boolean; className?: string }) {
  return (
    <span
      aria-label={error ? 'map error' : `map ${state}`}
      className={cn('inline-block size-2 shrink-0 rounded-full', error ? 'bg-red-500' : mapDot[state], busy && 'animate-pulse', className)}
    />
  )
}
