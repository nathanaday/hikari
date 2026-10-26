import { sortFindings } from '@/lib/topology'
import type { Finding } from '@/lib/types'
import { cn } from '@/lib/utils'

export function SeverityDot({ severity }: { severity: Finding['severity'] }) {
  return (
    <span
      aria-label={severity}
      className={cn('inline-block size-2 shrink-0 rounded-full', severity === 'error' ? 'bg-[var(--lens-fail)]' : 'bg-[var(--lens-pending)]')}
    />
  )
}

// FindingsList lists findings, errors first, each with its line (rule
// YY3). onLine moves the editor to a line.
export function FindingsList({ findings, onLine, empty = 'No problems' }: { findings: Finding[]; onLine?: (line: number) => void; empty?: string }) {
  if (findings.length === 0) {
    return <p className="px-3 py-2 text-sm text-muted-foreground">{empty}</p>
  }
  return (
    <ul className="divide-y" data-testid="findings">
      {sortFindings(findings).map((f, i) => {
        const body = (
          <>
            <span className="pt-1.5">
              <SeverityDot severity={f.severity} />
            </span>
            <span className="min-w-0 flex-1">
              {f.line ? <span className="block text-xs text-muted-foreground">Line {f.line}</span> : null}
              <span className="block text-sm break-words">{f.message}</span>
            </span>
          </>
        )
        return (
          <li key={i}>
            {f.line && onLine ? (
              <button type="button" onClick={() => onLine(f.line!)} className="flex w-full gap-2.5 px-3 py-2 text-left hover:bg-neutral-50">
                {body}
              </button>
            ) : (
              <div className="flex gap-2.5 px-3 py-2">{body}</div>
            )}
          </li>
        )
      })}
    </ul>
  )
}
