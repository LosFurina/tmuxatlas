import type { HTMLAttributes } from 'react'
import { cn } from '../../lib/utils'

export function Kbd({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <kbd className={cn('ui-kbd', className)} {...props} />
}
