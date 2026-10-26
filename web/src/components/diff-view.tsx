import { hunks } from '@/lib/topology'
import type { DiffLine } from '@/lib/types'
import { cn } from '@/lib/utils'

// DiffView shows the changed lines of a compose file with three lines
// around each change, as a code review does (rules YY5 and F3).
export function DiffView({ diff }: { diff: DiffLine[] }) {
  const lines = hunks(diff)
  if (lines.length === 0) {
    return <p className="text-sm text-muted-foreground">The file does not change.</p>
  }
  return (
    <div className="overflow-x-auto rounded-md border font-mono text-xs leading-relaxed" data-testid="diff">
      <table className="w-full border-collapse">
        <tbody>
          {lines.map((l, i) =>
            l === null ? (
              <tr key={i} className="bg-neutral-50 text-muted-foreground">
                <td colSpan={4} className="px-3 py-0.5 select-none">
                  ⋯
                </td>
              </tr>
            ) : (
              <tr
                key={i}
                data-op={l.op}
                className={cn(l.op === '+' && 'bg-[color-mix(in_oklch,var(--lens-pass)_12%,white)]', l.op === '-' && 'bg-[color-mix(in_oklch,var(--lens-fail)_10%,white)]')}
              >
                <td className="w-10 px-2 text-right text-muted-foreground select-none">{l.old ?? ''}</td>
                <td className="w-10 px-2 text-right text-muted-foreground select-none">{l.new ?? ''}</td>
                <td className={cn('w-4 select-none', l.op === '+' && 'text-[var(--lens-pass)]', l.op === '-' && 'text-[var(--lens-fail)]')}>{l.op}</td>
                <td className="pr-3 whitespace-pre">{l.text || ' '}</td>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </div>
  )
}
