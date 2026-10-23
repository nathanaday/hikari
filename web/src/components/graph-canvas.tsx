import { useEffect, useRef, useState } from 'react'
import { Maximize } from 'lucide-react'
import { GraphEngine, type Handoff } from '@/components/graph-engine'
import type { Point } from '@/lib/camera'
import type { Graph, GraphNode } from '@/lib/graph'
import type { Paint } from '@/lib/lens'

type Props = {
  graph: Graph
  // graphKey names the graph; a new key replaces the graph on the canvas.
  graphKey: string
  handoff?: Handoff
  paint: (n: GraphNode) => Paint
  // card returns the hover card's lines for a node: a title and details.
  card: (n: GraphNode) => { title: string; lines: string[] }
  selected: string | null
  onClick: (id: string | null) => void
  onOpen: (id: string) => void
  onEscape: () => void
  children?: React.ReactNode
}

// GraphCanvas draws a graph with GraphEngine and shows a hover card and a
// fit button over it (rules U20 to U29).
export function GraphCanvas({ graph, graphKey, handoff, paint, card, selected, onClick, onOpen, onEscape, children }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const engineRef = useRef<GraphEngine | null>(null)
  const callbacks = useRef({ onClick, onOpen, onEscape })
  const [hover, setHover] = useState<{ node: GraphNode; at: Point } | null>(null)

  useEffect(() => {
    callbacks.current = { onClick, onOpen, onEscape }
  })

  useEffect(() => {
    const engine = new GraphEngine(canvasRef.current!, {
      onClick: (id) => callbacks.current.onClick(id),
      onOpen: (id) => callbacks.current.onOpen(id),
      onEscape: () => callbacks.current.onEscape(),
      onHover: (node, at) => setHover(node ? { node, at } : null),
    })
    engineRef.current = engine
    return () => {
      engine.destroy()
      engineRef.current = null
    }
  }, [])

  useEffect(() => {
    engineRef.current?.setScene(graph, graphKey, paint, handoff)
    // handoff applies only when the key changes; paint has its own effect.
  }, [graph, graphKey])

  useEffect(() => {
    engineRef.current?.setPaint(paint)
  }, [paint])

  useEffect(() => {
    engineRef.current?.setSelected(selected)
  }, [selected])

  const info = hover ? card(hover.node) : null
  return (
    <div className="relative min-h-0 flex-1 overflow-hidden bg-white" data-testid="graph-canvas">
      <canvas ref={canvasRef} tabIndex={0} aria-label="Graph" className="absolute inset-0 touch-none outline-none" />
      {hover && info && (
        <div
          className="pointer-events-none absolute z-10 max-w-64 rounded-md border bg-white px-2.5 py-1.5 text-xs shadow-sm"
          style={{ left: hover.at.x + 14, top: hover.at.y + 14 }}
          data-testid="hover-card"
        >
          <div className="font-medium">{info.title}</div>
          {info.lines.filter(Boolean).map((l) => (
            <div key={l} className="text-muted-foreground">
              {l}
            </div>
          ))}
        </div>
      )}
      <button
        type="button"
        onClick={() => engineRef.current?.fitView()}
        className="absolute right-3 bottom-3 rounded-md border bg-white p-1.5 text-muted-foreground shadow-sm hover:text-foreground"
        aria-label="Fit the graph to the view"
        title="Fit the graph to the view"
      >
        <Maximize className="size-4" />
      </button>
      {children}
    </div>
  )
}
