import type { State } from './store'
import type { Message } from './types'
import type { View } from './view'
import { openPulls } from './repos'

// AttentionItem is one thing that is blocked on you (rules AT1 and AT2 of
// the attention spec): a decision or a direct message waiting for your
// answer, a blocked task, a permission the hikari agent asked for, a pull
// request that cannot merge, or a forge or tracker that could not be read.
export type AttentionKind = 'decision' | 'message' | 'blocked' | 'prompt' | 'pull' | 'repo' | 'tracker'

export type AttentionItem = {
  key: string
  kind: AttentionKind
  // who is the agent or the repository the item concerns, when one does.
  who?: string
  text: string
  view: View
}

// answered reports whether a decision message has an answer.
export function answered(m: Message, messages: Message[]): boolean {
  return messages.some((x) => x.answers === m.id)
}

// pendingDecisions lists the decisions that wait for an answer, oldest
// first; task narrows them to one task's.
export function pendingDecisions(messages: Message[], me: string, task?: string): Message[] {
  return messages.filter((m) => m.decision && m.to === me && !answered(m, messages) && (!task || m.decision.task === task))
}

// unansweredMessages lists, for each agent, its last direct message to you
// when nothing of yours came after it: a question you have not answered.
function unansweredMessages(state: State): Message[] {
  const last = new Map<string, Message>()
  for (const m of state.messages) {
    if (m.to === state.me && m.from !== state.me) last.set(m.from, m)
    else if (m.from === state.me && m.to) last.delete(m.to)
  }
  return [...last.values()].filter((m) => !m.decision && state.agents.some((a) => a.name === m.from))
}

export function attentionItems(state: State): AttentionItem[] {
  const items: AttentionItem[] = []
  for (const m of pendingDecisions(state.messages, state.me)) {
    const task = state.modules.tasks?.tasks.find((t) => t.id === m.decision!.task)
    items.push({
      key: `decision:${m.id}`,
      kind: 'decision',
      who: m.from === 'hikari' ? m.decision!.reply_to : m.from,
      text: m.decision!.question,
      view: task ? { kind: 'work', issue: task.issue ?? task.id } : { kind: 'dm', name: m.from === 'hikari' ? (m.decision!.reply_to ?? m.from) : m.from },
    })
  }
  for (const m of unansweredMessages(state)) {
    items.push({ key: `message:${m.id}`, kind: 'message', who: m.from, text: firstLine(m.body), view: { kind: 'dm', name: m.from } })
  }
  for (const p of state.assistant.prompts) {
    if (!p.answer) items.push({ key: `prompt:${p.id}`, kind: 'prompt', text: `The hikari agent asks to run ${p.tool}`, view: state.view })
  }
  const work = state.modules.tasks
  for (const t of work?.tasks ?? []) {
    if (t.status === 'blocked') {
      const why = t.block || t.parts.find((p) => p.status === 'blocked')?.partition
      items.push({ key: `blocked:${t.id}`, kind: 'blocked', text: `${t.name} is blocked${why ? `: ${why}` : ''}`, view: { kind: 'work', issue: t.issue ?? t.id } })
    }
  }
  for (const r of state.repos) {
    for (const p of openPulls(r)) {
      if (p.checks.state === 'failing') items.push({ key: `pull:${r.name}:${p.number}`, kind: 'pull', who: r.name, text: `#${p.number} ${p.title}: checks failing`, view: { kind: 'repo', name: r.name } })
      else if (p.mergeable === 'conflicting') items.push({ key: `pull:${r.name}:${p.number}`, kind: 'pull', who: r.name, text: `#${p.number} ${p.title}: conflicts with ${p.base}`, view: { kind: 'repo', name: r.name } })
    }
    if (r.error) items.push({ key: `repo:${r.name}`, kind: 'repo', who: r.name, text: `${r.name}: the forge could not be read`, view: { kind: 'repo', name: r.name } })
    else if (r.git_error) items.push({ key: `repo:${r.name}:git`, kind: 'repo', who: r.name, text: `${r.name}: ${r.git_error}`, view: { kind: 'repo', name: r.name } })
  }
  for (const tr of work?.trackers ?? []) {
    if (tr.error && !state.repos.some((r) => r.name === tr.repo && r.error)) items.push({ key: `tracker:${tr.repo}`, kind: 'tracker', who: tr.repo, text: `${tr.repo}: its issues could not be read`, view: { kind: 'work' } })
  }
  return items
}

// attentionFor counts the items of one agent (its decisions and messages)
// or one repository.
export function attentionFor(items: AttentionItem[], who: string): number {
  return items.filter((i) => i.who === who).length
}

function firstLine(s: string): string {
  const l = s.split('\n').find((x) => x.trim()) ?? ''
  return l.length > 80 ? `${l.slice(0, 77)}…` : l
}
