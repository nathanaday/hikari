// documentBody returns a feature document's markdown without its property
// header, with each [[id]] or [[id|text]] link made a link to the id, which
// the feature panel turns into a selection on the canvas.
export function documentBody(text: string): string {
  let body = text
  if (body.startsWith('---\n')) {
    const end = body.indexOf('\n---', 4)
    if (end >= 0) body = body.slice(body.indexOf('\n', end + 1) + 1)
  }
  return body.replace(/\[\[([^[\]|#]+)(?:\|([^[\]]*))?\]\]/g, (_, id: string, text?: string) => `[${text || linkTarget(id)}](${encodeURI(linkTarget(id))})`).trim()
}

// linkTarget names the document a link names: its last path segment,
// without .md, as Obsidian completes links (rule N1 of the wikis spec).
export function linkTarget(target: string): string {
  const t = target.trim()
  return t.slice(t.lastIndexOf('/') + 1).replace(/\.md$/, '').trim()
}
