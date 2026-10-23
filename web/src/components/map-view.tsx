import type { State } from '@/lib/store'
import type { View } from '@/lib/view'

// MapView draws the topology map and the feature map of one system.
export function MapView({ state }: { state: State; show: (v: View) => void }) {
  return <div className="min-h-0 flex-1" data-testid="map-view" data-system={state.view.kind === 'map' ? state.view.system : undefined} />
}
