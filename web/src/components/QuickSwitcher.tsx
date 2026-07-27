import { useEffect, useMemo, useRef, useState } from 'react'
import type { RegisteredCommand } from '../commands/registry'
import { formatShortcut } from '../commands/registry'
import { cn } from '../lib/utils'
import type { WorkspaceSession, WorkspaceViewModel } from '../workspace/model'
import { isAttentionStatus, workspaceStatusLabel } from '../workspace/model'
import { Dialog } from './ui'

interface QuickSwitcherProps {
  workspace: WorkspaceViewModel
  commands: RegisteredCommand[]
  pinnedTargets: string[]
  recentTargets: string[]
  onSelect: (target: string, windowIndex?: number) => void
  onClose: () => void
}

type PaletteGroup = 'Needs Attention' | 'Commands' | 'Pinned' | 'Recent' | 'Hosts' | 'Sessions' | 'Windows' | 'Agents'

interface PaletteItem {
  id: string
  group: PaletteGroup
  label: string
  detail?: string
  keywords: string
  disabled: boolean
  target?: string
  windowIndex?: number
  command?: RegisteredCommand
}

const groupOrder: PaletteGroup[] = ['Needs Attention', 'Commands', 'Pinned', 'Recent', 'Hosts', 'Sessions', 'Windows', 'Agents']

function fuzzyScore(query: string, text: string): number | null {
  const normalizedQuery = query.trim().toLowerCase()
  const normalizedText = text.toLowerCase()
  if (!normalizedQuery) return 0
  const direct = normalizedText.indexOf(normalizedQuery)
  if (direct >= 0) return direct
  let queryIndex = 0
  let score = 100
  let previousMatch = -2
  for (let index = 0; index < normalizedText.length && queryIndex < normalizedQuery.length; index++) {
    if (normalizedText[index] !== normalizedQuery[queryIndex]) continue
    score += index === previousMatch + 1 ? 0 : 3
    score += index
    previousMatch = index
    queryIndex++
  }
  return queryIndex === normalizedQuery.length ? score : null
}

function sessionItem(session: WorkspaceSession, group: PaletteGroup): PaletteItem {
  const agentDetail = session.agents.length > 0 ? ` · ${session.agents.join(', ')}` : ''
  return {
    id: `session:${session.key}`,
    group,
    label: session.name,
    detail: `${session.hostName} · ${workspaceStatusLabel(session.status)}${agentDetail}`,
    keywords: session.searchText,
    disabled: false,
    target: session.key,
  }
}

export function QuickSwitcher({
  workspace,
  commands,
  pinnedTargets,
  recentTargets,
  onSelect,
  onClose,
}: QuickSwitcherProps) {
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const allItems = useMemo<PaletteItem[]>(() => {
    const pinned = new Set(pinnedTargets)
    const recent = new Set(recentTargets)
    const items: PaletteItem[] = commands.filter(command => command.id !== 'palette.open').map(command => ({
      id: `command:${command.id}`,
      group: 'Commands',
      label: command.label,
      detail: command.enabled ? formatShortcut(command.shortcut) || command.category : 'Unavailable in this context',
      keywords: `${command.id} ${command.label} ${command.category} ${command.shortcut || ''}`,
      disabled: !command.enabled,
      command,
    }))

    for (const host of workspace.hosts) {
      items.push({
        id: `host:${host.id}`,
        group: 'Hosts',
        label: host.name,
        detail: `${host.sessionCount} session${host.sessionCount === 1 ? '' : 's'} · ${workspaceStatusLabel(host.status)} · ${host.id}`,
        keywords: `${host.name} ${host.id} ${host.status}`,
        disabled: host.sessions.length === 0,
        target: host.sessions[0]?.key,
      })
    }

    for (const session of workspace.sessions) {
      const group: PaletteGroup = isAttentionStatus(session.status)
        ? 'Needs Attention'
        : pinned.has(session.key)
          ? 'Pinned'
          : recent.has(session.key)
            ? 'Recent'
            : 'Sessions'
      items.push(sessionItem(session, group))

      for (const window of session.source.windows) {
        items.push({
          id: `window:${session.key}:${window.index}`,
          group: 'Windows',
          label: `${session.name} / ${window.name}`,
          detail: `${session.hostName} · Window ${window.index}${window.active ? ' · Active' : ''}`,
          keywords: `${session.searchText} ${window.name} window ${window.index}`,
          disabled: session.status === 'offline',
          target: session.key,
          windowIndex: window.index,
        })
      }

      for (const agent of session.agents) {
        items.push({
          id: `agent:${session.key}:${agent}`,
          group: 'Agents',
          label: agent,
          detail: `${session.hostName} / ${session.name} · ${workspaceStatusLabel(session.status)}`,
          keywords: `${session.searchText} ${agent}`,
          disabled: false,
          target: session.key,
        })
      }
    }
    return items
  }, [commands, pinnedTargets, recentTargets, workspace])

  const filtered = useMemo(() => {
    const scored = allItems.flatMap(item => {
      const score = fuzzyScore(query, `${item.label} ${item.detail || ''} ${item.keywords}`)
      return score === null ? [] : [{ item, score }]
    })
    return scored.sort((left, right) => (
      left.score - right.score
      || groupOrder.indexOf(left.item.group) - groupOrder.indexOf(right.item.group)
      || Number(left.item.disabled) - Number(right.item.disabled)
      || left.item.label.localeCompare(right.item.label)
    )).map(value => value.item)
  }, [allItems, query])

  const grouped = useMemo(() => groupOrder.flatMap(group => {
    const items = filtered.filter(item => item.group === group)
    return items.length > 0 ? [{ group, items }] : []
  }), [filtered])

  useEffect(() => {
    const firstEnabled = filtered.findIndex(item => !item.disabled)
    setSelectedIndex(firstEnabled < 0 ? 0 : firstEnabled)
  }, [query, filtered.length])

  useEffect(() => {
    const option = document.getElementById(`command-palette-option-${selectedIndex}`)
    option?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  const execute = (item: PaletteItem) => {
    if (item.disabled) return
    onClose()
    if (item.command) {
      void item.command.run({ source: 'palette' })
      return
    }
    if (item.target) onSelect(item.target, item.windowIndex)
  }

  const moveSelection = (direction: 1 | -1) => {
    if (filtered.length === 0) return
    setSelectedIndex(current => {
      let next = current
      for (let step = 0; step < filtered.length; step++) {
        next = (next + direction + filtered.length) % filtered.length
        if (!filtered[next].disabled) return next
      }
      return current
    })
  }

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      moveSelection(1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      moveSelection(-1)
    } else if (event.key === 'Enter') {
      event.preventDefault()
      const item = filtered[selectedIndex]
      if (item) execute(item)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClose()
    } else if (event.key === 'Tab') {
      // The Palette has one tab stop; keep keyboard focus inside the modal.
      event.preventDefault()
      inputRef.current?.focus()
    }
  }

  return (
    <Dialog
      open
      onOpenChange={open => { if (!open) onClose() }}
      title="Command Palette"
      description="Search commands and stable Workspace targets."
      closeLabel="Close Command Palette"
      initialFocusRef={inputRef}
      className="command-palette-dialog"
    >
      <div data-command-palette className="flex min-h-0 flex-1 flex-col">
        <div className="border-b border-border p-3 sm:px-4">
          <input
            ref={inputRef}
            role="combobox"
            aria-label="Search commands, hosts, sessions, windows, and agents"
            aria-autocomplete="list"
            aria-expanded="true"
            aria-controls="command-palette-results"
            aria-activedescendant={filtered[selectedIndex] ? `command-palette-option-${selectedIndex}` : undefined}
            value={query}
            onChange={event => setQuery(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Search commands, hosts, sessions, windows, or agents…"
            className="h-11 w-full border-none bg-transparent text-base text-foreground outline-none placeholder:text-muted-foreground"
          />
        </div>
        <div id="command-palette-results" ref={listRef} role="listbox" className="flex-1 overflow-y-auto overscroll-contain py-2">
          {filtered.length === 0 && (
            <div className="px-4 py-8 text-center text-sm text-muted-foreground">No matching commands or targets</div>
          )}
          {grouped.map(({ group, items }) => (
            <div key={group} role="group" aria-labelledby={`palette-group-${group.replace(/\s+/g, '-').toLowerCase()}`}>
              <div id={`palette-group-${group.replace(/\s+/g, '-').toLowerCase()}`} className="px-4 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {group}
              </div>
              {items.map(item => {
                const index = filtered.indexOf(item)
                return (
                  <div
                    id={`command-palette-option-${index}`}
                    key={item.id}
                    role="option"
                    aria-selected={index === selectedIndex}
                    aria-disabled={item.disabled || undefined}
                    onMouseDown={event => event.preventDefault()}
                    onClick={() => execute(item)}
                    onMouseEnter={() => { if (!item.disabled) setSelectedIndex(index) }}
                    className={cn(
                      'mx-2 flex min-h-11 items-center gap-3 rounded-lg px-3 py-2 text-sm',
                      item.disabled ? 'cursor-not-allowed opacity-45' : 'cursor-pointer',
                      index === selectedIndex && !item.disabled ? 'bg-primary/15 text-foreground' : 'text-foreground hover:bg-muted/70',
                    )}
                  >
                    <span aria-hidden="true" className={cn(
                      'h-2 w-2 shrink-0 rounded-full',
                      item.group === 'Needs Attention' ? 'bg-warning' : item.command ? 'bg-primary' : 'bg-muted-foreground/60',
                    )} />
                    <span className="min-w-0 flex-1 truncate font-medium">{item.label}</span>
                    {item.detail && <span className="max-w-[55%] truncate text-xs text-muted-foreground">{item.detail}</span>}
                  </div>
                )
              })}
            </div>
          ))}
        </div>
        <div className="flex gap-4 border-t border-border px-4 py-2 text-[11px] text-muted-foreground">
          <span>↑↓ Navigate</span><span>↵ Run</span><span>Esc Close</span>
        </div>
      </div>
    </Dialog>
  )
}
