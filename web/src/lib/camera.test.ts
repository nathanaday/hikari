import { describe, expect, it } from 'vitest'
import { between, bounds, center, easeInOut, fit, focus, progress, toScreen, toWorld } from './camera'

describe('camera', () => {
  it('fits a box with padding and a zoom limit', () => {
    const b = bounds([{ x: -100, y: -50, r: 0 }, { x: 100, y: 50, r: 0 }])
    const c = fit(b, 500, 300, 50)
    expect(c.k).toBeCloseTo(2)
    expect(toScreen(c, { x: 0, y: 0 })).toEqual({ x: 250, y: 150 })
    const tight = fit(b, 300, 300, 50)
    expect(tight.k).toBeCloseTo(1)
    const one = fit(bounds([{ x: 5, y: 5, r: 1 }]), 400, 400)
    expect(one.k).toBe(2)
    expect(fit(bounds([{ x: 0, y: 0, r: 0 }, { x: 1e6, y: 0, r: 0 }]), 400, 400).k).toBe(0.2)
  })

  it('focuses a point and converts both ways', () => {
    const c = focus({ x: 10, y: 20 }, 400, 200, 3)
    expect(toScreen(c, { x: 10, y: 20 })).toEqual({ x: 200, y: 100 })
    expect(toWorld(c, { x: 200, y: 100 })).toEqual({ x: 10, y: 20 })
    expect(center(c, 400, 200)).toEqual({ x: 10, y: 20 })
  })

  it('moves the center in a line and the zoom on a log scale', () => {
    const a = focus({ x: 0, y: 0 }, 400, 400, 1)
    const b = focus({ x: 100, y: 0 }, 400, 400, 4)
    const mid = between(a, b, 0.5, 400, 400)
    expect(mid.k).toBeCloseTo(2)
    expect(center(mid, 400, 400).x).toBeCloseTo(50)
    expect(between(a, b, 0, 400, 400)).toEqual(a)
    const end = between(a, b, 1, 400, 400)
    expect(end.k).toBeCloseTo(4)
    expect(center(end, 400, 400).x).toBeCloseTo(100)
  })

  it('eases and measures progress', () => {
    expect(easeInOut(0)).toBe(0)
    expect(easeInOut(0.5)).toBe(0.5)
    expect(easeInOut(1)).toBe(1)
    expect(easeInOut(0.25)).toBeLessThan(0.25)
    expect(progress(150, 100, 100)).toBe(0.5)
    expect(progress(50, 100, 100)).toBe(0)
    expect(progress(500, 100, 100)).toBe(1)
    expect(progress(0, 0, 0)).toBe(1)
  })
})
