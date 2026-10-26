import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DiffView } from '@/components/diff-view'
import { FindingsList } from '@/components/findings-list'
import type { YamlEditorHandle } from '@/components/yaml-editor'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { diffText, hasError, lintText, markers, putSource } from '@/lib/topology'
import type { Finding, Mode, TopologyState } from '@/lib/types'

// CodeMirror loads with the YAML tab, so that the rest of the UI does not
// wait for it.
const YamlEditor = lazy(() => import('@/components/yaml-editor').then((m) => ({ default: m.YamlEditor })))

type Base = { revision: string; text: string }

// YamlTab edits the compose file as text (rules YY1 to YY8): lint as you
// type, the findings beside the editor, and a save that shows its diff.
export function YamlTab({ data, mode, reload, line }: { data: TopologyState; mode: Mode; reload: () => void; line?: number }) {
  const editor = useRef<YamlEditorHandle>(null)
  // base is the saved text that the buffer started from.
  const [base, setBase] = useState<Base>({ revision: data.revision, text: data.text })
  const [buffer, setBuffer] = useState(data.text)
  const [findings, setFindings] = useState<Finding[]>(data.findings)
  // stale is the file's text when it changed on disk under a change of yours.
  const [stale, setStale] = useState<Base | null>(null)
  const [review, setReview] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const dirty = buffer !== base.text

  // A new revision of the file: take it, or offer it (decision 3, YY8).
  useEffect(() => {
    if (data.revision === base.revision) return
    if (buffer === base.text) {
      setBase({ revision: data.revision, text: data.text })
      setBuffer(data.text)
      setFindings(data.findings)
      setStale(null)
    } else setStale({ revision: data.revision, text: data.text })
    // Only a new revision from the server starts this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data.revision])

  // Lint half a second after the last keystroke (YY2).
  useEffect(() => {
    if (buffer === data.text && data.revision === base.revision) {
      setFindings(data.findings)
      return
    }
    const t = window.setTimeout(async () => {
      const res = await lintText(buffer)
      if (res.ok) setFindings(res.data.findings)
    }, 500)
    return () => window.clearTimeout(t)
  }, [buffer, data, base.revision])

  useEffect(() => {
    if (line) editor.current?.goto(line)
  }, [line])

  const marks = useMemo(() => markers(findings, buffer.split('\n').length), [findings, buffer])
  const openReview = useCallback(() => {
    if (buffer !== base.text) setReview(true)
  }, [buffer, base.text])

  const loadFile = (b: Base) => {
    setBase(b)
    setBuffer(b.text)
    setStale(null)
  }
  const keepMine = (b: Base) => {
    setBase(b)
    setStale(null)
  }

  const save = async () => {
    const res = await putSource(base.revision, buffer)
    if (res.ok) {
      setBase({ revision: res.data.revision, text: buffer })
      setReview(false)
      setNotice(res.data.commit ? `Saved and committed ${res.data.commit.slice(0, 7)}.` : `Saved. ${res.data.note ?? ''}`)
      reload()
    } else if (res.conflict) {
      setStale({ revision: res.conflict.revision, text: res.conflict.text })
      setReview(false)
    } else setNotice(res.error)
  }

  const errors = findings.filter((f) => f.severity === 'error').length
  const warnings = findings.length - errors

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="yaml-tab">
      {stale && (
        <div className="flex shrink-0 flex-wrap items-center gap-3 border-b bg-[color-mix(in_oklch,var(--lens-pending)_14%,white)] px-4 py-2 text-sm md:px-6" data-testid="stale">
          <span className="flex-1">The file changed on disk while you edited it.</span>
          <Button size="xs" variant="outline" onClick={() => loadFile(stale)}>
            Load the file's text
          </Button>
          <Button size="xs" variant="outline" onClick={() => keepMine(stale)}>
            Keep mine
          </Button>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <div className="min-h-64 min-w-0 flex-1">
          <Suspense fallback={<p className="px-4 py-3 text-sm text-muted-foreground">Loading the editor…</p>}>
            <YamlEditor ref={editor} value={buffer} markers={marks} onChange={setBuffer} onSave={openReview} />
          </Suspense>
        </div>
        <aside className="flex max-h-60 shrink-0 flex-col border-t md:max-h-none md:w-80 md:border-t-0 md:border-l" data-testid="problems">
          <div className="flex items-baseline justify-between border-b px-3 py-2">
            <h2 className="text-sm font-medium">Problems</h2>
            <span className="text-xs text-muted-foreground">
              {errors} {errors === 1 ? 'error' : 'errors'}, {warnings} {warnings === 1 ? 'warning' : 'warnings'}
            </span>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            <FindingsList findings={findings} onLine={(l) => editor.current?.goto(l)} />
          </div>
        </aside>
      </div>
      {(dirty || notice) && (
        <div className="flex shrink-0 items-center gap-3 border-t bg-neutral-50 px-4 py-2 md:px-6" data-testid="save-bar">
          <span className="flex-1 text-sm text-muted-foreground">{dirty ? 'Unsaved changes' : notice}</span>
          {dirty && (
            <>
              <Button size="sm" variant="ghost" onClick={() => setBuffer(base.text)}>
                Revert
              </Button>
              <Button size="sm" onClick={openReview}>
                Review and save
                <kbd className="ml-1 text-[11px] opacity-60">⌘S</kbd>
              </Button>
            </>
          )}
        </div>
      )}
      <SaveReview open={review} onOpenChange={setReview} before={base.text} after={buffer} findings={findings} running={mode === 'running'} onSave={save} />
    </div>
  )
}

// SaveReview shows what a save of the YAML writes before it writes it
// (rules YY5 to YY7).
export function SaveReview({
  open,
  onOpenChange,
  before,
  after,
  findings,
  running,
  onSave,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  before: string
  after: string
  findings: Finding[]
  running: boolean
  onSave: () => void
}) {
  const diff = useMemo(() => (open ? diffText(before, after) : []), [open, before, after])
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-2xl" data-testid="review">
        <SheetHeader>
          <SheetTitle>Review your changes</SheetTitle>
          <SheetDescription>The lines of the compose file that this save changes.</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4">
          <DiffView diff={diff} />
          {hasError(findings) && <p className="text-sm">The file has errors; hikari will not start until you fix them.</p>}
          {findings.length > 0 && (
            <div className="-mx-3">
              <FindingsList findings={findings} />
            </div>
          )}
          {running && <p className="text-sm text-muted-foreground">The agents run the file they started with. This change applies at the next start.</p>}
        </div>
        <SheetFooter className="flex-row justify-end border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Keep editing
          </Button>
          <Button onClick={onSave} data-testid="save">
            Save
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
