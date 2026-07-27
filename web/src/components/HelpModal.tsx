import { useMemo } from 'react'
import type { RegisteredCommand } from '../commands/registry'
import { formatShortcut } from '../commands/registry'
import { Dialog, Kbd } from './ui'

interface HelpModalProps {
  commands: RegisteredCommand[]
  onClose: () => void
}

export function HelpModal({ commands, onClose }: HelpModalProps) {
  const groups = useMemo(() => {
    const result = new Map<string, RegisteredCommand[]>()
    for (const command of commands) {
      const values = result.get(command.category) || []
      values.push(command)
      result.set(command.category, values)
    }
    return [...result.entries()]
  }, [commands])

  return (
    <Dialog
      open
      onOpenChange={open => { if (!open) onClose() }}
      title="Commands & keyboard shortcuts"
      description="Generated from the active command registry."
      closeLabel="Close keyboard shortcuts"
    >
      {groups.map(([category, categoryCommands]) => (
        <section key={category} aria-labelledby={`help-${category.toLowerCase()}`} className="mb-5 last:mb-0">
          <h3 id={`help-${category.toLowerCase()}`} className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{category}</h3>
          {categoryCommands.map(command => (
            <div key={command.id} className="flex min-h-9 items-center justify-between gap-4 py-1">
              <span className="text-sm text-foreground">{command.label}</span>
              {command.shortcut ? <Kbd>{formatShortcut(command.shortcut)}</Kbd> : <span className="text-xs text-muted-foreground">Palette</span>}
            </div>
          ))}
        </section>
      ))}
    </Dialog>
  )
}
