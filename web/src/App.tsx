import { useCallback, useEffect, useRef, useState } from 'react'
import { AppSidebar } from '@/components/app-sidebar'
import { AssistantPanel } from '@/components/assistant-panel'
import { Cat } from '@/components/cat'
import { Composer } from '@/components/composer'
import { ConversationHeader } from '@/components/conversation-header'
import { FeedView } from '@/components/feed-view'
import { MapView } from '@/components/map-view'
import { MessageList } from '@/components/message-list'
import { PublicWiki } from '@/components/public-wiki'
import { SignIn } from '@/components/sign-in'
import { ReleaseView } from '@/components/release-view'
import { SharedView } from '@/components/shared-view'
import { PartitionDetail } from '@/components/partition-detail'
import { TopologyView } from '@/components/topology-view'
import { VocabularyPanel } from '@/components/vocabulary-panel'
import { WorkView } from '@/components/work-view'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useHikari } from '@/hooks/use-hikari'
import { moodOf, pendingPrompt } from '@/lib/assistant'
import { orchestrationOf } from '@/lib/rules'
import { viewKey } from '@/lib/view'

export default function App() {
  const { state, show, send, setPanel } = useHikari()
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const key = viewKey(state.view)
  const draft = drafts[key] ?? ''
  const setDraft = useCallback((text: string) => setDrafts((d) => ({ ...d, [key]: text })), [key])

  // Esc closes the panel (rule HA12).
  useEffect(() => {
    if (!state.panel) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPanel(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [state.panel, setPanel])

  if (state.connection === 'signed-out') return <SignIn />
  if (!state.loaded) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Connecting…</div>
  }

  const v = state.view
  let main: React.ReactNode
  if (v.kind === 'all') {
    main = <FeedView state={state} show={show} />
  } else if (v.kind === 'map') {
    main = <MapView state={state} show={show} />
  } else if (v.kind === 'partition') {
    main = <PartitionDetail state={state} partition={state.partitions.find((x) => x.partition_agent === v.name)!} />
  } else if (v.kind === 'release') {
    main = <ReleaseView key={`${v.name}/${v.deliverable}@${v.version}`} partition={v.name} deliverable={v.deliverable} version={v.version} />
  } else if (v.kind === 'shared') {
    main = <SharedView />
  } else if (v.kind === 'work') {
    main = <WorkView state={state} show={show} task={v.task} send={send} />
  } else if (v.kind === 'room' && v.tab === 'wiki') {
    main = <PublicWiki orchestration={orchestrationOf(state.participants)} wiki={state.wikis.find((w) => w.ref === 'public')} show={show} />
  } else if (v.kind === 'room' && v.tab === 'vocabulary') {
    main = <VocabularyPanel orchestration={orchestrationOf(state.participants)} vocabulary={state.vocabulary} />
  } else {
    main = <MessageList state={state} />
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
        <SidebarInset className="h-svh min-w-0 flex-row">
          <div className="relative flex h-full min-w-0 flex-1 flex-col">
            {v.kind === 'topology' ? (
              <TopologyView state={state} show={show} />
            ) : (
              <>
                <ConversationHeader state={state} show={show} onMention={mention} />
                {main}
                {v.kind !== 'map' && v.kind !== 'work' && <Composer key={key} state={state} draft={draft} setDraft={setDraft} send={send} inputRef={inputRef} />}
              </>
            )}
            {state.mode === 'running' && (
              <Cat
                mood={moodOf(state)}
                attention={pendingPrompt(state) ? 'prompt' : state.assistantUnread ? 'unread' : undefined}
                open={state.panel}
                onClick={() => setPanel(!state.panel)}
                className="absolute top-1.5 right-3"
              />
            )}
          </div>
          {state.mode === 'running' && state.panel && <AssistantPanel state={state} close={() => setPanel(false)} />}
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  )
}
