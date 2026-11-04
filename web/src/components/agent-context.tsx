import { useCallback, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { asJSON, useFetch } from '@/hooks/use-vault-file'
import type { State } from '@/lib/store'
import type { AgentContext } from '@/lib/types'

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3 text-sm">
      <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 flex-1">{children}</span>
    </div>
  )
}

function Names({ names, empty, hint }: { names: string[]; empty: string; hint?: (n: string) => string | undefined }) {
  if (names.length === 0) return <span className="text-muted-foreground">{empty}</span>
  return (
    <span className="flex flex-wrap gap-1.5">
      {names.map((n) => (
        <code key={n} className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs" title={hint?.(n)}>
          {n}
        </code>
      ))}
    </span>
  )
}

// AgentContextBody shows what an agent's container puts before Claude Code
// on each turn (the agent context view): how it works, what shapes its
// prompt, and the prompt itself.
export function AgentContextBody({ name, ctx, state }: { name: string; ctx: AgentContext; state: State }) {
  const [copied, setCopied] = useState(false)
  const agent = state.agents.find((a) => a.name === name)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(ctx.prompt)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }
  const words = ctx.prompt.split(/\s+/).filter(Boolean).length
  return (
    <div className="min-h-0 flex-1 overflow-y-auto" data-testid="agent-context">
      <div className="max-w-4xl space-y-6 px-4 py-5 md:px-6">
        <p className="text-sm text-muted-foreground">
          What {name} sees before every turn: its system prompt, built from the topology and the vault. Policies apply to every agent; styles and skills are granted to this one, by its grants or its profile. A change in the vault reaches the next turn; a change of the topology or a profile needs hikari up again.
        </p>
        <section className="space-y-1.5">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">How it works</h2>
          <Row label="Role">{agent?.role === 'orchestration' ? 'orchestration agent' : 'partition agent'}</Row>
          <Row label="Profile">{ctx.profile ? <code className="text-xs">{ctx.profile}</code> : <span className="text-muted-foreground">none</span>}</Row>
          <Row label="Model">{ctx.model ? <code className="text-xs">{ctx.model}</code> : <span className="text-muted-foreground">the default model</span>}</Row>
          <Row label="Effort">{ctx.effort ? <code className="text-xs">{ctx.effort}</code> : <span className="text-muted-foreground">the default effort</span>}</Row>
          {agent?.role !== 'orchestration' && <Row label="Review">{ctx.review === 'required' ? 'required before done' : <span className="text-muted-foreground">none</span>}</Row>}
        </section>
        <section className="space-y-1.5">
          <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">What shapes the prompt</h2>
          <Row label="Pack">
            <Names names={ctx.pack} empty="no built-in skills" hint={() => 'from Hikari, for the tasks of its role'} />
          </Row>
          <Row label="Skills">
            <Names names={ctx.skills} empty="no granted skill" hint={(n) => `/hikari/skills/${n}`} />
          </Row>
          <Row label="Styles">
            <Names names={ctx.styles} empty="no granted style" hint={(n) => `/hikari/styles/${n}.md`} />
            {ctx.missing?.length ? <span className="ml-2 text-xs text-amber-700">missing in the vault: {ctx.missing.join(', ')}</span> : null}
          </Row>
          <Row label="Policies">
            <Names names={ctx.policies} empty="no policy in vault/public/policies/" hint={(n) => `/hikari/public/policies/${n}.md`} />
          </Row>
        </section>
        <section className="space-y-1.5">
          <div className="flex items-center justify-between">
            <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">System prompt · {words} words</h2>
            <Button size="sm" variant="ghost" onClick={copy}>
              {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
          <pre className="max-h-[70vh] overflow-auto rounded-md border bg-neutral-50 p-3 text-xs leading-relaxed whitespace-pre-wrap" data-testid="agent-prompt">
            {ctx.prompt}
          </pre>
        </section>
      </div>
    </div>
  )
}

// AgentContextView loads an agent's context from the running hikari.
export function AgentContextView({ name, state }: { name: string; state: State }) {
  const parse = useCallback((r: Response) => asJSON<AgentContext>(r), [])
  const { data, error } = useFetch(`/api/ui/agents/${encodeURIComponent(name)}/context`, parse)
  if (error) return <p className="px-6 py-10 text-sm text-muted-foreground">{error}</p>
  if (!data) return <p className="px-6 py-10 text-sm text-muted-foreground">Loading…</p>
  return <AgentContextBody name={name} ctx={data} state={state} />
}
