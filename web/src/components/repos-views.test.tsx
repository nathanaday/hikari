import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { AppSidebar } from '@/components/app-sidebar'
import { RepositoriesView, RepoView } from '@/components/repos-view'
import { SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { branchView, pullRequest, repoView } from '@/lib/fixtures'
import { branchLine, repoGit, repoHealth } from '@/lib/repos'
import { initialState, reducer, type State } from '@/lib/store'
import type { Checks } from '@/lib/types'
import { parseHash, viewHash, viewKey } from '@/lib/view'

const checks = (state: Checks['state'], runs: Checks['runs'] = []): Checks => ({ state, passed: runs.filter((r) => r.state === 'passing').length, failed: runs.filter((r) => r.state === 'failing').length, pending: runs.filter((r) => r.state === 'pending').length, runs })

const shop = repoView('shop', {
  branches: [branchView('hikari', { yours: true, ahead: 3, behind: 1, pull_request: 5 }), branchView('main', { default: true }), branchView('feat', { local: false, ahead: 1, merged: false, subject: 'try a thing', author: 'bob' })],
  pull_requests: [
    pullRequest(5, { title: 'Cancel orders', author: 'alex', mergeable: 'mergeable', review: 'approved', additions: 10, deletions: 2, files: 3, closes: [12], checks: checks('passing', [{ name: 'go test', state: 'passing', url: 'u' }]) }),
    pullRequest(4, { title: 'Broken', head: 'feat', mergeable: 'conflicting', checks: checks('failing', [{ name: 'lint', state: 'failing' }]) }),
    pullRequest(3, { title: 'Done long ago', state: 'merged', merged: '2026-11-01T00:00:00Z' }),
  ],
})
const site = repoView('site', { branches: [branchView('hikari', { yours: true, remote: false, unpushed: 2 }), branchView('main', { default: true })] })
const project = repoView('project', { project: true, partitions: [], forge: undefined, url: undefined })

function stateWith(): State {
  return { ...initialState({ kind: 'repos' }), loaded: true, repos: [shop, site, project] }
}

const noop = () => {}

describe('lib/repos', () => {
  it('rates a repository for the sidebar dot (UI1)', () => {
    expect(repoHealth(shop)).toBe('bad')
    expect(repoHealth(site)).toBe('warn')
    expect(repoHealth(project)).toBe('good')
    expect(repoHealth(repoView('ok'))).toBe('good')
    expect(repoHealth(repoView('err', { error: 'github: Bad credentials' }))).toBe('bad')
    expect(repoHealth(repoView('pend', { pull_requests: [pullRequest(1, { checks: checks('pending') })] }))).toBe('warn')
  })

  it('colors a repository for the git lens (UI3)', () => {
    expect(repoGit(shop)).toBe('failing')
    expect(repoGit(site)).toBe('pending')
    expect(repoGit(repoView('d', { dirty: true }))).toBe('dirty')
    expect(repoGit(repoView('p', { pull_requests: [pullRequest(1, { checks: checks('passing') })] }))).toBe('passing')
    expect(repoGit(repoView('c'))).toBe('clean')
  })

  it('says how your branch stands (RP2)', () => {
    expect(branchLine(shop)).toBe('3 ahead of main · 1 behind · pushed')
    expect(branchLine(site)).toBe('0 ahead of main · 0 behind · not pushed yet')
    expect(branchLine(repoView('m', { branches: [branchView('hikari', { yours: true, merged: true }), branchView('main', { default: true })] }))).toBe('0 ahead of main · 0 behind · pushed · merged into main')
    expect(branchLine(repoView('none', { branches: [] }))).toBe('hikari · not created yet')
  })
})

describe('RepositoriesView', () => {
  it('lists each repository with its branch and pull requests (UI1)', () => {
    const html = renderToStaticMarkup(<RepositoriesView state={stateWith()} show={noop} />)
    expect(html).toContain('data-repo="shop"')
    expect(html).toContain('3 ahead of main · 1 behind · pushed')
    expect(html).toContain('2 open pull requests')
    expect(html).toContain('1 failing')
    expect(html).toContain('acme/shop')
    expect(html).toContain('not pushed yet')
    expect(html).toContain('project repository')
    expect(html).toContain('no forge')
  })
})

describe('RepoView', () => {
  it('shows your branch, the pull requests with their checks, and the branches (UI2)', () => {
    const html = renderToStaticMarkup(<RepoView state={stateWith()} name="shop" show={noop} />)
    expect(html).toContain('data-testid="your-branch"')
    expect(html).toContain('#5 Cancel orders')
    expect(html).toContain('data-checks="passing"')
    expect(html).toContain('data-checks="failing"')
    expect(html).toContain('>approved<')
    expect(html).toContain('conflicts with main')
    expect(html).toContain('+10')
    expect(html).toContain('closes #12')
    expect(html).toContain('>merged<')
    // Merge is offered only for the open, mergeable, passing pull request.
    const pr5 = html.slice(html.indexOf('data-pr="5"'), html.indexOf('data-pr="4"'))
    const pr4 = html.slice(html.indexOf('data-pr="4"'), html.indexOf('data-pr="3"'))
    const pr3 = html.slice(html.indexOf('data-pr="3"'))
    expect(pr5).toContain('Merge')
    expect(pr4).not.toContain('>Merge<')
    expect(pr3).not.toContain('>Merge<')
    for (const b of ['hikari', 'main', 'feat']) expect(html).toContain(`data-branch="${b}"`)
    expect(html).toContain('>yours<')
    expect(html).toContain('>default<')
    expect(html).toContain('try a thing · bob')
    expect(html).toContain('>orders<')
  })

  it('offers Push for unpushed work, and Open pull request once pushed', () => {
    const pushed = renderToStaticMarkup(<RepoView state={stateWith()} name="site" show={noop} />)
    const yours = pushed.slice(pushed.indexOf('data-testid="your-branch"'), pushed.indexOf('data-testid="pull-requests"'))
    const button = (label: string) => yours.split('<button').find((b) => b.includes(`${label}</button>`))!
    expect(button('Push')).not.toContain('disabled=""')
    expect(button('Open pull request')).toContain('disabled=""')
    const shopPage = renderToStaticMarkup(<RepoView state={stateWith()} name="shop" show={noop} />)
    const shopYours = shopPage.slice(shopPage.indexOf('data-testid="your-branch"'), shopPage.indexOf('data-testid="pull-requests"'))
    expect(shopYours.split('<button').find((b) => b.includes('Push</button>'))).toContain('disabled=""')
    expect(shopYours).toContain('#5 Cancel orders')
    const none = renderToStaticMarkup(<RepoView state={stateWith()} name="nope" show={noop} />)
    expect(none).toContain('No repository nope')
  })
})

describe('the sidebar and the routes', () => {
  it('lists the repositories with a dot each (UI1)', () => {
    const html = renderToStaticMarkup(
      <TooltipProvider>
        <SidebarProvider>
          <AppSidebar state={stateWith()} show={noop} />
        </SidebarProvider>
      </TooltipProvider>,
    )
    expect(html).toContain('Repositories')
    expect(html).toContain('data-health="bad"')
    expect(html).toContain('data-health="warn"')
    expect(html).toContain('data-health="good"')
  })

  it('routes #/repos and #/repos/<name>', () => {
    expect(parseHash('#/repos')).toEqual({ kind: 'repos' })
    expect(parseHash('#/repos/shop')).toEqual({ kind: 'repo', name: 'shop' })
    expect(viewHash({ kind: 'repo', name: 'shop' })).toBe('#/repos/shop')
    expect(viewKey({ kind: 'repos' })).toBe('repos')
    expect(viewKey({ kind: 'repo', name: 'shop' })).toBe('repo:shop')
  })

  it('keeps the repo views current from the repo event, and leaves a missing repo', () => {
    let s = reducer(initialState({ kind: 'repo', name: 'shop' }), { type: 'event', event: { type: 'snapshot', snapshot: { me: 'user', agents: [], participants: [], messages: [], partitions: [], vocabulary: { version: 1, terms: [], interfaces: [] }, repos: [shop] } } })
    expect(s.view).toEqual({ kind: 'repo', name: 'shop' })
    s = reducer(s, { type: 'event', event: { type: 'repo', repo: { ...shop, dirty: true } } })
    expect(s.repos[0].dirty).toBe(true)
    s = reducer(s, { type: 'event', event: { type: 'repo', repo: site } })
    expect(s.repos.map((r) => r.name)).toEqual(['shop', 'site'])
    expect(reducer(s, { type: 'view', view: { kind: 'repo', name: 'gone' } }).view).toEqual({ kind: 'repos' })
  })
})
