import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { AppSidebar } from '@/components/app-sidebar'
import { DecisionCard } from '@/components/decision-card'
import { GridView } from '@/components/grid-view'
import { currentStep, held } from '@/lib/grid'
import { MessageList } from '@/components/message-list'
import { SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { agent, task } from '@/lib/fixtures'
import { initialState, type State } from '@/lib/store'
import type { Activity, Message } from '@/lib/types'
import { parseHash, viewHash, viewKey } from '@/lib/view'

function msg(id: number, from: string, to: string | undefined, body: string, extra: Partial<Message> = {}): Message {
  return { id, from, to, body, time: `2026-11-04T10:00:${String(id).padStart(2, '0')}Z`, ...extra }
}

const decision = msg(1, 'lead', 'user', 'Wrap style?\n1. One style\n2. Let the customer pick', { decision: { question: 'Wrap style?', options: ['One style', 'Let the customer pick'], reply_to: 'lead' } })

function stateWith(extra: Partial<State>): State {
  return { ...initialState({ kind: 'all' }), loaded: true, agents: [agent('lead', 'working', 'orchestration'), agent('orders')], ...extra }
}

const noop = () => {}

describe('DecisionCard', () => {
  it('shows the numbered choices while it waits, and the pick once answered (DC5)', () => {
    const open = renderToStaticMarkup(<DecisionCard m={decision} hot />)
    expect(open).toContain('data-answered="false"')
    expect(open).toContain('Wrap style?')
    expect((open.match(/data-testid="decision-option"/g) ?? []).length).toBe(2)
    expect(open).toContain('Press a number')
    const answered = renderToStaticMarkup(<DecisionCard m={decision} answer={msg(2, 'user', 'lead', '2. Let the customer pick\nfor now', { answers: 1 })} />)
    expect(answered).toContain('data-answered="true"')
    expect(answered).toContain('your pick')
    expect(answered).not.toContain('data-testid="decision-option"')
    expect(answered).toContain('for now')
  })

  it('shows a proposed rule with its name, its sentence, its reason, and the accept choice', () => {
    const proposal = msg(5, 'orders', 'user', 'orders proposes a rule', { decision: { question: 'orders proposes a rule for every agent: A retry is logged. Accept it into the public wiki?', options: ['Accept: hikari writes it into the public wiki as the rule no-silent-retries', 'Decline'], reply_to: 'orders', proposal: { id: 'no-silent-retries', name: 'No silent retries', rule: 'A retry is logged.', reason: 'A block took an hour to find.' } } })
    const html = renderToStaticMarkup(<DecisionCard m={proposal} hot />)
    expect(html).toContain('data-proposal="true"')
    expect(html).toContain('Proposed rule · No silent retries')
    expect(html).toContain('A retry is logged.')
    expect(html).toContain('A block took an hour to find.')
    expect(html).toContain('no-silent-retries')
    expect((html.match(/data-testid="decision-option"/g) ?? []).length).toBe(2)
  })

  it('renders in the direct messages as a card, not as text', () => {
    const s = stateWith({ view: { kind: 'dm', name: 'lead' }, messages: [decision] })
    const html = renderToStaticMarkup(<MessageList state={s} />)
    expect(html).toContain('data-testid="decision"')
    expect(html).toContain('data-testid="decision-option"')
  })
})

describe('the sidebar inbox', () => {
  const render = (s: State) =>
    renderToStaticMarkup(
      <TooltipProvider>
        <SidebarProvider>
          <AppSidebar state={s} show={noop} />
        </SidebarProvider>
      </TooltipProvider>,
    )

  it('lists what needs you at the top, and marks the agent (AT1 to AT3)', () => {
    const s = stateWith({ messages: [decision, msg(3, 'orders', 'user', 'Which currency?')] })
    const html = render(s)
    expect(html).toContain('data-testid="needs-you"')
    expect(html).toContain('Needs you (2)')
    expect(html).toContain('Wrap style?')
    expect(html).toContain('Which currency?')
    expect((html.match(/data-testid="needs"/g) ?? []).length).toBe(2)
    expect(html).toContain('Grid')
  })

  it('shows no inbox when nothing waits', () => {
    expect(render(stateWith({}))).not.toContain('data-testid="needs-you"')
  })
})

describe('GridView', () => {
  const steps: Activity[] = [
    { id: 1, turn: 1, time: '2026-11-04T10:00:00Z', kind: 'turn', text: 'woken by user directly', done: true },
    { id: 2, turn: 1, time: '2026-11-04T10:00:01Z', kind: 'tool', tool: 'Read', input: { file_path: 'src/checkout/Checkout.tsx' }, done: false },
  ]

  it('shows a tile per agent with its step, its tasks, its last message, and whether it waits for you (GR1, GR2)', () => {
    const s = stateWith({
      activity: { lead: steps },
      messages: [decision],
      modules: {
        tasks: {
          issues: [],
          trackers: [],
          tasks: [task('cancel', { name: 'Cancel orders', status: 'started', partitions: ['orders'], parts: [{ partition: 'orders', status: 'testing' }] }), task('later', { status: 'queued', partitions: ['orders'] })],
          queues: [],
          problems: [],
          orchestration: 'lead',
          partitions: ['orders'],
          work_queue: 'work',
        },
      },
    })
    const html = renderToStaticMarkup(<GridView state={s} show={noop} />)
    expect((html.match(/data-testid="agent-tile"/g) ?? []).length).toBe(2)
    expect(html).toContain('Reading Checkout.tsx')
    expect(html).toContain('needs you')
    expect(html).toContain('Cancel orders')
    expect(html).toContain('>testing<')
    expect(html).toContain('lead → you: Wrap style?')
    expect(currentStep(s, 'lead')).toEqual({ text: 'Reading Checkout.tsx', live: true })
    expect(currentStep(s, 'orders')).toBeUndefined()
    expect(held(s, agent('orders')).map((t) => t.id)).toEqual(['cancel'])
    expect(held(s, agent('lead', 'idle', 'orchestration')).map((t) => t.id)).toEqual(['cancel'])
  })

  it('routes #/grid', () => {
    expect(parseHash('#/grid')).toEqual({ kind: 'grid' })
    expect(viewHash({ kind: 'grid' })).toBe('#/grid')
    expect(viewKey({ kind: 'grid' })).toBe('grid')
  })
})
