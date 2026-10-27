import type { MapState, Role } from './types'

export const mapStateLabel: Record<MapState, string> = {
  current: 'map current',
  stale: 'map stale',
  unbuilt: 'map not built',
}

export const roleLabel: Record<Role, string> = {
  orchestration: 'orchestration agent',
  partition: 'partition agent',
  operator: 'operator',
}
