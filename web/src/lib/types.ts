// Wire types of the server's UI API. They mirror internal/api and the
// snapshot in internal/server.

// Message is a post to the room, or a direct message when to is set.
export type Message = {
  id: number
  from: string
  to?: string
  body: string
  time: string
  woke?: string[]
}

export type Role = 'orchestrator' | 'owner' | 'operator'

export type Peer = {
  name: string
  role: Role
  description?: string
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
  role: Role
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
  terms: { term: string; definition: string; avoid?: string[] }[]
  interfaces: Interface[]
}

// DocKind is the kind of a wiki document: agents write features,
// principles, rules, and stories; the user writes plans and notes.
export type DocKind = 'feature' | 'principle' | 'rule' | 'story' | 'plan' | 'note'

// WikiDoc is one document of a wiki, without its body.
export type WikiDoc = {
  id: string
  kind: DocKind
  name?: string
  description?: string
  // links are the documents it links, in any wiki.
  links: string[]
  status?: string
  // health is a feature's; support sums the features that a document of
  // the public wiki links.
  health?: Health
  support?: Health
  created?: string
  // file is the document's path in the store; hash changes with its text.
  file: string
  hash: string
}

// WikiView is the wiki of a system, or the public wiki. ref is
// "system:<name>" or "public".
export type WikiView = {
  ref: string
  scope: 'system' | 'public'
  name: string
  documents: WikiDoc[]
  problems?: string[]
  warnings?: string[]
}

export type Mode = 'editing' | 'starting' | 'running' | 'stopping'

export type Snapshot = {
  version?: string
  project?: string
  // mode says what hikari up does, and file and revision which compose
  // file it edits (rule UP9 of the topology API spec).
  mode?: Mode
  file?: string
  revision?: string
  start_findings?: Finding[]
  me: string
  agents: Agent[]
  // participants lists everyone in the room: the orchestrator first and
  // the user last.
  participants: Peer[]
  messages: Message[]
  systems: SystemView[]
  vocabulary: Vocabulary
  repos?: RepoInfo[]
  deliverables?: DeliverableInfo[]
  wikis?: WikiView[]
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
  | { type: 'wiki'; wiki: WikiView }
  | { type: 'topology'; revision: string }

// SendRequest posts to the room, or sends a direct message when to is set.
export type SendRequest = { to?: string; body: string }

// The topology API (section 6 of the topology API spec).

export type Severity = 'error' | 'warning'

export type Finding = {
  severity: Severity
  stage: 'yaml' | 'compose' | 'topology' | 'start'
  message: string
  path?: string
  line?: number
}

// orchestration is the service id of the orchestrator, or "".
export type TopologyProject = { id: string; name: string; store: string; repo: boolean; branch: string; orchestration: string }
export type TopologyRepo = { id: string; name: string; url: string; path: string; branch: string; line: number; id_locked: boolean }
export type TopologySystem = { repo: string; scope: string; read: string[]; test: string }
export type TopologyAgent = {
  id: string
  name: string
  description: string
  instructions: string
  model: string
  system: TopologySystem | null
  releases: string[]
  deliverables: string[]
  image: string
  build: string
  template: string
  compose: string[]
  line: number
  id_locked: boolean
}
export type TopologySidecar = { id: string; serves: string; image: string; line: number }
export type TopologyModel = {
  project: TopologyProject
  repos: TopologyRepo[]
  agents: TopologyAgent[]
  sidecars: TopologySidecar[]
}

export type TopologyState = {
  file: string
  exists: boolean
  revision: string
  text: string
  model: TopologyModel | null
  findings: Finding[]
  mode: Mode
  commits: boolean
}

export type DiffLine = { op: ' ' | '+' | '-'; text: string; old?: number; new?: number }

export type Occurrence = { key?: string; file: string; line: number; text: string; replace: string; from: string; to: string }

export type Plan = {
  revision: string
  text: string
  diff: DiffLine[]
  findings: Finding[]
  effects: string[]
  occurrences: Occurrence[]
  summary: string[]
  applies_at_start: boolean
  // model is the model of the new text (decision 1 of the structure view
  // spec).
  model: TopologyModel | null
}

export type Saved = { revision: string; commit?: string; note?: string; skipped: string[] }
