import type { Activity, Agent, DeliverableInfo, Entry, Finding, Message, Mode, Modules, Peer, RepoInfo, ServerEvent, PartitionView, Vocabulary, WikiView } from './types'
import { allView, conversationKeys, viewKey, type View } from './view'

export type Connection = 'connecting' | 'live' | 'reconnecting' | 'signed-out'

export type State = {
  connection: Connection
  loaded: boolean
  // version is the version of the running hikari.
  version: string
  // project is the project's name.
  project: string
  // mode, file, and revision follow hikari up and its topology file, and
  // startFindings say why the last start failed (rule YS1).
  mode: Mode
  file: string
  revision: string
  startFindings: Finding[]
  me: string
  agents: Agent[]
  // participants lists everyone in the room: the orchestration agent first and
  // the user last.
  participants: Peer[]
  messages: Message[]
  partitions: PartitionView[]
  vocabulary: Vocabulary
  // repos and deliverables let the map draw the topology.
  repos: RepoInfo[]
  deliverables: DeliverableInfo[]
  // wikis holds the public wiki and the wiki of every partition.
  wikis: WikiView[]
  // activity holds the recent steps of each agent, oldest first.
  activity: Record<string, Activity[]>
  // modules holds the view of each module that the project turns on.
  modules: Modules
  // feed holds the recent entries of the feed, oldest first, and cost the
  // run's cost so far.
  feed: Entry[]
  cost: number
  view: View
  // unread counts messages per conversation key that arrived while another
  // view was open.
  unread: Record<string, number>
}

export type Action =
  | { type: 'event'; event: ServerEvent }
  | { type: 'view'; view: View }
  | { type: 'connection'; connection: Connection }

export function initialState(view: View): State {
  return {
    connection: 'connecting',
    loaded: false,
    version: '',
    project: '',
    mode: 'running',
    file: '',
    revision: '',
    startFindings: [],
    me: 'user',
    agents: [],
    participants: [],
    messages: [],
    partitions: [],
    vocabulary: emptyVocabulary,
    repos: [],
    deliverables: [],
    wikis: [],
    activity: {},
    modules: {},
    feed: [],
    cost: 0,
    view,
    unread: {},
  }
}

// maxFeed is the most entries kept, as the server keeps them.
const maxFeed = 500

const emptyVocabulary: Vocabulary = { version: 1, terms: [], interfaces: [] }

// resolve returns v if it names an agent or a partition of s, and All activity
// if not. Without agents, only the Topology view exists (rule YV5).
function resolve(v: View, s: Pick<State, 'agents' | 'partitions' | 'mode' | 'modules'>): View {
  if (v.kind === 'topology') return v
  if (s.mode !== 'running') return { kind: 'topology' }
  if (v.kind === 'room') return v
  if (v.kind === 'work') return s.modules.tasks ? v : allView
  if (v.kind === 'dm' && s.agents.some((a) => a.name === v.name)) return v
  if ((v.kind === 'partition' || v.kind === 'release') && s.partitions.some((x) => x.partition_agent === v.name)) return v
  if (v.kind === 'shared') return v
  if (v.kind === 'map') {
    if (v.partition) return s.partitions.some((x) => x.partition_agent === v.partition) ? v : { kind: 'map' }
    return v
  }
  return allView
}

export function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'connection':
      return { ...s, connection: a.connection }
    case 'view': {
      const view = s.loaded ? resolve(a.view, s) : a.view
      const unread = { ...s.unread }
      delete unread[viewKey(view)]
      return { ...s, view, unread }
    }
    case 'event':
      return applyEvent(s, a.event)
  }
}

function applyEvent(s: State, e: ServerEvent): State {
  switch (e.type) {
    case 'snapshot': {
      const { me, agents } = e.snapshot
      const partitions = e.snapshot.partitions ?? []
      return {
        ...s,
        loaded: true,
        version: e.snapshot.version ?? '',
        project: e.snapshot.project ?? '',
        mode: e.snapshot.mode ?? 'running',
        file: e.snapshot.file ?? '',
        revision: e.snapshot.revision ?? '',
        startFindings: e.snapshot.start_findings ?? [],
        me,
        agents,
        participants: e.snapshot.participants ?? [],
        partitions,
        messages: e.snapshot.messages ?? [],
        vocabulary: e.snapshot.vocabulary ?? emptyVocabulary,
        repos: e.snapshot.repos ?? [],
        deliverables: e.snapshot.deliverables ?? [],
        wikis: e.snapshot.wikis ?? [],
        activity: e.snapshot.activity ?? {},
        modules: e.snapshot.modules ?? {},
        feed: e.snapshot.feed ?? [],
        cost: e.snapshot.cost ?? 0,
        view: resolve(s.view, { agents, partitions, mode: e.snapshot.mode ?? 'running', modules: e.snapshot.modules ?? {} }),
        unread: {},
      }
    }
    case 'message': {
      const unread = { ...s.unread }
      if (e.message.from !== s.me) {
        for (const k of conversationKeys(e.message, s.me)) {
          if (k !== viewKey(s.view)) unread[k] = (unread[k] ?? 0) + 1
        }
      }
      return { ...s, messages: [...s.messages, e.message], unread }
    }
    case 'status':
      return {
        ...s,
        agents: s.agents.map((ag) => (ag.name === e.agent ? { ...ag, status: e.status } : ag)),
        cost: s.cost + (e.status.cost ?? 0),
      }
    case 'feed':
      return { ...s, feed: [...s.feed.slice(-(maxFeed - 1)), e.entry] }
    case 'activity': {
      const list = s.activity[e.agent] ?? []
      return { ...s, activity: { ...s.activity, [e.agent]: replaceBy(list, e.activity, (x) => String(x.id)) } }
    }
    case 'partition':
      return { ...s, partitions: replaceBy(s.partitions, e.partition, (x) => x.partition_agent) }
    case 'vocabulary':
      return { ...s, vocabulary: e.vocabulary }
    case 'wiki':
      return { ...s, wikis: replaceBy(s.wikis, e.wiki, (x) => x.ref) }
    case 'topology':
      return { ...s, revision: e.revision }
    case 'module':
      return { ...s, modules: { ...s.modules, [e.module]: e.view } }
  }
}

function replaceBy<T>(list: T[], item: T, key: (x: T) => string): T[] {
  const k = key(item)
  return list.some((x) => key(x) === k) ? list.map((x) => (key(x) === k ? item : x)) : [...list, item]
}
