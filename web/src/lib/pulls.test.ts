import { describe, expect, it } from 'vitest'
import { closesLine, scopePulls } from '@/lib/pulls'
import { branchView, issue, partition, pullRequest, repoView, task } from '@/lib/fixtures'

describe('a pull request from a scope', () => {
  const orders = partition('orders')
  const inventory = partition('inventory')
  inventory.folders = [{ place: 'stock', repo: 'stock', path: '', read_only: false }, { place: 'shop/proto', repo: 'shop', path: 'proto', read_only: true }]
  const cancel = task('shop-12', { name: 'Cancel orders', issue: 'shop#12', status: 'done', partitions: ['orders', 'inventory'], features: ['cancel-order', 'release-stock'] })
  const shop12 = issue('shop#12', { title: 'Cancel an order before it ships' })

  it('writes the closing line for the issue\'s repository, another repository of the forge, and a local issue', () => {
    const shop = repoView('shop')
    const stock = repoView('stock')
    expect(closesLine(shop12, shop, [shop, stock])).toBe('Closes #12')
    expect(closesLine(shop12, stock, [shop, stock])).toBe('Closes acme/shop#12')
    expect(closesLine(issue('idea'), shop, [shop])).toBe('')
    expect(closesLine(shop12, repoView('other', { forge: { kind: 'gitlab', host: 'gitlab.com', owner: 'a', name: 'o', web: '', api: '' } }), [shop])).toBe('')
  })

  it('names each repository the scope wrote to, with what is next', () => {
    const shop = repoView('shop', { branches: [branchView('hikari', { yours: true, ahead: 2 }), branchView('main', { default: true })] })
    const stock = repoView('stock', { branches: [branchView('hikari', { yours: true, remote: false, unpushed: 2 }), branchView('main', { default: true })] })
    const pulls = scopePulls(cancel, shop12, [shop, stock], [orders, inventory])
    expect(pulls.map((p) => [p.repo, p.state])).toEqual([
      ['shop', 'open'],
      ['stock', 'push'],
    ])
    expect(pulls[0].title).toBe('Cancel an order before it ships')
    expect(pulls[0].body).toContain('Closes #12')
    expect(pulls[0].body).toContain('the work of orders on the scope `shop-12`')
    expect(pulls[0].body).toContain('Features: cancel-order, release-stock.')
    expect(pulls[1].body).toContain('Closes acme/shop#12')
    expect(pulls[1].body).toContain('the work of inventory')
  })

  it('sees an open or a merged pull request, and a repository without a forge', () => {
    const opened = repoView('shop', { branches: [branchView('hikari', { yours: true, pull_request: 7 }), branchView('main', { default: true })], pull_requests: [pullRequest(7, { head: 'hikari' })] })
    const merged = repoView('shop', { branches: [branchView('hikari', { yours: true, merged: true }), branchView('main', { default: true })], pull_requests: [pullRequest(3, { head: 'hikari', state: 'merged' })] })
    const plain = repoView('shop', { forge: undefined })
    expect(scopePulls(cancel, shop12, [opened], [orders])[0]).toMatchObject({ state: 'opened', pull: 7 })
    expect(scopePulls(cancel, shop12, [merged], [orders])[0]).toMatchObject({ state: 'merged', pull: 3 })
    expect(scopePulls(cancel, shop12, [plain], [orders])[0].state).toBe('no-forge')
    expect(scopePulls(task('x', { partitions: ['web'] }), shop12, [opened], [orders])).toEqual([])
  })
})
