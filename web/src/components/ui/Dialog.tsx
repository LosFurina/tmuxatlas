import {
  useEffect,
  useId,
  useRef,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from 'react'
import { createPortal } from 'react-dom'
import { cn } from '../../lib/utils'
import { IconButton } from './IconButton'
import { focusableElements, makeOutsideInert } from './overlayAccessibility'

interface ModalFrameProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  className?: string
  initialFocusRef?: RefObject<HTMLElement | null>
  closeLabel?: string
  closeOnBackdrop?: boolean
  role?: 'dialog' | 'alertdialog'
  kind: 'dialog' | 'sheet'
  side?: 'bottom' | 'left' | 'right'
}

function ModalFrame({
  children,
  className,
  closeLabel = 'Close',
  closeOnBackdrop = true,
  description,
  footer,
  initialFocusRef,
  kind,
  onOpenChange,
  open,
  role = 'dialog',
  side,
  title,
}: ModalFrameProps) {
  const titleId = useId()
  const descriptionId = useId()
  const overlayRef = useRef<HTMLDivElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const restoreFocusRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open || !overlayRef.current || !panelRef.current) return
    const overlay = overlayRef.current
    const panel = panelRef.current
    restoreFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const restoreBackground = makeOutsideInert(overlay)
    const animationFrame = window.requestAnimationFrame(() => {
      const firstTarget = initialFocusRef?.current ?? focusableElements(panel)[0] ?? panel
      firstTarget.focus()
    })

    const keepFocusInside = (event: FocusEvent) => {
      if (!panel.contains(event.target as Node)) {
        const firstTarget = initialFocusRef?.current ?? focusableElements(panel)[0] ?? panel
        firstTarget.focus()
      }
    }
    document.addEventListener('focusin', keepFocusInside)

    return () => {
      window.cancelAnimationFrame(animationFrame)
      document.removeEventListener('focusin', keepFocusInside)
      restoreBackground()
      const restoreTarget = restoreFocusRef.current
      window.requestAnimationFrame(() => {
        if (restoreTarget?.isConnected) restoreTarget.focus()
      })
    }
  }, [initialFocusRef, open])

  if (!open || typeof document === 'undefined') return null

  const close = () => onOpenChange(false)

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
      return
    }
    if (event.key !== 'Tab' || !panelRef.current) return
    const targets = focusableElements(panelRef.current)
    if (targets.length === 0) {
      event.preventDefault()
      panelRef.current.focus()
      return
    }
    const first = targets[0]
    const last = targets[targets.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  return createPortal(
    <div
      ref={overlayRef}
      className="ui-overlay-root"
      data-ui-overlay-root=""
      onKeyDown={handleKeyDown}
      onMouseDown={(event) => {
        if (closeOnBackdrop && event.target === event.currentTarget) close()
      }}
    >
      <div
        ref={panelRef}
        role={role}
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descriptionId : undefined}
        tabIndex={-1}
        className={cn(kind === 'dialog' ? 'ui-dialog' : 'ui-sheet', className)}
        data-side={kind === 'sheet' ? side : undefined}
      >
        <header className="ui-overlay__header">
          <div>
            <h2 id={titleId} className="ui-overlay__title">{title}</h2>
            {description && (
              <p id={descriptionId} className="ui-overlay__description">{description}</p>
            )}
          </div>
          <IconButton aria-label={closeLabel} size="sm" onClick={close}>
            <span aria-hidden="true">×</span>
          </IconButton>
        </header>
        <div className="ui-overlay__body">{children}</div>
        {footer && <footer className="ui-overlay__footer">{footer}</footer>}
      </div>
    </div>,
    document.body,
  )
}

export interface DialogProps extends Omit<ModalFrameProps, 'kind' | 'side'> {}

export function Dialog(props: DialogProps) {
  return <ModalFrame {...props} kind="dialog" />
}

export interface SheetProps extends Omit<ModalFrameProps, 'kind'> {
  side?: 'bottom' | 'left' | 'right'
}

export function Sheet({ side = 'bottom', ...props }: SheetProps) {
  return <ModalFrame {...props} kind="sheet" side={side} />
}
