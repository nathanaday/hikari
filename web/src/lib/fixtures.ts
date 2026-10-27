import type { Agent, AgentState, DocKind, Feature, Health, Role, PartitionView, WikiDoc, WikiView } from './types'

// Test fixtures shaped like the shop demo.

export function feature(id: string, extra: Partial<Feature> = {}): Feature {
  return { id, name: id, tests: { passed: 0, failed: 0, pending: 0 }, health: 'untested', state: 'current', ...extra }
}

export function withHealth(id: string, health: Health, extra: Partial<Feature> = {}): Feature {
  return feature(id, { health, ...extra })
}

export function partition(partitionAgent: string, features: Feature[] = [], extra: Partial<PartitionView> = {}): PartitionView {
  return {
    version: 5,
    partition: { name: partitionAgent, interfaces: { provides: [], consumes: [] }, described_state: {} },
    features,
    state: 'current',
    private: { code_map: { version: 1, parts: [], features: {} }, tests: { version: 1, suites: {}, tests: {} } },
    releases: [],
    partition_agent: partitionAgent,
    folders: [{ place: `shop/services/${partitionAgent}`, repo: 'shop', path: `services/${partitionAgent}`, read_only: false }],
    repos: [{ name: 'shop', branch: 'hikari/demo', dirty: false }],
    flags: null,
    providers: null,
    dirty: false,
    grants: { releases: [], deliverables: [], skills: [] },
    ...extra,
  }
}

export function agent(name: string, state: AgentState = 'idle', role: Role = 'partition'): Agent {
  return { name, role, status: { state } }
}

export function doc(id: string, kind: DocKind = 'feature', extra: Partial<WikiDoc> = {}): WikiDoc {
  return { id, kind, name: id, links: [], file: `x/${id}.md`, hash: id, ...extra }
}

export function wikiOf(ref: string, documents: WikiDoc[]): WikiView {
  if (ref === 'public' || ref === 'orchestration') return { ref, scope: ref, name: ref, documents }
  return { ref, scope: 'partition', name: ref.slice(ref.indexOf(':') + 1), documents }
}
