import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { AssistantPanel } from '@/components/assistant-panel'
import { Cat } from '@/components/cat'
import { assistantOff, initialState, type State } from '@/lib/store'
import type { AssistantView } from '@/lib/types'
import { allView } from '@/lib/view'

const view: AssistantView = {
  ...assistantOff,
  enabled: true,
  session: 's1',
  model: 'opus',
  effort: 'high',
  permission_mode: 'default',
  cost: 0.42,
  commands: ['hikari:room', 'threads:run'],
  messages: [{ id: 1, time: '2026-10-28T14:00:00Z', text: 'What is **running**?' }],
  activity: [
    { id: 1, turn: 1, time: '2026-10-28T14:00:01Z', kind: 'turn', text: 'your message', done: true },
    { id: 2, turn: 1, time: '2026-10-28T14:00:02Z', kind: 'tool', tool: 'Bash', input: { command: 'hikari status' }, done: false },
  ],
  prompts: [
    { id: 'r1', turn: 1, time: '2026-10-28T14:00:03Z', tool: 'Bash', input: { command: 'hikari status topology.yaml' }, description: 'Check the status' },
  ],
}

function stateWith(a: AssistantView, extra: Partial<State> = {}): State {
  return { ...initialState(allView), loaded: true, me: 'user', assistant: a, panel: true, ...extra }
}

const noop = () => {}

describe('AssistantPanel', () => {
  it('shows the header with the state, the cost, and the menus (HA15)', () => {
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith({ ...view, state: 'working' })} close={noop} />)
    for (const want of ['Hikari', '>working<', '$0.42', 'data-testid="assistant-stop"', 'data-testid="assistant-model"', 'data-testid="assistant-effort"', 'data-testid="assistant-mode"', 'Bypass (skips every permission)', 'data-testid="assistant-new"']) {
      expect(html).toContain(want)
    }
    expect(html).toMatch(/<option value="opus" selected="">Opus<\/option>/)
    expect(html).toMatch(/<option value="high" selected="">High<\/option>/)
    expect(html).toContain('disabled=""')
  })

  it('shows the timeline: your message, the turn, and the pending prompt with Allow and Deny (HA16)', () => {
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith({ ...view, state: 'working' })} close={noop} />)
    expect(html).toContain('<strong>running</strong>')
    expect(html).toContain('data-testid="turn"')
    expect(html).toContain('data-testid="assistant-prompt"')
    expect(html).toContain('hikari status topology.yaml')
    expect(html).toContain('Check the status')
    expect(html).toContain('data-testid="assistant-allow"')
    expect(html).toContain('data-testid="assistant-deny"')
    expect(html).toContain('Hikari is working…')
  })

  it('shows the reply under a folded turn (HA16)', () => {
    const ended = { ...view, activity: [...view.activity, { id: 3, turn: 1, time: '2026-10-28T14:00:04Z', kind: 'done' as const, text: 'The shop runs **three** agents.', done: true }] }
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith(ended)} close={noop} />)
    expect(html).toContain('data-testid="closing"')
    expect(html).toContain('>Reply<')
    expect(html).toContain('<strong>three</strong>')
  })

  it('indents the stopped line of a folded turn like its header', () => {
    const stopped = { ...view, activity: [...view.activity, { id: 3, turn: 1, time: '2026-10-28T14:00:04Z', kind: 'done' as const, text: 'stopped', done: true, error: true }] }
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith(stopped)} close={noop} />)
    expect(html).toMatch(/<div class="px-3 pb-2"><p class="[^"]*" data-testid="closing">/)
    expect(html).toContain('stopped')
  })

  it('shows an answered prompt as its answer', () => {
    const answered = { ...view, prompts: [{ ...view.prompts[0], answer: 'deny' as const, message: 'Not now' }] }
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith(answered)} close={noop} />)
    expect(html).toContain('Denied: Not now')
    expect(html).not.toContain('data-testid="assistant-allow"')
    expect(html).toContain('Ask Hikari, or type / for a skill')
  })

  it('says why the hikari agent is off (HA14)', () => {
    const html = renderToStaticMarkup(<AssistantPanel state={stateWith({ ...assistantOff, reason: 'claude is not on the PATH' })} close={noop} />)
    expect(html).toContain('data-testid="assistant-off"')
    expect(html).toContain('claude is not on the PATH')
  })
})

describe('Cat', () => {
  it('names her mood and shows what needs attention (HA12)', () => {
    const html = renderToStaticMarkup(<Cat mood="alarmed" attention="prompt" open={false} onClick={noop} />)
    expect(html).toContain('data-mood="alarmed"')
    expect(html).toContain('aria-label="Hikari, alarmed: open the panel"')
    expect(html).toContain('data-testid="cat-attention"')
    expect(html).toContain('<rect')
    const asleep = renderToStaticMarkup(<Cat mood="asleep" open={true} onClick={noop} />)
    expect(asleep).toContain('aria-pressed="true"')
    expect(asleep).toContain('data-testid="cat-mark"')
    expect(asleep).not.toContain('data-testid="cat-attention"')
  })
})
