import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Occurrences } from '@/components/plan-review'
import { AgentInspector, HubInspector, type InspectorProps } from '@/components/structure-inspectors'
import { EntityList } from '@/components/structure-tab'
import plan from '@/lib/fixtures/topology/plan.json'
import { findingsOf, parseSelection, roles, selectionKey, worst } from '@/lib/structure'
import type { Finding, Plan, TopologyModel } from '@/lib/types'

const p = plan as Plan
const model = p.model as TopologyModel
const findings: Finding[] = [
  { severity: 'error', stage: 'topology', message: 'unknown agent', path: 'x-hikari.hubs.main.members[0]', line: 7 },
  { severity: 'warning', stage: 'start', message: 'no credential' },
]
const props: InspectorProps = {
  model,
  findings,
  options: { deliverables: [], models: ['sonnet', 'opus', 'haiku'], templates: ['lead', 'dev'] },
  add: async () => null,
  select: () => {},
  editYAML: () => {},
}

describe('the structure helpers', () => {
  it('round-trips a selection (SL4)', () => {
    for (const k of ['project', 'hub:main', 'agent:dev', 'add:repo']) expect(selectionKey(parseSelection(k)!)).toBe(k)
    expect(parseSelection('nonsense')).toBeUndefined()
  })
  it('gives each finding to the entity whose path holds it (SL3)', () => {
    expect(findingsOf(findings, { kind: 'hub', id: 'main' })).toHaveLength(1)
    expect(findingsOf(findings, { kind: 'hub', id: 'mai' })).toHaveLength(0)
    expect(worst(findingsOf(findings, { kind: 'agent', id: 'lead' }))).toBeNull()
  })
  it('says what each agent does (decision 2)', () => {
    expect(roles(model.agents.find((a) => a.id === 'lead')!, model)).toBe('orchestrates #core')
    expect(roles(model.agents.find((a) => a.id === 'dev')!, model)).toBe('member of #core')
  })
})

describe('EntityList', () => {
  it('lists each group, names with ids, roles, and finding dots (SL1 to SL3)', () => {
    const html = renderToStaticMarkup(<EntityList model={model} findings={findings} selected="hub:main" select={() => {}} />)
    for (const text of ['Project', 'Repos', 'Hubs', 'Agents', 'Sidecars', '#core', '1 member', 'orchestrates #core']) expect(html).toContain(text)
    expect(html).toMatch(/#core<span[^>]*>main<\/span>/)
    expect(html).toContain('aria-label="error"')
    expect(html).toContain('aria-current="true"')
  })
})

describe('the inspectors', () => {
  it('draws a hub with its orchestrator and members, and keeps a locked id (SI3, SI6)', () => {
    const html = renderToStaticMarkup(<HubInspector {...props} id="main" />)
    expect(html).toContain('data-testid="hub-diagram"')
    expect(html).toContain('orchestrator')
    expect(html).toContain('The store is keyed by this id')
    expect(html).toContain('unknown agent')
  })
  it('shows an agent’s container and lets a new agent change its id (SI4, SI6)', () => {
    const html = renderToStaticMarkup(<AgentInspector {...props} id="dev" />)
    expect(html).toContain('hikari-agent')
    expect(html).toContain('Edit in YAML')
    expect(html).toContain('It can change until hikari keeps something under it')
    expect(html).toContain('Builds it.')
  })
})

describe('Occurrences', () => {
  it('puts each place under a checkbox, and an unkeyed one without (SR2)', () => {
    const occ = [...p.occurrences, { file: 'systems/dev/map.yaml', line: 3, text: 'summary: for #main', replace: 'summary: for #core', from: '#main', to: '#core' }]
    const html = renderToStaticMarkup(<Occurrences occurrences={occ} chosen={new Set([p.occurrences[0].key!])} toggle={() => {}} />)
    expect(html.match(/type="checkbox"/g)).toHaveLength(1)
    expect(html).toContain('checked=""')
    expect(html).toContain('Ship it in #core')
    expect(html).toContain('its owner updates it')
  })
})
