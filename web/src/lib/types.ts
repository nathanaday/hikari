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

// hikari is hikari itself, which writes the messages of its modules.
export type Role = 'orchestration' | 'partition' | 'operator' | 'hikari'

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

export type EntryKind = 'message' | 'turn' | 'status' | 'module' | 'step'

// Entry is one entry of the feed (rule FD1 of the feed spec): what the
// terminal prints as one line. message, turn, and task link what it
// summarizes.
export type Entry = {
  id: number
  time: string
  kind: EntryKind
  // who is the sender, the agent, "hikari" for a module, or absent for a
  // step; to is a message's recipient, or absent for the room.
  who?: string
  to?: string
  text: string
  error?: boolean
  message?: number
  turn?: number
  task?: string
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

// Private holds the partition agent's private documents: where each feature lives in
// the code, and which features each test touches.
export type Private = {
  code_map: {
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
  partition: string
  released: string
  notes: string
  hikari: string
  // source_commits are the commits that the map describes, as
  // <repo>@<commit>; source_commit is the one commit of an older release.
  source_commits?: string[]
  source_commit?: string
  product: { path: string; files: number; bytes: number; digest: string }
  map: string
  certificates: Certificate[]
}

// ReleaseDetail is a release for the viewer: its files, relative to the
// store, and the digest of each product file, relative to the release.
// dir is the release's directory, relative to the vault.
export type ReleaseDetail = { manifest: Manifest; dir: string; files: string[]; digests: Record<string, string> }

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

// SharedListing is the vault's public area and your skills (rule V13 of
// the vault layout spec).
export type SharedListing = {
  policies: { name: string; path: string }[]
  skills: { name: string; path: string; holders: string[] }[]
  deliverables: Deliverable[]
}

// Folder is one folder of a partition's workspace, by its place under
// /workspace.
export type Folder = { place: string; repo: string; path: string; read_only: boolean }

// RepoState is a repo that a partition commits to: its branch, and whether
// the partition's folders in it hold uncommitted changes.
export type RepoState = { name: string; branch: string; dirty: boolean }

// PartitionView is everything about one partition: the public map as the partition agent
// sees it, its private documents, and its repo details.
export type PartitionView = {
  version: number
  partition: {
    name: string
    summary?: string
    responsibilities?: string[]
    interfaces: { provides: Interface[]; consumes: Interface[] }
    // commits holds one <repo>@<commit> for each repo of the writable
    // folders.
    described_state: { commits?: string[]; updated?: string }
  }
  features: Feature[]
  state: MapState
  changed?: string[]
  error?: string
  private_errors?: string[]
  private: Private
  releases: Manifest[] | null
  partition_agent: string
  folders: Folder[]
  repos: RepoState[]
  // dirty says whether a writable folder has uncommitted changes.
  dirty?: boolean
  grants?: { releases: string[]; deliverables: string[]; skills: string[] }
  flags: Flag[] | null
  providers: Record<string, string[]> | null
}

export type Vocabulary = {
  version: number
  terms: { term: string; definition: string; avoid?: string[] }[]
  interfaces: Interface[]
}

// DocKind is the kind of a wiki document: agents write features,
// principles, and rules; the user writes plans and notes.
export type DocKind = 'feature' | 'principle' | 'rule' | 'plan' | 'note'

export type WikiScope = 'partition' | 'orchestration' | 'public'

// WikiDoc is one document of a wiki, without its body.
export type WikiDoc = {
  id: string
  kind: DocKind
  name?: string
  description?: string
  // links are the documents it links, in any wiki.
  links: string[]
  status?: string
  // health is a feature's, which an orchestration feature takes from the
  // partition features it links; support sums the features that a rule or
  // a principle of the public wiki or the orchestration feature map links.
  health?: Health
  support?: Health
  created?: string
  // file is the document's path in the vault; hash changes with its text.
  file: string
  hash: string
}

// WikiView is the feature map of a partition or of the orchestration
// agent, or the public wiki. ref is "partition:<name>", "orchestration", or
// "public".
export type WikiView = {
  ref: string
  scope: WikiScope
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
  // participants lists everyone in the room: the orchestration agent first and
  // the user last.
  participants: Peer[]
  messages: Message[]
  partitions: PartitionView[]
  vocabulary: Vocabulary
  repos?: RepoInfo[]
  deliverables?: DeliverableInfo[]
  wikis?: WikiView[]
  activity?: Record<string, Activity[]>
  // modules holds the view of each module that the project turns on.
  modules?: Modules
  // feed holds the recent entries of the feed, and cost the run's cost so
  // far (rules FD2 and FD6).
  feed?: Entry[]
  cost?: number
  // assistant is the hikari agent's view, or why the run has none.
  assistant?: AssistantView
}

// The task pipeline, the tasks module (rule TK27 of the task pipeline spec).
export type TaskStatus = 'queued' | 'waiting' | 'started' | 'blocked' | 'testing' | 'validating' | 'done'

export type TaskPart = { partition: string; status: TaskStatus }

export type TaskView = {
  id: string
  name: string
  stage: 'stub' | 'scope'
  author?: string
  // status is empty for a stub and for a draft scope.
  status?: TaskStatus
  block?: string
  wait?: string
  queue?: string
  partitions: string[]
  features: string[]
  needs: string[]
  parts: TaskPart[]
  created?: string
  updated?: string
  file: string
  hash: string
}

export type QueueView = { id: string; name: string; state: 'running' | 'paused'; scopes: TaskView[]; file: string }

export type TasksView = {
  stubs: TaskView[]
  drafts: TaskView[]
  queues: QueueView[]
  problems: { file: string; reason: string }[]
  orchestration: string
  partitions: string[]
}

export type Modules = { tasks?: TasksView }

// The hikari agent (rule HA7 of the hikari agent spec).
export type AssistantMessage = { id: number; time: string; text: string }

// AssistantPrompt is a permission that a turn asked for; answer is set once
// given.
export type AssistantPrompt = {
  id: string
  turn: number
  time: string
  tool: string
  input?: unknown
  description?: string
  answer?: 'allow' | 'deny'
  message?: string
}

export type AssistantStatus = {
  session: string
  model: string
  effort: string
  permission_mode: string
  state: 'idle' | 'working'
  cost: number
  commands: string[]
}

export type AssistantView = AssistantStatus & {
  enabled: boolean
  reason?: string
  messages: AssistantMessage[]
  activity: Activity[]
  prompts: AssistantPrompt[]
}

export type AssistantEvent =
  | { type: 'message'; message: AssistantMessage }
  | { type: 'activity'; activity: Activity }
  | { type: 'prompt'; prompt: AssistantPrompt }
  | { type: 'status'; status: AssistantStatus }

// RepoInfo is one repository of the topology with the partitions in it.
export type RepoInfo = { name: string; branch: string; partitions: string[] }

// DeliverableInfo is a deliverable of the vault with the agents that hold it.
export type DeliverableInfo = { name: string; description?: string; holders: string[]; error?: string }

export type ServerEvent =
  | { type: 'snapshot'; snapshot: Snapshot }
  | { type: 'message'; message: Message }
  | { type: 'status'; agent: string; status: Status }
  | { type: 'activity'; agent: string; activity: Activity }
  | { type: 'partition'; partition: PartitionView }
  | { type: 'vocabulary'; vocabulary: Vocabulary }
  | { type: 'wiki'; wiki: WikiView }
  | { type: 'topology'; revision: string }
  | { type: 'module'; module: string; view: unknown }
  | { type: 'feed'; entry: Entry }
  | { type: 'assistant'; assistant: AssistantEvent }

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

// orchestration is the service id of the orchestration agent, or "".
export type TopologyProject = { id: string; name: string; vault: string; repo: boolean; branch: string; orchestration: string }
export type TopologyRepo = { id: string; name: string; url: string; path: string; branch: string; line: number; id_locked: boolean }
// TopologyFolder is a folder as the file writes it: repo is the repo's id.
export type TopologyFolder = { repo: string; path: string; read_only: boolean; at: string }
// old says that the file writes the form before folders.
export type TopologyPartition = { folders: TopologyFolder[]; test: string; old?: boolean }
export type TopologyAgent = {
  id: string
  name: string
  description: string
  instructions: string
  model: string
  partition: TopologyPartition | null
  releases: string[]
  deliverables: string[]
  skills: string[]
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
  // mounts maps each agent's service id to its mounts, when the text
  // passes the checks (rule V14).
  mounts: Record<string, Mount[]> | null
  mode: Mode
  commits: boolean
  // packs maps each role of an agent to its built-in skills (rule K14 of
  // the skill packs spec).
  packs: Packs
}

export type Packs = Partial<Record<'partition' | 'orchestration', string[]>>

// Mount is one mount of an agent's container. host is relative to the
// project when it is inside it, and "" for an empty directory that hides a
// path.
export type Mount = { container: string; host: string; mode: 'read-only' | 'writable' }

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
  mounts: Record<string, Mount[]> | null
  packs: Packs
}

export type Saved = { revision: string; commit?: string; note?: string; skipped: string[] }
