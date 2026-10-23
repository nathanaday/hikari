// Camera math for the graph canvas. A camera maps a world point p to the
// screen point (p.x * k + x, p.y * k + y), as a d3-zoom transform does.

export type Camera = { x: number; y: number; k: number }
export type Point = { x: number; y: number }
export type Bounds = { minX: number; minY: number; maxX: number; maxY: number }

export const minZoom = 0.2
export const maxZoom = 4

export const clampZoom = (k: number) => Math.min(maxZoom, Math.max(minZoom, k))

// bounds returns the box around circles; an empty list is a box at the
// origin.
export function bounds(circles: { x: number; y: number; r: number }[]): Bounds {
  if (circles.length === 0) return { minX: 0, minY: 0, maxX: 0, maxY: 0 }
  const b = { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity }
  for (const c of circles) {
    b.minX = Math.min(b.minX, c.x - c.r)
    b.minY = Math.min(b.minY, c.y - c.r)
    b.maxX = Math.max(b.maxX, c.x + c.r)
    b.maxY = Math.max(b.maxY, c.y + c.r)
  }
  return b
}

// fit returns the camera that shows the whole box in a view of width by
// height, with pad pixels around it and no more zoom than maxK (rule U28).
export function fit(b: Bounds, width: number, height: number, pad = 48, maxK = 2): Camera {
  const w = Math.max(b.maxX - b.minX, 1)
  const h = Math.max(b.maxY - b.minY, 1)
  const k = clampZoom(Math.min(maxK, (width - 2 * pad) / w, (height - 2 * pad) / h))
  return focus({ x: (b.minX + b.maxX) / 2, y: (b.minY + b.maxY) / 2 }, width, height, k)
}

// focus returns the camera with zoom k that puts p at the center of the
// view.
export function focus(p: Point, width: number, height: number, k: number): Camera {
  return { x: width / 2 - p.x * k, y: height / 2 - p.y * k, k }
}

// center returns the world point at the center of the view.
export function center(c: Camera, width: number, height: number): Point {
  return { x: (width / 2 - c.x) / c.k, y: (height / 2 - c.y) / c.k }
}

// between returns the camera at t of the way from a to b. The center moves
// in a straight line and the zoom changes at an even rate on a log scale, so
// a move that zooms in by 3× feels as even as one that zooms out.
export function between(a: Camera, b: Camera, t: number, width: number, height: number): Camera {
  const ca = center(a, width, height)
  const cb = center(b, width, height)
  const k = Math.exp(Math.log(a.k) + (Math.log(b.k) - Math.log(a.k)) * t)
  return focus({ x: ca.x + (cb.x - ca.x) * t, y: ca.y + (cb.y - ca.y) * t }, width, height, k)
}

export function toScreen(c: Camera, p: Point): Point {
  return { x: p.x * c.k + c.x, y: p.y * c.k + c.y }
}

export function toWorld(c: Camera, p: Point): Point {
  return { x: (p.x - c.x) / c.k, y: (p.y - c.y) / c.k }
}

// easeInOut is the cubic ease-in-out curve.
export function easeInOut(t: number): number {
  const u = Math.min(1, Math.max(0, t))
  return u < 0.5 ? 4 * u * u * u : 1 - Math.pow(-2 * u + 2, 3) / 2
}

// progress returns how far an animation that started at start and lasts
// duration is at now, from 0 to 1.
export function progress(now: number, start: number, duration: number): number {
  if (duration <= 0) return 1
  return Math.min(1, Math.max(0, (now - start) / duration))
}
