import type { HTMLAttributes } from 'react'
import { cn } from '../../lib/utils'

export interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  label?: string
}

export function Skeleton({ className, label, ...props }: SkeletonProps) {
  return (
    <div
      className={cn('ui-skeleton', className)}
      role={label ? 'status' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      {...props}
    />
  )
}
