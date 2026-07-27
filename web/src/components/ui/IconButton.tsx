import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { cn } from '../../lib/utils'
import type { ButtonSize, ButtonVariant } from './Button'

export interface IconButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label' | 'children'> {
  'aria-label': string
  children: React.ReactNode
  variant?: ButtonVariant
  size?: ButtonSize
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  {
    'aria-label': accessibleName,
    children,
    className,
    size = 'md',
    type = 'button',
    variant = 'ghost',
    ...props
  },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn('ui-icon-button ui-focus-ring', className)}
      data-size={size}
      data-variant={variant}
      aria-label={accessibleName}
      {...props}
    >
      {children}
    </button>
  )
})
