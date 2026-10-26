import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { DiffView } from '@/components/diff-view'
import { FindingsList } from '@/components/findings-list'
import plan from '@/lib/fixtures/topology/plan.json'
import type { Plan } from '@/lib/types'

describe('DiffView', () => {
  it('shows the added and removed lines of a plan with their numbers (YY5)', () => {
    const html = renderToStaticMarkup(<DiffView diff={(plan as Plan).diff} />)
    expect(html).toContain('data-op="+"')
    expect(html).toContain('data-op="-"')
    expect(html).toContain('name: core')
  })
  it('says when nothing changes', () => {
    expect(renderToStaticMarkup(<DiffView diff={[{ op: ' ', text: 'a', old: 1, new: 1 }]} />)).toContain('does not change')
  })
})

describe('FindingsList', () => {
  it('lists each finding with its line, and says when there is none (YY3)', () => {
    const html = renderToStaticMarkup(
      <FindingsList findings={[{ severity: 'error', stage: 'topology', message: 'hub "main" references unknown agent "ghost"', line: 7 }]} onLine={() => {}} />,
    )
    expect(html).toContain('Line 7')
    expect(html).toContain('aria-label="error"')
    expect(renderToStaticMarkup(<FindingsList findings={[]} />)).toContain('No problems')
  })
})
