import { useEffect } from 'react'
import { usePreferences } from '../hooks/usePreferences'

interface HelpModalProps {
  onClose: () => void
}

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent)
const mod = isMac ? '⌘' : 'Ctrl'

const shortcutLabels: Record<string, string> = {
  'ctrl+k': `${mod}+K`,
  'ctrl+p': `${mod}+P`,
  'ctrl+space': `${mod}+Space`,
}

type ShortcutItem = { section: string } | { keys: string[]; label: string }

function getShortcuts(quickSwitcherKey: string): ShortcutItem[] {
  return [
    { section: 'Navigation' },
    { keys: [shortcutLabels[quickSwitcherKey] || `${mod}+K`], label: 'Quick Switcher' },
    { keys: [`${mod}+J`], label: 'Jump to next alert' },
    { keys: [`${mod}+H`], label: 'Overview' },
    { keys: [`${mod}+,`], label: 'Settings' },
    { keys: [`${mod}+/`], label: 'Help' },
    { keys: [`${mod}+L`], label: 'Lock / Sign out' },

    { keys: [`${mod}+\\`], label: 'Toggle sidebar' },

    { section: 'Terminal' },
    { keys: [`${mod}+Shift+F`], label: 'Toggle fullscreen' },
    { keys: ['Esc'], label: 'Exit fullscreen' },

    { section: 'Quick Switcher' },
    { keys: ['↑ ↓'], label: 'Navigate items' },
    { keys: ['↵'], label: 'Select / Create' },
    { keys: ['Esc'], label: 'Close' },
  ]
}

function Kbd({ children }: { children: string }) {
  return (
    <kbd className="inline-flex items-center justify-center min-w-[24px] h-6 px-1.5 rounded border border-border bg-muted text-foreground text-xs font-mono">
      {children}
    </kbd>
  )
}

export function HelpModal({ onClose }: HelpModalProps) {
  const { prefs } = usePreferences()
  const shortcuts = getShortcuts(prefs.quick_switcher_shortcut || 'ctrl+k')

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopImmediatePropagation()
        onClose()
      }
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="bg-card border border-border rounded-lg shadow-lg w-full max-w-md mx-4 overflow-hidden">
        <div className="flex items-center justify-between px-5 py-3 border-b border-border">
          <h2 className="text-sm font-semibold text-foreground tracking-wider uppercase">Keyboard Shortcuts</h2>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground text-lg leading-none px-1">×</button>
        </div>
        <div className="px-5 py-4 max-h-[70vh] overflow-y-auto">
          {shortcuts.map((item, i) => {
            if ('section' in item && item.section) {
              return (
                <div key={i} className={`text-xs font-semibold text-muted-foreground uppercase tracking-wider ${i > 0 ? 'mt-4' : ''} mb-2`}>
                  {item.section}
                </div>
              )
            }
            return (
              <div key={i} className="flex items-center justify-between py-1.5">
                <span className="text-sm text-foreground">{'label' in item ? item.label : ''}</span>
                <div className="flex items-center gap-1">
                  {'keys' in item && item.keys?.map((k, j) => (
                    <Kbd key={j}>{k}</Kbd>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
        <div className="px-5 py-2.5 border-t border-border text-xs text-muted-foreground">
          Press <Kbd>Esc</Kbd> to close
        </div>
      </div>
    </div>
  )
}
