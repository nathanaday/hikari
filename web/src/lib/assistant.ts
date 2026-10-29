import type { State } from './store'

// Mood is what Hikari, the cat, shows (rule HA12 of the hikari agent spec).
export type Mood = 'asleep' | 'content' | 'alarmed' | 'thinking'

// moodOf follows the run: thinking while the hikari agent works; alarmed
// when an agent is in error or exited with a reason, a scope of a running
// queue is blocked, or the hikari agent's last turn failed; content while
// an agent works; and asleep otherwise.
export function moodOf(s: Pick<State, 'agents' | 'modules' | 'assistant'>): Mood {
  if (s.assistant.state === 'working') return 'thinking'
  const failed = s.agents.some((a) => a.status.state === 'error' || (a.status.state === 'exited' && !!a.status.detail))
  const blocked = (s.modules.tasks?.queues ?? []).some((q) => q.state === 'running' && q.scopes.some((t) => t.status === 'blocked'))
  // A turn that you stopped is not a failure.
  const last = s.assistant.activity.findLast((a) => a.kind === 'done')
  if (failed || blocked || (!!last?.error && last.text !== 'stopped')) return 'alarmed'
  if (s.agents.some((a) => a.status.state === 'working' || a.status.state === 'indexing')) return 'content'
  return 'asleep'
}

// pendingPrompt is the prompt that waits for an answer, if any.
export function pendingPrompt(s: Pick<State, 'assistant'>) {
  return s.assistant.prompts.find((p) => !p.answer)
}

// The menus of the panel (decisions 6 and 7).
export const models = [
  { value: '', label: 'Default model' },
  { value: 'fable', label: 'Fable' },
  { value: 'opus', label: 'Opus' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'haiku', label: 'Haiku' },
]
export const efforts = [
  { value: '', label: 'Default effort' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra high' },
  { value: 'max', label: 'Max' },
]
export const permissionModes = [
  { value: 'default', label: 'Ask' },
  { value: 'acceptEdits', label: 'Accept edits' },
  { value: 'plan', label: 'Plan' },
  { value: 'bypassPermissions', label: 'Bypass (skips every permission)' },
]

// completions are the slash commands that match a draft that starts with
// "/" and holds no space yet.
export function completions(draft: string, commands: string[]): string[] {
  if (!draft.startsWith('/') || /\s/.test(draft)) return []
  const prefix = draft.slice(1).toLowerCase()
  return commands.filter((c) => c.toLowerCase().includes(prefix)).slice(0, 12)
}

async function call(path: string, method: string, body?: unknown): Promise<string | null> {
  const res = await fetch(`/api/ui/assistant/${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const data = (await res.json().catch(() => ({}))) as { error?: string }
  return data.error ?? res.statusText
}

// api talks to the panel's routes (rule HA9). Each call returns an error
// message, or null.
export const api = {
  send: (text: string) => call('send', 'POST', { text }),
  answer: (id: string, allow: boolean, message = '') => call('answer', 'POST', { id, allow, message }),
  stop: () => call('stop', 'POST'),
  newChat: () => call('new', 'POST'),
  options: (model: string, effort: string, permission_mode: string) => call('options', 'PUT', { model, effort, permission_mode }),
}
