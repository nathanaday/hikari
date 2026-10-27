import type { Agent, AgentState, DocKind, Feature, Health, Role, SystemView, WikiDoc, WikiView } from './types'

// Test fixtures shaped like the shop demo.

export function feature(id: string, extra: Partial<Feature> = {}): Feature {
  return { id, name: id, tests: { passed: 0, failed: 0, pending: 0 }, health: 'untested', state: 'current', ...extra }
}

export function withHealth(id: string, health: Health, extra: Partial<Feature> = {}): Feature {
  return feature(id, { health, ...extra })
}

export function system(owner: string, features: Feature[] = [], extra: Partial<SystemView> = {}): SystemView {
  return {
    version: 4,
    system: { name: owner, interfaces: { provides: [], consumes: [] }, described_state: {} },
    features,
    state: 'current',
    private: { layout: { version: 1, parts: [], features: {} }, tests: { version: 1, suites: {}, tests: {} } },
    releases: [],
    owner,
    repo: 'shop',
    scope: `services/${owner}`,
    branch: 'hikari/demo',
    flags: null,
    providers: null,
    dirty: false,
    grants: { releases: [], deliverables: [] },
    ...extra,
  }
}

export function agent(name: string, state: AgentState = 'idle', role: Role = 'owner'): Agent {
  return { name, role, status: { state } }
}

export function doc(id: string, kind: DocKind = 'feature', extra: Partial<WikiDoc> = {}): WikiDoc {
  return { id, kind, name: id, links: [], file: `x/${id}.md`, hash: id, ...extra }
}

export function wikiOf(ref: string, documents: WikiDoc[]): WikiView {
  if (ref === 'public') return { ref, scope: 'public', name: 'public', documents }
  return { ref, scope: 'system', name: ref.slice(ref.indexOf(':') + 1), documents }
}
