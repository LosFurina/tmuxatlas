export type CommandScope = 'overlay' | 'workspace' | 'terminal'

export type CommandCategory = 'Navigation' | 'Session' | 'Terminal' | 'Workspace' | 'Account'

export type CommandId =
  | 'palette.open'
  | 'help.open'
  | 'navigation.overview'
  | 'navigation.settings'
  | 'session.new'
  | 'connection.reconnect'
  | 'terminal.fullscreen'
  | 'terminal.zen'
  | 'workspace.sidebar.toggle'
  | 'attention.next'
  | 'account.sign-out'

export interface CommandEnvironment {
  hasTerminalTarget: boolean
  canReconnect: boolean
  canSignOut: boolean
  hasAttention: boolean
}

export interface CommandHandlerContext {
  source: 'shortcut' | 'palette' | 'help' | 'button'
}

export type CommandHandler = (context: CommandHandlerContext) => void | Promise<void>

export interface CommandDefinition {
  id: CommandId
  label: string
  category: CommandCategory
  scope: CommandScope
  shortcut?: string
  focusAfterRun: 'restore' | 'terminal' | 'none'
  enablement: (environment: CommandEnvironment) => boolean
}

export interface RegisteredCommand extends Omit<CommandDefinition, 'enablement'> {
  enabled: boolean
  run: CommandHandler
}

export interface CreateCommandRegistryOptions {
  environment: CommandEnvironment
  handlers: Partial<Record<CommandId, CommandHandler>>
  quickSwitcherShortcut?: string
}

const always = () => true
const terminalOnly = (environment: CommandEnvironment) => environment.hasTerminalTarget

const definitions: readonly CommandDefinition[] = [
  {
    id: 'palette.open',
    label: 'Open Command Palette',
    category: 'Navigation',
    scope: 'overlay',
    shortcut: 'mod+k',
    focusAfterRun: 'restore',
    enablement: always,
  },
  {
    id: 'help.open',
    label: 'Keyboard shortcuts',
    category: 'Navigation',
    scope: 'overlay',
    shortcut: 'mod+/',
    focusAfterRun: 'restore',
    enablement: always,
  },
  {
    id: 'navigation.overview',
    label: 'Go to Overview',
    category: 'Navigation',
    scope: 'workspace',
    focusAfterRun: 'none',
    enablement: always,
  },
  {
    id: 'navigation.settings',
    label: 'Open Settings',
    category: 'Navigation',
    scope: 'workspace',
    shortcut: 'mod+,',
    focusAfterRun: 'none',
    enablement: always,
  },
  {
    id: 'session.new',
    label: 'New Session',
    category: 'Session',
    scope: 'workspace',
    focusAfterRun: 'restore',
    enablement: always,
  },
  {
    id: 'connection.reconnect',
    label: 'Reconnect Terminal',
    category: 'Terminal',
    scope: 'terminal',
    focusAfterRun: 'terminal',
    enablement: environment => environment.hasTerminalTarget && environment.canReconnect,
  },
  {
    id: 'terminal.fullscreen',
    label: 'Toggle Terminal Fullscreen',
    category: 'Terminal',
    scope: 'terminal',
    shortcut: 'mod+shift+f',
    focusAfterRun: 'terminal',
    enablement: terminalOnly,
  },
  {
    id: 'terminal.zen',
    label: 'Toggle Zen Mode',
    category: 'Terminal',
    scope: 'terminal',
    shortcut: 'mod+shift+z',
    focusAfterRun: 'terminal',
    enablement: terminalOnly,
  },
  {
    id: 'workspace.sidebar.toggle',
    label: 'Toggle Sidebar',
    category: 'Workspace',
    scope: 'workspace',
    focusAfterRun: 'restore',
    enablement: always,
  },
  {
    id: 'attention.next',
    label: 'Go to Next Alert',
    category: 'Workspace',
    scope: 'workspace',
    focusAfterRun: 'terminal',
    enablement: environment => environment.hasAttention,
  },
  {
    id: 'account.sign-out',
    label: 'Lock / Sign out',
    category: 'Account',
    scope: 'workspace',
    focusAfterRun: 'none',
    enablement: environment => environment.canSignOut,
  },
] as const

function normalizeConfiguredShortcut(shortcut: string | undefined): string {
  if (!shortcut) return 'mod+k'
  return shortcut.trim().toLowerCase().replace(/^ctrl\+/, 'mod+')
}

export function createCommandRegistry({
  environment,
  handlers,
  quickSwitcherShortcut,
}: CreateCommandRegistryOptions): RegisteredCommand[] {
  const paletteShortcut = normalizeConfiguredShortcut(quickSwitcherShortcut)
  return definitions.map(definition => ({
    ...definition,
    shortcut: definition.id === 'palette.open' ? paletteShortcut : definition.shortcut,
    enabled: definition.enablement(environment) && Boolean(handlers[definition.id]),
    run: handlers[definition.id] || (() => {}),
  }))
}

export function getCommand(commands: readonly RegisteredCommand[], id: CommandId) {
  return commands.find(command => command.id === id)
}

export function isMacPlatform(platform = typeof navigator === 'undefined' ? '' : navigator.platform): boolean {
  return /Mac|iPhone|iPad/i.test(platform)
}

export function formatShortcut(shortcut: string | undefined, mac = isMacPlatform()): string {
  if (!shortcut) return ''
  const labels: Record<string, string> = mac
    ? { mod: '⌘', shift: '⇧', alt: '⌥', enter: '↵', space: 'Space' }
    : { mod: 'Ctrl', shift: 'Shift', alt: 'Alt', enter: 'Enter', space: 'Space' }
  return shortcut
    .split('+')
    .map(part => labels[part.toLowerCase()] || (part.length === 1 ? part.toUpperCase() : part))
    .join('+')
}

export interface ShortcutEventLike {
  key: string
  code?: string
  ctrlKey: boolean
  metaKey: boolean
  shiftKey: boolean
  altKey: boolean
  defaultPrevented: boolean
  preventDefault(): void
  stopPropagation(): void
  stopImmediatePropagation?: () => void
}

function shortcutMatches(event: ShortcutEventLike, shortcut: string, mac: boolean): boolean {
  const parts = shortcut.toLowerCase().split('+')
  const key = parts[parts.length - 1] || ''
  const wantsMod = parts.includes('mod')
  const wantsShift = parts.includes('shift')
  const wantsAlt = parts.includes('alt')
  const actualMod = mac ? event.metaKey : event.ctrlKey
  const wrongPlatformMod = mac ? event.ctrlKey : event.metaKey
  const eventKey = event.key.toLowerCase() === ' ' ? 'space' : event.key.toLowerCase()
  return eventKey === key
    && actualMod === wantsMod
    && !wrongPlatformMod
    && event.shiftKey === wantsShift
    && event.altKey === wantsAlt
}

function commandAllowedInScope(command: RegisteredCommand, activeScope: CommandScope): boolean {
  if (activeScope === 'overlay') return command.scope === 'overlay'
  if (command.scope === 'overlay') return true
  return command.scope === activeScope
}

export function dispatchCommandShortcut(
  event: ShortcutEventLike,
  commands: readonly RegisteredCommand[],
  activeScope: CommandScope,
  mac = isMacPlatform(),
): RegisteredCommand | null {
  if (event.defaultPrevented || event.altKey && event.ctrlKey) return null
  const command = commands.find(candidate => (
    candidate.enabled
    && candidate.shortcut
    && commandAllowedInScope(candidate, activeScope)
    && shortcutMatches(event, candidate.shortcut, mac)
  ))
  if (!command) return null

  // Capture before xterm's textarea listener. An executed application command must
  // result in zero PTY bytes, while an unregistered Ctrl key is left untouched.
  event.preventDefault()
  event.stopPropagation()
  event.stopImmediatePropagation?.()
  void command.run({ source: 'shortcut' })
  return command
}
