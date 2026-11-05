import type { State } from './store'
import { groupTurns, seconds, stepLabel } from './turns'
import type { Agent, TaskView } from './types'

// held lists the tasks an agent holds right now: for a partition agent, the
// started scopes with a part for it; for the orchestration agent, every
// started scope (rule GR2 of the attention spec).
export function held(state: State, agent: Agent): TaskView[] {
  const tasks = state.modules.tasks?.tasks ?? []
  const active = (t: TaskView) => !!t.status && !['queued', 'waiting', 'done'].includes(t.status)
  if (agent.role === 'orchestration') return tasks.filter(active)
  return tasks.filter((t) => active(t) && t.partitions.includes(agent.name))
}

// currentStep is what the agent does right now, or what it did last.
export function currentStep(state: State, agent: string): { text: string; live: boolean } | undefined {
  const turns = groupTurns(agent, state.activity[agent] ?? [])
  const t = turns.at(-1)
  if (!t) return undefined
  if (t.live) {
    const step = t.steps.at(-1)
    return { text: step ? stepLabel(step) : (t.trigger ?? 'starting a turn'), live: true }
  }
  const tools = t.steps.filter((s) => s.kind === 'tool').length
  return { text: `last turn: ${tools} tool ${tools === 1 ? 'call' : 'calls'}${t.end?.duration ? ` in ${seconds(t.end.duration)}` : ''}`, live: false }
}
