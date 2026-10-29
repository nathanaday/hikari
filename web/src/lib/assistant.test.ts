import { describe, expect, it } from 'vitest'
import { completions, moodOf, pendingPrompt } from './assistant'
import { agent } from './fixtures'
import { frame } from './sprite'
import { assistantOff, initialState, reducer, type State } from './store'
import type { AssistantStatus, AssistantView, TasksView } from './types'
import { allView } from './view'

const idle: AssistantView = { ...assistantOff, enabled: true }

function s(extra: Partial<State>): State {
  return { ...initialState(allView), ...extra }
}

describe('moodOf (HA12)', () => {
  it('sleeps when nothing happens', () => {
    expect(moodOf(s({ agents: [agent('orders')], assistant: idle }))).toBe('asleep')
  })
  it('is content while an agent works', () => {
    expect(moodOf(s({ agents: [agent('orders', 'working')], assistant: idle }))).toBe('content')
    expect(moodOf(s({ agents: [agent('orders', 'indexing')], assistant: idle }))).toBe('content')
  })
  it('is alarmed on an error, an exit with a reason, a blocked scope, or a failed turn', () => {
    expect(moodOf(s({ agents: [agent('orders', 'error'), agent('inventory', 'working')], assistant: idle }))).toBe('alarmed')
    expect(moodOf(s({ agents: [{ ...agent('orders', 'exited'), status: { state: 'exited', detail: 'killed' } }], assistant: idle }))).toBe('alarmed')
    expect(moodOf(s({ agents: [agent('orders', 'exited')], assistant: idle }))).toBe('asleep')
    const tasks: TasksView = { stubs: [], drafts: [], problems: [], orchestration: 'lead', partitions: [], queues: [{ id: 'q', name: 'Q', state: 'running', file: 'x', scopes: [{ id: 't', name: 't', stage: 'scope', status: 'blocked', partitions: [], features: [], needs: [], parts: [], file: 'x', hash: '1' }] }] }
    expect(moodOf(s({ agents: [], modules: { tasks }, assistant: idle }))).toBe('alarmed')
    expect(moodOf(s({ agents: [], modules: { tasks: { ...tasks, queues: [{ ...tasks.queues[0], state: 'paused' }] } }, assistant: idle }))).toBe('asleep')
    expect(moodOf(s({ agents: [], assistant: { ...idle, activity: [{ id: 1, turn: 1, time: 't', kind: 'done', done: true, error: true }] } }))).toBe('alarmed')
    expect(moodOf(s({ agents: [], assistant: { ...idle, activity: [{ id: 1, turn: 1, time: 't', kind: 'done', done: true, error: true, text: 'stopped' }] } }))).toBe('asleep')
  })
  it('thinks while the hikari agent works, before anything else', () => {
    expect(moodOf(s({ agents: [agent('orders', 'error')], assistant: { ...idle, state: 'working' } }))).toBe('thinking')
  })
})

describe('completions (HA17)', () => {
  const commands = ['hikari:hikari', 'hikari:room', 'threads:run']
  it('lists the commands that match a draft of a slash and a word', () => {
    expect(completions('/', commands)).toEqual(commands)
    expect(completions('/hik', commands)).toEqual(['hikari:hikari', 'hikari:room'])
    expect(completions('/room', commands)).toEqual(['hikari:room'])
    expect(completions('/hikari:room what', commands)).toEqual([])
    expect(completions('hello', commands)).toEqual([])
  })
})

describe('the sprite (HA13)', () => {
  it('draws each mood as sixteen rows of sixteen pixels, with its eyes', () => {
    for (const mood of ['asleep', 'content', 'alarmed', 'thinking'] as const) {
      const rows = frame(mood, 0)
      expect(rows).toHaveLength(16)
      for (const r of rows) expect(r).toHaveLength(16)
    }
    expect(frame('asleep', 0)[6]).toContain('-')
    expect(frame('content', 0)[6]).toContain('e')
    expect(frame('content', 5)[6]).toContain('-')
    expect(frame('alarmed', 0)[5]).toContain('ee')
    expect(frame('thinking', 0)[5]).toContain('e')
    expect(frame('content', 0)).not.toEqual(frame('content', 1))
  })
})

describe('the store and the hikari agent (HA18)', () => {
  const snapshot = { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] }, assistant: idle }
  const loaded = () => reducer(initialState(allView), { type: 'event', event: { type: 'snapshot', snapshot } })
  const status = (state: 'idle' | 'working', session = 's1'): AssistantStatus => ({ session, model: '', effort: '', permission_mode: 'default', state, cost: 0.1, commands: ['hikari:room'] })

  it('takes the view from the snapshot and applies each event', () => {
    let st = loaded()
    expect(st.assistant.enabled).toBe(true)
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'message', message: { id: 1, time: 't', text: 'hi' } } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'activity', activity: { id: 1, turn: 1, time: 't', kind: 'turn', done: true } } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'activity', activity: { id: 1, turn: 1, time: 't', kind: 'turn', text: 'your message', done: true } } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'prompt', prompt: { id: 'r1', turn: 1, time: 't', tool: 'Bash' } } } })
    expect(pendingPrompt(st)?.id).toBe('r1')
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'prompt', prompt: { id: 'r1', turn: 1, time: 't', tool: 'Bash', answer: 'allow' } } } })
    expect(pendingPrompt(st)).toBeUndefined()
    expect(st.assistant.messages).toHaveLength(1)
    expect(st.assistant.activity).toHaveLength(1)
    expect(st.assistant.activity[0].text).toBe('your message')
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('working') } } })
    expect(st.assistant.state).toBe('working')
    expect(st.assistant.commands).toEqual(['hikari:room'])
  })

  it('counts a turn that ends while the panel is closed, and clears it when it opens', () => {
    let st = reducer(loaded(), { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('working') } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('idle') } } })
    expect(st.assistantUnread).toBe(true)
    st = reducer(st, { type: 'panel', open: true })
    expect(st.assistantUnread).toBe(false)
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('working') } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('idle') } } })
    expect(st.assistantUnread).toBe(false)
  })

  it('empties the transcript on a new chat', () => {
    let st = reducer(loaded(), { type: 'event', event: { type: 'assistant', assistant: { type: 'message', message: { id: 1, time: 't', text: 'hi' } } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('idle') } } })
    st = reducer(st, { type: 'event', event: { type: 'assistant', assistant: { type: 'status', status: status('idle', '') } } })
    expect(st.assistant.messages).toEqual([])
  })
})
