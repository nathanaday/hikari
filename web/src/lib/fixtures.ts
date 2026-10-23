import type { Agent, AgentState, DocKind, Feature, Health, SystemView, WikiDoc, WikiView } from './types'

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

export function agent(name: string, state: AgentState = 'idle'): Agent {
  return { name, hubs: ['shop'], status: { state } }
}

export function doc(id: string, kind: DocKind = 'feature', extra: Partial<WikiDoc> = {}): WikiDoc {
  return { id, kind, name: id, links: [], file: `x/${id}.md`, hash: id, ...extra }
}

export function wikiOf(ref: string, documents: WikiDoc[]): WikiView {
  const [scope, name] = ref.split(':')
  return { ref, scope: scope as 'hub' | 'system', name, documents }
}
