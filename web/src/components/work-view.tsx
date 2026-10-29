import { useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, Pause, Play, Plus, TriangleAlert, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Markdown } from '@/components/markdown'
import type { State } from '@/lib/store'
import type { QueueView, TaskStatus, TaskView, TasksView } from '@/lib/types'
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
  if (!status) return <span className="rounded px-1.5 py-0.5 text-xs text-muted-foreground ring-1 ring-border">draft</span>
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs ${statusClass[status]}`} data-status={status}>
      {label ? `${label} ` : ''}
      {status}
    </span>
  )
}

// call sends one change of the pipeline, and returns an error message, or
// null when the server took it.
async function call(path: string, body: unknown): Promise<string | null> {
  const res = await fetch(`/api/ui/tasks${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const e = (await res.json().catch(() => ({}))) as { error?: string }
  return e.error ?? res.statusText
}

// movable says whether a scope may move or leave its queue (rule TK11).
const movable = (t: TaskView) => !t.status || t.status === 'queued' || t.status === 'waiting'

export function WorkView({
  state,
  show,
  task,
  send,
}: {
  state: State
  show: (v: View) => void
  task?: string
  send: (req: { to?: string; body: string }) => Promise<string | null>
}) {
  const work = state.modules.tasks as TasksView
  const [error, setError] = useState<string | null>(null)
  const [stub, setStub] = useState('')
  const [queueName, setQueueName] = useState('')
  const all = useMemo(() => [...work.stubs, ...work.drafts, ...work.queues.flatMap((q) => q.scopes)], [work])
  const open = all.find((t) => t.id === task)

  const run = async (p: Promise<string | null>) => setError(await p)
  const plant = async () => {
    const lines = stub.trim()
    if (!lines) return
    const err = await call('/stub', { name: lines })
    setError(err)
    if (!err) setStub('')
  }
  const scope = (t: TaskView) =>
    run(
      work.orchestration
        ? send({ to: work.orchestration, body: `Please scope [[${t.id}]]. Follow the Scope a stub procedure of the hikari-scope-work skill.` })
        : Promise.resolve('The project has no orchestration agent to scope it.'),
    )
  const queueOp = (op: Record<string, unknown>) => run(call('/queue', op))

  return (
    <div className="flex min-h-0 flex-1" data-testid="work-view">
      <div className="min-h-0 flex-1 space-y-6 overflow-y-auto px-4 py-5 md:px-6">
        {error && (
          <p className="flex items-start gap-2 text-sm text-red-600" role="alert">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" />
            {error}
          </p>
        )}
        <section className="space-y-2">
          <h2 className="text-sm font-semibold">Plant a stub</h2>
          <div className="flex gap-2">
            <Textarea
              value={stub}
              onChange={(e) => setStub(e.target.value)}
              placeholder="An idea, in a few words. The first line is its name."
              rows={2}
              aria-label="New stub"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) plant()
              }}
            />
            <Button onClick={plant} disabled={!stub.trim()}>
              Plant
            </Button>
          </div>
        </section>

        <div className="grid gap-6 md:grid-cols-2">
          <section className="space-y-2" data-testid="stubs">
            <h2 className="text-sm font-semibold">Stubs ({work.stubs.length})</h2>
            {work.stubs.length === 0 && <p className="text-sm text-muted-foreground">No stubs. Plant one above, or in vault/tasks/ with Obsidian.</p>}
            {work.stubs.map((t) => (
              <Card key={t.id} task={t} active={t.id === task} onOpen={() => show({ kind: 'work', task: t.id })}>
                <Button size="sm" variant="secondary" onClick={() => scope(t)}>
                  Scope
                </Button>
              </Card>
            ))}
          </section>
          <section className="space-y-2" data-testid="drafts">
            <h2 className="text-sm font-semibold">Draft scopes ({work.drafts.length})</h2>
            {work.drafts.length === 0 && <p className="text-sm text-muted-foreground">No drafts. The orchestration agent writes one when it scopes a stub with you.</p>}
            {work.drafts.map((t) => (
              <Card key={t.id} task={t} active={t.id === task} onOpen={() => show({ kind: 'work', task: t.id })}>
                <QueueMenu queues={work.queues} label="Add to queue" onPick={(q) => queueOp({ op: 'add', queue: q, scope: t.id })} />
              </Card>
            ))}
          </section>
        </div>

        <section className="space-y-2">
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
          {work.queues.length === 0 && <p className="text-sm text-muted-foreground">No queues. A scope in a queue is approved, and a running queue starts its scopes in order.</p>}
          <div className="flex gap-4 overflow-x-auto pb-2">
            {work.queues.map((q) => (
              <QueueColumn key={q.id} queue={q} queues={work.queues} open={task} show={show} run={run} queueOp={queueOp} />
            ))}
          </div>
        </section>

        {work.problems.length > 0 && (
          <section className="space-y-1">
            <h2 className="text-sm font-semibold">Problems</h2>
            {work.problems.map((p) => (
              <p key={p.file} className="text-sm text-red-600">
                <code className="text-xs">{p.file}</code>: {p.reason}
              </p>
            ))}
          </section>
        )}
      </div>
      {open && <TaskPanel key={open.id + open.hash} task={open} state={state} close={() => show({ kind: 'work' })} run={run} />}
    </div>
  )
}

function Card({ task: t, active, onOpen, children }: { task: TaskView; active: boolean; onOpen: () => void; children?: React.ReactNode }) {
  return (
    <div className={`space-y-1.5 rounded-md border p-2.5 ${active ? 'border-primary' : ''}`} data-task={t.id}>
      <div className="flex items-start justify-between gap-2">
        <button type="button" className="min-w-0 text-left text-sm font-medium hover:underline" onClick={onOpen}>
          {t.name}
        </button>
        {t.stage === 'scope' && <StatusChip status={t.status} />}
      </div>
      <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
        <code>{t.id}</code>
        {t.author && <span>· by {t.author}</span>}
        {t.parts.length > 0
          ? t.parts.map((p) => <StatusChip key={p.partition} status={p.status} label={p.partition} />)
          : t.partitions.map((p) => (
              <Badge key={p} variant="outline" className="font-normal">
                {p}
              </Badge>
            ))}
      </div>
      {t.wait && <p className="text-xs text-amber-700 dark:text-amber-400">Waits: {t.wait}</p>}
      {t.block && <p className="text-xs text-red-600">Blocked: {t.block}</p>}
      {children && <div className="flex flex-wrap gap-1.5">{children}</div>}
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

function QueueColumn({
  queue: q,
  queues,
  open,
  show,
  run,
  queueOp,
}: {
  queue: QueueView
  queues: QueueView[]
  open?: string
  show: (v: View) => void
  run: (p: Promise<string | null>) => void
  queueOp: (op: Record<string, unknown>) => void
}) {
  const running = q.state === 'running'
  return (
    <div className="w-72 shrink-0 space-y-2 rounded-lg bg-muted/40 p-2.5" data-queue={q.id}>
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
      {q.scopes.length === 0 && <p className="text-xs text-muted-foreground">Empty. Add a draft scope to it.</p>}
      {q.scopes.map((t, n) => {
        const canMove = movable(t)
        return (
          <Card key={t.id} task={t} active={t.id === open} onOpen={() => show({ kind: 'work', task: t.id })}>
            {canMove && (
              <>
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
              </>
            )}
          </Card>
        )
      })}
    </div>
  )
}

// TaskPanel shows one task's document, its parts, and its log (rule TK28).
function TaskPanel({ task: t, state, close, run }: { task: TaskView; state: State; close: () => void; run: (p: Promise<string | null>) => void }) {
  const [text, setText] = useState<string | null>(null)
  useEffect(() => {
    let live = true
    fetch(`/api/ui/tasks/tasks/${encodeURIComponent(t.id)}`)
      .then((r) => (r.ok ? r.json() : Promise.reject(r.statusText)))
      .then((d: { text: string }) => live && setText(d.text))
      .catch(() => live && setText(''))
    return () => {
      live = false
    }
  }, [t.id, t.hash])
  const body = useMemo(() => (text ?? '').replace(/^---\n[\s\S]*?\n---\n/, ''), [text])
  const names = useMemo(() => new Set(state.participants.map((p) => p.name)), [state.participants])
  const setStatus = (part: string | undefined, status: string) => run(call('/status', { task: t.id, part, status }))

  return (
    <aside className="hidden w-[28rem] shrink-0 overflow-y-auto border-l px-4 py-5 lg:block" data-testid="task-panel">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="text-base font-semibold">{t.name}</h2>
          <p className="text-xs text-muted-foreground">
            <code>{t.file}</code>
            {t.queue && <> · in {t.queue}</>}
          </p>
        </div>
        <Button size="icon-sm" variant="ghost" aria-label="Close" onClick={close}>
          <X className="size-4" />
        </Button>
      </div>
      {t.stage === 'scope' && (
        <div className="mt-3 space-y-2 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip status={t.status} />
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
        </div>
      )}
      <div className="mt-4" data-testid="task-body">
        {text === null ? <p className="text-sm text-muted-foreground">Loading…</p> : <Markdown text={body || '_No text yet._'} names={names} />}
      </div>
    </aside>
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
