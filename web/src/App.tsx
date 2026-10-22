import { useCallback, useRef, useState } from 'react'
import { AppSidebar } from '@/components/app-sidebar'
import { Composer } from '@/components/composer'
import { ConversationHeader } from '@/components/conversation-header'
import { MessageList } from '@/components/message-list'
import { SignIn } from '@/components/sign-in'
import { ReleaseView } from '@/components/release-view'
import { SharedView } from '@/components/shared-view'
import { SystemDetail } from '@/components/system-detail'
import { VocabularyPanel } from '@/components/vocabulary-panel'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useHikari } from '@/hooks/use-hikari'
import { viewKey } from '@/lib/view'

export default function App() {
  const { state, show, send } = useHikari()
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const key = viewKey(state.view)
  const draft = drafts[key] ?? ''
  const setDraft = useCallback((text: string) => setDrafts((d) => ({ ...d, [key]: text })), [key])

  if (state.connection === 'signed-out') return <SignIn />
  if (!state.loaded) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Connecting…</div>
  }

  const v = state.view
  let main: React.ReactNode
  if (v.kind === 'system') {
    main = <SystemDetail state={state} system={state.systems.find((x) => x.owner === v.name)!} />
  } else if (v.kind === 'release') {
    main = <ReleaseView key={`${v.name}/${v.deliverable}@${v.version}`} system={v.name} deliverable={v.deliverable} version={v.version} />
  } else if (v.kind === 'shared') {
    main = <SharedView />
  } else if (v.kind === 'hub' && v.tab === 'vocabulary') {
    main = <VocabularyPanel hub={state.hubs.find((h) => h.name === v.name)!} vocabulary={state.vocabularies.find((x) => x.hub === v.name)} />
  } else {
    main = <MessageList state={state} show={show} />
  }

  const mention = (name: string) => {
    const sep = draft && !/\s$/.test(draft) ? ' ' : ''
    setDraft(`${draft}${sep}@${name} `)
    inputRef.current?.focus()
  }

  return (
    <TooltipProvider>
      <SidebarProvider>
        <AppSidebar state={state} show={show} />
        <SidebarInset className="h-svh min-w-0">
          <ConversationHeader state={state} show={show} onMention={mention} />
          {main}
          <Composer key={key} state={state} draft={draft} setDraft={setDraft} send={send} inputRef={inputRef} />
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  )
}
