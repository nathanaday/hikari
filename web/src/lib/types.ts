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

export type AgentState = 'starting' | 'idle' | 'working' | 'error' | 'exited'

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

export type Snapshot = {
  me: string
  agents: Agent[]
  hubs: Hub[]
  messages: Message[]
}

export type ServerEvent =
  | { type: 'snapshot'; snapshot: Snapshot }
  | { type: 'message'; message: Message }
  | { type: 'status'; node: string; status: Status }

export type SendRequest = { hub: string; body: string } | { to: string; body: string }
