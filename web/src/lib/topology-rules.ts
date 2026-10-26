import type { TopologyModel } from './types'

// The rules of names, branches, and urls repeat internal/topology, and the
// tests of both run the cases of internal/topology/testdata/rules.json
// (rules SV1 to SV4 of the structure view spec).

const validNameRE = /^[a-z][a-z0-9-]{0,31}$/
const validBranchRE = /^[A-Za-z0-9._/-]+$/

export const validName = (s: string) => validNameRE.test(s)

export function validBranch(b: string): boolean {
  return (
    validBranchRE.test(b) &&
    !b.startsWith('-') &&
    !b.startsWith('/') &&
    !b.endsWith('/') &&
    !b.endsWith('.') &&
    !b.endsWith('.lock') &&
    !b.includes('..') &&
    !b.includes('//')
  )
}

// eslint-disable-next-line no-control-regex
export const validURL = (u: string) => u !== '' && !u.startsWith('-') && !/[\x00-\x20\x7f]/.test(u)

export const validProjectName = (s: string) => s.trim() !== '' && [...s].length <= 64 && !/[\r\n]/.test(s)

// isExpression reports whether compose fills a value in (rule SI8).
export const isExpression = (v: string) => /\$[{A-Za-z_]/.test(v.replaceAll('$$', ''))

export type Kind = 'agent' | 'hub' | 'repo' | 'sidecar'

// nameProblem says why value cannot be the name or the id of an entity of
// kind, other than the one whose id is self, or returns null (rule SV1).
// Agents, hubs, sidecars, and user share a namespace; repos have their own.
export function nameProblem(value: string, kind: Kind, self: string | null, m: TopologyModel): string | null {
  if (!validName(value)) return 'Use a lowercase letter, then lowercase letters, digits, and dashes, up to 32 characters.'
  if (kind === 'repo') {
    if (value === 'project') return 'The name project is reserved for the project repository.'
    if (m.repos.some((r) => r.id !== self && (r.id === value || r.name === value))) return `Another repo is ${value}.`
    return null
  }
  if (value === 'user') return 'The name user is reserved for you.'
  const taken = [
    ...m.agents.map((a) => ({ id: a.id, name: a.name, kind: 'agent' })),
    ...m.hubs.map((h) => ({ id: h.id, name: h.name, kind: 'hub' })),
    ...m.sidecars.map((s) => ({ id: s.id, name: s.id, kind: 'sidecar' })),
  ].find((e) => e.id !== self && (e.id === value || e.name === value))
  return taken ? `The ${taken.kind} ${taken.id} is ${value} already.` : null
}

export function branchProblem(b: string): string | null {
  if (isExpression(b)) return null
  return validBranch(b) ? null : 'Not a valid branch name.'
}

export function urlProblem(u: string): string | null {
  return validURL(u) ? null : 'A url has no spaces and does not start with -.'
}
