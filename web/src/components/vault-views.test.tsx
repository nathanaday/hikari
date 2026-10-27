import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { DeliverableEntry } from '@/components/shared-view'
import { PartitionDetail } from '@/components/partition-detail'
import { ReleaseBody } from '@/components/release-view'
import { partition } from '@/lib/fixtures'
import { initialState } from '@/lib/store'
import type { Deliverable, ReleaseDetail } from '@/lib/types'

const deliverable: Deliverable = {
  name: 'python-library',
  path: 'public/deliverables/python-library',
  skill: 'public/deliverables/python-library/SKILL.md',
  description: 'A Python package.',
  layout: { require: ['README.md'], forbid: ['**/*.pyc'] },
  targets: [{ type: 'github-release', repo: 'acme/inventory' }],
  holders: ['inventory', 'orders'],
}

const dir = 'partitions/inventory/read-only/releases/python-library/0.1.0'
const base = dir + '/'
const release: ReleaseDetail = {
  manifest: {
    deliverable: 'python-library',
    version: '0.1.0',
    partition: 'inventory',
    released: '2026-10-22T20:17:05Z',
    notes: 'First release.',
    hikari: '0.0.1',
    source_commits: ['shop@23732d5585d3cec9b4798f825ba99c519488b2e0'],
    product: { path: 'product/', files: 3, bytes: 2569, digest: 'sha256:d8c523f2' },
    map: 'features/map.yaml',
    certificates: [
      { type: 'github-release', repo: 'acme/inventory', tag: 'v0.1.0', url: 'https://github.com/acme/inventory/releases/tag/v0.1.0' },
      { type: 'dockerhub', repository: 'acme/inventory', tag: '0.1.0', digest: 'sha256:abc' },
    ],
  },
  dir,
  files: [base + 'SHA256SUMS', base + 'features/map.yaml', base + 'product/README.md', base + 'product/inventory/stock.py', base + 'release.yaml'],
  digests: { 'product/README.md': '0ffc73ea41e7', 'product/inventory/stock.py': 'a40eab3a026b' },
}

describe('DeliverableEntry', () => {
  it('shows the rules, the targets, and the holders (U12)', () => {
    const html = renderToStaticMarkup(<DeliverableEntry d={deliverable} />)
    for (const want of ['python-library', 'A Python package.', 'held by inventory, orders', 'README.md', '**/*.pyc', 'github-release acme/inventory', 'public/deliverables/python-library/']) {
      expect(html).toContain(want)
    }
    expect(html).not.toContain('No layout rules')
  })

  it('shows an invalid deliverable with its problem', () => {
    const html = renderToStaticMarkup(<DeliverableEntry d={{ ...deliverable, description: '', layout: {}, targets: [], holders: [], error: 'has no SKILL.md' }} />)
    expect(html).toContain('invalid')
    expect(html).toContain('has no SKILL.md')
    expect(html).toContain('no holder')
  })
})

// marked returns the highlighted file button, the one shown as the main page.
const marked = (html: string) => {
  const at = html.indexOf('bg-neutral-100 font-medium')
  return html.slice(at, html.indexOf('</button>', at))
}

describe('ReleaseBody', () => {
  it('shows the manifest, the certificates, and the files with digests (U14)', () => {
    const html = renderToStaticMarkup(<ReleaseBody data={release} />)
    for (const want of ['First release.', '3 files, 2569 bytes', 'sha256:d8c523f2', '23732d5', 'href="https://github.com/acme/inventory/releases/tag/v0.1.0"', 'acme/inventory · v0.1.0', 'sha256:abc', 'product/inventory/stock.py', 'a40eab3a', 'sha256 0ffc73ea41e7']) {
      expect(html).toContain(want)
    }
    expect(html.match(/data-testid="certificate"/g)).toHaveLength(2)
    expect(marked(html)).toContain('product/README.md')
  })

  it('opens release.yaml when the product has no README', () => {
    const data = { ...release, files: release.files.filter((f) => !f.endsWith('README.md')) }
    const html = renderToStaticMarkup(<ReleaseBody data={data} />)
    expect(marked(html)).toContain('release.yaml')
  })
})

describe('PartitionDetail', () => {
  it('shows the folders by place and each repo with its branch and changes (F15)', () => {
    const v = partition('orders', [], {
      folders: [
        { place: 'shop/services/orders', repo: 'shop', path: 'services/orders', read_only: false },
        { place: 'protos', repo: 'protos', path: 'orders', read_only: true },
      ],
      repos: [
        { name: 'shop', branch: 'hikari/work', dirty: true },
        { name: 'protos', branch: 'hikari/work', dirty: false },
      ],
    })
    const html = renderToStaticMarkup(<PartitionDetail state={initialState({ kind: 'room' })} partition={v} />)
    for (const text of ['/workspace/shop/services/orders', 'services/orders of shop', '/workspace/protos', 'orders of protos, read-only', 'with uncommitted changes', ', clean']) expect(html).toContain(text)
  })
})
