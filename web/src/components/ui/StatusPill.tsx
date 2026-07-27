import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../../lib/utils'

export type StatusPillVariant = 'neutral' | 'info' | 'running' | 'waiting' | 'done' | 'error' | 'offline'

const defaultMarks: Record<StatusPillVariant, ReactNode> = {
  neutral: '—',
  info: 'i',
  running: '●',
  waiting: '…',
  done: '✓',
  error: '!',
  offline: '○',
}

export interface StatusPillProps extends HTMLAttributes<HTMLSpanElement> {
  status?: StatusPillVariant
  mark?: ReactNode
}

export function StatusPill({
  children,
  className,
  mark,
  status = 'neutral',
  ...props
}: StatusPillProps) {
  return (
    <span className={cn('ui-status-pill', className)} data-status={status} {...props}>
      <span className="ui-status-pill__mark" aria-hidden="true">
        {mark ?? defaultMarks[status]}
      </span>
      <span>{children}</span>
    </span>
  )
}
