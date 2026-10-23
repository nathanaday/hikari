import { forceCollide, forceLink, forceManyBody, forceSimulation, forceX, forceY, type Simulation, type SimulationNodeDatum } from 'd3-force'
import { select, type Selection } from 'd3-selection'
import { zoom, zoomIdentity, type ZoomBehavior } from 'd3-zoom'
import { between, bounds, easeInOut, fit, focus, maxZoom, minZoom, progress, toScreen, toWorld, type Camera, type Point } from '@/lib/camera'
import type { Graph, GraphNode } from '@/lib/graph'
import type { Color, Paint } from '@/lib/lens'

// GraphEngine lays out, draws, and animates one graph at a time on a
// canvas (rules U20 to U29 and U41 to U44). It knows nodes and edges, not
// systems or features.

type SimNode = SimulationNodeDatum & { id: string; node: GraphNode }
type SimLink = { source: SimNode; target: SimNode; dashed: boolean }

// Handoff says how a new graph relates to the one on the canvas: it opens
// out of node from of the old graph, or the old graph folds into node to of
// the new one.
export type Handoff = { kind: 'in'; from: string } | { kind: 'out'; to: string }

export type Callbacks = {
  onClick: (id: string | null) => void
  // onSelect reports a selection made with the keyboard.
  onSelect: (id: string | null) => void
  onOpen: (id: string) => void
  onEscape: () => void
  onHover: (node: GraphNode | null, at: Point) => void
}

// Frozen is the old graph during a transition: positions and paints as they
// were when the new graph arrived.
type Frozen = { nodes: { id: string; node: GraphNode; x: number; y: number; paint: Paint }[]; links: { a: string; b: string; dashed: boolean }[] }

type Transition = { kind: 'in' | 'out' | 'fade'; start: number; via: string; at: Point; from: Camera; to: Camera | null }

const inMove = 400
const inSwap = 200
const outMove = 400
const fadeTime = 150
const dimTime = 150
const pulsePeriod = 1600

export class GraphEngine {
  private ctx: CanvasRenderingContext2D
  private sel: Selection<HTMLCanvasElement, unknown, null, undefined>
  private zoomer: ZoomBehavior<HTMLCanvasElement, unknown>
  private sim: Simulation<SimNode, SimLink>
  private nodes: SimNode[] = []
  private links: SimLink[] = []
  private byID = new Map<string, SimNode>()
  private key = ''
  private paint: (n: GraphNode) => Paint = () => ({ fill: 'white', stroke: 'outline', opacity: 1, pulse: false, dashed: false })
  private selected: string | null = null
  private hover: string | null = null
  private dim = 0
  private dimFrom = 0
  private dimStart = 0
  private width = 0
  private height = 0
  private dpr = 1
  private camera: Camera = { x: 0, y: 0, k: 1 }
  private tween: { from: Camera; to: Camera; start: number; duration: number } | null = null
  private follow = false
  private needsFit = false
  private transition: Transition | null = null
  private outgoing: Frozen | null = null
  private holdUntil = 0
  private cache = new Map<string, { at: Map<string, Point>; camera: Camera }>()
  private colors: Record<Color | 'edge' | 'label' | 'selected', string>
  private frame = 0
  private drag: { node: SimNode; x: number; y: number; moved: boolean } | null = null
  private down: { x: number; y: number } | null = null
  private reduced: boolean
  private observer: ResizeObserver

  private canvas: HTMLCanvasElement
  private cb: Callbacks

  constructor(canvas: HTMLCanvasElement, cb: Callbacks) {
    this.canvas = canvas
    this.cb = cb
    this.ctx = canvas.getContext('2d')!
    this.colors = readColors()
    this.reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    this.sim = forceSimulation<SimNode, SimLink>()
      .force('link', forceLink<SimNode, SimLink>().id((d) => d.id).distance((l) => 36 + l.source.node.radius + l.target.node.radius).strength(0.5))
      .force('charge', forceManyBody<SimNode>().strength((d) => -110 - d.node.radius * 8))
      .force('collide', forceCollide<SimNode>((d) => d.node.radius + 6))
      .force('x', forceX<SimNode>(0).strength(0.05))
      .force('y', forceY<SimNode>(0).strength(0.05))
      .stop()
    this.sel = select(canvas)
    this.zoomer = zoom<HTMLCanvasElement, unknown>()
      .scaleExtent([minZoom, maxZoom])
      .filter((e: MouseEvent | WheelEvent | TouchEvent) => {
        if (this.transition) return false
        if (e.type === 'wheel') return true
        if ('button' in e && e.button) return false
        if ('clientX' in e && this.hitAt(this.local(e))) return false
        return true
      })
      .on('zoom', (e) => {
        this.camera = { x: e.transform.x, y: e.transform.y, k: e.transform.k }
        if (e.sourceEvent) {
          this.follow = false
          this.tween = null
        }
        this.request()
      })
    this.sel.call(this.zoomer).on('dblclick.zoom', null)
    canvas.addEventListener('pointerdown', this.pointerDown)
    canvas.addEventListener('pointermove', this.pointerMove)
    canvas.addEventListener('pointerup', this.pointerUp)
    canvas.addEventListener('pointerleave', this.pointerLeave)
    canvas.addEventListener('keydown', this.keyDown)
    this.observer = new ResizeObserver(() => this.resize())
    this.observer.observe(canvas.parentElement ?? canvas)
    this.resize()
  }

  destroy() {
    cancelAnimationFrame(this.frame)
    this.observer.disconnect()
    this.sim.stop()
    this.sel.on('.zoom', null)
    this.canvas.removeEventListener('pointerdown', this.pointerDown)
    this.canvas.removeEventListener('pointermove', this.pointerMove)
    this.canvas.removeEventListener('pointerup', this.pointerUp)
    this.canvas.removeEventListener('pointerleave', this.pointerLeave)
    this.canvas.removeEventListener('keydown', this.keyDown)
  }

  // setScene gives the engine a graph. The same key updates the graph on
  // the canvas in place; a new key replaces it, with a transition when
  // handoff says how the two relate.
  setScene(graph: Graph, key: string, paint: (n: GraphNode) => Paint, handoffFor?: (from: string, to: string) => Handoff | undefined) {
    if (key === this.key) {
      this.paint = paint
      this.merge(graph)
      this.request()
      return
    }
    const handoff = this.key ? handoffFor?.(this.key, key) : undefined
    const now = performance.now()
    const old = this.freeze()
    if (this.key) this.cache.set(this.key, { at: new Map(this.nodes.map((n) => [n.id, { x: n.x!, y: n.y! }])), camera: this.camera })
    const from = this.camera
    this.key = key
    this.paint = paint
    this.setHover(null)
    this.dim = 0
    this.follow = false
    this.tween = null
    const viaOld = handoff?.kind === 'in' ? old.nodes.find((n) => n.id === handoff.from) : undefined
    const cached = this.cache.get(key)
    if (viaOld) {
      this.build(graph, { x: viaOld.x, y: viaOld.y })
    } else {
      this.build(graph, { x: 0, y: 0 }, cached?.at)
    }
    if (!this.width || !old.nodes.length) {
      this.outgoing = null
      this.transition = null
      this.needsFit = !cached
      if (cached) this.setCamera(cached.camera)
      else if (this.width) this.fitNow()
      this.request()
      return
    }
    const target = cached ? cached.camera : this.fitCamera()
    if (this.reduced || !handoff || (handoff.kind === 'in' && !viaOld)) {
      this.outgoing = old
      this.transition = { kind: 'fade', start: now, via: '', at: { x: 0, y: 0 }, from, to: target }
      this.setCamera(target)
    } else if (handoff.kind === 'in') {
      this.outgoing = old
      this.transition = { kind: 'in', start: now, via: handoff.from, at: { x: viaOld!.x, y: viaOld!.y }, from, to: null }
      this.holdUntil = now + inMove
    } else {
      const to = this.byID.get(handoff.to)
      this.outgoing = old
      this.transition = { kind: 'out', start: now, via: handoff.to, at: to ? { x: to.x!, y: to.y! } : { x: 0, y: 0 }, from, to: target }
    }
    this.request()
  }

  setSelected(id: string | null) {
    if (id === this.selected) return
    this.selected = id
    this.startDim()
    this.request()
  }

  // fitView moves the camera to show the whole graph (rule U28).
  fitView() {
    this.follow = false
    this.tweenTo(this.fitCamera(), 300)
  }

  // build replaces the simulation's nodes. New nodes start around at, on a
  // small spiral, so that the same graph lands in the same shape; a node
  // whose position is in known starts there.
  private build(graph: Graph, at: Point, known?: Map<string, Point>) {
    this.nodes = graph.nodes.map((node, i) => {
      const p = known?.get(node.id) ?? spiral(at, i, known ? 30 : 4)
      return { id: node.id, node, x: p.x, y: p.y }
    })
    this.byID = new Map(this.nodes.map((n) => [n.id, n]))
    this.links = this.linksOf(graph)
    this.center(at)
    this.sim.nodes(this.nodes)
    ;(this.sim.force('link') as ReturnType<typeof forceLink<SimNode, SimLink>>).links(this.links)
    if (known && graph.nodes.every((n) => known.has(n.id))) {
      this.sim.alpha(0)
    } else if (known || at.x !== 0 || at.y !== 0) {
      this.sim.alpha(1)
    } else {
      // A map opened directly settles before its first frame.
      this.sim.alpha(1)
      for (let i = 0; i < 300 && this.sim.alpha() > this.sim.alphaMin(); i++) this.sim.tick()
    }
  }

  // merge applies a change to the graph on the canvas: nodes keep their
  // positions, new ones start beside a neighbor.
  private merge(graph: Graph) {
    const next = graph.nodes.map((node, i) => {
      const old = this.byID.get(node.id)
      if (old) {
        old.node = node
        return old
      }
      const near = graph.edges.find((e) => e.source === node.id || e.target === node.id)
      const anchor = near ? this.byID.get(near.source === node.id ? near.target : near.source) : undefined
      const p = spiral(anchor ? { x: anchor.x!, y: anchor.y! } : this.centerPoint(), i, 8)
      return { id: node.id, node, x: p.x, y: p.y } as SimNode
    })
    const changed = next.length !== this.nodes.length || next.some((n, i) => n !== this.nodes[i]) || graph.edges.length !== this.links.length
    this.nodes = next
    this.byID = new Map(this.nodes.map((n) => [n.id, n]))
    this.links = this.linksOf(graph)
    this.sim.nodes(this.nodes)
    ;(this.sim.force('link') as ReturnType<typeof forceLink<SimNode, SimLink>>).links(this.links)
    if (changed) this.sim.alpha(Math.max(this.sim.alpha(), 0.3))
    if (this.hover && !this.byID.has(this.hover)) this.setHover(null)
    this.request()
  }

  private linksOf(graph: Graph): SimLink[] {
    return graph.edges
      .filter((e) => this.byID.has(e.source) && this.byID.has(e.target))
      .map((e) => ({ source: this.byID.get(e.source)!, target: this.byID.get(e.target)!, dashed: e.dashed }))
  }

  private center(at: Point) {
    ;(this.sim.force('x') as ReturnType<typeof forceX<SimNode>>).x(at.x)
    ;(this.sim.force('y') as ReturnType<typeof forceY<SimNode>>).y(at.y)
  }

  private centerPoint(): Point {
    if (!this.nodes.length) return { x: 0, y: 0 }
    const sx = this.nodes.reduce((s, n) => s + n.x!, 0)
    const sy = this.nodes.reduce((s, n) => s + n.y!, 0)
    return { x: sx / this.nodes.length, y: sy / this.nodes.length }
  }

  private freeze(): Frozen {
    return {
      nodes: this.nodes.map((n) => ({ id: n.id, node: n.node, x: n.x!, y: n.y!, paint: this.paint(n.node) })),
      links: this.links.map((l) => ({ a: l.source.id, b: l.target.id, dashed: l.dashed })),
    }
  }

  private fitCamera(): Camera {
    return fit(bounds(this.nodes.map((n) => ({ x: n.x!, y: n.y!, r: n.node.radius }))), this.width, this.height)
  }

  private fitNow() {
    this.needsFit = false
    this.setCamera(this.fitCamera())
  }

  private setCamera(c: Camera) {
    this.camera = c
    this.zoomer.transform(this.sel, zoomIdentity.translate(c.x, c.y).scale(c.k))
  }

  private tweenTo(to: Camera, duration: number) {
    if (this.reduced) {
      this.setCamera(to)
      this.request()
      return
    }
    this.tween = { from: this.camera, to, start: performance.now(), duration }
    this.request()
  }

  private resize() {
    const box = (this.canvas.parentElement ?? this.canvas).getBoundingClientRect()
    if (!box.width || !box.height) return
    this.dpr = window.devicePixelRatio || 1
    if (this.width && this.height && !this.needsFit) {
      // Keep the world point at the center of the view where it was.
      this.setCamera({ ...this.camera, x: this.camera.x + (box.width - this.width) / 2, y: this.camera.y + (box.height - this.height) / 2 })
    }
    this.width = box.width
    this.height = box.height
    this.canvas.width = Math.round(box.width * this.dpr)
    this.canvas.height = Math.round(box.height * this.dpr)
    this.canvas.style.width = `${box.width}px`
    this.canvas.style.height = `${box.height}px`
    if (this.needsFit && this.nodes.length) this.fitNow()
    this.request()
  }

  private request() {
    if (!this.frame) this.frame = requestAnimationFrame(this.loop)
  }

  private loop = (now: number) => {
    this.frame = 0
    let active = false
    if (this.sim.alpha() > this.sim.alphaMin() && now >= this.holdUntil) {
      this.sim.tick()
      active = true
    }
    if (this.transition) active = this.stepTransition(now) || active
    if (this.tween) {
      const t = progress(now, this.tween.start, this.tween.duration)
      this.setCamera(between(this.tween.from, this.tween.to, easeInOut(t), this.width, this.height))
      if (t >= 1) this.tween = null
      else active = true
    }
    if (this.follow) {
      const goal = this.fitCamera()
      this.setCamera(between(this.camera, goal, 0.12, this.width, this.height))
      if (this.sim.alpha() < 0.05) this.follow = false
      active = true
    }
    if (this.dimProgress(now) < 1) active = true
    if (this.nodes.some((n) => this.paint(n.node).pulse)) active = true
    this.draw(now)
    if (active) this.request()
  }

  // stepTransition moves the camera of a transition, and reports whether
  // the transition still runs.
  private stepTransition(now: number): boolean {
    const tr = this.transition!
    const t = now - tr.start
    if (tr.kind === 'fade') {
      if (t >= fadeTime) this.endTransition()
      return t < fadeTime
    }
    if (tr.kind === 'in') {
      this.setCamera(between(tr.from, focus(tr.at, this.width, this.height, 3), easeInOut(progress(now, tr.start, inMove)), this.width, this.height))
      if (t >= inMove + inSwap) {
        this.endTransition()
        this.follow = true
      }
      return true
    }
    this.setCamera(between(tr.from, tr.to!, easeInOut(progress(now, tr.start, outMove)), this.width, this.height))
    if (t >= outMove + 100) this.endTransition()
    return true
  }

  private endTransition() {
    this.transition = null
    this.outgoing = null
  }

  // incomingAlpha and outgoingAlpha give each node's opacity during a
  // transition (rules U41, U42, U44).
  private incomingAlpha(id: string, now: number): number {
    const tr = this.transition
    if (!tr) return 1
    const t = now - tr.start
    if (tr.kind === 'fade') return progress(now, tr.start, fadeTime)
    if (tr.kind === 'in') return progress(t, inMove, inSwap)
    return id === tr.via ? progress(t, 0, 300) : progress(t, 200, 300)
  }

  private outgoingAlpha(id: string, now: number): number {
    const tr = this.transition!
    const t = now - tr.start
    if (tr.kind === 'fade') return 1 - progress(now, tr.start, fadeTime)
    if (tr.kind === 'in') return id === tr.via ? 1 - progress(t, inMove, inSwap) : 1 - progress(t, 0, inMove * 0.75)
    return 1 - progress(t, 0, outMove)
  }

  private outgoingPosition(p: Point, now: number): Point {
    const tr = this.transition!
    if (tr.kind !== 'out') return p
    const e = easeInOut(progress(now, tr.start, outMove))
    return { x: p.x + (tr.at.x - p.x) * e, y: p.y + (tr.at.y - p.y) * e }
  }

  private focusSet(): Set<string> | null {
    const id = this.hover ?? this.selected
    if (!id || !this.byID.has(id)) return null
    const out = new Set([id])
    for (const l of this.links) {
      if (l.source.id === id) out.add(l.target.id)
      if (l.target.id === id) out.add(l.source.id)
    }
    return out
  }

  private startDim() {
    this.dimFrom = this.currentDim(performance.now())
    this.dimStart = performance.now()
  }

  private dimProgress(now: number) {
    return progress(now, this.dimStart, dimTime)
  }

  private currentDim(now: number) {
    const target = this.focusSet() ? 1 : 0
    return this.dimFrom + (target - this.dimFrom) * this.dimProgress(now)
  }

  private draw(now: number) {
    const { ctx, camera: cam } = this
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0)
    ctx.clearRect(0, 0, this.width, this.height)
    this.dim = this.currentDim(now)
    const lit = this.focusSet()
    const shade = (id: string) => (lit && !lit.has(id) ? 1 - 0.8 * this.dim : 1)
    ctx.save()
    ctx.translate(cam.x, cam.y)
    ctx.scale(cam.k, cam.k)
    const labels: { text: string; at: Point; r: number; alpha: number }[] = []
    if (this.outgoing && this.transition) {
      const pos = new Map(this.outgoing.nodes.map((n) => [n.id, this.outgoingPosition(n, now)]))
      for (const l of this.outgoing.links) {
        const alpha = Math.min(this.outgoingAlpha(l.a, now), this.outgoingAlpha(l.b, now))
        this.drawEdge(pos.get(l.a)!, pos.get(l.b)!, l.dashed, alpha)
      }
      for (const n of this.outgoing.nodes) {
        const alpha = this.outgoingAlpha(n.id, now)
        if (alpha <= 0.01) continue
        const p = pos.get(n.id)!
        this.drawNode(n.node, p, n.paint, alpha, false, now)
        labels.push({ text: n.node.label, at: p, r: n.node.radius, alpha })
      }
    }
    for (const l of this.links) {
      const alpha = Math.min(this.incomingAlpha(l.source.id, now), this.incomingAlpha(l.target.id, now))
      const touches = lit && lit.has(l.source.id) && lit.has(l.target.id) && (l.source.id === (this.hover ?? this.selected) || l.target.id === (this.hover ?? this.selected))
      this.drawEdge(l.source as Point, l.target as Point, l.dashed, alpha * (lit && !touches ? 1 - 0.8 * this.dim : 1))
    }
    for (const n of this.nodes) {
      const alpha = this.incomingAlpha(n.id, now) * shade(n.id)
      if (alpha <= 0.01) continue
      this.drawNode(n.node, n as Point, this.paint(n.node), alpha, n.id === this.selected, now)
      if (cam.k >= 0.6 || (lit && lit.has(n.id)) || n.id === this.selected) {
        labels.push({ text: n.node.label, at: n as Point, r: n.node.radius, alpha })
      }
    }
    ctx.restore()
    ctx.font = '11px ui-sans-serif, system-ui, sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillStyle = this.colors.label
    for (const l of labels) {
      const s = toScreen(cam, l.at)
      ctx.globalAlpha = l.alpha
      ctx.fillText(l.text, s.x, s.y + l.r * cam.k + 4)
    }
    ctx.globalAlpha = 1
  }

  private drawEdge(a: Point, b: Point, dashed: boolean, alpha: number) {
    if (alpha <= 0.01) return
    const { ctx } = this
    const k = this.camera.k
    ctx.globalAlpha = alpha
    ctx.strokeStyle = this.colors.edge
    ctx.lineWidth = 1 / k
    ctx.setLineDash(dashed ? [4 / k, 3 / k] : [])
    ctx.beginPath()
    ctx.moveTo(a.x, a.y)
    ctx.lineTo(b.x, b.y)
    ctx.stroke()
    ctx.setLineDash([])
  }

  private drawNode(node: GraphNode, p: Point, paint: Paint, alpha: number, selected: boolean, now: number) {
    const { ctx } = this
    const k = this.camera.k
    const r = node.radius
    shapePath(ctx, node.shape, p, r)
    ctx.globalAlpha = alpha
    ctx.fillStyle = this.colors.white
    ctx.fill()
    ctx.globalAlpha = alpha * paint.opacity
    ctx.fillStyle = this.colors[paint.fill]
    ctx.fill()
    ctx.globalAlpha = alpha
    ctx.strokeStyle = this.colors[paint.stroke]
    ctx.lineWidth = 1.5 / k
    ctx.setLineDash(paint.dashed ? [3 / k, 2 / k] : [])
    ctx.stroke()
    ctx.setLineDash([])
    if (paint.pulse && !this.reduced) {
      const s = (now % pulsePeriod) / pulsePeriod
      shapePath(ctx, node.shape, p, r + (2 + 6 * s) / k)
      ctx.globalAlpha = alpha * (1 - s) * 0.7
      ctx.strokeStyle = this.colors[paint.stroke]
      ctx.lineWidth = 1.5 / k
      ctx.stroke()
    }
    if (selected) {
      shapePath(ctx, node.shape, p, r + 3.5 / k)
      ctx.globalAlpha = alpha
      ctx.strokeStyle = this.colors.selected
      ctx.lineWidth = 2 / k
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  }

  private local(e: { clientX: number; clientY: number }): Point {
    const box = this.canvas.getBoundingClientRect()
    return { x: e.clientX - box.left, y: e.clientY - box.top }
  }

  private hitAt(screen: Point): SimNode | null {
    if (this.transition) return null
    const w = toWorld(this.camera, screen)
    for (let i = this.nodes.length - 1; i >= 0; i--) {
      const n = this.nodes[i]
      const pad = 4 / this.camera.k
      if (Math.hypot(n.x! - w.x, n.y! - w.y) <= n.node.radius + pad) return n
    }
    return null
  }

  private setHover(n: SimNode | null) {
    const id = n?.id ?? null
    if (id === this.hover) return
    this.hover = id
    this.startDim()
    this.canvas.style.cursor = n ? 'pointer' : ''
    this.cb.onHover(n?.node ?? null, n ? toScreen(this.camera, n as Point) : { x: 0, y: 0 })
    this.request()
  }

  private pointerDown = (e: PointerEvent) => {
    const p = this.local(e)
    const hit = this.hitAt(p)
    this.down = p
    if (hit) {
      this.drag = { node: hit, x: p.x, y: p.y, moved: false }
      this.canvas.setPointerCapture(e.pointerId)
    }
  }

  private pointerMove = (e: PointerEvent) => {
    const p = this.local(e)
    const d = this.drag
    if (d) {
      if (!d.moved && Math.hypot(p.x - d.x, p.y - d.y) > 3) {
        d.moved = true
        this.sim.alphaTarget(0.1)
      }
      if (d.moved) {
        const w = toWorld(this.camera, p)
        d.node.fx = w.x
        d.node.fy = w.y
        this.request()
      }
      return
    }
    if (e.buttons) return
    this.setHover(this.hitAt(p))
  }

  private pointerUp = (e: PointerEvent) => {
    const p = this.local(e)
    const d = this.drag
    this.drag = null
    if (d) {
      d.node.fx = null
      d.node.fy = null
      this.sim.alphaTarget(0)
      if (!d.moved) this.cb.onClick(d.node.id)
      this.request()
    } else if (this.down && Math.hypot(p.x - this.down.x, p.y - this.down.y) < 4 && !this.transition) {
      this.cb.onClick(null)
    }
    this.down = null
  }

  private pointerLeave = () => {
    if (!this.drag) this.setHover(null)
  }

  private keyDown = (e: KeyboardEvent) => {
    const order = [...this.nodes].sort((a, b) => a.node.label.localeCompare(b.node.label))
    if (e.key === 'Tab' && order.length) {
      const i = order.findIndex((n) => n.id === this.selected)
      const next = e.shiftKey ? (i < 0 ? order.length - 1 : i - 1) : i + 1
      if (next < 0 || next >= order.length) return
      e.preventDefault()
      this.cb.onSelect(order[next].id)
    } else if (e.key === 'Enter' && this.selected) {
      e.preventDefault()
      this.cb.onOpen(this.selected)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (this.selected) this.cb.onSelect(null)
      else this.cb.onEscape()
    }
  }
}

// spiral places node i on a small sunflower spiral around at, so that the
// same graph starts in the same shape.
function spiral(at: Point, i: number, step: number): Point {
  const r = step * Math.sqrt(0.5 + i)
  const a = i * Math.PI * (3 - Math.sqrt(5))
  return { x: at.x + r * Math.cos(a), y: at.y + r * Math.sin(a) }
}

function shapePath(ctx: CanvasRenderingContext2D, shape: GraphNode['shape'], p: Point, r: number) {
  ctx.beginPath()
  if (shape === 'square') {
    ctx.roundRect(p.x - r, p.y - r, 2 * r, 2 * r, r * 0.3)
  } else if (shape === 'hexagon') {
    for (let i = 0; i < 6; i++) {
      const a = Math.PI / 6 + (i * Math.PI) / 3
      const x = p.x + r * 1.1 * Math.cos(a)
      const y = p.y + r * 1.1 * Math.sin(a)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    }
    ctx.closePath()
  } else if (shape === 'diamond') {
    const d = r * 1.25
    ctx.moveTo(p.x, p.y - d)
    ctx.lineTo(p.x + d, p.y)
    ctx.lineTo(p.x, p.y + d)
    ctx.lineTo(p.x - d, p.y)
    ctx.closePath()
  } else {
    ctx.arc(p.x, p.y, r, 0, 2 * Math.PI)
  }
}

// readColors reads the lens color tokens once (rule U46).
function readColors(): Record<Color | 'edge' | 'label' | 'selected', string> {
  const style = getComputedStyle(document.documentElement)
  const v = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback
  return {
    active: v('--lens-active', '#06b6d4'),
    waiting: v('--lens-waiting', '#a5d8e2'),
    pass: v('--lens-pass', '#10b981'),
    pending: v('--lens-pending', '#facc15'),
    fail: v('--lens-fail', '#ef4444'),
    dirty: v('--lens-dirty', '#d946ef'),
    none: v('--lens-none', '#d4d4d4'),
    outline: v('--lens-outline', '#a3a3a3'),
    feature: v('--kind-feature', '#3b82f6'),
    story: v('--kind-story', '#8b5cf6'),
    rule: v('--kind-rule', '#f59e0b'),
    principle: v('--kind-principle', '#475569'),
    plan: v('--kind-plan', '#a16207'),
    white: '#ffffff',
    edge: v('--graph-edge', '#d4d4d4'),
    label: v('--graph-label', '#525252'),
    selected: v('--graph-selected', '#171717'),
  }
}
