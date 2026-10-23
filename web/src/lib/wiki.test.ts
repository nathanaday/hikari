import { describe, expect, it } from 'vitest'
import { documentBody } from './wiki'

describe('documentBody', () => {
  it('drops the header and turns wiki links into links', () => {
    const doc = '---\nid: a\nrelated: ["[[b]]"]\n---\n\n## Purpose\n\nSee [[b]] and [[c|the c feature]].\n'
    expect(documentBody(doc)).toBe('## Purpose\n\nSee [b](b) and [the c feature](c).')
  })

  it('keeps a document without a header', () => {
    expect(documentBody('Just text with [[x]].')).toBe('Just text with [x](x).')
  })
})
