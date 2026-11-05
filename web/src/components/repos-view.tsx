import { useState } from 'react'
import { ExternalLink, GitBranch, GitMerge, GitPullRequest, RefreshCw, TriangleAlert, Upload } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { RepoDot } from '@/components/app-sidebar'
import { ago, branchLine, openPulls, repoHealth, yourBranch } from '@/lib/repos'
import type { State } from '@/lib/store'
import type { BranchView, CheckState, PullRequestView, RepoView as Repo } from '@/lib/types'
import { cn } from '@/lib/utils'
import type { View } from '@/lib/view'

// call sends one action of a repository, and returns an error message, or
// null when the server took it (rule RP6).
async function call(path: string, body?: unknown): Promise<string | null> {
  const res = await fetch(`/api/ui/repos${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const e = (await res.json().catch(() => ({}))) as { error?: string }
  return e.error ?? res.statusText
}

const checkClass: Record<CheckState, string> = {
  passing: 'bg-emerald-100 text-emerald-800',
  failing: 'bg-red-100 text-red-800',
  pending: 'bg-amber-100 text-amber-800',
  skipped: 'bg-neutral-100 text-neutral-600',
  none: 'bg-neutral-100 text-neutral-600',
}

// ChecksChip sums the checks of a pull request's head; its runs unfold.
export function ChecksChip({ checks }: { checks: PullRequestView['checks'] }) {
  const label = checks.state === 'none' ? 'no checks' : `checks ${checks.state}`
  if (checks.runs.length === 0) return <span className={`rounded px-1.5 py-0.5 text-xs ${checkClass[checks.state]}`}>{label}</span>
  return (
    <details className="inline-block text-xs">
      <summary className={`cursor-pointer list-none rounded px-1.5 py-0.5 ${checkClass[checks.state]}`} data-checks={checks.state}>
        {label} · {checks.passed} passed{checks.failed ? `, ${checks.failed} failed` : ''}
        {checks.pending ? `, ${checks.pending} pending` : ''}
      </summary>
      <ul className="mt-1 space-y-0.5 rounded border bg-white p-1.5">
        {checks.runs.map((r) => (
          <li key={r.name} className="flex items-center gap-1.5">
            <span className={`inline-block size-1.5 rounded-full ${r.state === 'passing' ? 'bg-emerald-500' : r.state === 'failing' ? 'bg-red-500' : r.state === 'pending' ? 'bg-amber-500' : 'bg-neutral-300'}`} />
            {r.url ? (
              <a href={r.url} target="_blank" rel="noreferrer" className="underline underline-offset-2">
                {r.name}
              </a>
            ) : (
              r.name
            )}
            <span className="text-muted-foreground">{r.state}</span>
          </li>
        ))}
      </ul>
    </details>
  )
}

const prState: Record<PullRequestView['state'], string> = {
  open: 'bg-emerald-100 text-emerald-800',
  merged: 'bg-violet-100 text-violet-800',
  closed: 'bg-neutral-100 text-neutral-600',
}

function Chip({ className, children }: { className?: string; children: React.ReactNode }) {
  return <span className={cn('rounded px-1.5 py-0.5 text-xs', className)}>{children}</span>
}

function ForgeLink({ repo }: { repo: Repo }) {
  if (!repo.forge) return repo.url ? <span className="text-xs text-muted-foreground">{repo.url}</span> : <span className="text-xs text-muted-foreground">no origin</span>
  return (
    <a href={repo.forge.web} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs underline underline-offset-2">
      {repo.forge.owner}/{repo.forge.name}
      <ExternalLink className="size-3" />
    </a>
  )
}

// RepositoriesView lists every repository (rule UI1).
export function RepositoriesView({ state, show }: { state: State; show: (v: View) => void }) {
  const repos = [...state.repos].sort((a, b) => Number(!!a.project) - Number(!!b.project) || a.name.localeCompare(b.name))
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 md:px-6" data-testid="repos-view">
      {repos.length === 0 && <p className="text-sm text-muted-foreground">The topology names no repository.</p>}
      <div className="grid gap-3 md:grid-cols-2">
        {repos.map((r) => {
          const open = openPulls(r)
          const failing = open.filter((p) => p.checks.state === 'failing').length
          const pending = open.filter((p) => p.checks.state === 'pending').length
          return (
            <div key={r.name} className="space-y-1.5 rounded-md border p-3" data-repo={r.name}>
              <div className="flex items-center justify-between gap-2">
                <button type="button" className="flex items-center gap-2 text-sm font-semibold hover:underline" onClick={() => show({ kind: 'repo', name: r.name })}>
                  <RepoDot health={repoHealth(r)} />
                  {r.name}
                  {r.project && <span className="text-xs font-normal text-muted-foreground">project repository</span>}
                </button>
                <ForgeLink repo={r} />
              </div>
              <p className="text-sm">
                <code className="text-xs">{r.branch}</code> <span className="text-muted-foreground">· {branchLine(r)}</span>
              </p>
              <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                {r.forge ? (
                  <>
                    <span>
                      {open.length} open pull request{open.length === 1 ? '' : 's'}
                    </span>
                    {failing > 0 && <Chip className={checkClass.failing}>{failing} failing</Chip>}
                    {pending > 0 && <Chip className={checkClass.pending}>{pending} pending</Chip>}
                    {open.length > 0 && failing === 0 && pending === 0 && open.every((p) => p.checks.state === 'passing') && <Chip className={checkClass.passing}>checks passing</Chip>}
                    {r.synced && <span>· synced {ago(r.synced)}</span>}
                  </>
                ) : (
                  <span>no forge</span>
                )}
              </p>
              {r.error && <p className="text-xs text-red-700">{r.error}</p>}
              {r.partitions.length > 0 && <p className="text-xs text-muted-foreground">partitions: {r.partitions.join(', ')}</p>}
              {r.uncovered?.length ? (
                <p className="text-xs text-muted-foreground">
                  {r.uncovered.length} path{r.uncovered.length === 1 ? '' : 's'} in no partition
                </p>
              ) : null}
              {r.drift?.some((d) => d.paths.length > 0) && <Chip className={checkClass.pending}>{r.default_branch} moved under {r.drift.filter((d) => d.paths.length > 0).map((d) => d.partition).join(', ')}</Chip>}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// RepoView is one repository's page (rule UI2): your branch, the pull
// requests, the branches, and the partitions in it.
export function RepoView({ state, name, show }: { state: State; name: string; show: (v: View) => void }) {
  const repo = state.repos.find((r) => r.name === name)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  if (!repo) return <p className="flex-1 py-12 text-center text-sm text-muted-foreground">No repository {name}.</p>
  const run = async (what: string, p: Promise<string | null>) => {
    setBusy(what)
    setError(await p)
    setBusy(null)
  }
  const yours = yourBranch(repo)
  const openPR = yours?.pull_request ? repo.pull_requests.find((p) => p.number === yours.pull_request) : undefined
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="repo-view">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        {error && (
          <p className="flex items-start gap-2 text-sm text-red-600" role="alert">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" />
            {error}
          </p>
        )}
        {repo.error && <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">The forge could not be read: {repo.error}</p>}
        {repo.git_error && <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">The last fetch failed: {repo.git_error}</p>}
        <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
          <ForgeLink repo={repo} />
          {repo.default_branch && (
            <span>
              default branch <code className="text-xs">{repo.default_branch}</code>
            </span>
          )}
          {repo.synced && <span>synced {ago(repo.synced)}</span>}
          {repo.fetched && <span>fetched {ago(repo.fetched)}</span>}
          <Button size="sm" variant="outline" disabled={busy !== null} onClick={() => run('refresh', call(`/${encodeURIComponent(name)}/refresh`))}>
            <RefreshCw className={cn('size-3.5', busy === 'refresh' && 'animate-spin')} />
            Refresh
          </Button>
        </div>

        <YourBranch repo={repo} yours={yours} openPR={openPR} busy={busy} run={run} />

        <section className="space-y-2" data-testid="pull-requests">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Pull requests ({openPulls(repo).length} open)</h2>
          {repo.pull_requests.length === 0 ? (
            <p className="text-sm text-muted-foreground">{repo.forge ? 'No pull requests.' : 'No forge: the origin of this repository is not on a host that hikari knows.'}</p>
          ) : (
            <ul className="divide-y border-y">
              {repo.pull_requests.map((p) => (
                <PullRequestRow key={p.number} repo={repo} pr={p} busy={busy} run={run} />
              ))}
            </ul>
          )}
        </section>

        <section className="space-y-2" data-testid="branches">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Branches ({repo.branches.length})</h2>
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted-foreground">
              <tr>
                <th className="py-1 font-medium">Branch</th>
                <th className="py-1 font-medium">Where</th>
                <th className="py-1 font-medium">Against {repo.default_branch || 'the default branch'}</th>
                <th className="py-1 font-medium">Last commit</th>
              </tr>
            </thead>
            <tbody>
              {repo.branches.map((b) => (
                <BranchRow key={b.name} repo={repo} b={b} />
              ))}
            </tbody>
          </table>
        </section>

        <Uncovered repo={repo} />
        <Drift repo={repo} show={show} />

        <section className="space-y-2">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Partitions</h2>
          {repo.partitions.length === 0 ? (
            <p className="text-sm text-muted-foreground">No partition has a writable folder in this repository.</p>
          ) : (
            <ul className="flex flex-wrap gap-2 text-sm">
              {repo.partitions.map((p) => (
                <li key={p}>
                  <button type="button" className="underline underline-offset-2" onClick={() => show({ kind: 'partition', name: p })}>
                    {p}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  )
}

// Uncovered lists the code that no partition owns: the paths of your branch
// outside every writable folder. An agent can change none of it.
function Uncovered({ repo }: { repo: Repo }) {
  if (repo.project || repo.partitions.length === 0) return null
  const paths = repo.uncovered ?? []
  return (
    <section className="space-y-2" data-testid="uncovered">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Not in any partition ({paths.length})</h2>
      {paths.length === 0 ? (
        <p className="text-sm text-muted-foreground">Every tracked path of this repository is in a partition.</p>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">No agent can change these paths. Give them to a partition in the topology, or leave them yours.</p>
          <ul className="flex flex-wrap gap-1.5">
            {paths.map((p) => (
              <li key={p}>
                <code className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs">{p}</code>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

// Drift lists, for each partition with a folder in the repository, the paths
// the default branch changed since your branch diverged: work the partition
// agent does not see in its workspace.
function Drift({ repo, show }: { repo: Repo; show: (v: View) => void }) {
  const drift = (repo.drift ?? []).filter((d) => d.paths.length > 0)
  if (drift.length === 0) return null
  return (
    <section className="space-y-2 rounded-md border border-amber-200 bg-amber-50 p-3" data-testid="drift">
      <h2 className="text-xs font-medium tracking-wide text-amber-900 uppercase">
        <code>{repo.default_branch}</code> moved under {drift.length === 1 ? 'a partition' : `${drift.length} partitions`}
      </h2>
      <p className="text-sm text-amber-900">
        Since your branch diverged, {repo.default_branch} changed files in these partitions. Their agents work on your branch and do not see these changes until you bring them in.
      </p>
      <ul className="space-y-1.5 text-sm">
        {drift.map((d) => (
          <li key={d.partition} data-drift={d.partition}>
            <button type="button" className="font-medium underline underline-offset-2" onClick={() => show({ kind: 'partition', name: d.partition })}>
              {d.partition}
            </button>{' '}
            <span className="text-muted-foreground">
              {d.paths.length} path{d.paths.length === 1 ? '' : 's'}:
            </span>{' '}
            <span className="text-xs">{d.paths.slice(0, 8).join(', ')}{d.paths.length > 8 ? `, and ${d.paths.length - 8} more` : ''}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function YourBranch({
  repo,
  yours,
  openPR,
  busy,
  run,
}: {
  repo: Repo
  yours?: BranchView
  openPR?: PullRequestView
  busy: string | null
  run: (what: string, p: Promise<string | null>) => void
}) {
  const [opening, setOpening] = useState(false)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [draft, setDraft] = useState(false)
  const canPush = !!repo.url && !!yours?.local && (!yours.remote || yours.unpushed > 0)
  const canOpen = !!repo.forge && !!yours?.remote && yours.unpushed === 0 && !openPR && !yours.default
  const path = `/${encodeURIComponent(repo.name)}`
  return (
    <section className="space-y-2 rounded-md border p-3" data-testid="your-branch">
      <div className="flex flex-wrap items-center gap-2">
        <GitBranch className="size-4 text-muted-foreground" />
        <span className="text-sm font-semibold">Your branch</span>
        <code className="text-xs">{repo.branch}</code>
        {yours?.merged && repo.default_branch && !yours.default && <Badge variant="secondary">merged into {repo.default_branch}</Badge>}
        {repo.dirty && <Badge variant="outline">uncommitted changes</Badge>}
      </div>
      <p className="text-sm text-muted-foreground">{branchLine(repo)}</p>
      {yours?.subject && (
        <p className="text-xs text-muted-foreground">
          last commit: {yours.subject}
          {yours.author ? ` · ${yours.author}` : ''}
          {yours.time ? ` · ${ago(yours.time)}` : ''}
        </p>
      )}
      {openPR && (
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <GitPullRequest className="size-4 text-muted-foreground" />
          <a href={openPR.url} target="_blank" rel="noreferrer" className="underline underline-offset-2">
            #{openPR.number} {openPR.title}
          </a>
          <ChecksChip checks={openPR.checks} />
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={!canPush || busy !== null} onClick={() => run('push', call(`${path}/push`))}>
          <Upload className="size-3.5" />
          Push
        </Button>
        {!openPR && (
          <Button size="sm" variant="secondary" disabled={!canOpen || busy !== null} onClick={() => setOpening((o) => !o)}>
            <GitPullRequest className="size-3.5" />
            Open pull request
          </Button>
        )}
      </div>
      {opening && (
        <form
          className="space-y-2"
          data-testid="new-pull-request"
          onSubmit={(e) => {
            e.preventDefault()
            if (!title.trim()) return
            run(
              'open',
              call(`${path}/pulls`, { title: title.trim(), body, draft }).then((err) => {
                if (!err) {
                  setOpening(false)
                  setTitle('')
                  setBody('')
                }
                return err
              }),
            )
          }}
        >
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Title" aria-label="Pull request title" />
          <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="What changed, and which issues it closes, such as Closes #12." rows={3} aria-label="Pull request body" />
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={draft} onChange={(e) => setDraft(e.target.checked)} />
            draft
          </label>
          <div className="flex gap-2">
            <Button size="sm" type="submit" disabled={!title.trim() || busy !== null}>
              Open {repo.branch} → {repo.default_branch}
            </Button>
            <Button size="sm" type="button" variant="ghost" onClick={() => setOpening(false)}>
              Cancel
            </Button>
          </div>
        </form>
      )}
    </section>
  )
}

function PullRequestRow({ repo, pr, busy, run }: { repo: Repo; pr: PullRequestView; busy: string | null; run: (what: string, p: Promise<string | null>) => void }) {
  const [method, setMethod] = useState('merge')
  const canMerge = pr.state === 'open' && pr.mergeable !== 'conflicting' && pr.checks.state !== 'failing' && !pr.draft
  return (
    <li className="space-y-1 py-2" data-pr={pr.number}>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <a href={pr.url} target="_blank" rel="noreferrer" className="font-medium underline underline-offset-2">
          #{pr.number} {pr.title}
        </a>
        <Chip className={prState[pr.state]}>{pr.state}</Chip>
        {pr.draft && <Chip className="bg-neutral-100 text-neutral-600">draft</Chip>}
        <span className="text-xs text-muted-foreground">
          <code>{pr.head}</code> → <code>{pr.base}</code>
          {pr.author ? ` · by ${pr.author}` : ''}
          {pr.updated ? ` · ${ago(pr.updated)}` : ''}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {pr.state === 'open' && <ChecksChip checks={pr.checks} />}
        {pr.review && <Chip className={pr.review === 'approved' ? 'bg-emerald-100 text-emerald-800' : 'bg-amber-100 text-amber-800'}>{pr.review === 'approved' ? 'approved' : 'changes requested'}</Chip>}
        {pr.state === 'open' && pr.mergeable === 'conflicting' && <Chip className="bg-red-100 text-red-800">conflicts with {pr.base}</Chip>}
        {pr.state === 'open' && pr.mergeable === 'mergeable' && <Chip className="bg-neutral-100 text-neutral-600">mergeable</Chip>}
        {(pr.additions || pr.deletions) && (
          <span>
            <span className="text-emerald-700">+{pr.additions ?? 0}</span> <span className="text-red-700">−{pr.deletions ?? 0}</span>
            {pr.files ? ` in ${pr.files} file${pr.files === 1 ? '' : 's'}` : ''}
          </span>
        )}
        {pr.closes?.length ? <span>closes {pr.closes.map((n) => `#${n}`).join(', ')}</span> : null}
        {canMerge && (
          <span className="flex items-center gap-1">
            <select aria-label="Merge method" className="h-7 rounded-md border bg-background px-1.5 text-xs" value={method} onChange={(e) => setMethod(e.target.value)}>
              <option value="merge">merge</option>
              <option value="squash">squash</option>
              <option value="rebase">rebase</option>
            </select>
            <Button size="sm" variant="secondary" disabled={busy !== null} onClick={() => run('merge', call(`/${encodeURIComponent(repo.name)}/pulls/${pr.number}/merge`, { method }))}>
              <GitMerge className="size-3.5" />
              Merge
            </Button>
          </span>
        )}
      </div>
    </li>
  )
}

function BranchRow({ repo, b }: { repo: Repo; b: BranchView }) {
  const pr = b.pull_request ? repo.pull_requests.find((p) => p.number === b.pull_request) : undefined
  return (
    <tr className="border-t align-top" data-branch={b.name}>
      <td className="py-1.5 pr-2">
        <code className="text-xs">{b.name}</code>
        {b.yours && (
          <Badge variant="secondary" className="ml-1.5 font-normal">
            yours
          </Badge>
        )}
        {b.default && (
          <Badge variant="outline" className="ml-1.5 font-normal">
            default
          </Badge>
        )}
        {pr && (
          <a href={pr.url} target="_blank" rel="noreferrer" className="ml-1.5 text-xs underline underline-offset-2">
            #{pr.number}
          </a>
        )}
      </td>
      <td className="py-1.5 pr-2 text-xs text-muted-foreground">
        {[b.local && 'local', b.remote && 'origin'].filter(Boolean).join(' · ')}
        {b.local && repo.url && (b.remote ? (b.unpushed > 0 ? ` · ${b.unpushed} unpushed` : '') : ' · not pushed')}
      </td>
      <td className="py-1.5 pr-2 text-xs text-muted-foreground">{b.default ? '—' : b.merged ? 'merged' : `${b.ahead} ahead · ${b.behind} behind`}</td>
      <td className="py-1.5 text-xs text-muted-foreground">
        {b.subject}
        {b.author ? ` · ${b.author}` : ''}
        {b.time ? ` · ${ago(b.time)}` : ''}
      </td>
    </tr>
  )
}
