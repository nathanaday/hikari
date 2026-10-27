import type { DocKind, Health, WikiDoc, WikiScope, WikiView } from './types'

// The kinds that agents write, and the kinds that are the user's.
export const agentKinds: DocKind[] = ['feature', 'principle', 'rule']
export const userKinds: DocKind[] = ['plan', 'note']

export const kindLabel: Record<DocKind, string> = {
  feature: 'feature',
  principle: 'principle',
  rule: 'rule',
  plan: 'plan',
  note: 'note',
}

export const isUserKind = (k: DocKind) => k === 'plan' || k === 'note'

export const publicRef = 'public'
export const orchestrationRef = 'orchestration'
export const featureMapRef = (partition: string) => `partition:${partition}`

// Located is a document with the wiki it is in.
export type Located = { wiki: WikiView; doc: WikiDoc }

// WikiIndex finds any document of the vault by id, and the documents that
// link it (rule N6 of the wikis spec).
export type WikiIndex = { byID: Map<string, Located>; backlinks: Map<string, Located[]> }

export function wikiIndex(wikis: WikiView[]): WikiIndex {
  const byID = new Map<string, Located>()
  const backlinks = new Map<string, Located[]>()
  for (const wiki of wikis) {
    for (const doc of wiki.documents) {
      if (!byID.has(doc.id)) byID.set(doc.id, { wiki, doc })
    }
  }
  for (const wiki of wikis) {
    for (const doc of wiki.documents) {
      for (const target of new Set(doc.links)) {
        if (target === doc.id || !byID.has(target)) continue
        const list = backlinks.get(target) ?? []
        list.push({ wiki, doc })
        backlinks.set(target, list)
      }
    }
  }
  for (const list of backlinks.values()) list.sort((a, b) => a.doc.id.localeCompare(b.doc.id))
  return { byID, backlinks }
}

// docHealth is what the tests lens shows for a document: a feature's
// health, or a rule's or a principle's support.
export function docHealth(d: WikiDoc): Health | undefined {
  return d.health ?? d.support
}

// orderDocs sorts documents for a list: by kind in the order of the kinds
// table, then by name.
export function orderDocs(docs: WikiDoc[]): WikiDoc[] {
  const order: DocKind[] = ['feature', 'rule', 'principle', 'plan', 'note']
  return [...docs].sort((a, b) => order.indexOf(a.kind) - order.indexOf(b.kind) || (a.name || a.id).localeCompare(b.name || b.id))
}

// wikiName says which wiki a ref names, for a label: "the public wiki",
// "the orchestration feature map", or "orders".
export function wikiName(ref: string): string {
  if (ref === publicRef) return 'the public wiki'
  if (ref === orchestrationRef) return 'the orchestration feature map'
  return ref.slice(ref.indexOf(':') + 1)
}

// userDocName reports why a name cannot name a user document (rule D2), or
// null when it can.
export function userDocProblem(name: string): string | null {
  if (name.trim() === '') return 'Give the document a name.'
  if (new TextEncoder().encode(name).length > 200) return 'The name holds at most 200 bytes.'
  if (name !== name.trim()) return 'The name cannot start or end with a space.'
  if (name.startsWith('.')) return 'The name cannot start with a dot.'
  if (/[*"\\/<>:|?#^[\]]/.test(name)) return 'The name cannot hold * " \\ / < > : | ? # ^ [ or ].'
  return null
}

// kindsOf returns the kinds a wiki holds: the agent kinds of its scope,
// then the user's (section 2 of the wikis spec).
export function kindsOf(scope: WikiScope): DocKind[] {
  return scope === 'public' ? ['rule', 'principle', 'plan', 'note'] : ['feature', 'principle', 'rule', 'plan', 'note']
}

// template is the text of a new document of kind: a plan or a note needs
// only its kind; an agent kind needs the header and a Purpose.
export function template(kind: DocKind, name: string): string {
  if (kind === 'plan') return `---\nkind: plan\nstatus: idea\n---\n\n`
  if (kind === 'note') return `---\nkind: note\n---\n\n`
  return `---\nid: ${name}\nkind: ${kind}\nname: \ndescription: \n---\n\n## Purpose\n\n`
}

// agentIDProblem says why a name cannot be an agent document's id, or null.
export function agentIDProblem(id: string): string | null {
  return /^[a-z][a-z0-9-]{0,47}$/.test(id) ? null : 'The id is lowercase letters, digits, and dashes, starting with a letter.'
}

function docURL(ref: string, id: string): string {
  return `/api/ui/wiki/${encodeURIComponent(ref)}/docs/${encodeURIComponent(id)}`
}

// request sends one change of the user to the server and returns its
// error, or null.
async function request(url: string, method: string, body?: unknown): Promise<string | null> {
  const res = await fetch(url, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const out = (await res.json().catch(() => ({}))) as { error?: string }
  return out.error ?? res.statusText
}

// writeDoc creates or replaces a document with the text the user gives
// (rules U6 and U8).
export const writeDoc = (ref: string, id: string, text: string) => request(docURL(ref, id), 'PUT', { text })
export const removeDoc = (ref: string, id: string) => request(docURL(ref, id), 'DELETE')
