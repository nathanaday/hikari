import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { yaml } from '@codemirror/lang-yaml'
import { lintGutter, setDiagnostics, type Diagnostic } from '@codemirror/lint'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { basicSetup } from 'codemirror'
import type { Marker } from '@/lib/topology'

export type YamlEditorHandle = { goto: (line: number) => void }

type Props = {
  value: string
  markers: Marker[]
  onChange: (text: string) => void
  onSave: () => void
}

// The editor's look follows the UI's: its type, its neutral grays, and the
// lens colors for findings (decision 2 of the YAML view spec).
const theme = EditorView.theme({
  '&': { height: '100%', fontSize: '13px', backgroundColor: 'var(--background)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', lineHeight: '1.6' },
  '.cm-content': { padding: '12px 0' },
  '.cm-gutters': { backgroundColor: 'var(--background)', border: 'none', color: 'var(--muted-foreground)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'oklch(0.985 0 0)' },
  '.cm-lintRange-error': { backgroundImage: 'none', textDecoration: 'underline wavy var(--lens-fail)', textUnderlineOffset: '3px' },
  '.cm-lintRange-warning': { backgroundImage: 'none', textDecoration: 'underline wavy var(--lens-pending)', textUnderlineOffset: '3px' },
  '.cm-lint-marker': { width: '7px', height: '7px', borderRadius: '9999px', margin: '0 4px' },
  '.cm-lint-marker-error': { content: 'none', backgroundImage: 'none', backgroundColor: 'var(--lens-fail)' },
  '.cm-lint-marker-warning': { content: 'none', backgroundImage: 'none', backgroundColor: 'var(--lens-pending)' },
  '.cm-tooltip-lint': { fontFamily: 'inherit', fontSize: '12px' },
  '.cm-diagnostic-error': { borderLeftColor: 'var(--lens-fail)' },
  '.cm-diagnostic-warning': { borderLeftColor: 'var(--lens-pending)' },
})

// YamlEditor edits the topology file's text with CodeMirror 6 (rules YY1 and
// YY2). It owns its document; a new value replaces it only when it differs.
export const YamlEditor = forwardRef<YamlEditorHandle, Props>(function YamlEditor({ value, markers, onChange, onSave }, ref) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const handlers = useRef({ onChange, onSave })
  handlers.current = { onChange, onSave }

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          yaml(),
          lintGutter(),
          theme,
          keymap.of([{ key: 'Mod-s', preventDefault: true, run: () => (handlers.current.onSave(), true) }]),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) handlers.current.onChange(u.state.doc.toString())
          }),
        ],
      }),
    })
    view.current = v
    return () => v.destroy()
    // The editor is made once; value updates arrive through the effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const v = view.current
    if (v && v.state.doc.toString() !== value) v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } })
  }, [value])

  useEffect(() => {
    const v = view.current
    if (!v) return
    const diagnostics: Diagnostic[] = markers.map((m) => {
      const line = v.state.doc.line(Math.min(m.line, v.state.doc.lines))
      return { from: line.from, to: line.to, severity: m.severity, message: m.message }
    })
    v.dispatch(setDiagnostics(v.state, diagnostics))
  }, [markers, value])

  useImperativeHandle(ref, () => ({
    goto: (n: number) => {
      const v = view.current
      if (!v) return
      const line = v.state.doc.line(Math.min(Math.max(1, n), v.state.doc.lines))
      v.dispatch({ selection: { anchor: line.from }, scrollIntoView: true })
      v.focus()
    },
  }))

  return <div ref={host} className="h-full min-h-0 overflow-hidden" data-testid="yaml-editor" />
})
