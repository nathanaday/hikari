// Wire types of the server's UI API. They mirror internal/api and the
// snapshot in internal/server.

export type Message = {
  id: number
  from: string
  hub?: string
  to?: string
  body: string
  time: string
  woke?: string[]
}

export type Role = 'orchestrator' | 'member' | 'operator'

export type Peer = {
  name: string
  role: Role
  description?: string
}

export type Hub = {
  name: string
  description?: string
  participants: Peer[]
}

export type AgentState = 'starting' | 'idle' | 'working' | 'indexing' | 'error' | 'exited'

export type Status = {
  state: AgentState
  detail?: string
}

export type Agent = {
  name: string
  description?: string
  hubs: string[]
  status: Status
}

export type Interface = { name: string; kind: string; description?: string }

export type Health = 'passing' | 'failing' | 'stale' | 'untested'

export type MapState = 'unbuilt' | 'current' | 'stale'

export type Feature = {
  id: string
  name: string
  description?: string
  interfaces?: string[]
  code?: string[]
  tests: { refs?: string[]; passed: number; failed: number; ran?: string }
  health: Health
  findings?: string[]
  plans?: string[]
  related?: string[]
}

export type Flag = { interface: string; problem: string }

// SystemView is the full view of one system map, with its repo details.
export type SystemView = {
  version: number
  system: {
    name: string
    summary?: string
    responsibilities?: string[]
    interfaces: { provides: Interface[]; consumes: Interface[] }
    layout?: { path: string; description: string }[]
    test_command?: string
    described_state: { commit?: string; updated?: string }
  }
  features: Feature[]
  state: MapState
  changed?: string[]
  error?: string
  owner: string
  repo: string
  scope: string
  branch: string
  flags: Flag[] | null
  providers: Record<string, string[]> | null
}

export type Vocabulary = {
  version: number
  hub: string
  terms: { term: string; definition: string; avoid?: string[] }[]
  interfaces: Interface[]
}

export type Snapshot = {
  me: string
  agents: Agent[]
  hubs: Hub[]
  messages: Message[]
  systems: SystemView[]
  vocabularies: Vocabulary[]
}

export type ServerEvent =
  | { type: 'snapshot'; snapshot: Snapshot }
  | { type: 'message'; message: Message }
  | { type: 'status'; node: string; status: Status }
  | { type: 'system'; system: SystemView }
  | { type: 'vocabulary'; vocabulary: Vocabulary }

export type SendRequest = { hub: string; body: string } | { to: string; body: string }
