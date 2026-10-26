import { useCallback, useEffect, useState } from 'react'
import { Plus } from 'lucide-react'
import { SeverityDot } from '@/components/findings-list'
import { PlanReview } from '@/components/plan-review'
import { AddForm, AgentInspector, HubInspector, type InspectorProps, type Options, ProjectInspector, RepoInspector, SidecarInspector } from '@/components/structure-inspectors'
import { Button } from '@/components/ui/button'
import { exists, findingsOf, parseSelection, roles, selectionKey, worst, type AddKind, type Op, type Selection } from '@/lib/structure'
import { applyOps, getOptions, planOps } from '@/lib/topology'
import type { Finding, Mode, Plan, TopologyModel, TopologyState } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { View } from '@/lib/view'

type Props = {
  data: TopologyState
  mode: Mode
  reload: () => void
  show: (v: View) => void
  select?: string
  onPending: (n: number) => void
}

// StructureTab is the form of the topology (rules SL1 to SR4): a list of
// entities, an inspector of the selected one, and pending changes that
// the server plans as you make them.
export function StructureTab({ data, mode, reload, show, select: selectKey, onPending }: Props) {
  const [ops, setOps] = useState<Op[]>([])
  const [plan, setPlan] = useState<Plan | null>(null)
  const [options, setOptions] = useState<Options>({ deliverables: [], models: ['sonnet', 'opus', 'haiku'], templates: [] })
  const [review, setReview] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [planned, setPlanned] = useState(data.revision)

  useEffect(() => {
    getOptions().then((res) => res.ok && setOptions(res.data))
  }, [data.revision])

  const pending = (next: Op[], p: Plan | null) => {
    setOps(next)
    setPlan(p)
    onPending(next.length)
  }

  // The file changed on disk under pending changes: plan them again on it.
  useEffect(() => {
    if (data.revision === planned) return
    setPlanned(data.revision)
    if (ops.length === 0) return
    planOps(data.revision, ops).then((res) => {
      if (res.ok) {
        setPlan(res.data)
        setConflict(false)
      } else {
        pending([], null)
        setNotice(`The file changed on disk, and your changes no longer apply: ${res.error}`)
      }
    })
    // Only a new revision starts this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data.revision])

  const model: TopologyModel = (plan?.model ?? data.model)!
  const findings: Finding[] = plan?.findings ?? data.findings
  let selection = parseSelection(selectKey) ?? { kind: 'project' }
  if (!exists(selection, model)) selection = { kind: 'project' }
  const select = useCallback((s: Selection) => show({ kind: 'topology', select: selectionKey(s) }), [show])

  // add plans the pending changes with one more operation; a refusal
  // leaves them as they were (rules SP1 and SP2).
  const add = async (op: Op): Promise<string | null> => {
    const next = [...ops, op]
    const res = await planOps(data.revision, next)
    if (!res.ok) return res.error
    pending(next, res.data)
    setNotice(null)
    return null
  }

  const save = async (propagate: string[]) => {
    const res = await applyOps(data.revision, ops, propagate)
    if (res.ok) {
      pending([], null)
      setReview(false)
      const parts = [res.data.commit ? `Saved and committed ${res.data.commit.slice(0, 7)}.` : `Saved. ${res.data.note ?? ''}`]
      if (res.data.skipped.length) parts.push(`${res.data.skipped.length} place(s) changed since the plan and stayed as they were.`)
      setNotice(parts.join(' '))
      reload()
    } else if (res.conflict) setConflict(true)
    else setNotice(res.error)
  }

  const planAgain = () => {
    setConflict(false)
    reload()
  }

  // The YAML tab edits the saved file, so pending changes are saved or
  // discarded first: Edit in YAML opens their review.
  const editYAML = (line: number) => {
    if (ops.length) setReview(true)
    else show({ kind: 'topology', tab: 'yaml', line })
  }
  const inspector: InspectorProps = { model, findings, options, add, select, editYAML }
  const key = selectionKey(selection)

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="structure-tab">
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <EntityList model={model} findings={findings} selected={key} select={select} />
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 md:px-8" data-testid="inspector">
          <div className="mx-auto max-w-2xl" key={key}>
            {selection.kind === 'project' && <ProjectInspector {...inspector} />}
            {selection.kind === 'repo' && <RepoInspector {...inspector} id={selection.id} />}
            {selection.kind === 'hub' && <HubInspector {...inspector} id={selection.id} />}
            {selection.kind === 'agent' && <AgentInspector {...inspector} id={selection.id} />}
            {selection.kind === 'sidecar' && <SidecarInspector {...inspector} id={selection.id} />}
            {selection.kind === 'add' && <AddForm {...inspector} what={selection.what} />}
          </div>
        </div>
      </div>
      {(ops.length > 0 || notice) && (
        <div className="flex shrink-0 items-center gap-3 border-t bg-neutral-50 px-4 py-2 md:px-6" data-testid="pending-bar">
          <span className="flex-1 text-sm text-muted-foreground">{ops.length > 0 ? (ops.length === 1 ? '1 change' : `${ops.length} changes`) : notice}</span>
          {ops.length > 0 && (
            <>
              <Button size="sm" variant="ghost" onClick={() => pending([], null)}>
                Discard
              </Button>
              <Button size="sm" onClick={() => setReview(true)} data-testid="review-changes">
                Review and save
              </Button>
            </>
          )}
        </div>
      )}
      {plan && (
        <PlanReview
          key={plan.text}
          plan={{ ...plan, applies_at_start: mode === 'running' }}
          open={review}
          onOpenChange={setReview}
          onSave={save}
          conflict={conflict}
          onPlanAgain={planAgain}
        />
      )}
    </div>
  )
}

function Group({ title, add, onAdd, children }: { title: string; add?: AddKind; onAdd?: (k: AddKind) => void; children: React.ReactNode }) {
  return (
    <section className="py-2">
      <div className="flex items-center justify-between px-3 pb-1">
        <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>
        {add && onAdd && (
          <button type="button" aria-label={`Add a ${add}`} title={`Add a ${add}`} className="rounded p-0.5 text-muted-foreground hover:bg-neutral-100 hover:text-foreground" onClick={() => onAdd(add)}>
            <Plus className="size-3.5" />
          </button>
        )}
      </div>
      <ul>{children}</ul>
    </section>
  )
}

function Row({ active, title, id, sub, severity, onClick }: { active: boolean; title: string; id?: string; sub?: string; severity: Finding['severity'] | null; onClick: () => void }) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        aria-current={active}
        className={cn('flex w-full items-start gap-2 px-3 py-1.5 text-left', active ? 'bg-neutral-100' : 'hover:bg-neutral-50')}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm">
            {title}
            {id && id !== title.replace(/^#/, '') && <span className="ml-1.5 text-xs text-muted-foreground">{id}</span>}
          </span>
          {sub && <span className="block truncate text-xs text-muted-foreground">{sub}</span>}
        </span>
        {severity && (
          <span className="pt-1.5">
            <SeverityDot severity={severity} />
          </span>
        )}
      </button>
    </li>
  )
}

// EntityList is the list of the tab (rules SL1 to SL3).
export function EntityList({ model, findings, selected, select }: { model: TopologyModel; findings: Finding[]; selected: string; select: (s: Selection) => void }) {
  const sev = (s: Selection) => worst(findingsOf(findings, s))
  const onAdd = (what: AddKind) => select({ kind: 'add', what })
  return (
    <nav className="max-h-56 shrink-0 overflow-y-auto border-b md:max-h-none md:w-72 md:border-r md:border-b-0" data-testid="entity-list">
      <Group title="Project">
        <Row active={selected === 'project'} title={model.project.name} id={model.project.id} sub={model.project.repo ? 'a project repository' : undefined} severity={sev({ kind: 'project' })} onClick={() => select({ kind: 'project' })} />
      </Group>
      <Group title="Repos" add="repo" onAdd={onAdd}>
        {model.repos.map((r) => (
          <Row key={r.id} active={selected === `repo:${r.id}`} title={r.name} id={r.id} sub={r.url || r.path} severity={sev({ kind: 'repo', id: r.id })} onClick={() => select({ kind: 'repo', id: r.id })} />
        ))}
      </Group>
      <Group title="Hubs" add="hub" onAdd={onAdd}>
        {model.hubs.map((h) => (
          <Row
            key={h.id}
            active={selected === `hub:${h.id}`}
            title={`#${h.name}`}
            id={h.id}
            sub={h.members.length === 1 ? '1 member' : `${h.members.length} members`}
            severity={sev({ kind: 'hub', id: h.id })}
            onClick={() => select({ kind: 'hub', id: h.id })}
          />
        ))}
      </Group>
      <Group title="Agents" add="agent" onAdd={onAdd}>
        {model.agents.map((a) => (
          <Row key={a.id} active={selected === `agent:${a.id}`} title={a.name} id={a.id} sub={roles(a, model)} severity={sev({ kind: 'agent', id: a.id })} onClick={() => select({ kind: 'agent', id: a.id })} />
        ))}
      </Group>
      <Group title="Sidecars" add="sidecar" onAdd={onAdd}>
        {model.sidecars.map((s) => (
          <Row
            key={s.id}
            active={selected === `sidecar:${s.id}`}
            title={s.id}
            sub={`serves ${model.agents.find((a) => a.id === s.serves)?.name ?? s.serves}`}
            severity={sev({ kind: 'sidecar', id: s.id })}
            onClick={() => select({ kind: 'sidecar', id: s.id })}
          />
        ))}
      </Group>
    </nav>
  )
}
