import type { MapState } from './types'

export const mapStateLabel: Record<MapState, string> = {
  current: 'map current',
  stale: 'map stale',
  unbuilt: 'map not built',
}
