import type { Agent, Hub, Message, ServerEvent } from './types'
import { allView, conversationKeys, viewKey, type View } from './view'

export type Connection = 'connecting' | 'live' | 'reconnecting' | 'signed-out'

export type State = {
  connection: Connection
  loaded: boolean
  me: string
  agents: Agent[]
  hubs: Hub[]
  messages: Message[]
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
    me: 'user',
    agents: [],
    hubs: [],
    messages: [],
    view,
    unread: {},
  }
}

// resolve returns v if it names a hub or agent of s, and All activity if not.
function resolve(v: View, s: Pick<State, 'hubs' | 'agents'>): View {
  if (v.kind === 'hub' && s.hubs.some((h) => h.name === v.name)) return v
  if (v.kind === 'dm' && s.agents.some((a) => a.name === v.name)) return v
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
      const { me, agents, hubs, messages } = e.snapshot
      return { ...s, loaded: true, me, agents, hubs, messages: messages ?? [], view: resolve(s.view, { agents, hubs }), unread: {} }
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
        agents: s.agents.map((ag) => (ag.name === e.node ? { ...ag, status: e.status } : ag)),
      }
  }
}
