import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { cn } from '../../lib/utils'
import { IconButton } from './IconButton'

const MenuContext = createContext<{ close: (restoreFocus?: boolean) => void } | null>(null)

export interface MenuProps {
  label: string
  trigger: ReactNode
  children: ReactNode
  align?: 'start' | 'end'
  className?: string
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

function getMenuItems(container: HTMLElement | null) {
  if (!container) return []
  return Array.from(container.querySelectorAll<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])'))
}

export function Menu({
  align = 'end',
  children,
  className,
  label,
  onOpenChange,
  open: controlledOpen,
  trigger,
}: MenuProps) {
  const menuId = useId()
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false)
  const open = controlledOpen ?? uncontrolledOpen
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)

  const setOpen = (next: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(next)
    onOpenChange?.(next)
  }

  const close = (restoreFocus = true) => {
    setOpen(false)
    if (restoreFocus) window.requestAnimationFrame(() => triggerRef.current?.focus())
  }

  useEffect(() => {
    if (!open) return
    const animationFrame = window.requestAnimationFrame(() => getMenuItems(contentRef.current)[0]?.focus())
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) close(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => {
      window.cancelAnimationFrame(animationFrame)
      document.removeEventListener('pointerdown', onPointerDown)
    }
  }, [open])

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const items = getMenuItems(contentRef.current)
    const currentIndex = items.indexOf(document.activeElement as HTMLElement)
    if (event.key === 'Escape') {
      event.preventDefault()
      close()
    } else if (event.key === 'ArrowDown') {
      event.preventDefault()
      items[(currentIndex + 1 + items.length) % items.length]?.focus()
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      items[(currentIndex - 1 + items.length) % items.length]?.focus()
    } else if (event.key === 'Home') {
      event.preventDefault()
      items[0]?.focus()
    } else if (event.key === 'End') {
      event.preventDefault()
      items[items.length - 1]?.focus()
    } else if (event.key === 'Tab') {
      setOpen(false)
    }
  }

  return (
    <div ref={rootRef} className={cn('ui-menu', className)} onKeyDown={handleKeyDown}>
      <IconButton
        ref={triggerRef}
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen(!open)}
      >
        {trigger}
      </IconButton>
      {open && (
        <div
          ref={contentRef}
          id={menuId}
          role="menu"
          aria-label={label}
          className="ui-menu__content"
          data-align={align}
        >
          <MenuContext.Provider value={{ close }}>{children}</MenuContext.Provider>
        </div>
      )}
    </div>
  )
}

export interface MenuItemProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  onSelect?: () => void
  variant?: 'default' | 'destructive'
}

export function MenuItem({
  children,
  className,
  disabled,
  onClick,
  onSelect,
  variant = 'default',
  ...props
}: MenuItemProps) {
  const context = useContext(MenuContext)
  return (
    <button
      type="button"
      role="menuitem"
      tabIndex={-1}
      className={cn('ui-menu__item ui-focus-ring', className)}
      data-variant={variant}
      disabled={disabled}
      aria-disabled={disabled || undefined}
      onClick={(event) => {
        onClick?.(event)
        if (event.defaultPrevented || disabled) return
        onSelect?.()
        context?.close()
      }}
      {...props}
    >
      {children}
    </button>
  )
}
