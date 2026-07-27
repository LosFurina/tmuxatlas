import { useEffect, useRef, type KeyboardEvent } from 'react'

export interface TerminalMenuPosition {
  x: number
  y: number
}

interface TerminalContextMenuProps {
  position: TerminalMenuPosition
  canCopy: boolean
  canPaste: boolean
  onCopy: () => void
  onPaste: () => void
  onFind: () => void
  onSelectAll: () => void
  onClose: () => void
}

export function TerminalContextMenu({
  position,
  canCopy,
  canPaste,
  onCopy,
  onPaste,
  onFind,
  onSelectAll,
  onClose,
}: TerminalContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const firstEnabled = menuRef.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')
    firstEnabled?.focus()
  }, [])

  const run = (action: () => void) => {
    action()
    onClose()
  }

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    event.preventDefault()
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [],
    )
    if (items.length === 0) return
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const delta = event.key === 'ArrowDown' ? 1 : -1
    items[(current + delta + items.length) % items.length]?.focus()
  }

  return (
    <>
      <button
        type="button"
        aria-label="Close Terminal menu"
        className="fixed inset-0 z-40 cursor-default bg-transparent"
        onClick={onClose}
      />
      <div
        ref={menuRef}
        role="menu"
        aria-label="Terminal actions"
        onKeyDown={onKeyDown}
        className="fixed z-50 min-w-40 rounded border border-border bg-popover p-1 text-sm text-popover-foreground shadow-lg"
        style={{
          left: Math.max(8, Math.min(position.x, window.innerWidth - 176)),
          top: Math.max(8, Math.min(position.y, window.innerHeight - 190)),
        }}
      >
        <MenuItem label="Copy selection" disabled={!canCopy} onClick={() => run(onCopy)} />
        <MenuItem label="Paste" disabled={!canPaste} onClick={() => run(onPaste)} />
        <div className="my-1 border-t border-border" />
        <MenuItem label="Find in Terminal" onClick={() => run(onFind)} />
        <MenuItem label="Select all" onClick={() => run(onSelectAll)} />
      </div>
    </>
  )
}

function MenuItem({
  label,
  disabled,
  onClick,
}: {
  label: string
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className="flex min-h-9 w-full items-center rounded px-3 text-left hover:bg-accent focus:bg-accent disabled:opacity-40"
    >
      {label}
    </button>
  )
}
