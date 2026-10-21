import { KeyRound } from 'lucide-react'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function SignIn() {
  return (
    <div className="flex h-full items-center justify-center p-4">
      <Card className="w-full max-w-sm shadow-none">
        <CardHeader>
          <KeyRound className="mb-2 size-5 text-muted-foreground" />
          <CardTitle>Open the link that hikari printed</CardTitle>
          <CardDescription>
            Each run of <code className="rounded bg-neutral-100 px-1">hikari up</code> prints a web UI link with a new
            token. This page has no valid token, possibly because the server restarted.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}
