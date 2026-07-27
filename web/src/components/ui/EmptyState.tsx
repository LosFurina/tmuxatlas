import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../../lib/utils'

export interface EmptyStateProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  title: ReactNode
  description?: ReactNode
  icon?: ReactNode
  action?: ReactNode
}

export function EmptyState({
  action,
  className,
  description,
  icon,
  title,
  ...props
}: EmptyStateProps) {
  return (
    <div className={cn('ui-empty-state', className)} {...props}>
      {icon && <div className="ui-empty-state__icon" aria-hidden="true">{icon}</div>}
      <div>
        <h2 className="ui-empty-state__title">{title}</h2>
        {description && <p className="ui-empty-state__description">{description}</p>}
      </div>
      {action}
    </div>
  )
}
