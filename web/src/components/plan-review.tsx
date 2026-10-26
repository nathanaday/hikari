import { useState } from 'react'
import { DiffView } from '@/components/diff-view'
import { FindingsList } from '@/components/findings-list'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { hasError } from '@/lib/topology'
import type { Occurrence, Plan } from '@/lib/types'

// Occurrences lists the places a rename can propagate to, each under a
// checkbox (rule SR2). One without a key is its owner's to change.
export function Occurrences({ occurrences, chosen, toggle }: { occurrences: Occurrence[]; chosen: Set<string>; toggle: (key: string) => void }) {
  return (
    <ul className="divide-y rounded-md border text-sm" data-testid="occurrences">
      {occurrences.map((o, i) => (
        <li key={o.key ?? `${o.file}:${o.line}:${i}`} className="flex gap-2.5 px-3 py-2">
          {o.key ? (
            <input type="checkbox" className="mt-1" checked={chosen.has(o.key)} onChange={() => toggle(o.key!)} aria-label={`Change ${o.file} line ${o.line}`} />
          ) : (
            <span className="w-3.5" />
          )}
          <span className="min-w-0 flex-1">
            <span className="block text-xs text-muted-foreground">
              {o.file}, line {o.line}
              {!o.key && ' · its owner updates it'}
            </span>
            <span className="block font-mono text-xs break-words">{o.replace}</span>
          </span>
        </li>
      ))}
    </ul>
  )
}

// PlanReview shows what the pending changes do before Save applies them
// (rules SR1 to SR4).
export function PlanReview({
  plan,
  open,
  onOpenChange,
  onSave,
  conflict,
  onPlanAgain,
}: {
  plan: Plan
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (propagate: string[]) => void
  conflict: boolean
  onPlanAgain: () => void
}) {
  const keyed = plan.occurrences.filter((o) => o.key).map((o) => o.key!)
  const [unchecked, setUnchecked] = useState<Set<string>>(new Set())
  const chosen = new Set(keyed.filter((k) => !unchecked.has(k)))
  const toggle = (k: string) => {
    const next = new Set(unchecked)
    if (next.has(k)) next.delete(k)
    else next.add(k)
    setUnchecked(next)
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-2xl" data-testid="plan-review">
        <SheetHeader>
          <SheetTitle>Review your changes</SheetTitle>
          <SheetDescription>What the save writes into the compose file, and what it changes outside it.</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4">
          <ul className="list-disc space-y-0.5 pl-5 text-sm" data-testid="summary">
            {plan.summary.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ul>
          <DiffView diff={plan.diff} />
          {plan.occurrences.length > 0 && (
            <section className="space-y-2">
              <h3 className="text-sm font-medium">Carry the new names into the wikis</h3>
              <p className="text-sm text-muted-foreground">These places mention an old name. Checked ones change with the save, as your commit.</p>
              <Occurrences occurrences={plan.occurrences} chosen={chosen} toggle={toggle} />
            </section>
          )}
          {plan.effects.length > 0 && (
            <section className="space-y-1">
              <h3 className="text-sm font-medium">Outside the file</h3>
              <ul className="list-disc space-y-0.5 pl-5 text-sm text-muted-foreground">
                {plan.effects.map((e, i) => (
                  <li key={i}>{e}</li>
                ))}
              </ul>
            </section>
          )}
          {hasError(plan.findings) && <p className="text-sm">The file will have errors; hikari will not start until you fix them.</p>}
          {plan.findings.length > 0 && (
            <div className="-mx-3">
              <FindingsList findings={plan.findings} />
            </div>
          )}
          {plan.applies_at_start && <p className="text-sm text-muted-foreground">The agents run the file they started with. These changes apply at the next start.</p>}
        </div>
        <SheetFooter className="flex-row items-center justify-end border-t">
          {conflict ? (
            <>
              <span className="flex-1 text-sm">The file changed on disk since you planned these changes.</span>
              <Button onClick={onPlanAgain}>Plan again</Button>
            </>
          ) : (
            <>
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                Keep editing
              </Button>
              <Button onClick={() => onSave([...chosen])} data-testid="apply">
                Save
              </Button>
            </>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
