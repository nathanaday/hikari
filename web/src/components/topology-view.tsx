import { useCallback, useEffect, useState } from 'react'
import { FileCode2 } from 'lucide-react'
import { FindingsList } from '@/components/findings-list'
import { YamlTab } from '@/components/yaml-tab'
import { Button } from '@/components/ui/button'
import { SidebarTrigger } from '@/components/ui/sidebar'
import type { State } from '@/lib/store'
import { createProject, getTopology, hasError, modeLabel, startAgents, stopAgents } from '@/lib/topology'
import type { Finding, Mode, TopologyState } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { TopologyTab, View } from '@/lib/view'

const modeDot: Record<Mode, string> = {
  editing: 'border border-[var(--lens-outline)] bg-white',
  starting: 'bg-[var(--lens-active)] animate-pulse',
  running: 'bg-[var(--lens-pass)]',
  stopping: 'bg-[var(--lens-active)] animate-pulse',
}

// useTopology loads the topology API's state when the view opens, and
// again when the file's revision or the mode changes (rule YS2).
function useTopology(revision: string, mode: Mode) {
  const [data, setData] = useState<TopologyState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let stale = false
    getTopology().then((res) => {
      if (stale) return
      if (res.ok) {
        setData(res.data)
        setError(null)
      } else setError(res.error)
    })
    return () => {
      stale = true
    }
  }, [revision, mode, tick])
  const reload = useCallback(() => setTick((t) => t + 1), [])
  return { data, error, reload }
}

// TopologyView is the compose file that the project runs from, with Start
// and Stop, and its Structure and YAML tabs (rules YV1 to YV6).
export function TopologyView({ state, show }: { state: State; show: (v: View) => void }) {
  const { data, error, reload } = useTopology(state.revision, state.mode)
  const [runError, setRunError] = useState<{ message: string; findings: Finding[] } | null>(null)
  const tab: TopologyTab = state.view.kind === 'topology' && state.view.tab === 'yaml' ? 'yaml' : 'structure'

  const start = async () => {
    const res = await startAgents()
    setRunError(res.ok ? null : { message: res.error, findings: res.findings ?? [] })
  }
  const stop = async () => {
    const res = await stopAgents()
    setRunError(res.ok ? null : { message: res.error, findings: [] })
  }

  const savedErrors = data ? hasError(data.findings) : false
  const tabButton = (t: TopologyTab, label: string) => (
    <button
      type="button"
      onClick={() => show({ kind: 'topology', tab: t === 'structure' ? undefined : t })}
      aria-pressed={tab === t}
      className={cn(
        '-mb-px border-b-2 px-1 pb-2 text-sm transition-colors',
        tab === t ? 'border-foreground font-medium' : 'border-transparent text-muted-foreground hover:text-foreground',
      )}
    >
      {label}
    </button>
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="topology-view">
      <header className="flex shrink-0 items-start gap-2 border-b px-4 py-3 md:px-6">
        <SidebarTrigger className="-ml-2 md:hidden" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <h1 className="flex items-center gap-1.5 text-base font-semibold">
              <FileCode2 className="size-4 text-muted-foreground" />
              Topology
            </h1>
            <div className="flex items-center gap-3">
              <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground" data-testid="mode">
                <span className={cn('inline-block size-2 rounded-full', modeDot[state.mode])} />
                Agents {modeLabel[state.mode]}
              </span>
              {state.mode === 'running' ? (
                <Button size="sm" variant="outline" onClick={stop}>
                  Stop agents
                </Button>
              ) : (
                <Button
                  size="sm"
                  onClick={start}
                  disabled={state.mode !== 'editing' || !data?.exists || savedErrors}
                  title={savedErrors ? 'The compose file has errors; fix them first' : undefined}
                  data-testid="start"
                >
                  Start agents
                </Button>
              )}
            </div>
          </div>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {data?.exists ? (
              <>
                The topology is <code className="text-xs">{data.file}</code>, a Docker compose file. {data.commits ? 'Each save is a commit in the project repository.' : 'Saves are not committed: the topology is not in a project repository.'}
              </>
            ) : (
              'The topology is a Docker compose file. Its x-hikari keys describe the hubs, the agents, and their systems.'
            )}
          </p>
          {data?.exists && (
            <div className="mt-3 -mb-3 flex gap-4">
              {tabButton('structure', 'Structure')}
              {tabButton('yaml', 'YAML')}
            </div>
          )}
        </div>
      </header>

      {(runError || state.startFindings.length > 0) && (
        <div className="shrink-0 border-b bg-[color-mix(in_oklch,var(--lens-fail)_6%,white)] px-4 py-2 md:px-6" data-testid="start-findings">
          <p className="text-sm font-medium">{runError ? runError.message : 'The agents did not start'}</p>
          <div className="-mx-3">
            <FindingsList findings={runError ? runError.findings : state.startFindings} onLine={(l) => show({ kind: 'topology', tab: 'yaml', line: l })} empty="" />
          </div>
        </div>
      )}

      {error && <p className="px-6 py-4 text-sm text-red-600">{error}</p>}
      {data && !data.exists && <CreateProject file={data.file} onCreated={() => (reload(), show({ kind: 'topology', tab: 'yaml' }))} />}
      {data?.exists && tab === 'yaml' && <YamlTab data={data} mode={state.mode} reload={reload} line={state.view.kind === 'topology' ? state.view.line : undefined} />}
      {data?.exists && tab === 'structure' && (
        <div className="flex-1 px-6 py-8 text-sm text-muted-foreground">
          The form of the topology is not built yet.{' '}
          <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'topology', tab: 'yaml' })}>
            Edit the file in the YAML tab
          </button>
          .
        </div>
      )}
    </div>
  )
}

// CreateProject offers to create the compose file, and says what create
// writes (rule YV6).
function CreateProject({ file, onCreated }: { file: string; onCreated: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const create = async () => {
    setBusy(true)
    const res = await createProject()
    setBusy(false)
    if (res.ok) onCreated()
    else setError(res.error)
  }
  return (
    <div className="flex-1 overflow-y-auto px-4 py-10" data-testid="create-project">
      <div className="mx-auto max-w-xl space-y-4">
        <h2 className="text-lg font-semibold">There is no {file} yet</h2>
        <p className="text-sm text-muted-foreground">
          hikari reads the topology from {file}. Create the project, and hikari writes a starter topology with one orchestrator and one hub.
        </p>
        <ul className="space-y-1.5 text-sm">
          <li>
            In an empty directory, it makes a project repository: the compose file, the store, <code className="text-xs">.env.example</code>,{' '}
            <code className="text-xs">.gitignore</code>, and the host skills, in a first commit.
          </li>
          <li>In a directory that holds other files, it writes only the compose file.</li>
        </ul>
        <div className="flex items-center gap-3">
          <Button onClick={create} disabled={busy} data-testid="create">
            Create the project
          </Button>
          {error && <span className="text-sm text-red-600">{error}</span>}
        </div>
      </div>
    </div>
  )
}
