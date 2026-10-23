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
  cost?: number
}

export type ActivityKind = 'turn' | 'thinking' | 'text' | 'tool' | 'done'

// Activity is one step of an agent's turn. The driver sends it when the
// step starts and again as it changes, with the same id.
export type Activity = {
  id: number
  turn: number
  time: string
  kind: ActivityKind
  text?: string
  done: boolean
  tool?: string
  input?: unknown
  output?: string
  error?: boolean
  cost?: number
  duration?: number
  unheard?: boolean
}

export type Agent = {
  name: string
  description?: string
  hubs: string[]
  status: Status
}

export type Interface = { name: string; kind: string; description?: string }

export type Health = 'passing' | 'failing' | 'pending' | 'untested'

export type MapState = 'unbuilt' | 'current' | 'stale'

// FeatureState says whether a feature's document describes its code.
export type FeatureState = 'undescribed' | 'stale' | 'current'

export type Feature = {
  id: string
  name: string
  description?: string
  interfaces?: string[]
  tests: { passed: number; failed: number; pending: number; ran?: string }
  health: Health
  state?: FeatureState
  related?: string[]
  // created is when the feature's document was created; older documents
  // have none.
  created?: string
}

export type Flag = { interface: string; problem: string }

// Private holds the owner's private documents: where each feature lives in
// the code, and which features each test touches.
export type Private = {
  layout: {
    version: number
    parts: { path: string; description: string }[] | null
    features: Record<string, { code: string[] | null; notes?: string }>
  }
  tests: {
    version: number
    suites: Record<string, { command: string; description?: string }>
    // tests maps each key of the test map to its links, "[[feature-id]]".
    tests: Record<string, string[] | null>
    gaps?: Record<string, string[] | null>
  }
}

export type Certificate = {
  type: string
  repo?: string
  repository?: string
  tag?: string
  url?: string
  digest?: string
  kind?: string
  revision?: string
  assets?: { name: string; sha256: string }[]
}

// Manifest is release.yaml: hikari writes every field when it publishes.
export type Manifest = {
  deliverable: string
  version: string
  system: string
  released: string
  notes: string
  hikari: string
  source_commit?: string
  hubs: string[]
  product: { path: string; files: number; bytes: number; digest: string }
  map: string
  certificates: Certificate[]
}

// ReleaseDetail is a release for the viewer: its files, relative to the
// store, and the digest of each product file, relative to the release.
export type ReleaseDetail = { manifest: Manifest; files: string[]; digests: Record<string, string> }

export type Layout = { require?: string[]; allow?: string[]; forbid?: string[] }

export type Target = { type: string; repo?: string; repository?: string; kind?: string }

export type Deliverable = {
  name: string
  path: string
  skill: string
  description: string
  layout: Layout
  targets: Target[]
  holders: string[]
  error?: string
}

export type SharedListing = {
  policies: { name: string; path: string }[]
  skills: { name: string; path: string }[]
  agents: { name: string; path: string }[]
  deliverables: Deliverable[]
}

// SystemView is everything about one system: the public map as the owner
// sees it, its private documents, and its repo details.
export type SystemView = {
  version: number
  system: {
    name: string
    summary?: string
    responsibilities?: string[]
    interfaces: { provides: Interface[]; consumes: Interface[] }
    described_state: { commit?: string; updated?: string }
  }
  features: Feature[]
  state: MapState
  changed?: string[]
  error?: string
  private_errors?: string[]
  private: Private
  releases: Manifest[] | null
  owner: string
  repo: string
  scope: string
  branch: string
  // dirty says whether the scope has uncommitted changes.
  dirty?: boolean
  grants?: { releases: string[]; deliverables: string[] }
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
  version?: string
  me: string
  agents: Agent[]
  hubs: Hub[]
  messages: Message[]
  systems: SystemView[]
  vocabularies: Vocabulary[]
  repos?: RepoInfo[]
  deliverables?: DeliverableInfo[]
  activity?: Record<string, Activity[]>
}

// RepoInfo is one repository of the topology with the systems in it.
export type RepoInfo = { name: string; branch: string; systems: string[] }

// DeliverableInfo is a deliverable of the store with the agents that hold it.
export type DeliverableInfo = { name: string; description?: string; holders: string[]; error?: string }

export type ServerEvent =
  | { type: 'snapshot'; snapshot: Snapshot }
  | { type: 'message'; message: Message }
  | { type: 'status'; node: string; status: Status }
  | { type: 'activity'; node: string; activity: Activity }
  | { type: 'system'; system: SystemView }
  | { type: 'vocabulary'; vocabulary: Vocabulary }

export type SendRequest = { hub: string; body: string } | { to: string; body: string }
