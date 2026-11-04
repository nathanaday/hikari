import { describe, expect, it } from 'vitest'
import { attentionFor, attentionItems, pendingDecisions } from '@/lib/attention'
import { agent, pullRequest, repoView, task } from '@/lib/fixtures'
import { initialState, type State } from '@/lib/store'
import type { Message } from '@/lib/types'

function msg(id: number, from: string, to: string | undefined, body: string, extra: Partial<Message> = {}): Message {
  return { id, from, to, body, time: `2026-11-04T10:00:${String(id).padStart(2, '0')}Z`, ...extra }
}

function stateWith(extra: Partial<State>): State {
  return { ...initialState({ kind: 'all' }), loaded: true, agents: [agent('lead', 'idle', 'orchestration'), agent('orders')], ...extra }
}

describe('attentionItems', () => {
  it('lists a pending decision, and drops it once answered (AT1, DC3)', () => {
    const d = msg(1, 'lead', 'user', 'Wrap style?', { decision: { question: 'Wrap style?', options: ['One', 'Many'], reply_to: 'lead' } })
    let s = stateWith({ messages: [d] })
    const items = attentionItems(s)
    expect(items.map((i) => i.kind)).toEqual(['decision'])
    expect(items[0].who).toBe('lead')
    expect(items[0].view).toEqual({ kind: 'dm', name: 'lead' })
    expect(pendingDecisions(s.messages, 'user')).toHaveLength(1)
    s = stateWith({ messages: [d, msg(2, 'user', 'lead', '1. One', { answers: 1 })] })
    expect(attentionItems(s)).toEqual([])
  })

  it('opens a decision of a task on its issue, also when hikari asks (DC2)', () => {
    const d = msg(1, 'hikari', 'user', 'Proceed?', { decision: { question: 'Proceed?', options: ['Yes', 'No'], reply_to: 'lead', task: 'shop-12' } })
    const s = stateWith({
      messages: [d],
      modules: { tasks: { issues: [], trackers: [], tasks: [task('shop-12', { issue: 'shop#12' })], queues: [], problems: [], orchestration: 'lead', partitions: [], work_queue: 'work' } },
    })
    const [item] = attentionItems(s)
    expect(item.view).toEqual({ kind: 'work', issue: 'shop#12' })
    expect(item.who).toBe('lead')
  })

  it("lists an agent's last direct message to you until you answer (AT1)", () => {
    let s = stateWith({ messages: [msg(1, 'orders', 'user', 'Which currency?\nMore words.')] })
    let items = attentionItems(s)
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ kind: 'message', who: 'orders', text: 'Which currency?', view: { kind: 'dm', name: 'orders' } })
    expect(attentionFor(items, 'orders')).toBe(1)
    s = stateWith({ messages: [msg(1, 'orders', 'user', 'Which currency?'), msg(2, 'user', 'orders', 'USD')] })
    expect(attentionItems(s)).toEqual([])
    // A message between agents, or from someone who is not an agent, is not yours to answer.
    s = stateWith({ messages: [msg(1, 'orders', 'lead', 'x'), msg(2, 'hikari', 'user', 'The queue started')] })
    expect(attentionItems(s)).toEqual([])
  })

  it('lists blocked tasks, pending permissions, failing pull requests, and unreadable forges and trackers (AT2)', () => {
    const r = repoView('shop', {
      pull_requests: [pullRequest(5, { title: 'Cancel', checks: { state: 'failing', passed: 0, failed: 1, pending: 0, runs: [] } }), pullRequest(6, { title: 'Ok', mergeable: 'conflicting' })],
      git_error: 'git fetch origin: gave up',
    })
    const s = stateWith({
      repos: [r],
      assistant: { ...initialState({ kind: 'all' }).assistant, prompts: [{ id: 'p1', turn: 1, time: '', tool: 'Bash' }, { id: 'p2', turn: 1, time: '', tool: 'Bash', answer: 'allow' }] },
      modules: {
        tasks: {
          issues: [],
          trackers: [{ repo: 'shop', kind: 'github', error: 'Bad credentials' }],
          tasks: [task('cancel', { name: 'Cancel orders', status: 'blocked', block: 'needs a decision' }), task('fine', { status: 'started' })],
          queues: [],
          problems: [],
          orchestration: 'lead',
          partitions: [],
          work_queue: 'work',
        },
      },
    })
    const items = attentionItems(s)
    expect(items.map((i) => i.kind)).toEqual(['prompt', 'blocked', 'pull', 'pull', 'repo', 'tracker'])
    expect(items.find((i) => i.kind === 'blocked')).toMatchObject({ text: 'Cancel orders is blocked: needs a decision', view: { kind: 'work', issue: 'cancel' } })
    expect(items.filter((i) => i.kind === 'pull').map((i) => i.text)).toEqual(['#5 Cancel: checks failing', '#6 Ok: conflicts with main'])
    expect(attentionFor(items, 'shop')).toBe(4)
  })
})
