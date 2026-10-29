import type { Mood } from './assistant'

// Hikari, the cat: a sprite of pixel rows, one character per pixel, drawn
// as SVG rectangles (rule HA13 of the hikari agent spec). "." is empty,
// "o" the outline, "w" fur, "p" pink, "e" an eye, and "-" a closed eye.
const base = [
  '..o.........o...',
  '.owo.......owo..',
  '.opwo.....owpo..',
  '.owwwooooowwwo..',
  '.owwwwwwwwwwwo..',
  'owwwwwwwwwwwwwo.',
  'owwwwwwwwwwwwwo.',
  'owwwwwwwwwwwwwo.',
  'owwwwwwpwwwwwwo.',
  '.owwwwwwwwwwwo..',
  '..ooowwwwwooo...',
  '...owwwwwwwo....',
  '...owwwwwwwo..o.',
  '...owwwwwwwo.ow.',
  '...oowwwwwoo.ow.',
  '....oo...oo..o..',
]

// The tail's other position, for the wag.
const tailUp: Record<number, string> = {
  11: '...owwwwwwwo..o.',
  12: '...owwwwwwwo.ow.',
  13: '...owwwwwwwo.ow.',
  14: '...oowwwwwoo.o..',
  15: '....oo...oo.....',
}

export const colors: Record<string, string> = {
  o: 'var(--cat-outline, #52525b)',
  w: 'var(--cat-fur, #ffffff)',
  p: 'var(--cat-pink, #f9a8d4)',
  e: 'var(--cat-eye, #27272a)',
  '-': 'var(--cat-eye, #27272a)',
}

// frame draws the cat of a mood at a tick of the animation.
export function frame(mood: Mood, tick: number): string[] {
  const rows = base.map((r) => r.split(''))
  const put = (x: number, y: number, c: string) => {
    rows[y][x] = c
  }
  const wag = tick % 2 === 1
  if (wag) for (const [y, row] of Object.entries(tailUp)) rows[Number(y)] = row.split('')
  switch (mood) {
    case 'asleep':
      put(4, 6, '-')
      put(10, 6, '-')
      break
    case 'content': {
      const blink = tick % 6 === 5
      put(4, 6, blink ? '-' : 'e')
      put(10, 6, blink ? '-' : 'e')
      break
    }
    case 'alarmed':
      for (const [x, y] of [[3, 5], [4, 5], [3, 6], [4, 6], [10, 5], [11, 5], [10, 6], [11, 6]]) put(x, y, 'e')
      put(7, 9, 'e')
      break
    case 'thinking':
      put(4, 5, 'e')
      put(10, 5, 'e')
      break
  }
  return rows.map((r) => r.join(''))
}
