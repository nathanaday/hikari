import { describe, expect, it } from 'vitest'
import plan from './fixtures/topology/plan.json'
import stateFixture from './fixtures/topology/state.json'
import { initialState, reducer } from './store'
import { diffText, hunks, markers, sortFindings } from './topology'
import type { Finding, Plan, Snapshot, TopologyState } from './types'
import { parseHash, viewHash } from './view'

const snapshot = (mode: Snapshot['mode']): Snapshot => ({
  version: '0.0.1',
  mode,
  file: 'topology.yaml',
  revision: 'r1',
  start_findings: mode === 'editing' ? [{ severity: 'error', stage: 'start', message: 'a repo has uncommitted changes' }] : undefined,
  me: 'user',
  agents: [],
  participants: [],
  messages: [],
  partitions: [],
  vocabulary: { version: 1, terms: [], interfaces: [] },
})

describe('the topology fixtures', () => {
  it('read as the wire types (A5)', () => {
    const s = stateFixture as TopologyState
    expect(s.exists).toBe(true)
    expect(s.model?.project.orchestration).toBe('lead')
    const p = plan as Plan
    expect(p.diff.some((l) => l.op === '+')).toBe(true)
    expect(p.summary).toContain('rename agent lead to chief')
  })
})

describe('routing (YV1)', () => {
  it('maps the tabs to hashes and back', () => {
    expect(viewHash({ kind: 'topology' })).toBe('#/topology')
    expect(viewHash({ kind: 'topology', tab: 'yaml' })).toBe('#/topology/yaml')
    expect(parseHash('#/topology/yaml')).toEqual({ kind: 'topology', tab: 'yaml' })
    expect(parseHash('#/topology')).toEqual({ kind: 'topology' })
  })
})

describe('state (YS1, YV5)', () => {
  it('keeps the mode, the file, the revision, and the start findings', () => {
    let s = reducer(initialState({ kind: 'all' }), { type: 'event', event: { type: 'snapshot', snapshot: snapshot('editing') } })
    expect(s.mode).toBe('editing')
    expect(s.revision).toBe('r1')
    expect(s.startFindings).toHaveLength(1)
    expect(s.view).toEqual({ kind: 'topology' })
    s = reducer(s, { type: 'view', view: { kind: 'shared' } })
    expect(s.view).toEqual({ kind: 'topology' })
    s = reducer(s, { type: 'event', event: { type: 'topology', revision: 'r2' } })
    expect(s.revision).toBe('r2')
  })

  it('opens every view while agents run', () => {
    let s = reducer(initialState({ kind: 'shared' }), { type: 'event', event: { type: 'snapshot', snapshot: snapshot('running') } })
    expect(s.view).toEqual({ kind: 'shared' })
    expect(s.startFindings).toEqual([])
    s = reducer(s, { type: 'view', view: { kind: 'topology', tab: 'yaml' } })
    expect(s.view).toEqual({ kind: 'topology', tab: 'yaml' })
  })
})

describe('findings (YY2, YY3)', () => {
  const fs: Finding[] = [
    { severity: 'warning', stage: 'start', message: 'no credential' },
    { severity: 'error', stage: 'topology', message: 'unknown agent', line: 7, path: 'x-hikari.orchestration' },
    { severity: 'warning', stage: 'topology', message: 'unused repo', line: 3 },
    { severity: 'error', stage: 'yaml', message: 'did not find expected node', line: 40 },
  ]
  it('sorts errors first, then by line', () => {
    expect(sortFindings(fs).map((f) => f.message)).toEqual(['unknown agent', 'did not find expected node', 'unused repo', 'no credential'])
  })
  it('marks the lines that exist, and nothing for a finding without a line', () => {
    expect(markers(fs, 20)).toEqual([
      { line: 7, severity: 'error', message: 'unknown agent' },
      { line: 3, severity: 'warning', message: 'unused repo' },
    ])
  })
})

describe('the review diff (YY5)', () => {
  it('matches the server diff of the plan fixture', () => {
    const p = plan as Plan
    const before = (stateFixture as TopologyState).text
    expect(diffText(before, p.text)).toEqual(p.diff)
  })
  it('keeps three lines around each change', () => {
    const before = Array.from({ length: 20 }, (_, i) => `line ${i}`).join('\n')
    const after = before.replace('line 10', 'line ten')
    const h = hunks(diffText(before, after))
    expect(h[0]).toBeNull()
    expect(h.filter((l) => l !== null)).toHaveLength(8)
    expect(h[h.length - 1]).toBeNull()
  })
})
