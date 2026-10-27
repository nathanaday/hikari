import { User } from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { cn } from '@/lib/utils'

function initials(name: string): string {
  const parts = name.split('-').filter(Boolean)
  return (parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)).toUpperCase()
}

export function ParticipantAvatar({ name, me, className }: { name: string; me: boolean; className?: string }) {
  return (
    <Avatar className={cn('size-8 rounded-md', className)}>
      <AvatarFallback className="rounded-md bg-neutral-100 text-[11px] font-medium text-neutral-600">
        {me ? <User className="size-4" /> : initials(name)}
      </AvatarFallback>
    </Avatar>
  )
}
