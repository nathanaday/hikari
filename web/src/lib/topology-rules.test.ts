import { describe, expect, it } from 'vitest'
import rulesJSON from '../../../internal/topology/testdata/rules.json'
import stateFixture from './fixtures/topology/state.json'
import { isExpression, nameProblem, validBranch, validName, validProjectName, validURL } from './topology-rules'
import type { TopologyState } from './types'

type Case = { value: string; valid: boolean }
const cases = rulesJSON as Record<string, Case[]>
const rules: Record<string, (s: string) => boolean> = { name: validName, branch: validBranch, url: validURL, project: validProjectName }

describe('the shared rules (SV4)', () => {
  for (const [rule, cs] of Object.entries(cases)) {
    it(`${rule} matches the Go cases`, () => {
      for (const c of cs) expect([c.value, rules[rule](c.value)]).toEqual([c.value, c.valid])
    })
  }
})

describe('names (SV1)', () => {
  const m = (stateFixture as TopologyState).model!
  it('refuses a taken name, a reserved one, and a bad one', () => {
    expect(nameProblem('lead', 'agent', null, m)).toContain('agent lead')
    expect(nameProblem('main', 'agent', null, m)).toContain('hub main')
    expect(nameProblem('lead', 'agent', 'lead', m)).toBeNull()
    expect(nameProblem('user', 'hub', null, m)).toContain('reserved')
    expect(nameProblem('Dev', 'hub', null, m)).toContain('lowercase')
    expect(nameProblem('main', 'repo', null, m)).toBeNull()
    expect(nameProblem('project', 'repo', null, m)).toContain('reserved')
  })
  it('knows an expression (SI8)', () => {
    expect(isExpression('${HIKARI_BRANCH:-hikari}')).toBe(true)
    expect(isExpression('costs $$5')).toBe(false)
  })
})
