import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Markdown } from '@/components/markdown'

const html = (text: string) => renderToStaticMarkup(<Markdown text={text} names={new Set(['dev'])} />)

describe('Markdown', () => {
  it('marks mentions of known names only and keeps the text around them', () => {
    expect(html('@dev and @ghost, done')).toContain('<p><span class="mention">@dev</span> and @ghost, done</p>')
  })

  it('keeps single newlines as line breaks', () => {
    expect(html('**Answer**\nApproach:')).toContain('<strong>Answer</strong><br/>')
  })

  it('leaves mentions in code alone', () => {
    expect(html('`@dev`')).toContain('<code>@dev</code>')
  })

  it('renders raw HTML as text', () => {
    expect(html('<img src=x onerror=alert(1)>')).not.toContain('<img')
  })
})
