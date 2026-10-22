import type { Activity, Message } from './types'
import { conversationKeys, viewKey, type View } from './view'

// Turn is one turn of an agent: its steps, from the entry that opened it to
// the one that closed it.
export type Turn = {
  agent: string
  id: number
  time: string
  trigger?: string
  steps: Activity[]
  end?: Activity
  live: boolean
}

export function groupTurns(agent: string, entries: Activity[]): Turn[] {
  const turns: Turn[] = []
  for (const a of entries) {
    let t = turns.at(-1)
    if (!t || t.id !== a.turn) {
      t = { agent, id: a.turn, time: a.time, steps: [], live: true }
      turns.push(t)
    }
    if (a.kind === 'turn') t.trigger = a.text
    else if (a.kind === 'done') {
      t.end = a
      t.live = false
    } else t.steps.push(a)
  }
  return turns
}

// turnKeys returns the conversations that show a turn: the ones of the
// messages that woke the agent, read from its trigger, or the agent's hubs
// when nothing woke it, such as indexing.
export function turnKeys(t: Turn, hubsOf: string[], me: string): string[] {
  const keys = new Set<string>()
  const trigger = t.trigger ?? ''
  for (const m of trigger.matchAll(/(\S+) in #([\w-]+)/g)) keys.add(`hub:${m[2]}`)
  for (const m of trigger.matchAll(/(\S+) by direct message/g)) {
    const fake: Message = { id: 0, from: m[1], to: t.agent, body: '', time: t.time }
    for (const k of conversationKeys(fake, me)) keys.add(k)
  }
  if (keys.size === 0) for (const h of hubsOf) keys.add(`hub:${h}`)
  return [...keys]
}

export function turnInView(t: Turn, v: View, hubsOf: string[], me: string): boolean {
  if (v.kind === 'all') return true
  if (v.kind === 'dm') return v.name === t.agent && turnKeys(t, hubsOf, me).includes(viewKey(v))
  return turnKeys(t, hubsOf, me).includes(viewKey(v))
}

export type Item = { kind: 'message'; message: Message } | { kind: 'turn'; turn: Turn }

// timeline merges messages and turns by time. A turn sorts after a message
// of the same time, because the message woke it.
export function timeline(messages: Message[], turns: Turn[]): Item[] {
  const items: Item[] = [
    ...messages.map((message) => ({ kind: 'message', message }) as Item),
    ...turns.map((turn) => ({ kind: 'turn', turn }) as Item),
  ]
  const at = (i: Item) => Date.parse(i.kind === 'message' ? i.message.time : i.turn.time)
  return items.sort((a, b) => at(a) - at(b) || (a.kind === 'message' ? -1 : 1) - (b.kind === 'message' ? -1 : 1))
}

const hikariTool = /^mcp__hikari__/

type Input = Record<string, unknown>

function str(input: unknown, key: string): string | undefined {
  const v = (input as Input | undefined)?.[key]
  return typeof v === 'string' ? v : undefined
}

function base(path: string): string {
  return path.split('/').filter(Boolean).at(-1) ?? path
}

// stepLabel names a step in a few words, in the past tense once it is done.
export function stepLabel(a: Activity): string {
  if (a.kind === 'thinking') return a.done ? `Thought for ${seconds(a.duration)}` : 'Thinking'
  if (a.kind === 'text') return a.done ? 'Wrote' : 'Writing'
  if (a.kind !== 'tool') return a.kind
  const tool = a.tool ?? 'tool'
  const path = str(a.input, 'file_path')
  const done = a.done
  switch (tool) {
    case 'Read':
      return path ? `${done ? 'Read' : 'Reading'} ${base(path)}` : 'Reading'
    case 'Edit':
    case 'MultiEdit':
    case 'NotebookEdit':
      return path ? `${done ? 'Edited' : 'Editing'} ${base(path)}` : 'Editing'
    case 'Write':
      return path ? `${done ? 'Wrote' : 'Writing'} ${base(path)}` : 'Writing a file'
    case 'Bash':
      return str(a.input, 'description') ?? str(a.input, 'command') ?? 'Running a command'
    case 'Grep':
    case 'Glob':
      return `${done ? 'Searched' : 'Searching'} ${str(a.input, 'pattern') ?? ''}`.trim()
    case 'Skill':
      return `${done ? 'Used' : 'Using'} the ${str(a.input, 'skill') ?? ''} skill`.replace('the  skill', 'a skill')
    case 'Agent':
      return str(a.input, 'description') ?? 'Ran a subagent'
    case 'ToolSearch':
      return done ? 'Loaded tools' : 'Loading tools'
    case 'WebFetch':
    case 'WebSearch':
      return `${done ? 'Fetched' : 'Fetching'} ${str(a.input, 'url') ?? str(a.input, 'query') ?? ''}`.trim()
  }
  if (hikariTool.test(tool)) {
    const name = tool.replace(hikariTool, '')
    const to = str(a.input, 'hub') ? `#${str(a.input, 'hub')}` : str(a.input, 'to')
    if (name === 'post_to_hub' || name === 'send_direct_message') return `${done ? 'Sent' : 'Sending'} a message to ${to}`
    return name.replaceAll('_', ' ')
  }
  return tool
}

// stepSummary describes a turn in one line once it has ended.
export function turnSummary(t: Turn): string {
  const tools = t.steps.filter((s) => s.kind === 'tool').length
  const files = new Set(
    t.steps.filter((s) => s.kind === 'tool' && ['Edit', 'MultiEdit', 'Write', 'NotebookEdit'].includes(s.tool ?? '')).map((s) => str(s.input, 'file_path')),
  )
  const parts: string[] = []
  if (t.end?.duration) parts.push(seconds(t.end.duration))
  if (tools) parts.push(`${tools} tool ${tools === 1 ? 'call' : 'calls'}`)
  if (files.size) parts.push(`${files.size} ${files.size === 1 ? 'file' : 'files'} changed`)
  if (t.end?.cost) parts.push(`$${t.end.cost.toFixed(2)}`)
  return parts.join(' · ')
}

// asksSomething reports whether a closing note ends with, or contains, a
// question, so the view can call it out as one that may need your answer.
export function asksSomething(text: string): boolean {
  return /\?(\s|$)/.test(text)
}

export function seconds(ms: number | undefined): string {
  const s = (ms ?? 0) / 1000
  if (s < 10) return `${s.toFixed(1)}s`
  if (s < 90) return `${Math.round(s)}s`
  return `${Math.round(s / 60)}m`
}
