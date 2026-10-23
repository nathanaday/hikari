import type { DocKind, Health, WikiDoc, WikiView } from './types'

// The kinds that agents write, and the kinds that are the user's.
export const agentKinds: DocKind[] = ['feature', 'principle', 'rule', 'story']
export const userKinds: DocKind[] = ['plan', 'note']

export const kindLabel: Record<DocKind, string> = {
  feature: 'feature',
  principle: 'principle',
  rule: 'rule',
  story: 'story',
  plan: 'plan',
  note: 'note',
}

export const isUserKind = (k: DocKind) => k === 'plan' || k === 'note'

export const hubRef = (hub: string) => `hub:${hub}`
export const systemRef = (system: string) => `system:${system}`

// Located is a document with the wiki it is in.
export type Located = { wiki: WikiView; doc: WikiDoc }

// WikiIndex finds any document of the store by id, and the documents that
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
// health, or a story's or a rule's support.
export function docHealth(d: WikiDoc): Health | undefined {
  return d.health ?? d.support
}

// orderDocs sorts documents for a list: key stories first, then by kind in
// the order of the kinds table, then by name.
export function orderDocs(docs: WikiDoc[]): WikiDoc[] {
  const order: DocKind[] = ['story', 'rule', 'principle', 'feature', 'plan', 'note']
  return [...docs].sort(
    (a, b) =>
      Number(!!b.key) - Number(!!a.key) ||
      order.indexOf(a.kind) - order.indexOf(b.kind) ||
      (a.name || a.id).localeCompare(b.name || b.id),
  )
}

// wikiName says which wiki a ref names, for a label: "#shop" or "orders".
export function wikiName(ref: string): string {
  const [scope, name] = ref.split(':')
  return scope === 'hub' ? `#${name}` : name
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
export function kindsOf(scope: 'hub' | 'system'): DocKind[] {
  return scope === 'hub' ? ['story', 'rule', 'principle', 'plan', 'note'] : ['feature', 'principle', 'rule', 'plan', 'note']
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

function docURL(ref: string, part: 'docs' | 'key', id: string): string {
  const [scope, name] = ref.split(':')
  return `/api/ui/wiki/${scope}/${name}/${part}/${encodeURIComponent(id)}`
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
export const writeDoc = (ref: string, id: string, text: string) => request(docURL(ref, 'docs', id), 'PUT', { text })
export const removeDoc = (ref: string, id: string) => request(docURL(ref, 'docs', id), 'DELETE')
export const setKey = (ref: string, id: string, key: boolean) => request(docURL(ref, 'key', id), 'PUT', { key })

// approveGap approves or withdraws the approval of a gap of a hub's plan
// (rule GA1 of the tester spec).
export const approveGap = (hub: string, gap: string, approved: boolean) =>
  request(`/api/ui/e2e/${hub}/gaps/${encodeURIComponent(gap)}`, 'PUT', { approved })

// stopEnvironment stops a hub's environment (rule UE1 of the environments
// spec).
export const stopEnvironment = (hub: string) => request(`/api/ui/e2e/${hub}/environment/stop`, 'POST')

// recordBench records the user's result of a bench procedure (rule BR1 of
// the bench spec).
export const recordBench = (hub: string, id: string, passed: boolean, note: string) =>
  request(`/api/ui/e2e/${hub}/bench/${encodeURIComponent(id)}`, 'PUT', { passed, note })
