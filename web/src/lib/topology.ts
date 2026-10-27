import type { DiffLine, Finding, Mode, Plan, Saved, TopologyState } from './types'

// The result of a call to the topology API: the answer, or the error that
// the server gave. A conflict carries the file's text (rule TA2).
export type Result<T> = { ok: true; data: T } | { ok: false; error: string; status: number; conflict?: { revision: string; text: string }; findings?: Finding[] }

async function call<T>(method: string, url: string, body?: unknown): Promise<Result<T>> {
  const res = await fetch(url, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).catch(() => undefined)
  if (!res) return { ok: false, error: 'The hikari server is not reachable.', status: 0 }
  const data = (await res.json().catch(() => ({}))) as Record<string, unknown>
  if (res.ok) return { ok: true, data: data as T }
  const out: Result<T> = { ok: false, error: (data.error as string) ?? res.statusText, status: res.status }
  if (res.status === 409 && typeof data.text === 'string') out.conflict = { revision: data.revision as string, text: data.text }
  if (Array.isArray(data.findings)) out.findings = data.findings as Finding[]
  return out
}

export const getTopology = () => call<TopologyState>('GET', '/api/ui/topology')
export const lintText = (text: string) => call<{ findings: Finding[] }>('POST', '/api/ui/topology/lint', { text })
export const putSource = (revision: string, text: string) => call<Saved>('PUT', '/api/ui/topology/source', { revision, text })
export const createProject = () => call<Saved>('POST', '/api/ui/topology/create')
export const planOps = (revision: string, operations: unknown[]) => call<Plan>('POST', '/api/ui/topology/plan', { revision, operations })
export const applyOps = (revision: string, operations: unknown[], propagate: string[]) =>
  call<Saved>('POST', '/api/ui/topology/apply', { revision, operations, propagate })
export const startAgents = () => call<unknown>('POST', '/api/ui/run/start')
export const stopAgents = () => call<unknown>('POST', '/api/ui/run/stop')

// sortFindings puts errors first, then by line, findings without a line
// last (rule YY3).
export function sortFindings(fs: Finding[]): Finding[] {
  const rank = (f: Finding) => (f.severity === 'error' ? 0 : 1)
  return [...fs].sort((a, b) => rank(a) - rank(b) || (a.line || Infinity) - (b.line || Infinity))
}

export const hasError = (fs: Finding[]) => fs.some((f) => f.severity === 'error')

// Marker is a finding placed on a line of the editor (rule YY2). A finding
// without a line marks nothing.
export type Marker = { line: number; severity: Finding['severity']; message: string }

export function markers(fs: Finding[], lines: number): Marker[] {
  return fs.filter((f) => f.line && f.line <= lines).map((f) => ({ line: f.line!, severity: f.severity, message: f.message }))
}

// diffText makes the line diff of the review of a save (rule YY5), as the
// server makes a plan's: the longest common subsequence of lines.
export function diffText(before: string, after: string): DiffLine[] {
  const split = (s: string) => (s === '' ? [] : s.replace(/\n$/, '').split('\n'))
  const a = split(before)
  const b = split(after)
  const n = a.length
  const m = b.length
  const lcs = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      out.push({ op: ' ', text: a[i], old: i + 1, new: j + 1 })
      i++
      j++
    } else if (i < n && (j === m || lcs[i + 1][j] >= lcs[i][j + 1])) {
      // A removed line comes before the line that replaces it.
      out.push({ op: '-', text: a[i], old: i + 1 })
      i++
    } else {
      out.push({ op: '+', text: b[j], new: j + 1 })
      j++
    }
  }
  return out
}

// hunks keeps the changed lines of a diff with ctx lines around each, and
// marks each run of lines it leaves out with null.
export function hunks(diff: DiffLine[], ctx = 3): (DiffLine | null)[] {
  const keep = new Set<number>()
  diff.forEach((l, i) => {
    if (l.op !== ' ') for (let k = i - ctx; k <= i + ctx; k++) keep.add(k)
  })
  const out: (DiffLine | null)[] = []
  let gap = false
  diff.forEach((l, i) => {
    if (keep.has(i)) {
      if (gap) out.push(null)
      out.push(l)
      gap = false
    } else gap = true
  })
  if (gap && out.length) out.push(null)
  return out
}

export const modeLabel: Record<Mode, string> = {
  editing: 'stopped',
  starting: 'starting',
  running: 'running',
  stopping: 'stopping',
}

export const getOptions = () => call<{ deliverables: string[]; skills: string[]; models: string[]; templates: string[] }>('GET', '/api/ui/topology/options')
