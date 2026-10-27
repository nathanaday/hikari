import { describe, expect, it } from 'vitest'
import { asksSomething, groupTurns, stepLabel, timeline, turnInView, turnKeys, turnSummary } from './turns'
import type { Activity, Message } from './types'

const t0 = '2026-01-01T00:00:00Z'
const step = (a: Partial<Activity>): Activity => ({ id: 1, turn: 1, time: t0, kind: 'tool', done: true, ...a })

describe('groupTurns', () => {
  it('splits entries into turns and marks the open one live', () => {
    const turns = groupTurns('dev', [
      step({ id: 1, kind: 'turn', text: 'woken by user in the room' }),
      step({ id: 2, tool: 'Read', input: { file_path: '/w/a.py' } }),
      step({ id: 3, kind: 'done', text: 'Done.', duration: 2500, cost: 0.03 }),
      step({ id: 4, turn: 2, kind: 'turn', text: 'woken by lead directly' }),
      step({ id: 5, turn: 2, kind: 'thinking', done: false }),
    ])
    expect(turns.map((t) => [t.id, t.live, t.steps.length])).toEqual([
      [1, false, 1],
      [2, true, 1],
    ])
    expect(turns[0].trigger).toBe('woken by user in the room')
    expect(turns[0].end?.text).toBe('Done.')
    expect(turnSummary(turns[0])).toBe('2.5s · 1 tool call · $0.03')
  })
})

describe('turnKeys', () => {
  const turn = (trigger?: string) => groupTurns('dev', [step({ kind: 'turn', text: trigger })])[0]
  it('shows a turn where the messages that woke it are', () => {
    expect(turnKeys(turn('woken by user in the room, lead directly'), 'user')).toEqual(['room'])
    expect(turnKeys(turn('woken by user directly'), 'user')).toEqual(['dm:dev'])
    expect(turnKeys(turn('indexing the partition'), 'user')).toEqual(['room'])
  })
  it('keeps a direct message between agents out of the user\'s threads', () => {
    expect(turnInView(turn('woken by lead directly'), { kind: 'dm', name: 'dev' }, 'user')).toBe(false)
    expect(turnInView(turn('woken by lead directly'), { kind: 'all' }, 'user')).toBe(true)
    expect(turnInView(turn('woken by user in the room'), { kind: 'room' }, 'user')).toBe(true)
    expect(turnInView(turn('woken by user in the room'), { kind: 'dm', name: 'dev' }, 'user')).toBe(false)
  })
})

describe('timeline', () => {
  it('puts a turn after the message of the same time', () => {
    const m: Message = { id: 1, from: 'user', body: 'go', time: t0 }
    const later: Message = { id: 2, from: 'dev', body: 'done', time: '2026-01-01T00:00:05Z' }
    const turn = groupTurns('dev', [step({ kind: 'turn', time: t0 })])[0]
    expect(timeline([later, m], [turn]).map((i) => i.kind)).toEqual(['message', 'turn', 'message'])
  })
})

describe('stepLabel', () => {
  it('names tools by what they do', () => {
    expect(stepLabel(step({ tool: 'Read', input: { file_path: '/w/src/a.py' } }))).toBe('Read a.py')
    expect(stepLabel(step({ tool: 'Edit', input: { file_path: '/w/a.py' }, done: false }))).toBe('Editing a.py')
    expect(stepLabel(step({ tool: 'Bash', input: { command: 'npm test', description: 'Run the tests' } }))).toBe('Run the tests')
    expect(stepLabel(step({ tool: 'Bash', input: { command: 'npm test' } }))).toBe('npm test')
    expect(stepLabel(step({ tool: 'Skill', input: { skill: 'hikari-partition' } }))).toBe('Used the hikari-partition skill')
    expect(stepLabel(step({ tool: 'mcp__hikari__post', input: { body: 'hi' } }))).toBe('Posted to the room')
    expect(stepLabel(step({ tool: 'mcp__hikari__send_direct_message', input: { to: 'lead', body: 'hi' } }))).toBe('Sent a message to lead')
    expect(stepLabel(step({ tool: 'mcp__hikari__upsert_features' }))).toBe('upsert features')
    expect(stepLabel(step({ kind: 'thinking', duration: 3200 }))).toBe('Thought for 3.2s')
    expect(stepLabel(step({ kind: 'thinking', done: false }))).toBe('Thinking')
  })
})

describe('asksSomething', () => {
  it('finds a question in a closing note', () => {
    expect(asksSomething('Should I add it?')).toBe(true)
    expect(asksSomething('Two options: A or B? I lean to A.')).toBe(true)
    expect(asksSomething('Done. Waiting on inventory.')).toBe(false)
    expect(asksSomething('See https://x.y/?q=1 for details.')).toBe(false)
  })
})
