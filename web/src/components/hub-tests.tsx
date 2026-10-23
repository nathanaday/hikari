import { useState } from 'react'
import { Star } from 'lucide-react'
import { HealthBadge } from '@/components/health'
import { Markdown } from '@/components/markdown'
import { Button } from '@/components/ui/button'
import type { Gap, StoryHealth, TestPlan } from '@/lib/types'
import type { View } from '@/lib/view'
import { approveGap } from '@/lib/wikis'

const noNames: ReadonlySet<string> = new Set()

// HubTests shows the E2E test plan of a hub: the stories with their health,
// the gaps with your approval, the plan's parts, and the release set (rule
// UT1 of the tester spec).
export function HubTests({ plan, show }: { plan: TestPlan; show: (v: View) => void }) {
  const components = Object.entries(plan.plan.components ?? {}).sort(([a], [b]) => a.localeCompare(b))
  const actors = Object.entries(plan.plan.actors ?? {}).sort(([a], [b]) => a.localeCompare(b))
  const scenarios = Object.entries(plan.plan.scenarios ?? {}).sort(([a], [b]) => a.localeCompare(b))
  const stories = [...plan.stories].sort((a, b) => Number(!!b.key) - Number(!!a.key) || a.id.localeCompare(b.id))
  const findings = [...(plan.problems ?? []), ...(plan.warnings ?? [])]
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4" data-testid="hub-tests">
      <div className="mx-auto max-w-3xl space-y-6">
        <p className="text-sm text-muted-foreground">
          {plan.tester} proves the stories and rules of #{plan.hub} against the releases of its systems. Each story's health comes from its tests; its support is the
          health of the features it links.
        </p>
        {findings.length > 0 && (
          <ul className="list-disc space-y-0.5 rounded-md border border-amber-200 bg-amber-50 py-2 pr-3 pl-7 text-xs text-amber-900" data-testid="plan-findings">
            {plan.problems?.map((p) => (
              <li key={p} className="text-red-800">
                {p}
              </li>
            ))}
            {plan.warnings?.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        )}

        <Section title="Stories and rules">
          {stories.length === 0 ? (
            <p className="text-sm text-muted-foreground">The wiki of #{plan.hub} has no stories or rules yet.</p>
          ) : (
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-muted-foreground">
                <tr>
                  <th className="py-1 font-normal">Story or rule</th>
                  <th className="py-1 font-normal">E2E health</th>
                  <th className="py-1 font-normal">Support</th>
                  <th className="py-1 font-normal">Notes</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {stories.map((s) => (
                  <StoryRow key={s.id} story={s} plan={plan} show={show} />
                ))}
              </tbody>
            </table>
          )}
        </Section>

        <Section title="Gaps">
          {plan.gaps.length === 0 ? (
            <p className="text-sm text-muted-foreground">The plan names no gap.</p>
          ) : (
            <ul className="space-y-3">
              {plan.gaps.map((g) => (
                <GapItem key={g.id} hub={plan.hub} gap={g} />
              ))}
            </ul>
          )}
        </Section>

        {plan.body && (
          <Section title="The plan">
            <Markdown text={plan.body} names={noNames} />
          </Section>
        )}

        {components.length > 0 && (
          <Section title="Components">
            <Table
              head={['Name', 'What', 'Stands in for', 'Note']}
              rows={components.map(([name, c]) => [
                name,
                c.release ? `release ${c.release}` : c.image ? `image ${c.image}` : c.fixture ? 'fixture' : `simulator`,
                c.substitutes ?? c.simulates ?? '',
                c.note ?? '',
              ])}
            />
          </Section>
        )}
        {actors.length > 0 && (
          <Section title="Actors">
            <Table head={['Name', 'Runs', 'Count', 'Note']} rows={actors.map(([name, a]) => [name, a.runs, String(a.count), a.note ?? ''])} />
          </Section>
        )}
        {scenarios.length > 0 && (
          <Section title="Scenarios">
            <Table
              head={['Name', 'Stories', 'Actors', 'Minutes']}
              rows={scenarios.map(([name, s]) => [
                name,
                s.stories.join(', '),
                Object.entries(s.actors ?? {})
                  .map(([a, n]) => `${n} ${a}`)
                  .join(', '),
                s.minutes ? String(s.minutes) : '',
              ])}
            />
          </Section>
        )}
        <Section title="Release set">
          {(plan.lock.releases ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">{plan.tester} has pinned no release.</p>
          ) : (
            <Table
              head={['Release', 'Version', 'Digest']}
              rows={(plan.lock.releases ?? []).map((r) => [`${r.system}/${r.deliverable}`, r.version, r.digest.slice(0, 19)])}
            />
          )}
        </Section>
      </div>
    </div>
  )
}

function StoryRow({ story, plan, show }: { story: StoryHealth; plan: TestPlan; show: (v: View) => void }) {
  const gaps = (story.gaps ?? []).map((id) => plan.gaps.find((g) => g.id === id)).filter((g): g is Gap => !!g)
  return (
    <tr className="align-top" data-testid="story-health">
      <td className="py-1.5 pr-3">
        <button type="button" onClick={() => show({ kind: 'map', hub: plan.hub, doc: story.id })} className="flex items-center gap-1.5 text-left underline-offset-2 hover:underline">
          {story.key && <Star className="size-3.5 shrink-0 fill-current text-amber-500" aria-label="key story" />}
          {story.name || story.id}
        </button>
        <span className="text-xs text-muted-foreground">{story.kind}</span>
      </td>
      <td className="py-1.5 pr-3">
        <HealthBadge health={story.health} />
        {story.passed + story.failed + story.pending > 0 && (
          <div className="text-xs text-muted-foreground">
            {story.passed} passed, {story.failed} failed, {story.pending} pending
          </div>
        )}
      </td>
      <td className="py-1.5 pr-3">{story.support ? <HealthBadge health={story.support} /> : <span className="text-xs text-muted-foreground">none</span>}</td>
      <td className="py-1.5 text-xs text-muted-foreground">
        {story.suspects && story.suspects.length > 0 && <div className="text-red-700">suspects: {story.suspects.join(', ')}</div>}
        {gaps.map((g) => (
          <div key={g.id}>
            {g.approved ? 'out of reach' : 'gap proposed'}: {g.what}
          </div>
        ))}
      </td>
    </tr>
  )
}

function GapItem({ hub, gap }: { hub: string; gap: Gap }) {
  const [error, setError] = useState<string | null>(null)
  const toggle = async () => setError(await approveGap(hub, gap.id, !gap.approved))
  return (
    <li className="rounded-md border px-3 py-2" data-testid="gap">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium">{gap.what}</div>
          <div className="text-sm text-muted-foreground">{gap.why}</div>
          <div className="text-xs text-muted-foreground">touches {gap.touches.join(', ')}</div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className={gap.approved ? 'text-xs text-emerald-700' : 'text-xs text-amber-700'}>{gap.approved ? 'approved' : 'proposed'}</span>
          <Button size="sm" variant="outline" onClick={toggle}>
            {gap.approved ? 'Withdraw' : 'Approve'}
          </Button>
        </div>
      </div>
      {error && <p className="mt-1 text-xs text-red-700">{error}</p>}
    </li>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">{title}</h2>
      {children}
    </section>
  )
}

function Table({ head, rows }: { head: string[]; rows: string[][] }) {
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-xs text-muted-foreground">
        <tr>
          {head.map((h) => (
            <th key={h} className="py-1 font-normal">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody className="divide-y">
        {rows.map((r) => (
          <tr key={r[0]}>
            {r.map((c, i) => (
              <td key={i} className="py-1.5 pr-3 align-top">
                {c}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
