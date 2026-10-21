import { useState, type RefObject } from 'react'
import { ArrowUp, Lock, Zap } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { wakeTargets } from '@/lib/rules'
import type { State } from '@/lib/store'
import type { SendRequest } from '@/lib/types'

type Props = {
  state: State
  draft: string
  setDraft: (text: string) => void
  send: (req: SendRequest) => Promise<string | null>
  inputRef: RefObject<HTMLTextAreaElement | null>
}

export function Composer({ state, draft, setDraft, send, inputRef }: Props) {
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const v = state.view
  if (v.kind === 'all') return null

  const hub = v.kind === 'hub' ? state.hubs.find((h) => h.name === v.name) : undefined
  const woken = hub ? wakeTargets(hub, state.me, draft) : []
  const hint = hub ? (
    <>
      <Zap className="size-3" />
      <span data-testid="wake-preview">Wakes {woken.join(', ')}</span>
      <span className="hidden sm:inline">· write @name to wake a member</span>
    </>
  ) : (
    <>
      <Lock className="size-3" />
      Only {v.name} receives this
    </>
  )

  const submit = async () => {
    if (!draft.trim() || sending) return
    setSending(true)
    const req: SendRequest = hub ? { hub: hub.name, body: draft } : { to: v.name, body: draft }
    const err = await send(req)
    setSending(false)
    setError(err)
    if (!err) setDraft('')
    inputRef.current?.focus()
  }

  return (
    <div className="shrink-0 px-4 pb-4 md:px-6">
      {error && <p className="mb-2 text-sm text-red-600">{error}</p>}
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="rounded-lg border bg-background focus-within:border-neutral-400"
      >
        <Textarea
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault()
              submit()
            }
          }}
          rows={1}
          placeholder={hub ? `Message #${hub.name}` : `Message ${v.name}`}
          aria-label="Message"
          className="max-h-48 min-h-11 resize-none border-0 px-3 py-2.5 shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center justify-between gap-2 px-3 pb-2">
          <p className="flex min-w-0 items-center gap-1 truncate text-xs text-muted-foreground">{hint}</p>
          <Button type="submit" size="icon" className="size-7 shrink-0" disabled={!draft.trim() || sending} aria-label="Send">
            <ArrowUp className="size-4" />
          </Button>
        </div>
      </form>
    </div>
  )
}
