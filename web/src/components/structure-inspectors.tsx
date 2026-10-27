import { useState } from 'react'
import { ArrowDown, ArrowUp, Boxes, FileCode2, FolderGit2, Package, X } from 'lucide-react'
import { FindingsList } from '@/components/findings-list'
import { Section } from '@/components/partition-detail'
import { Chips, Field, Select, TextField } from '@/components/topology-fields'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { findingsOf, partitionAgents, partitionsIn, placeOf, type AddKind, type Op, type Selection } from '@/lib/structure'
import { branchProblem, nameProblem, urlProblem, validProjectName } from '@/lib/topology-rules'
import type { Finding, Mount, Packs, TopologyAgent, TopologyFolder, TopologyModel } from '@/lib/types'

export type Options = { deliverables: string[]; skills: string[]; models: string[]; templates: string[] }

export type InspectorProps = {
  model: TopologyModel
  findings: Finding[]
  // mounts maps each agent's id to its mounts, or is null when the text
  // does not pass the checks.
  mounts: Record<string, Mount[]> | null
  // packs are the built-in skills of each role, from the server.
  packs: Packs
  options: Options
  // add plans one more operation, and returns the refusal, if any.
  add: (op: Op) => Promise<string | null>
  select: (s: Selection) => void
  editYAML: (line: number) => void
}

function Header({ icon, name, id, children }: { icon: React.ReactNode; name: string; id?: string; children?: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="flex items-center gap-1.5 text-base font-semibold">
          {icon}
          {name}
        </h2>
        {id !== undefined && id !== name && <p className="text-xs text-muted-foreground">id {id}</p>}
      </div>
      {children}
    </div>
  )
}

function Problems({ findings }: { findings: Finding[] }) {
  if (findings.length === 0) return null
  return (
    <div className="-mx-3 rounded-md border bg-[color-mix(in_oklch,var(--lens-fail)_4%,white)]" data-testid="entity-findings">
      <FindingsList findings={findings} />
    </div>
  )
}

// Identity edits the name and the id of an agent or a repo (rule SI6). A rename keeps the id; the id changes only while nothing outside
// the file is keyed by it.
function Identity({ kind, id, name, locked, p }: { kind: 'agent' | 'repo'; id: string; name: string; locked: boolean; p: InspectorProps }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <TextField
        label="Name"
        value={name}
        check={(v) => (v === id ? null : nameProblem(v, kind, id, p.model))}
        onCommit={(v) => p.add({ op: 'rename', kind, id, name: v })}
        hint={kind === 'repo' ? 'What the UI calls it.' : `What agents and you call it: @${name}.`}
      />
      {locked ? (
        <Field label="Id" hint="The vault is keyed by this id, so it stays. Rename it instead.">
          <code className="block rounded-md border bg-neutral-50 px-3 py-1.5 text-xs">{id}</code>
        </Field>
      ) : (
        <TextField
          label="Id"
          value={id}
          mono
          check={(v) => nameProblem(v, kind, id, p.model)}
          onCommit={async (v) => {
            if (!(await p.add({ op: 'change_id', kind, id, new_id: v }))) p.select({ kind, id: v })
          }}
          hint="The key in the file. It can change until hikari keeps something under it."
        />
      )}
    </div>
  )
}

function RemoveButton({ label, disabled, reason, onRemove }: { label: string; disabled?: boolean; reason?: string; onRemove: () => void }) {
  const [confirm, setConfirm] = useState(false)
  return (
    <div className="flex flex-wrap items-center gap-3 border-t pt-4">
      {confirm ? (
        <>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {label}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setConfirm(false)}>
            Keep it
          </Button>
        </>
      ) : (
        <Button size="sm" variant="outline" disabled={disabled} onClick={() => setConfirm(true)}>
          {label}
        </Button>
      )}
      {disabled && reason && <span className="text-xs text-muted-foreground">{reason}</span>}
    </div>
  )
}

// ProjectInspector edits the project (rule SI1).
export function ProjectInspector(p: InspectorProps) {
  const pr = p.model.project
  return (
    <div className="space-y-6">
      <Header icon={<FileCode2 className="size-4 text-muted-foreground" />} name={pr.name} id={pr.id} />
      <Problems findings={findingsOf(p.findings, { kind: 'project' })} />
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField
          label="Name"
          value={pr.name}
          check={(v) => (validProjectName(v) ? null : 'One line of 1 to 64 characters.')}
          onCommit={(v) => p.add({ op: 'set_project', name: v })}
          hint="Shown in the UI and the terminal."
        />
        <Field label="Compose project" hint="It names the containers, networks, and volumes, so it stays.">
          <code className="block rounded-md border bg-neutral-50 px-3 py-1.5 text-xs">{pr.id}</code>
        </Field>
      </div>
      <Section title="Project repository">
        <p className="text-sm text-muted-foreground">
          {pr.repo
            ? 'The directory of the topology file is a git repository: hikari commits the vault to it, and clones linked repos into repos/.'
            : 'The vault is not versioned. Turn the project repository on to keep its history in git and to link repos by url.'}
        </p>
        {pr.repo && (
          <TextField label="Branch" value={pr.branch} mono check={branchProblem} onCommit={(v) => p.add({ op: 'set_project', branch: v })} hint="Agents commit here, and you merge it." />
        )}
        <Button size="sm" variant="outline" onClick={() => p.add({ op: pr.repo ? 'disable_project' : 'enable_project' })}>
          {pr.repo ? 'Turn off the project repository' : 'Turn on the project repository'}
        </Button>
      </Section>
      <Section title="Orchestration agent">
        <Field label="Orchestration agent" hint="It plans from the feature maps, writes the vocabulary and the public wiki, and every post wakes it. It has no partition; every other agent has one.">
          <Select
            value={pr.orchestration}
            options={[{ value: '', label: 'None' }, ...p.model.agents.filter((a) => !a.partition || a.id === pr.orchestration).map((a) => ({ value: a.id, label: a.name }))]}
            onChange={(v) => p.add({ op: 'set_orchestration', agent: v })}
          />
        </Field>
      </Section>
      <Section title="Vault">
        <TextField label="Directory" value={pr.vault} mono placeholder="vault" onCommit={(v) => p.add({ op: 'set_project', vault: v })} hint="Relative to the topology file. Empty means vault/." />
      </Section>
    </div>
  )
}

// RepoInspector edits a repo (rule SI2).
export function RepoInspector({ id, ...p }: InspectorProps & { id: string }) {
  const r = p.model.repos.find((x) => x.id === id)!
  const partitions = partitionsIn(id, p.model)
  return (
    <div className="space-y-6">
      <Header icon={<FolderGit2 className="size-4 text-muted-foreground" />} name={r.name} id={r.id} />
      <Problems findings={findingsOf(p.findings, { kind: 'repo', id })} />
      <Identity kind="repo" id={r.id} name={r.name} locked={r.id_locked} p={p} />
      <Section title="Where it lives">
        {r.url ? (
          <TextField label="Url" value={r.url} mono check={urlProblem} onCommit={(v) => p.add({ op: 'set_repo', id, url: v })} hint="hikari sync clones it into repos/." />
        ) : (
          <TextField label="Path" value={r.path} mono check={(v) => (v ? null : 'A repo needs a path or a url.')} onCommit={(v) => p.add({ op: 'set_repo', id, path: v })} hint="On this machine; relative paths start at the topology file." />
        )}
        <TextField label="Branch" value={r.branch} mono placeholder={r.url ? 'the project branch' : ''} check={(v) => (v ? branchProblem(v) : null)} onCommit={(v) => p.add({ op: 'set_repo', id, branch: v })} />
      </Section>
      <Section title="Partitions in it">
        {partitions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No partition uses it yet.</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {partitions.map((a) => (
              <li key={a.id}>
                <button type="button" className="underline underline-offset-2" onClick={() => p.select({ kind: 'agent', id: a.id })}>
                  {a.name}
                </button>{' '}
                <span className="text-muted-foreground">
                  {a
                    .partition!.folders.filter((f) => f.repo === id)
                    .map((f) => (f.path || 'the whole repo') + (f.read_only ? ', read-only' : ''))
                    .join('; ')}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <RemoveButton
        label="Remove the repo"
        disabled={partitions.length > 0}
        reason={`Used by ${partitions.map((a) => a.name).join(', ')}.`}
        onRemove={async () => {
          if (!(await p.add({ op: 'remove_repo', id }))) p.select({ kind: 'project' })
        }}
      />
    </div>
  )
}

const modelOptions = (opts: Options, current: string) => {
  const list = [{ value: '', label: 'The default model' }, ...opts.models.map((m) => ({ value: m, label: m }))]
  if (current && !opts.models.includes(current)) list.push({ value: current, label: current })
  return list
}

// AgentInspector edits an agent (rule SI4).
export function AgentInspector({ id, ...p }: InspectorProps & { id: string }) {
  const a = p.model.agents.find((x) => x.id === id)!
  const orchestration = p.model.project.orchestration === id
  return (
    <div className="space-y-6">
      <Header icon={<Boxes className="size-4 text-muted-foreground" />} name={a.name} id={a.id} />
      <Problems findings={findingsOf(p.findings, { kind: 'agent', id })} />
      <Identity kind="agent" id={a.id} name={a.name} locked={a.id_locked} p={p} />
      <TextField label="Description" value={a.description} multiline onCommit={(v) => p.add({ op: 'set_agent', id, description: v })} hint="Every agent in the room sees it." />
      <TextField label="Instructions" value={a.instructions} multiline onCommit={(v) => p.add({ op: 'set_agent', id, instructions: v })} hint="Only this agent sees them, in its system prompt." />
      <Field label="Model">
        <Select value={a.model} options={modelOptions(p.options, a.model)} onChange={(v) => p.add({ op: 'set_agent', id, model: v })} />
      </Field>

      <PartitionSection a={a} orchestration={orchestration} p={p} />

      <Section title="Grants">
        {a.partition && (
          <Field label="Releases it reads" hint="It reads these partitions' releases; a container gets new grants when it starts.">
            <Chips
              items={a.releases.map((r) => ({ value: r, label: p.model.agents.find((x) => x.id === r)?.name ?? r }))}
              onRemove={(r) => p.add({ op: 'set_grants', id, releases: a.releases.filter((x) => x !== r) })}
              onAdd={(r) => p.add({ op: 'set_grants', id, releases: [...a.releases, r] })}
              choices={partitionAgents(p.model)
                .filter((o) => o.id !== id && !a.releases.includes(o.id))
                .map((o) => ({ value: o.id, label: o.name }))}
              addLabel="Grant the releases of…"
            />
          </Field>
        )}
        {a.partition && (
          <Field label="Deliverables it publishes">
            <Chips
              items={a.deliverables.map((d) => ({ value: d, label: d }))}
              onRemove={(d) => p.add({ op: 'set_grants', id, deliverables: a.deliverables.filter((x) => x !== d) })}
              onAdd={(d) => p.add({ op: 'set_grants', id, deliverables: [...a.deliverables, d] })}
              choices={p.options.deliverables.filter((d) => !a.deliverables.includes(d)).map((d) => ({ value: d, label: d }))}
              addLabel="Add a deliverable…"
              empty={p.options.deliverables.length ? 'None' : 'None; the vault defines no deliverable yet.'}
            />
          </Field>
        )}
        <BuiltinSkills skills={(orchestration ? p.packs.orchestration : a.partition ? p.packs.partition : undefined) ?? []} />
        <Field label="Skills it uses" hint="Your skills in the vault's skills/, which its container reads at /hikari/skills/<name>.">
          <Chips
            items={a.skills.map((s) => ({ value: s, label: s }))}
            onRemove={(s) => p.add({ op: 'set_grants', id, skills: a.skills.filter((x) => x !== s) })}
            onAdd={(s) => p.add({ op: 'set_grants', id, skills: [...a.skills, s] })}
            choices={p.options.skills.filter((s) => !a.skills.includes(s) && !a.deliverables.includes(s)).map((s) => ({ value: s, label: s }))}
            addLabel="Grant a skill…"
            empty={p.options.skills.length ? 'None' : 'None; the vault has no skill yet.'}
          />
        </Field>
      </Section>

      <Mounts mounts={p.mounts?.[id]} />

      <Section title="Container">
        <ContainerSummary a={a} />
        <Button size="sm" variant="outline" onClick={() => p.editYAML(a.line)}>
          Edit in YAML
        </Button>
      </Section>

      <RemoveButton
        label={orchestration ? 'Remove the agent, and leave the project without an orchestration agent' : 'Remove the agent'}
        onRemove={async () => {
          if (!(await p.add({ op: 'remove_agent', id }))) p.select({ kind: 'project' })
        }}
      />
    </div>
  )
}

// BuiltinSkills lists the pack of hikari's own skills that the agent's role
// gets (rule K14 of the skill packs spec). You cannot change it.
function BuiltinSkills({ skills }: { skills: string[] }) {
  if (skills.length === 0) return null
  return (
    <Field label="Skills from Hikari" hint="Hikari installs these in its container for the tasks of its role.">
      <div className="flex flex-wrap gap-1.5" data-testid="builtin-skills">
        {skills.map((s) => (
          <Badge key={s} variant="secondary" className="gap-1 font-normal">
            {s}
            <span className="text-muted-foreground">· from Hikari</span>
          </Badge>
        ))}
      </div>
    </Field>
  )
}

// Mounts shows what the agent's container mounts, from the function that
// writes the override file (rule V14).
function Mounts({ mounts }: { mounts: Mount[] | undefined }) {
  return (
    <Section title="Mounts">
      {mounts ? (
        <table className="w-full text-sm" data-testid="mounts">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
              <th className="py-1 pr-3 font-normal">In the container</th>
              <th className="py-1 pr-3 font-normal">On the host</th>
              <th className="py-1 font-normal">Mode</th>
            </tr>
          </thead>
          <tbody>
            {mounts.map((m) => (
              <tr key={m.container} className="border-t align-top">
                <td className="py-1 pr-3">
                  <code className="text-xs break-all">{m.container}</code>
                </td>
                <td className="py-1 pr-3">{m.host ? <code className="text-xs break-all">{m.host}</code> : <span className="text-xs text-muted-foreground">an empty directory, which hides it</span>}</td>
                <td className="py-1 text-xs whitespace-nowrap">{m.mode}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="text-sm text-muted-foreground">The topology has errors, so hikari cannot say what this container mounts. Fix the errors to see its mounts.</p>
      )}
    </Section>
  )
}

function ContainerSummary({ a }: { a: TopologyAgent }) {
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
      {a.image && (
        <>
          <dt className="text-muted-foreground">Image</dt>
          <dd>
            <code className="text-xs">{a.image}</code>
          </dd>
        </>
      )}
      {a.build && (
        <>
          <dt className="text-muted-foreground">Build</dt>
          <dd>
            <code className="text-xs">{a.build}</code>
          </dd>
        </>
      )}
      {a.template && (
        <>
          <dt className="text-muted-foreground">Merges</dt>
          <dd>
            <code className="text-xs">*{a.template}</code>
          </dd>
        </>
      )}
      <dt className="text-muted-foreground">Compose keys</dt>
      <dd className="text-xs text-muted-foreground">{a.compose.length ? a.compose.join(', ') : 'none besides x-hikari'}</dd>
    </dl>
  )
}

function repoOptions(m: TopologyModel) {
  return [...(m.project.repo ? [{ value: 'project', label: 'the project repository' }] : []), ...m.repos.map((r) => ({ value: r.id, label: r.name }))]
}

const relativeProblem = (v: string) => (v.startsWith('/') || v.split('/').includes('..') ? 'A relative path.' : null)

function PartitionSection({ a, orchestration, p }: { a: TopologyAgent; orchestration: boolean; p: InspectorProps }) {
  const repos = repoOptions(p.model)
  if (!a.partition) {
    return (
      <Section title="Partition">
        <p className="text-sm text-muted-foreground">
          {orchestration
            ? 'The orchestration agent owns no code: it plans from the feature maps of the partitions.'
            : 'It owns no code yet. Every agent but the orchestration agent has a partition: the folders of the repos that only it changes, which it sees as one workspace.'}
        </p>
        {!orchestration &&
          (repos.length ? (
            <Select
              value=""
              placeholder="Own a partition in…"
              options={repos}
              onChange={(v) => v && p.add({ op: 'set_partition', id: a.id, folders: [{ repo: v, path: '', read_only: false, at: '' }] })}
              className="w-auto text-xs text-muted-foreground"
            />
          ) : (
            <p className="text-sm text-muted-foreground">Add a repo first.</p>
          ))}
      </Section>
    )
  }
  const s = a.partition
  return (
    <Section title="Partition">
      {s.old && <p className="text-xs text-muted-foreground">The file writes this partition with repo, scope, and read. A change here writes it as folders.</p>}
      <FolderList a={a} p={p} />
      <TextField label="Test command" value={s.test} mono placeholder="none" onCommit={(v) => p.add({ op: 'set_partition', id: a.id, test: v })} hint="Runs in /workspace. Seeds its first suite when it has no tests.yaml." />
      <Button size="sm" variant="ghost" onClick={() => p.add({ op: 'clear_partition', id: a.id })}>
        Clear the partition
      </Button>
    </Section>
  )
}

// FolderList edits the folders of a partition (rule F14): a row for each
// folder with its repo, path, place, and mode, and buttons to add, remove,
// and reorder rows. The first writable folder holds the private files.
function FolderList({ a, p }: { a: TopologyAgent; p: InspectorProps }) {
  const folders = a.partition!.folders
  const repos = repoOptions(p.model)
  const set = (next: TopologyFolder[]) => p.add({ op: 'set_partition', id: a.id, folders: next })
  const change = (i: number, patch: Partial<TopologyFolder>) => set(folders.map((f, j) => (j === i ? { ...f, ...patch } : f)))
  const move = (i: number, by: number) => {
    const next = [...folders]
    ;[next[i], next[i + by]] = [next[i + by], next[i]]
    set(next)
  }
  const home = folders.findIndex((f) => !f.read_only)
  return (
    <div className="space-y-2" data-testid="folders">
      <span className="text-xs font-medium text-muted-foreground">Folders</span>
      {folders.map((f, i) => (
        <div key={`${i}:${f.repo}:${f.path}`} className="space-y-2 rounded-md border p-2" data-testid="folder-row">
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate text-xs">/workspace/{placeOf(f, p.model)}</code>
            {i === home && (
              <Badge variant="secondary" className="font-normal" title="Its .hikari/private holds the code map and the test map.">
                private files
              </Badge>
            )}
            <Button size="icon-xs" variant="ghost" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}>
              <ArrowUp />
            </Button>
            <Button size="icon-xs" variant="ghost" aria-label="Move down" disabled={i === folders.length - 1} onClick={() => move(i, 1)}>
              <ArrowDown />
            </Button>
            <Button size="icon-xs" variant="ghost" aria-label="Remove the folder" disabled={folders.length === 1} onClick={() => set(folders.filter((_, j) => j !== i))}>
              <X />
            </Button>
          </div>
          <div className="grid gap-2 sm:grid-cols-3">
            <Field label="Repo">
              <Select value={f.repo} options={repos} onChange={(v) => change(i, { repo: v })} />
            </Field>
            <TextField label="Path" value={f.path} mono placeholder="the whole repo" check={relativeProblem} onCommit={(v) => change(i, { path: v })} />
            <TextField label="At" value={f.at} mono placeholder={placeOf({ ...f, at: '' }, p.model)} check={relativeProblem} onCommit={(v) => change(i, { at: v })} />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={f.read_only} onChange={(e) => change(i, { read_only: e.target.checked })} />
            Read-only
          </label>
        </div>
      ))}
      {repos.length > 0 && (
        <Select
          value=""
          placeholder="Add a folder in…"
          options={repos}
          onChange={(v) => v && set([...folders, { repo: v, path: '', read_only: true, at: '' }])}
          className="w-auto text-xs text-muted-foreground"
        />
      )}
    </div>
  )
}

// SidecarInspector edits a sidecar (rule SI5).
export function SidecarInspector({ id, ...p }: InspectorProps & { id: string }) {
  const s = p.model.sidecars.find((x) => x.id === id)!
  return (
    <div className="space-y-6">
      <Header icon={<Package className="size-4 text-muted-foreground" />} name={s.id} />
      <Problems findings={findingsOf(p.findings, { kind: 'sidecar', id })} />
      <Field label="Serves" hint="It joins only the network of the agent it serves.">
        <Select value={s.serves} options={p.model.agents.map((a) => ({ value: a.id, label: a.name }))} onChange={(v) => p.add({ op: 'set_serves', id, agent: v })} />
      </Field>
      <Section title="Container">
        <p className="text-sm">
          <code className="text-xs">{s.image || 'built from the topology file'}</code>
        </p>
        <Button size="sm" variant="outline" onClick={() => p.editYAML(s.line)}>
          Edit in YAML
        </Button>
      </Section>
      <RemoveButton
        label="Remove the sidecar"
        onRemove={async () => {
          if (!(await p.add({ op: 'remove_sidecar', id }))) p.select({ kind: 'project' })
        }}
      />
    </div>
  )
}

// AddForm makes a repo, an agent, or a sidecar (rule SI7).
export function AddForm({ what, ...p }: InspectorProps & { what: AddKind }) {
  const [id, setID] = useState('')
  const [a, setA] = useState('')
  const [b, setB] = useState('')
  const [urlKind, setUrlKind] = useState<'url' | 'path'>(p.model.project.repo ? 'url' : 'path')
  const [refusal, setRefusal] = useState<string | null>(null)
  const kind = what === 'repo' ? 'repo' : what
  const idProblem = id ? nameProblem(id, kind, null, p.model) : null
  const title = { repo: 'Add a repo', agent: 'Add an agent', sidecar: 'Add a sidecar' }[what]
  const ready = id && !idProblem && (what !== 'repo' || (a && (urlKind === 'path' || !urlProblem(a)))) && (what !== 'sidecar' || (a && b))

  const submit = async () => {
    let op: Op
    if (what === 'repo') op = { op: 'add_repo', id, [urlKind]: a, ...(b ? { branch: b } : {}) }
    else if (what === 'agent') op = { op: 'add_agent', id, template: a, description: b }
    else op = { op: 'add_sidecar', id, serves: a, image: b }
    const err = await p.add(op)
    setRefusal(err)
    if (!err) p.select({ kind, id })
  }

  const agents = p.model.agents.map((x) => ({ value: x.id, label: x.name }))
  return (
    <div className="space-y-5" data-testid="add-form">
      <h2 className="text-base font-semibold">{title}</h2>
      <Field label="Id" problem={idProblem} hint="The key in the file, and the name until you rename it.">
        <input
          autoFocus
          value={id}
          onChange={(e) => setID(e.target.value)}
          className="h-8 w-full rounded-md border border-input px-3 font-mono text-xs shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
        />
      </Field>
      {what === 'repo' && (
        <>
          <div className="flex gap-4 text-sm">
            {(['url', 'path'] as const).map((k) => (
              <label key={k} className="flex items-center gap-1.5">
                <input type="radio" checked={urlKind === k} disabled={k === 'url' && !p.model.project.repo} onChange={() => setUrlKind(k)} />
                {k === 'url' ? 'A remote url' : 'A path on this machine'}
              </label>
            ))}
          </div>
          {!p.model.project.repo && <p className="text-xs text-muted-foreground">A url needs the project repository; turn it on in the project.</p>}
          <Field label={urlKind === 'url' ? 'Url' : 'Path'} problem={urlKind === 'url' && a ? urlProblem(a) : null}>
            <input value={a} onChange={(e) => setA(e.target.value)} className="h-8 w-full rounded-md border border-input px-3 font-mono text-xs shadow-xs outline-none" />
          </Field>
          <Field label="Branch" problem={b ? branchProblem(b) : null} hint={urlKind === 'url' ? 'Empty uses the project branch.' : 'The branch agents commit to.'}>
            <input value={b} onChange={(e) => setB(e.target.value)} className="h-8 w-full rounded-md border border-input px-3 font-mono text-xs shadow-xs outline-none" />
          </Field>
        </>
      )}
      {what === 'agent' && (
        <>
          <Field label="Container" hint="The new agent copies the compose keys of this agent: its image or build, and its credentials.">
            <Select value={a} placeholder="hikari-agent, with the credentials" options={agents} onChange={setA} />
          </Field>
          <Field label="Description">
            <input value={b} onChange={(e) => setB(e.target.value)} className="h-8 w-full rounded-md border border-input px-3 text-sm shadow-xs outline-none" />
          </Field>
          <p className="text-xs text-muted-foreground">It joins the room. Give it a partition next, or make it the orchestration agent in the project.</p>
        </>
      )}
      {what === 'sidecar' && (
        <>
          <Field label="Serves">
            <Select value={a} placeholder="Choose an agent…" options={agents} onChange={setA} />
          </Field>
          <Field label="Image">
            <input value={b} onChange={(e) => setB(e.target.value)} placeholder="postgres:16" className="h-8 w-full rounded-md border border-input px-3 font-mono text-xs shadow-xs outline-none" />
          </Field>
        </>
      )}
      {refusal && <p className="text-sm text-[var(--lens-fail)]">{refusal}</p>}
      <div className="flex gap-2">
        <Button size="sm" disabled={!ready} onClick={submit}>
          {title}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => p.select({ kind: 'project' })}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
