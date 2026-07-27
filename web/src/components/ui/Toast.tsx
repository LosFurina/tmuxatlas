import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { IconButton } from './IconButton'

export type ToastVariant = 'info' | 'success' | 'warning' | 'error'

export interface ToastOptions {
  title: string
  description?: string
  variant?: ToastVariant
  duration?: number
}

interface ToastRecord extends ToastOptions {
  id: string
  variant: ToastVariant
}

interface ToastContextValue {
  toast: (options: ToastOptions) => string
  dismiss: (id: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

export interface ToastProps extends ToastRecord {
  onDismiss: (id: string) => void
}

export function Toast({ description, id, onDismiss, title, variant }: ToastProps) {
  const isUrgent = variant === 'error'
  return (
    <section
      className="ui-toast"
      data-variant={variant}
      role={isUrgent ? 'alert' : 'status'}
      aria-live={isUrgent ? 'assertive' : 'polite'}
      aria-atomic="true"
    >
      <div>
        <div className="ui-toast__title">{title}</div>
        {description && <div className="ui-toast__description">{description}</div>}
      </div>
      <IconButton aria-label="Dismiss notification" size="sm" onClick={() => onDismiss(id)}>
        <span aria-hidden="true">×</span>
      </IconButton>
    </section>
  )
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastRecord[]>([])
  const counterRef = useRef(0)
  const timersRef = useRef(new Map<string, number>())

  const dismiss = useCallback((id: string) => {
    const timer = timersRef.current.get(id)
    if (timer !== undefined) window.clearTimeout(timer)
    timersRef.current.delete(id)
    setToasts((current) => current.filter((item) => item.id !== id))
  }, [])

  const toast = useCallback((options: ToastOptions) => {
    const id = `toast-${++counterRef.current}`
    const record: ToastRecord = { ...options, id, variant: options.variant ?? 'info' }
    setToasts((current) => [...current, record])
    const duration = options.duration ?? 5000
    if (duration > 0 && Number.isFinite(duration)) {
      timersRef.current.set(id, window.setTimeout(() => dismiss(id), duration))
    }
    return id
  }, [dismiss])

  useEffect(() => () => {
    for (const timer of timersRef.current.values()) window.clearTimeout(timer)
    timersRef.current.clear()
  }, [])

  return (
    <ToastContext.Provider value={{ dismiss, toast }}>
      {children}
      {typeof document !== 'undefined' && createPortal(
        <div className="ui-toast-viewport" aria-label="Notifications">
          {toasts.map((item) => <Toast key={item.id} {...item} onDismiss={dismiss} />)}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  )
}

export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used within a ToastProvider.')
  return context
}
