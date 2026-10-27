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
  { severity: 'error', stage: 'topology', message: 'has no partition', path: 'services.lead', line: 9 },
  { severity: 'warning', stage: 'start', message: 'no credential' },
]
const props: InspectorProps = {
  model,
  findings,
  mounts: p.mounts,
  packs: p.packs,
  options: { deliverables: [], skills: ['python-style'], models: ['sonnet', 'opus', 'haiku'], templates: ['lead', 'dev'] },
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
    expect(roles(model.agents.find((a) => a.id === 'lead')!, model)).toBe('orchestration agent')
    expect(roles(model.agents.find((a) => a.id === 'dev')!, model)).toBe('owns project/app')
  })
})

describe('EntityList', () => {
  it('lists each group, names with ids, roles, and finding dots (SL1 to SL3)', () => {
    const html = renderToStaticMarkup(<EntityList model={model} findings={findings} selected="agent:lead" select={() => {}} />)
    for (const text of ['Project', 'Repositories', 'Agents', 'Sidecars', 'orchestration', 'owns project/app']) expect(html).toContain(text)
    expect(html).toMatch(/chief<span[^>]*>lead<\/span>/)
    expect(html).toContain('aria-label="error"')
    expect(html).toContain('aria-current="true"')
  })
})

describe('the inspectors', () => {
  it('picks the orchestration agent of the project, or none (H24)', () => {
    const html = renderToStaticMarkup(<ProjectInspector {...props} />)
    expect(html).toContain('Orchestration agent')
    expect(html).toContain('>None<')
    expect(html).toContain('>chief<')
    expect(html).not.toMatch(/<option[^>]*value="dev"/)
  })
  it('keeps a locked id, and shows what the orchestration agent is (SI4, SI6)', () => {
    const locked = { ...model, agents: model.agents.map((a) => (a.id === 'lead' ? { ...a, id_locked: true } : a)) }
    const html = renderToStaticMarkup(<AgentInspector {...props} model={locked} id="lead" />)
    expect(html).toContain('The vault is keyed by this id')
    expect(html).toContain('has no partition')
    expect(html).toContain('The orchestration agent owns no code')
    expect(html).toContain('without an orchestration agent')
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
  it('edits the folders of a partition, and marks the home of the private files (F14)', () => {
    const folders = [
      { repo: 'project', path: 'libs', read_only: true, at: '' },
      { repo: 'project', path: 'app', read_only: false, at: '' },
      { repo: 'project', path: 'proto', read_only: false, at: 'protos' },
    ]
    const two = { ...model, agents: model.agents.map((a) => (a.id === 'dev' ? { ...a, partition: { folders, test: '' } } : a)) }
    const html = renderToStaticMarkup(<AgentInspector {...props} model={two} id="dev" />)
    expect(html.match(/data-testid="folder-row"/g)).toHaveLength(3)
    for (const text of ['/workspace/project/libs', '/workspace/project/app', '/workspace/protos', 'Add a folder in…', 'aria-label="Move up"', 'aria-label="Remove the folder"', 'Runs in /workspace.']) expect(html).toContain(text)
    expect(html.match(/private files/g)).toHaveLength(1)
    expect(html.indexOf('private files')).toBeGreaterThan(html.indexOf('/workspace/project/app'))
    expect(html.indexOf('private files')).toBeLessThan(html.indexOf('/workspace/protos'))
    expect(html.match(/type="checkbox" checked=""/g)).toHaveLength(1)
    expect(roles(two.agents.find((a) => a.id === 'dev')!, two)).toBe('owns project/app, protos')
  })
  it('shows an agent’s mounts, or why it cannot (V14)', () => {
    const html = renderToStaticMarkup(<AgentInspector {...props} id="dev" />)
    expect(html).toContain('data-testid="mounts"')
    expect(html).toContain('/hikari/self')
    expect(html).toContain('vault/partitions/dev/read-only')
    expect(html).toContain('an empty directory, which hides it')
    expect(html).toContain('Skills it uses')
    const lead = renderToStaticMarkup(<AgentInspector {...props} id="lead" />)
    expect(lead).toContain('/hikari/partitions/dev')
    const broken = renderToStaticMarkup(<AgentInspector {...props} mounts={null} id="dev" />)
    expect(broken).toContain('The topology has errors, so hikari cannot say what this container mounts')
  })
  it('lists the skills of each agent: its pack from Hikari, and its granted skills (K14)', () => {
    const skills = (html: string) => [...html.matchAll(/(hikari-[a-z-]+)<span class="text-muted-foreground">· from Hikari/g)].map((m) => m[1])
    const dev = renderToStaticMarkup(<AgentInspector {...props} id="dev" />)
    expect(skills(dev)).toEqual(p.packs.partition)
    expect(dev).toContain('Skills it uses')
    const lead = renderToStaticMarkup(<AgentInspector {...props} id="lead" />)
    expect(skills(lead)).toEqual(p.packs.orchestration)
    expect(p.packs.orchestration).not.toEqual(p.packs.partition)
  })
})

describe('Occurrences', () => {
  it('puts each place under a checkbox, and an unkeyed one without (SR2)', () => {
    const occ = [...p.occurrences, { file: 'partitions/dev/read-only/features/map.yaml', line: 3, text: 'summary: for @lead', replace: 'summary: for @chief', from: '@lead', to: '@chief' }]
    const html = renderToStaticMarkup(<Occurrences occurrences={occ} chosen={new Set([p.occurrences[0].key!])} toggle={() => {}} />)
    expect(html.match(/type="checkbox"/g)).toHaveLength(1)
    expect(html).toContain('checked=""')
    expect(html).toContain('Ship it soon, with @chief.')
    expect(html).toContain('its partition agent updates it')
  })
})
