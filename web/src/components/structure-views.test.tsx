import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Occurrences } from '@/components/plan-review'
import { AgentInspector, AddForm, ProjectInspector, type InspectorProps } from '@/components/structure-inspectors'
import { EntityList } from '@/components/structure-tab'
import plan from '@/lib/fixtures/topology/plan.json'
import { findingsOf, parseSelection, roles, selectionKey, worst } from '@/lib/structure'
import type { Finding, Plan, TopologyModel } from '@/lib/types'

const p = plan as Plan
const model = p.model as TopologyModel
const findings: Finding[] = [
  { severity: 'error', stage: 'topology', message: 'owns no system', path: 'services.lead', line: 9 },
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
    for (const k of ['project', 'repo:app', 'agent:dev', 'add:repo']) expect(selectionKey(parseSelection(k)!)).toBe(k)
    expect(parseSelection('nonsense')).toBeUndefined()
    expect(parseSelection('team:main')).toBeUndefined()
  })
  it('gives each finding to the entity whose path holds it (SL3)', () => {
    expect(findingsOf(findings, { kind: 'agent', id: 'lead' })).toHaveLength(1)
    expect(findingsOf(findings, { kind: 'agent', id: 'lea' })).toHaveLength(0)
    expect(worst(findingsOf(findings, { kind: 'agent', id: 'dev' }))).toBeNull()
  })
  it('says what each agent does (decision 2)', () => {
    expect(roles(model.agents.find((a) => a.id === 'lead')!, model)).toBe('orchestrator')
    expect(roles(model.agents.find((a) => a.id === 'dev')!, model)).toBe('owns project/app')
  })
})

describe('EntityList', () => {
  it('lists each group, names with ids, roles, and finding dots (SL1 to SL3)', () => {
    const html = renderToStaticMarkup(<EntityList model={model} findings={findings} selected="agent:lead" select={() => {}} />)
    for (const text of ['Project', 'Repos', 'Agents', 'Sidecars', 'orchestrator', 'owns project/app']) expect(html).toContain(text)
    expect(html).toMatch(/chief<span[^>]*>lead<\/span>/)
    expect(html).toContain('aria-label="error"')
    expect(html).toContain('aria-current="true"')
  })
})

describe('the inspectors', () => {
  it('picks the orchestrator of the project, or none (H24)', () => {
    const html = renderToStaticMarkup(<ProjectInspector {...props} />)
    expect(html).toContain('Orchestrator')
    expect(html).toContain('>None<')
    expect(html).toContain('>chief<')
    expect(html).not.toMatch(/<option[^>]*value="dev"/)
  })
  it('keeps a locked id, and shows what the orchestrator is (SI4, SI6)', () => {
    const locked = { ...model, agents: model.agents.map((a) => (a.id === 'lead' ? { ...a, id_locked: true } : a)) }
    const html = renderToStaticMarkup(<AgentInspector {...props} model={locked} id="lead" />)
    expect(html).toContain('The store is keyed by this id')
    expect(html).toContain('owns no system')
    expect(html).toContain('The orchestrator owns no code')
    expect(html).toContain('without an orchestrator')
  })
  it('adds an agent that joins the room (SI7)', () => {
    const html = renderToStaticMarkup(<AddForm {...props} what="agent" />)
    expect(html).toContain('Add an agent')
    expect(html).toContain('It joins the room')
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
    const occ = [...p.occurrences, { file: 'systems/dev/map.yaml', line: 3, text: 'summary: for @lead', replace: 'summary: for @chief', from: '@lead', to: '@chief' }]
    const html = renderToStaticMarkup(<Occurrences occurrences={occ} chosen={new Set([p.occurrences[0].key!])} toggle={() => {}} />)
    expect(html.match(/type="checkbox"/g)).toHaveLength(1)
    expect(html).toContain('checked=""')
    expect(html).toContain('Ship it soon, with @chief.')
    expect(html).toContain('its owner updates it')
  })
})
