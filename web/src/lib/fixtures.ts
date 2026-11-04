import type { Agent, AgentState, BranchView, DocKind, Feature, Health, IssueView, PullRequestView, Role, PartitionView, RepoView, TaskView, WikiDoc, WikiView } from './types'

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

// repoView is a repository on GitHub whose hikari branch is one commit
// ahead of main and pushed.
export function repoView(name: string, extra: Partial<RepoView> = {}): RepoView {
  return {
    name,
    branch: 'hikari',
    partitions: ['orders'],
    url: `https://github.com/acme/${name}.git`,
    forge: { kind: 'github', host: 'github.com', owner: 'acme', name, web: `https://github.com/acme/${name}`, api: 'https://api.github.com' },
    default_branch: 'main',
    head: 'abc1234',
    branches: [branchView('hikari', { yours: true, ahead: 1 }), branchView('main', { default: true })],
    pull_requests: [],
    synced: '2026-11-04T12:00:00Z',
    ...extra,
  }
}

export function branchView(name: string, extra: Partial<BranchView> = {}): BranchView {
  return { name, sha: 'abc1234', subject: `work on ${name}`, author: 'orders (hikari)', time: '2026-11-04T11:00:00Z', local: true, remote: true, ahead: 0, behind: 0, unpushed: 0, ...extra }
}

export function pullRequest(number: number, extra: Partial<PullRequestView> = {}): PullRequestView {
  return {
    number,
    title: `PR ${number}`,
    url: `https://github.com/acme/shop/pull/${number}`,
    state: 'open',
    head: 'hikari',
    base: 'main',
    sha: 'abc1234',
    checks: { state: 'none', passed: 0, failed: 0, pending: 0, runs: [] },
    ...extra,
  }
}

export function task(id: string, extra: Partial<TaskView> = {}): TaskView {
  return { id, name: id, stage: 'scope', partitions: [], features: [], needs: [], parts: [], file: `tasks/${id}.md`, hash: '1', ...extra }
}

export function issue(ref: string, extra: Partial<IssueView> = {}): IssueView {
  const m = ref.match(/^(.+)#(\d+)$/)
  return { ref, repo: m?.[1], number: m ? Number(m[2]) : undefined, title: ref, state: 'open', labels: [], comments: 0, local: !m, ...extra }
}
