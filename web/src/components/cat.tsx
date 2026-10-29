import { useEffect, useState } from 'react'
import type { Mood } from '@/lib/assistant'
import { colors, frame } from '@/lib/sprite'
import { cn } from '@/lib/utils'

const marks: Record<Mood, string[]> = {
  asleep: ['z', 'Z', 'z'],
  content: [],
  alarmed: ['!'],
  thinking: ['·', '··', '···'],
}

const motion: Record<Mood, string> = {
  asleep: 'cat-breathe',
  content: 'cat-nod',
  alarmed: 'cat-shake',
  thinking: 'cat-bob',
}

// Cat is the toggle of the panel. attention is a question mark for a
// pending prompt, or a dot for an answer that waits (rule HA12).
export function Cat({ mood, attention, open, onClick, className }: { mood: Mood; attention?: 'prompt' | 'unread'; open: boolean; onClick: () => void; className?: string }) {
  const [tick, setTick] = useState(0)
  useEffect(() => {
    const id = window.setInterval(() => setTick((t) => t + 1), mood === 'alarmed' ? 250 : 600)
    return () => window.clearInterval(id)
  }, [mood])
  const rows = frame(mood, tick)
  const mark = marks[mood][tick % Math.max(1, marks[mood].length)] ?? ''
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={`Hikari, ${mood}${open ? ': close the panel' : ': open the panel'}`}
      aria-pressed={open}
      title={`Hikari is ${mood}. Click to ${open ? 'close' : 'open'} the panel.`}
      data-testid="cat"
      data-mood={mood}
      className={cn('group relative flex size-12 items-center justify-center rounded-lg transition-colors hover:bg-neutral-100', open && 'bg-neutral-100', className)}
    >
      <svg viewBox="0 0 20 18" width="40" height="36" shapeRendering="crispEdges" className={cn('overflow-visible', motion[mood])} aria-hidden>
        {mark && (
          <text x="16" y="4" fontSize="5" fontFamily="ui-monospace, monospace" fill="var(--cat-outline, #52525b)" className="cat-mark" data-testid="cat-mark">
            {mark}
          </text>
        )}
        <g transform="translate(2 2)">
          {rows.flatMap((row, y) =>
            row.split('').map((c, x) => {
              if (c === '.') return null
              const closed = c === '-'
              return <rect key={`${x}-${y}`} x={x} y={closed ? y + 0.5 : y} width={1} height={closed ? 0.5 : 1} fill={colors[c]} />
            }),
          )}
        </g>
      </svg>
      {attention === 'prompt' && (
        <span data-testid="cat-attention" className="absolute top-0.5 right-0.5 flex size-4 items-center justify-center rounded-full bg-amber-400 text-[10px] font-bold text-white">
          ?
        </span>
      )}
      {attention === 'unread' && <span data-testid="cat-attention" className="absolute top-1 right-1 size-2.5 rounded-full bg-[var(--lens-active)]" />}
    </button>
  )
}
