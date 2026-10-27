import { createElement, useState } from 'react'
import {
  AlertCircle,
  Brain,
  ChevronDown,
  ChevronRight,
  FilePen,
  FileText,
  Loader2,
  MessageSquareText,
  Search,
  Send,
  Sparkles,
  Terminal,
  Wrench,
} from 'lucide-react'
import { Markdown } from '@/components/markdown'
import { NodeAvatar } from '@/components/node-avatar'
import { asksSomething, seconds, stepLabel, turnSummary, type Turn } from '@/lib/turns'
import type { Activity } from '@/lib/types'
import { cn } from '@/lib/utils'

// liveSteps is how many steps a live card shows before it is expanded.
const liveSteps = 6

// TurnCard shows one turn of an agent in the timeline: its steps live while
// the agent works, then one summary line that expands to the whole turn.
export function TurnCard({ turn, names }: { turn: Turn; names: ReadonlySet<string> }) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? turn.live
  const steps = expanded && (open || turn.steps.length <= liveSteps) ? turn.steps : turn.steps.slice(-liveSteps)
  const hidden = turn.steps.length - steps.length
  const current = turn.live ? turn.steps.findLast((s) => !s.done) : undefined
  const failed = turn.end?.error

  return (
    <div data-testid="turn" data-live={turn.live || undefined} className="px-4 py-1.5 md:px-6">
      <div className={cn('rounded-lg border bg-white', turn.live && 'border-neutral-300', failed && 'border-red-200')}>
        <button
          type="button"
          onClick={() => setOpen(!expanded)}
          aria-expanded={expanded}
          className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-neutral-50"
        >
          <NodeAvatar name={turn.agent} me={false} className="size-6 text-[10px]" />
          <span className="font-medium">{turn.agent}</span>
          <span className="flex min-w-0 flex-1 items-center gap-1.5 truncate text-xs text-muted-foreground" data-testid="turn-status">
            {turn.live ? (
              <>
                <Loader2 className="size-3 shrink-0 animate-spin text-emerald-600" />
                {current ? stepLabel(current) : 'working'}
              </>
            ) : failed ? (
              <>
                <AlertCircle className="size-3 shrink-0 text-red-600" />
                <span className="text-red-600">failed</span>
                {turn.end?.duration ? ` · ${seconds(turn.end.duration)}` : ''}
              </>
            ) : (
              turnSummary(turn) || 'done'
            )}
          </span>
          {expanded ? <ChevronDown className="size-4 shrink-0 text-neutral-400" /> : <ChevronRight className="size-4 shrink-0 text-neutral-400" />}
        </button>
        {expanded && (
          <div className="border-t px-3 py-2">
            {turn.trigger && <p className="mb-1 text-xs text-muted-foreground">{turn.trigger}</p>}
            {hidden > 0 && (
              <button type="button" onClick={() => setOpen(true)} className="mb-1 text-xs text-muted-foreground hover:text-foreground">
                {hidden} earlier {hidden === 1 ? 'step' : 'steps'}…
              </button>
            )}
            <ol className="space-y-0.5">
              {steps.map((s) => (
                <Step key={s.id} step={s} names={names} />
              ))}
            </ol>
            {turn.end && <Closing end={turn.end} names={names} />}
          </div>
        )}
        {!expanded && turn.end?.unheard && <Closing end={turn.end} names={names} />}
      </div>
    </div>
  )
}

function Step({ step, names }: { step: Activity; names: ReadonlySet<string> }) {
  const [open, setOpen] = useState(false)
  if (step.kind === 'text') {
    return (
      <li className="flex gap-2 py-0.5 text-sm" data-testid="step">
        <MessageSquareText className="mt-1 size-3.5 shrink-0 text-neutral-400" />
        <div className="min-w-0 flex-1 text-neutral-700">
          <Markdown text={step.text ?? ''} names={names} />
        </div>
      </li>
    )
  }
  const details = step.kind === 'tool' && (step.input !== undefined || step.output)
  return (
    <li data-testid="step" data-error={step.error || undefined}>
      <button
        type="button"
        disabled={!details}
        onClick={() => setOpen(!open)}
        className={cn('flex w-full items-center gap-2 rounded py-0.5 text-left text-sm', details && 'hover:bg-neutral-50')}
      >
        {createElement(icon(step), { className: cn('size-3.5 shrink-0', step.error ? 'text-red-600' : 'text-neutral-400') })}
        <span className={cn('min-w-0 flex-1 truncate', step.error ? 'text-red-600' : step.done ? 'text-neutral-700' : 'text-foreground')}>
          {stepLabel(step)}
        </span>
        {!step.done && step.kind !== 'thinking' && <Loader2 className="size-3 shrink-0 animate-spin text-neutral-400" />}
        {step.done && step.kind === 'tool' && step.duration ? <span className="text-xs text-muted-foreground">{seconds(step.duration)}</span> : null}
      </button>
      {open && details && (
        <div className="mb-1 ml-5 space-y-1 text-xs">
          {step.input !== undefined && <pre className="overflow-x-auto rounded bg-neutral-50 p-2 whitespace-pre-wrap text-neutral-700">{JSON.stringify(step.input, null, 2)}</pre>}
          {step.output && (
            <pre className={cn('max-h-64 overflow-auto rounded p-2 whitespace-pre-wrap', step.error ? 'bg-red-50 text-red-800' : 'bg-neutral-50 text-neutral-600')}>
              {step.output}
            </pre>
          )}
        </div>
      )}
    </li>
  )
}

// Closing shows how the turn ended: its error, or its closing text. Text
// that reached no one is marked, because it may be a question for you.
function Closing({ end, names }: { end: Activity; names: ReadonlySet<string> }) {
  if (end.error) {
    return (
      <p className="mt-1 flex items-start gap-1.5 text-sm text-red-600" data-testid="closing">
        <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
        <span className="break-words">{end.text}</span>
      </p>
    )
  }
  if (!end.text) return null
  const question = end.unheard && asksSomething(end.text)
  return (
    <div
      data-testid="closing"
      data-unheard={end.unheard || undefined}
      className={cn('mt-1 rounded border px-2.5 py-1.5 text-sm', question ? 'border-amber-200 bg-amber-50' : 'border-neutral-100 bg-neutral-50')}
    >
      <p className={cn('mb-0.5 text-xs', question ? 'text-amber-800' : 'text-muted-foreground')}>
        {question ? 'Asks something, but sent it to no one. Reply by message.' : end.unheard ? 'Closing note · sent to no one' : 'Closing note'}
      </p>
      <Markdown text={end.text} names={names} />
    </div>
  )
}

function icon(step: Activity) {
  if (step.error) return AlertCircle
  if (step.kind === 'thinking') return Brain
  switch (step.tool) {
    case 'Read':
      return FileText
    case 'Edit':
    case 'MultiEdit':
    case 'Write':
    case 'NotebookEdit':
      return FilePen
    case 'Bash':
      return Terminal
    case 'Grep':
    case 'Glob':
    case 'WebSearch':
    case 'WebFetch':
      return Search
    case 'Skill':
    case 'Agent':
      return Sparkles
    case 'mcp__hikari__post':
    case 'mcp__hikari__send_direct_message':
      return Send
  }
  return Wrench
}
