// decide answers a decision, and returns an error message, or null.
export async function decide(message: number, choice: number, note: string): Promise<string | null> {
  const res = await fetch('/api/ui/decide', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, choice, note }),
  }).catch(() => undefined)
  if (!res) return 'The hikari server is not reachable.'
  if (res.ok) return null
  const e = (await res.json().catch(() => ({}))) as { error?: string }
  return e.error ?? res.statusText
}
