import { useState } from 'react'
import { X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { isExpression } from '@/lib/topology-rules'
import { cn } from '@/lib/utils'

// Field lays out a label, a control, and the line under it: a problem, or
// a hint.
export function Field({ label, problem, hint, children }: { label: string; problem?: string | null; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
      {problem ? <span className="block text-xs text-[var(--lens-fail)]">{problem}</span> : hint ? <span className="block text-xs text-muted-foreground">{hint}</span> : null}
    </label>
  )
}

type TextProps = {
  label: string
  value: string
  onCommit: (value: string) => void
  check?: (value: string) => string | null
  multiline?: boolean
  placeholder?: string
  hint?: React.ReactNode
  mono?: boolean
}

// TextField keeps a draft until you leave the field or press Enter, and
// commits it only when its check passes (decision 3 and rule SV3). Esc
// drops the draft. A value that compose fills in stays read-only (SI8).
export function TextField({ label, value, onCommit, check, multiline, placeholder, hint, mono }: TextProps) {
  const [draft, setDraft] = useState<string | null>(null)
  const shown = draft ?? value
  const problem = draft !== null && check ? check(draft) : null
  if (isExpression(value)) {
    return (
      <Field label={label} hint="Compose fills this value in when it reads the file. Change it in the YAML tab.">
        <code className="block rounded-md border bg-neutral-50 px-3 py-1.5 text-xs">{value}</code>
      </Field>
    )
  }
  const commit = () => {
    if (draft === null) return
    if (problem) return
    if (draft !== value) onCommit(draft)
    setDraft(null)
  }
  const common = {
    value: shown,
    placeholder,
    'aria-invalid': problem ? true : undefined,
    onBlur: commit,
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') setDraft(null)
      if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        commit()
      }
    },
  }
  return (
    <Field label={label} problem={problem} hint={hint}>
      {multiline ? (
        <Textarea {...common} rows={3} className={cn('min-h-16 text-sm', mono && 'font-mono text-xs')} onChange={(e) => setDraft(e.target.value)} />
      ) : (
        <Input {...common} className={cn('h-8', mono && 'font-mono text-xs')} onChange={(e) => setDraft(e.target.value)} />
      )}
    </Field>
  )
}

// Select is a native select in the look of the inputs.
export function Select({
  value,
  options,
  onChange,
  placeholder,
  className,
  ...rest
}: { value: string; options: { value: string; label: string }[]; onChange: (v: string) => void; placeholder?: string; className?: string } & Omit<
  React.ComponentProps<'select'>,
  'onChange' | 'value'
>) {
  return (
    <select
      {...rest}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={cn(
        'h-8 w-full rounded-md border border-input bg-transparent px-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50',
        className,
      )}
    >
      {placeholder !== undefined && <option value="">{placeholder}</option>}
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  )
}

// Chips shows a list as removable chips, with a picker or a text input to
// add one.
export function Chips({
  items,
  label,
  onRemove,
  onAdd,
  choices,
  addLabel,
  check,
  empty = 'None',
}: {
  items: { value: string; label: string }[]
  label?: (value: string) => string
  onRemove?: (value: string) => void
  onAdd?: (value: string) => void
  choices?: { value: string; label: string }[]
  addLabel: string
  check?: (value: string) => string | null
  empty?: string
}) {
  const [text, setText] = useState('')
  const problem = text && check ? check(text) : null
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap gap-1.5">
        {items.length === 0 && <span className="text-sm text-muted-foreground">{empty}</span>}
        {items.map((i) => (
          <span key={i.value} className="inline-flex h-6 items-center gap-1 rounded-md border px-2 text-xs">
            {label ? label(i.value) : i.label}
            {onRemove && (
              <button type="button" aria-label={`Remove ${i.label}`} className="text-muted-foreground hover:text-foreground" onClick={() => onRemove(i.value)}>
                <X className="size-3" />
              </button>
            )}
          </span>
        ))}
      </div>
      {onAdd &&
        (choices ? (
          choices.length > 0 && (
            <Select value="" placeholder={addLabel} options={choices} onChange={(v) => v && onAdd(v)} className="w-auto text-xs text-muted-foreground" />
          )
        ) : (
          <div>
            <Input
              value={text}
              placeholder={addLabel}
              aria-invalid={problem ? true : undefined}
              className="h-7 font-mono text-xs"
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && text && !problem) {
                  e.preventDefault()
                  onAdd(text)
                  setText('')
                }
              }}
            />
            {problem && <span className="text-xs text-[var(--lens-fail)]">{problem}</span>}
          </div>
        ))}
    </div>
  )
}
