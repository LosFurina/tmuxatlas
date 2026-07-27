import {
  cloneElement,
  isValidElement,
  useId,
  useState,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from 'react'
import { cn } from '../../lib/utils'

export interface TooltipProps {
  content: ReactNode
  children: ReactElement<{ 'aria-describedby'?: string }>
  side?: 'top' | 'bottom'
  className?: string
}

export function Tooltip({ children, className, content, side = 'top' }: TooltipProps) {
  const tooltipId = useId()
  const [open, setOpen] = useState(false)

  if (!isValidElement(children)) {
    throw new Error('Tooltip requires a single React element child.')
  }

  const describedBy = [children.props['aria-describedby'], open ? tooltipId : undefined]
    .filter(Boolean)
    .join(' ') || undefined

  const handleKeyDown = (event: KeyboardEvent<HTMLSpanElement>) => {
    if (event.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <span
      className="ui-tooltip-anchor"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocusCapture={() => setOpen(true)}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
      }}
      onKeyDown={handleKeyDown}
    >
      {cloneElement(children, { 'aria-describedby': describedBy })}
      {open && (
        <span id={tooltipId} role="tooltip" data-side={side} className={cn('ui-tooltip', className)}>
          {content}
        </span>
      )}
    </span>
  )
}
