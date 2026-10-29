import type { State } from './store'
import { groupTurns, turnKeys } from './turns'
import type { Entry } from './types'
import type { View } from './view'

// entryView is what an entry opens (rule FD10): the conversation of a
// message or a turn, an agent's direct messages, or a task in Work, or
// nothing for a step and for a private message between agents.
export function entryView(e: Entry, state: Pick<State, 'me' | 'activity'>): View | undefined {
  switch (e.kind) {
    case 'message':
      if (!e.to) return { kind: 'room' }
      if (e.who === state.me) return { kind: 'dm', name: e.to }
      if (e.to === state.me) return { kind: 'dm', name: e.who! }
      return undefined
    case 'turn': {
      const turn = groupTurns(e.who!, state.activity[e.who!] ?? []).find((t) => t.id === e.turn)
      const key = turn ? turnKeys(turn, state.me)[0] : undefined
      return key === 'room' ? { kind: 'room' } : { kind: 'dm', name: e.who! }
    }
    case 'status':
      return { kind: 'dm', name: e.who! }
    case 'module':
      return e.task ? { kind: 'work', task: e.task } : undefined
  }
  return undefined
}

// entryAnchor is the id of the element that shows the entry whole in its
// view, when the view has one.
export function entryAnchor(e: Entry): string | undefined {
  if (e.kind === 'message') return `message-${e.message}`
  if (e.kind === 'turn') return `turn-${e.who}-${e.turn}`
  return undefined
}
