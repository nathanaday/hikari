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
  key?: boolean
  status?: string
  // health is a feature's; support sums the features a hub's story or
  // rule links.
  health?: Health
  support?: Health
  created?: string
  // file is the document's path in the store; hash changes with its text.
  file: string
  hash: string
}

// WikiView is the wiki of a system or a hub. ref is "system:<name>" or
// "hub:<name>".
export type WikiView = {
  ref: string
  scope: 'system' | 'hub'
  name: string
  documents: WikiDoc[]
  problems?: string[]
  warnings?: string[]
  // tests is the E2E test plan of a hub that has a tester.
  tests?: TestPlan
}

export type Component = { release?: string; image?: string; fixture?: boolean; simulates?: string; substitutes?: string; note?: string }
export type Actor = { runs: string; count: number; note?: string }
export type Scenario = { stories: string[]; actors: Record<string, number> | null; minutes?: number }
export type Gap = { id: string; what: string; why: string; touches: string[]; approved: boolean }
export type LockedRelease = { system: string; deliverable: string; version: string; digest: string }

export type StoryHealth = {
  id: string
  kind: DocKind
  name?: string
  key?: boolean
  health: Health
  passed: number
  failed: number
  pending: number
  support?: Health
  suspects?: string[]
  gaps?: string[]
  bench?: string[]
}

// TestPlan is the plan of a tested hub as hikari reads it (rule PL7 of the
// tester spec).
export type TestPlan = {
  hub: string
  tester: string
  plan: {
    components: Record<string, Component> | null
    actors: Record<string, Actor> | null
    scenarios: Record<string, Scenario> | null
    gaps: Record<string, Omit<Gap, 'id' | 'approved'>> | null
  }
  body?: string
  lock: { version: number; releases: LockedRelease[] | null }
  problems?: string[]
  warnings?: string[]
  gaps: Gap[]
  bench?: BenchProcedure[]
  stories: StoryHealth[]
  environment?: EnvironmentStatus
}

// BenchProcedure is a check the user runs on hardware, with the last result
// (rule BR4 of the bench spec).
export type BenchProcedure = {
  id: string
  title: string
  touches: string[]
  procedure: string
  result?: { passed: boolean; note?: string; at: string; current: boolean }
}

export type EnvironmentService = { name: string; state: string; health?: string; image: string; platform?: string; replicas: number; emulated?: boolean }

// EnvironmentStatus is the state of a hub's environment (rule ST6 of the
// environments spec).
export type EnvironmentStatus = {
  state: 'none' | 'starting' | 'running' | 'failed' | 'stopped'
  scenario?: string
  reason?: string
  services?: EnvironmentService[]
  started?: string
  stops?: string
  logs?: string
  connected: boolean
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
  hubs: Hub[]
  messages: Message[]
  systems: SystemView[]
  vocabularies: Vocabulary[]
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

export type SendRequest = { hub: string; body: string } | { to: string; body: string }

// The topology API (section 6 of the topology API spec).

export type Severity = 'error' | 'warning'

export type Finding = {
  severity: Severity
  stage: 'yaml' | 'compose' | 'topology' | 'start'
  message: string
  path?: string
  line?: number
}

export type TopologyProject = { id: string; name: string; store: string; repo: boolean; branch: string }
export type TopologyRepo = { id: string; name: string; url: string; path: string; branch: string; line: number; id_locked: boolean }
export type TopologyHub = {
  id: string
  name: string
  description: string
  orchestrator: string
  members: string[]
  line: number
  id_locked: boolean
}
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
  tests: string
  e2e: { cpus: string; memory: string } | null
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
  hubs: TopologyHub[]
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
