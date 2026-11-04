import { useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ExternalLink, Pause, Pencil, Play, Plus, RefreshCw, TriangleAlert, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DecisionCard } from '@/components/decision-card'
import { Markdown } from '@/components/markdown'
import { pendingDecisions } from '@/lib/attention'
import { ago, issueRef } from '@/lib/repos'
import { duration, money, partCosts, timeline, totalCost, totalTurns } from '@/lib/scorecard'
import type { State } from '@/lib/store'
import type { IssueDetail, IssueView, QueueView, TaskStatus, TaskView, TasksView } from '@/lib/types'
import type { View } from '@/lib/view'

// The colors of the statuses (rule TK27 of the task pipeline spec).
const statusClass: Record<TaskStatus, string> = {
  queued: 'bg-neutral-100 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300',
  waiting: 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300',
  started: 'bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-300',
  blocked: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300',
  testing: 'bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-300',
  validating: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-950 dark:text-cyan-300',
  done: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300',
}

const statuses: TaskStatus[] = ['queued', 'waiting', 'started', 'blocked', 'testing', 'validating', 'done']

export function StatusChip({ status, label }: { status?: TaskStatus; label?: string }) {
  if (!status) return <span className="rounded px-1.5 py-0.5 text-xs text-muted-foreground ring-1 ring-border">not queued</span>
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs ${statusClass[status]}`} data-status={status}>
      {label ? `${label} ` : ''}
      {status}
    </span>
  )
}

// TaskChips are a task's status, with "scoping" while the orchestration
// agent has not written the scope yet (rule IS3), and its parts.
function TaskChips({ task: t }: { task: TaskView }) {
  return (
    <>
      <StatusChip status={t.status} label={t.stage !== 'scope' && t.status ? 'scoping' : undefined} />
      {t.parts.map((p) => (
        <StatusChip key={p.partition} status={p.status} label={p.partition} />
      ))}
      {t.parts.length === 0 &&
        t.partitions.map((p) => (
          <Badge key={p} variant="outline" className="font-normal">
            {p}
          </Badge>
        ))}
    </>
  )
}

// call sends one change of the pipeline, and returns an error message, or
// null when the server took it.
async function call(path: string, body: unknown, method = 'POST'): Promise<string | null> {
  const res = await fetch(`/api/ui/tasks${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const e = (await res.json().catch(() => ({}))) as { error?: string }
  return e.error ?? res.statusText
}

const issuePath = (ref: string) => `/issues/${encodeURIComponent(ref)}`

// movable says whether a task may move or leave its queue (rule TK11).
const movable = (t: TaskView) => !t.status || t.status === 'queued' || t.status === 'waiting'

// planMessage is what Plan sends the orchestration agent (rule UI5).
const planMessage = (ref: string) =>
  `Please plan the issue ${ref}: read it with get_issue and the feature maps, and comment on it with the partitions, the features, and the open questions, using comment_issue. Do not start work.`

type Filter = 'all' | 'open' | 'closed'

// WorkView is the issue board (rule UI4): the issues of every tracker and
// the local ones, the queues, and an issue's panel.
export function WorkView({
  state,
  show,
  issue: openRef,
  send,
}: {
  state: State
  show: (v: View) => void
  issue?: string
  send: (req: { to?: string; body: string }) => Promise<string | null>
}) {
  const work = state.modules.tasks as TasksView
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<Filter>('open')
  const [text, setText] = useState('')
  const [repoFilter, setRepoFilter] = useState('')
  const [queueName, setQueueName] = useState('')
  const [syncing, setSyncing] = useState(false)
  const issues = useMemo(() => work.issues ?? [], [work.issues])
  const byRef = useMemo(() => new Map(issues.map((i) => [i.ref, i])), [issues])
  const byTask = useMemo(() => new Map(issues.filter((i) => i.task).map((i) => [i.task!.id, i])), [issues])
  const open = openRef ? byRef.get(openRef) : undefined
  const shown = issues.filter(
    (i) =>
      (filter === 'all' || i.state === filter) &&
      (!repoFilter || (repoFilter === 'local' ? i.local : i.repo === repoFilter)) &&
      (!text.trim() || `${i.ref} ${i.title} ${i.labels.join(' ')}`.toLowerCase().includes(text.trim().toLowerCase())),
  )

  const run = async (p: Promise<string | null>) => setError(await p)
  const workOn = (ref: string) => run(call('/work', { issue: ref }))
  const plan = (ref: string) =>
    run(work.orchestration ? send({ to: work.orchestration, body: planMessage(ref) }) : Promise.resolve('The project has no orchestration agent to plan it.'))
  const queueOp = (op: Record<string, unknown>) => run(call('/queue', op))
  const sync = async () => {
    setSyncing(true)
    await run(call('/sync', {}))
    setSyncing(false)
  }
  // drop puts a dragged issue into a queue at a place (decision 12).
  const drop = (queue: string, ref: string, position?: number) => {
    const t = byRef.get(ref)?.task
    if (t && t.queue && !movable(t)) return
    queueOp({ op: t?.queue ? 'move' : 'add', queue, issue: ref, position })
  }

  return (
    <div className="flex min-h-0 flex-1" data-testid="work-view">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 md:px-6">
        {error && (
          <p className="mb-3 flex items-start gap-2 text-sm text-red-600" role="alert">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" />
            {error}
          </p>
        )}
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
          <section className="min-w-0 space-y-3" data-testid="issues">
            <NewIssue work={work} onError={setError} />
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <div role="radiogroup" aria-label="Filter" className="flex rounded-md border p-0.5">
                {(['open', 'closed', 'all'] as Filter[]).map((f) => (
                  <button key={f} type="button" role="radio" aria-checked={filter === f} onClick={() => setFilter(f)} className={`rounded px-2 py-0.5 ${filter === f ? 'bg-neutral-900 text-white' : 'text-muted-foreground'}`}>
                    {f}
                  </button>
                ))}
              </div>
              <Input value={text} onChange={(e) => setText(e.target.value)} placeholder="Filter by title or label" aria-label="Filter issues" className="h-7 w-44 text-xs" />
              {work.trackers.length > 1 && (
                <select aria-label="Repository" className="h-7 rounded-md border bg-background px-1.5 text-xs" value={repoFilter} onChange={(e) => setRepoFilter(e.target.value)}>
                  <option value="">every repository</option>
                  {work.trackers.map((tr) => (
                    <option key={tr.repo} value={tr.repo}>
                      {tr.repo}
                    </option>
                  ))}
                  <option value="local">local</option>
                </select>
              )}
            </div>
            {work.trackers.map((tr) => (
              <p key={tr.repo} className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground" data-tracker={tr.repo}>
                {tr.url ? (
                  <a href={tr.url} target="_blank" rel="noreferrer" className="underline underline-offset-2">
                    {tr.repo}
                  </a>
                ) : (
                  tr.repo
                )}
                · {tr.kind}
                {tr.synced && <> · synced {ago(tr.synced)}</>}
                <Button size="icon-sm" variant="ghost" aria-label="Sync" onClick={sync} disabled={syncing}>
                  <RefreshCw className={`size-3.5 ${syncing ? 'animate-spin' : ''}`} />
                </Button>
                {tr.error && <span className="text-red-600">{tr.error}</span>}
              </p>
            ))}
            {work.trackers.length === 0 && <p className="text-xs text-muted-foreground">No repository of the project is on a forge that hikari knows, so the issues live in vault/tasks/.</p>}
            <h2 className="text-sm font-semibold">
              Issues ({shown.length}
              {shown.length !== issues.length ? ` of ${issues.length}` : ''})
            </h2>
            {shown.length === 0 && <p className="text-sm text-muted-foreground">No issues here. Open one above, or on the tracker.</p>}
            <div className="space-y-2">
              {shown.map((i) => (
                <IssueCard key={i.ref} issue={i} active={i.ref === openRef} queues={work.queues} turns={state.turns} onOpen={() => show({ kind: 'work', issue: i.ref })} onWork={() => workOn(i.ref)} onPlan={() => plan(i.ref)} onQueue={(q) => queueOp({ op: 'add', queue: q, issue: i.ref })} />
              ))}
            </div>
          </section>

          <section className="min-w-0 space-y-2" data-testid="queues">
            <div className="flex items-center justify-between gap-2">
              <h2 className="text-sm font-semibold">Queues ({work.queues.length})</h2>
              <form
                className="flex gap-2"
                onSubmit={async (e) => {
                  e.preventDefault()
                  if (!queueName.trim()) return
                  const err = await call('/queue', { op: 'create', name: queueName.trim() })
                  setError(err)
                  if (!err) setQueueName('')
                }}
              >
                <Input value={queueName} onChange={(e) => setQueueName(e.target.value)} placeholder="Queue name" aria-label="New queue" className="h-8 w-40" />
                <Button size="sm" type="submit" disabled={!queueName.trim()}>
                  <Plus className="size-3.5" />
                  New queue
                </Button>
              </form>
            </div>
            {work.queues.length === 0 && <p className="text-sm text-muted-foreground">No queues. Work on this makes one and starts it; a running queue hands its issues to the agents in order.</p>}
            <div className="flex gap-4 overflow-x-auto pb-2">
              {work.queues.map((q) => (
                <QueueColumn key={q.id} queue={q} queues={work.queues} byTask={byTask} open={openRef} show={show} run={run} queueOp={queueOp} drop={drop} />
              ))}
            </div>
            {work.problems.length > 0 && (
              <div className="space-y-1">
                <h2 className="text-sm font-semibold">Problems</h2>
                {work.problems.map((p) => (
                  <p key={p.file} className="text-sm text-red-600">
                    <code className="text-xs">{p.file}</code>: {p.reason}
                  </p>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
      {open && <IssuePanel key={open.ref} issue={open} state={state} close={() => show({ kind: 'work' })} run={run} onWork={() => workOn(open.ref)} onPlan={() => plan(open.ref)} />}
    </div>
  )
}

// NewIssue opens an issue on a tracker, or in the vault (rule UI4).
function NewIssue({ work, onError }: { work: TasksView; onError: (e: string | null) => void }) {
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [where, setWhere] = useState(work.trackers[0]?.repo ?? 'local')
  const submit = async () => {
    if (!title.trim()) return
    const req = where === 'local' ? { title: title.trim(), body, local: true } : { title: title.trim(), body, repo: where }
    const err = await call('/issues', req)
    onError(err)
    if (!err) {
      setTitle('')
      setBody('')
    }
  }
  return (
    <form
      className="space-y-2"
      data-testid="new-issue"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <div className="flex gap-2">
        <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="A new issue: its title" aria-label="New issue" />
        {work.trackers.length > 0 && (
          <select aria-label="Where" className="h-9 rounded-md border bg-background px-1.5 text-xs" value={where} onChange={(e) => setWhere(e.target.value)}>
            {work.trackers.map((tr) => (
              <option key={tr.repo} value={tr.repo}>
                {tr.repo}
              </option>
            ))}
            <option value="local">local</option>
          </select>
        )}
        <Button type="submit" disabled={!title.trim()}>
          Open issue
        </Button>
      </div>
      <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="What and why, in a few sentences. Optional." rows={2} aria-label="New issue body" />
    </form>
  )
}

function Labels({ labels }: { labels: string[] }) {
  return (
    <>
      {labels.map((l) => (
        <span key={l} className={`rounded px-1.5 py-0.5 text-xs ${l.startsWith('hikari:') ? 'bg-sky-50 text-sky-800 ring-1 ring-sky-200' : 'bg-neutral-100 text-neutral-700'}`} data-label={l}>
          {l}
        </span>
      ))}
    </>
  )
}

// CardCost is the cost of a scope so far, on its card.
function CardCost({ task: t, turns }: { task: TaskView; turns: State['turns'] }) {
  if (!t.scorecard?.started) return null
  const cost = totalCost(partCosts(t.scorecard, turns))
  if (cost === 0) return null
  return <span data-testid="card-cost">· {money(cost)}</span>
}

function IssueCard({
  issue: i,
  active,
  queues,
  turns,
  onOpen,
  onWork,
  onPlan,
  onQueue,
}: {
  issue: IssueView
  active: boolean
  queues: QueueView[]
  turns: State['turns']
  onOpen: () => void
  onWork: () => void
  onPlan: () => void
  onQueue: (q: string) => void
}) {
  const t = i.task
  return (
    <div
      className={`space-y-1.5 rounded-md border p-2.5 ${active ? 'border-primary' : ''} ${i.state === 'closed' ? 'opacity-70' : ''}`}
      data-issue={i.ref}
      draggable={i.state === 'open'}
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', i.ref)
        e.dataTransfer.effectAllowed = 'move'
      }}
    >
      <div className="flex items-start justify-between gap-2">
        <button type="button" className="min-w-0 text-left text-sm font-medium hover:underline" onClick={onOpen}>
          {i.title}
        </button>
        <span className={`shrink-0 rounded px-1.5 py-0.5 text-xs ${i.state === 'open' ? 'bg-emerald-100 text-emerald-800' : 'bg-neutral-100 text-neutral-600'}`}>{i.state}</span>
      </div>
      <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
        <code>{i.local ? 'local' : i.ref}</code>
        {i.author && <span>· by {i.author}</span>}
        {i.comments > 0 && <span>· {i.comments} comment{i.comments === 1 ? '' : 's'}</span>}
        <Labels labels={i.labels} />
      </div>
      {t && (
        <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
          <TaskChips task={t} />
          {t.queue && <span>in {t.queue}</span>}
          <CardCost task={t} turns={turns} />
        </div>
      )}
      {i.state === 'open' && (
        <div className="flex flex-wrap gap-1.5">
          <Button size="sm" onClick={onWork} disabled={!!t?.status && t.status !== 'done'}>
            <Play className="size-3.5" />
            Work on this
          </Button>
          <Button size="sm" variant="ghost" onClick={onPlan}>
            Plan
          </Button>
          {!t?.queue && queues.length > 0 && <QueueMenu queues={queues} label="Add to queue" onPick={onQueue} />}
        </div>
      )}
    </div>
  )
}

function QueueMenu({ queues, label, exclude, onPick }: { queues: QueueView[]; label: string; exclude?: string; onPick: (q: string) => void }) {
  const choices = queues.filter((q) => q.id !== exclude)
  if (choices.length === 0) return null
  return (
    <select
      aria-label={label}
      className="h-7 rounded-md border bg-background px-1.5 text-xs"
      value=""
      onChange={(e) => {
        if (e.target.value) onPick(e.target.value)
      }}
    >
      <option value="">{label}…</option>
      {choices.map((q) => (
        <option key={q.id} value={q.id}>
          {q.name}
        </option>
      ))}
    </select>
  )
}

// QueueColumn is one queue, a drop target for issues (decision 12).
function QueueColumn({
  queue: q,
  queues,
  byTask,
  open,
  show,
  run,
  queueOp,
  drop,
}: {
  queue: QueueView
  queues: QueueView[]
  byTask: Map<string, IssueView>
  open?: string
  show: (v: View) => void
  run: (p: Promise<string | null>) => void
  queueOp: (op: Record<string, unknown>) => void
  drop: (queue: string, ref: string, position?: number) => void
}) {
  const running = q.state === 'running'
  const [over, setOver] = useState(false)
  const dragged = (e: React.DragEvent) => e.dataTransfer.getData('text/plain')
  return (
    <div
      className={`w-72 shrink-0 space-y-2 rounded-lg p-2.5 ${over ? 'bg-sky-50 ring-2 ring-sky-300' : 'bg-muted/40'}`}
      data-queue={q.id}
      onDragOver={(e) => {
        e.preventDefault()
        setOver(true)
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setOver(false)
        const ref = dragged(e)
        if (ref) drop(q.id, ref)
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{q.name}</div>
          <div className="text-xs text-muted-foreground">
            <code>{q.id}</code> · {q.state}
          </div>
        </div>
        <Button size="sm" variant={running ? 'secondary' : 'default'} onClick={() => run(call('/state', { queue: q.id, state: running ? 'paused' : 'running' }))}>
          {running ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
          {running ? 'Pause' : 'Start'}
        </Button>
      </div>
      {q.scopes.length === 0 && <p className="text-xs text-muted-foreground">Empty. Drag an issue here.</p>}
      {q.scopes.map((t, n) => {
        const canMove = movable(t)
        const i = byTask.get(t.id)
        const ref = issueRef(t)
        return (
          <div
            key={t.id}
            className={`space-y-1.5 rounded-md border bg-white p-2.5 ${ref === open ? 'border-primary' : ''}`}
            data-task={t.id}
            draggable={canMove}
            onDragStart={(e) => {
              e.dataTransfer.setData('text/plain', ref)
              e.stopPropagation()
            }}
            onDrop={(e) => {
              const dropped = dragged(e)
              if (!dropped || dropped === ref) return
              e.preventDefault()
              e.stopPropagation()
              setOver(false)
              drop(q.id, dropped, n + 1)
            }}
          >
            <div className="flex items-start justify-between gap-2">
              <button type="button" className="min-w-0 text-left text-sm font-medium hover:underline" onClick={() => show({ kind: 'work', issue: ref })}>
                {i?.title ?? t.name}
              </button>
            </div>
            <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
              <code>{t.issue ?? t.id}</code>
              <TaskChips task={t} />
            </div>
            {t.wait && <p className="text-xs text-amber-700 dark:text-amber-400">Waits: {t.wait}</p>}
            {t.block && <p className="text-xs text-red-600">Blocked: {t.block}</p>}
            {canMove && (
              <div className="flex flex-wrap gap-1.5">
                <Button size="icon-sm" variant="ghost" aria-label="Move up" disabled={n === 0 || !movable(q.scopes[n - 1])} onClick={() => queueOp({ op: 'move', queue: q.id, scope: t.id, position: n })}>
                  <ArrowUp className="size-3.5" />
                </Button>
                <Button size="icon-sm" variant="ghost" aria-label="Move down" disabled={n === q.scopes.length - 1} onClick={() => queueOp({ op: 'move', queue: q.id, scope: t.id, position: n + 2 })}>
                  <ArrowDown className="size-3.5" />
                </Button>
                <QueueMenu queues={queues} exclude={q.id} label="Move to queue" onPick={(to) => queueOp({ op: 'move', queue: to, scope: t.id })} />
                <Button size="icon-sm" variant="ghost" aria-label="Take out" onClick={() => queueOp({ op: 'out', scope: t.id })}>
                  <X className="size-3.5" />
                </Button>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

// IssuePanel shows one issue: its body, its comments, its labels, and its
// task's scope and log, with the actions (rule UI5).
function IssuePanel({
  issue: i,
  state,
  close,
  run,
  onWork,
  onPlan,
}: {
  issue: IssueView
  state: State
  close: () => void
  run: (p: Promise<string | null>) => void
  onWork: () => void
  onPlan: () => void
}) {
  const [detail, setDetail] = useState<IssueDetail | null>(null)
  const [editing, setEditing] = useState<'title' | 'body' | null>(null)
  const [draft, setDraft] = useState('')
  const [comment, setComment] = useState('')
  const [tick, setTick] = useState(0)
  const stamp = `${i.updated ?? ''}/${i.task?.hash ?? ''}/${tick}`
  useEffect(() => {
    let live = true
    fetch(`/api/ui/tasks${issuePath(i.ref)}`)
      .then((r) => (r.ok ? r.json() : Promise.reject(r.statusText)))
      .then((d: IssueDetail) => live && setDetail(d))
      .catch(() => live && setDetail({ issue: i, comments: [] }))
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [i.ref, stamp])
  const names = useMemo(() => new Set(state.participants.map((p) => p.name)), [state.participants])
  const t = i.task
  const taskBody = useMemo(() => (detail?.task?.text ?? '').replace(/^---\n[\s\S]*?\n---\n/, ''), [detail])
  const body = detail?.issue.body ?? i.body ?? ''
  const patch = async (p: Record<string, unknown>) => {
    const err = await call(issuePath(i.ref), p, 'PATCH')
    if (!err) {
      setEditing(null)
      setTick((n) => n + 1)
    }
    run(Promise.resolve(err))
  }
  const setStatus = (part: string | undefined, status: string) => t && run(call('/status', { task: t.id, part, status }))
  // The decisions that wait on this task (rule DC5).
  const decisions = useMemo(() => (t ? pendingDecisions(state.messages, state.me, t.id) : []), [state.messages, state.me, t])
  const closeOrRemove = () => {
    if (i.local) {
      if (window.confirm(`Remove the local issue ${i.ref}? Its document in vault/tasks/ is deleted.`)) patch({ state: 'closed' }).then(close)
      return
    }
    patch({ state: i.state === 'open' ? 'closed' : 'open' })
  }

  return (
    <aside className="hidden w-[30rem] shrink-0 overflow-y-auto border-l px-4 py-5 lg:block" data-testid="issue-panel">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          {editing === 'title' ? (
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                if (draft.trim()) patch({ title: draft.trim() })
              }}
            >
              <Input value={draft} onChange={(e) => setDraft(e.target.value)} aria-label="Issue title" autoFocus />
              <Button size="sm" type="submit">
                Save
              </Button>
              <Button size="sm" type="button" variant="ghost" onClick={() => setEditing(null)}>
                Cancel
              </Button>
            </form>
          ) : (
            <h2 className="flex items-center gap-1.5 text-base font-semibold">
              {detail?.issue.title ?? i.title}
              {i.state === 'open' && (
                <button
                  type="button"
                  aria-label="Edit title"
                  className="text-muted-foreground hover:text-foreground"
                  onClick={() => {
                    setDraft(detail?.issue.title ?? i.title)
                    setEditing('title')
                  }}
                >
                  <Pencil className="size-3.5" />
                </button>
              )}
            </h2>
          )}
          <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <code>{i.local ? `local · ${i.ref}` : i.ref}</code>
            <span className={`rounded px-1.5 py-0.5 ${i.state === 'open' ? 'bg-emerald-100 text-emerald-800' : 'bg-neutral-100 text-neutral-600'}`}>{i.state}</span>
            {i.author && <span>by {i.author}</span>}
            {i.url && (
              <a href={i.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 underline underline-offset-2">
                Open on GitHub
                <ExternalLink className="size-3" />
              </a>
            )}
          </p>
          {(detail?.issue.labels ?? i.labels).length > 0 && (
            <p className="mt-1 flex flex-wrap gap-1">
              <Labels labels={detail?.issue.labels ?? i.labels} />
            </p>
          )}
        </div>
        <Button size="icon-sm" variant="ghost" aria-label="Close" onClick={close}>
          <X className="size-4" />
        </Button>
      </div>
      {decisions.length > 0 && (
        <div className="mt-3" data-testid="issue-decisions">
          {decisions.map((m, k) => (
            <DecisionCard key={m.id} m={m} hot={k === decisions.length - 1} />
          ))}
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {i.state === 'open' && (
          <Button size="sm" onClick={onWork} disabled={!!t?.status && t.status !== 'done'}>
            <Play className="size-3.5" />
            Work on this
          </Button>
        )}
        {i.state === 'open' && (
          <Button size="sm" variant="secondary" onClick={onPlan}>
            Plan with the agent
          </Button>
        )}
        <Button size="sm" variant="ghost" onClick={closeOrRemove}>
          {i.local ? 'Remove' : i.state === 'open' ? 'Close' : 'Reopen'}
        </Button>
      </div>

      <section className="mt-4" data-testid="issue-body">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Issue</h3>
          {i.state === 'open' && editing !== 'body' && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setDraft(body)
                setEditing('body')
              }}
            >
              Edit
            </Button>
          )}
        </div>
        {editing === 'body' ? (
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault()
              patch({ body: draft })
            }}
          >
            <Textarea value={draft} onChange={(e) => setDraft(e.target.value)} rows={8} aria-label="Issue body" autoFocus />
            <div className="flex gap-2">
              <Button size="sm" type="submit">
                Save
              </Button>
              <Button size="sm" type="button" variant="ghost" onClick={() => setEditing(null)}>
                Cancel
              </Button>
            </div>
          </form>
        ) : (
          <Markdown text={body.trim() || '_No body._'} names={names} />
        )}
      </section>

      {!i.local && (
        <section className="mt-4 space-y-2" data-testid="issue-comments">
          <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Comments ({detail?.comments.length ?? i.comments})</h3>
          {detail === null && <p className="text-sm text-muted-foreground">Loading…</p>}
          {detail?.comments.map((c) => (
            <div key={c.id} className="rounded-md border p-2 text-sm">
              <p className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">{c.author ?? 'someone'}</span>
                {c.created ? ` · ${ago(c.created)}` : ''}
              </p>
              <Markdown text={c.body} names={names} />
            </div>
          ))}
          <form
            className="space-y-1.5"
            onSubmit={async (e) => {
              e.preventDefault()
              if (!comment.trim()) return
              const err = await call(`${issuePath(i.ref)}/comments`, { body: comment.trim() })
              run(Promise.resolve(err))
              if (!err) {
                setComment('')
                setTick((n) => n + 1)
              }
            }}
          >
            <Textarea value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Comment on the tracker, as you." rows={2} aria-label="New comment" />
            <Button size="sm" type="submit" variant="secondary" disabled={!comment.trim()}>
              Comment
            </Button>
          </form>
        </section>
      )}

      {t && (
        <section className="mt-4 space-y-2 text-sm" data-testid="issue-task">
          <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            The agents' work · <code className="normal-case">{t.file}</code>
            {t.queue && <> · in {t.queue}</>}
          </h3>
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip status={t.status} label={t.stage !== 'scope' && t.status ? 'scoping' : undefined} />
            {t.status && <StatusSelect label="Set status" onPick={(s) => setStatus(undefined, s)} />}
          </div>
          {t.partitions.map((p) => {
            const part = t.parts.find((x) => x.partition === p)
            return (
              <div key={p} className="flex items-center gap-2">
                <span className="w-24 truncate text-xs">{p}</span>
                {part ? <StatusChip status={part.status} /> : <span className="text-xs text-muted-foreground">not started</span>}
                {part && <StatusSelect label={`Set the status of ${p}`} onPick={(s) => setStatus(p, s)} parts />}
              </div>
            )
          })}
          {t.features.length > 0 && <p className="text-xs text-muted-foreground">Features: {t.features.map((f) => `[[${f}]]`).join(', ')}</p>}
          {t.needs.length > 0 && <p className="text-xs text-muted-foreground">Needs: {t.needs.map((f) => `[[${f}]]`).join(', ')}</p>}
          <ScopeScorecard task={t} state={state} />
          {t.stage === 'scope' && (
            <div data-testid="task-body">
              <Markdown text={taskBody || '_Loading…_'} names={names} />
            </div>
          )}
          <ScopeTimeline task={t} state={state} />
        </section>
      )}
    </aside>
  )
}

// ScopeScorecard sums how the work went (the scorecard of the task
// pipeline): its duration, blocks, refused dones, reviews, and the turns and
// the cost of each partition agent inside its part.
export function ScopeScorecard({ task: t, state }: { task: TaskView; state: State }) {
  const sc = t.scorecard
  if (!sc?.started) return null
  const parts = partCosts(sc, state.turns)
  const cost = totalCost(parts)
  const turns = totalTurns(parts)
  return (
    <div className="rounded-md border p-2 text-xs" data-testid="scorecard">
      <div className="flex flex-wrap gap-x-3 gap-y-1">
        <span>
          <span className="text-muted-foreground">{sc.finished ? 'took' : 'running for'}</span> {duration(sc)}
        </span>
        <span>
          <span className="text-muted-foreground">turns</span> {turns}
        </span>
        <span>
          <span className="text-muted-foreground">cost</span> {money(cost)}
        </span>
        <span className={sc.blocks ? 'text-red-700' : ''}>
          <span className="text-muted-foreground">blocked</span> {sc.blocks}×
        </span>
        <span className={sc.refusals ? 'text-amber-700' : ''}>
          <span className="text-muted-foreground">done refused</span> {sc.refusals}×
        </span>
        <span>
          <span className="text-muted-foreground">reviews</span> {sc.reviews}
        </span>
      </div>
      {parts.some((p) => p.turns > 0) && (
        <ul className="mt-1.5 flex flex-wrap gap-x-3 text-muted-foreground">
          {parts.map((p) => (
            <li key={p.partition} data-part-cost={p.partition}>
              {p.partition}: {p.turns} turn{p.turns === 1 ? '' : 's'} · {money(p.cost)}
              {p.open ? ' · open' : ''}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ScopeTimeline lists what happened to a scope in order: its log, the
// decisions hikari and the agents asked about it, and your answers.
export function ScopeTimeline({ task: t, state }: { task: TaskView; state: State }) {
  const entries = useMemo(() => timeline(t.scorecard, state.messages, t.id), [t.scorecard, state.messages, t.id])
  if (entries.length === 0) return null
  return (
    <details className="text-xs" data-testid="timeline">
      <summary className="cursor-pointer text-muted-foreground">Timeline ({entries.length})</summary>
      <ol className="mt-1 space-y-0.5 border-l pl-3">
        {entries.map((e, k) => (
          <li key={k} className={e.kind === 'decision' ? 'text-amber-800' : e.kind === 'answer' ? 'text-emerald-800' : ''} data-kind={e.kind}>
            <span className="text-muted-foreground">{new Date(e.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span> {e.text}
          </li>
        ))}
      </ol>
    </details>
  )
}

function StatusSelect({ label, onPick, parts }: { label: string; onPick: (s: string) => void; parts?: boolean }) {
  const options = parts ? (['queued', 'started', 'testing', 'blocked', 'done'] as TaskStatus[]) : statuses
  return (
    <select aria-label={label} className="h-7 rounded-md border bg-background px-1.5 text-xs" value="" onChange={(e) => e.target.value && onPick(e.target.value)}>
      <option value="">{label}…</option>
      {options.map((s) => (
        <option key={s} value={s}>
          {s}
        </option>
      ))}
    </select>
  )
}
