import { useState, useEffect, useCallback } from 'react'
import { themePresets, applyTheme } from '../theme'
import { usePreferences } from '../hooks/usePreferences'
import { usePushNotifications } from '../hooks/usePushNotifications'
import { cn } from '../lib/utils'

export interface AgentStatus {
  name: string
  key: string
  installed: boolean
  configured: boolean
  tracking_mode?: string
  setup_required?: boolean
}

export interface StatusResult {
  agents: AgentStatus[]
  setup_command: string
}

export function normalizeAgentStatus(value: unknown): StatusResult | null {
  if (!value || typeof value !== 'object') return null

  const result = value as Partial<StatusResult>
  const agents = Array.isArray(result.agents)
    ? result.agents.filter((agent): agent is AgentStatus => (
        Boolean(agent)
        && typeof agent.name === 'string'
        && typeof agent.key === 'string'
        && typeof agent.installed === 'boolean'
        && typeof agent.configured === 'boolean'
      ))
    : []

  return {
    agents: agents.map(agent => {
      const tracking_mode = agent.tracking_mode === 'passive' ? 'passive' : 'hook'
      return { ...agent, tracking_mode, setup_required: tracking_mode === 'hook' }
    }),
    setup_command: typeof result.setup_command === 'string' ? result.setup_command : '',
  }
}

export function AgentStatusList({ agents = [] }: { agents?: AgentStatus[] }) {
  return (
    <div className="space-y-2">
      {agents.map(agent => (
        <div key={agent.key} className="flex items-center justify-between py-2 px-3 rounded border border-border bg-background">
          <span className="text-sm text-foreground">{agent.name}</span>
          <div className="flex items-center gap-3 text-xs">
            <span className={agent.installed ? 'text-success' : 'text-muted-foreground'}>
              {agent.installed ? 'installed' : 'not found'}
            </span>
            {agent.installed && (
              <span className={agent.configured ? 'text-success' : 'text-warning'}>
                {agent.tracking_mode === 'passive' ? 'auto-detected' : agent.configured ? 'configured' : 'needs setup'}
              </span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

export function SetupCommandBox({ command }: { command: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }, [command])

  return (
    <button
      onClick={handleCopy}
      className="w-full flex items-center justify-between px-4 py-3 rounded border border-border bg-input text-foreground font-mono text-sm hover:border-primary transition-colors cursor-pointer"
    >
      <span>$ {command}</span>
      <span className="text-xs text-muted-foreground">
        {copied ? 'copied!' : 'click to copy'}
      </span>
    </button>
  )
}

export function Setup({ onComplete, fullPage = false }: { onComplete: () => void; fullPage?: boolean }) {
  const [status, setStatus] = useState<StatusResult | null>(null)
  const [loading, setLoading] = useState(true)
  const { prefs, updatePrefs } = usePreferences()
  const { pushState, subscribe: pushSubscribe } = usePushNotifications()
  const [step, setStep] = useState<'agents' | 'preferences'>('agents')

  const fetchStatus = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch('/api/agent-status')
      if (res.ok) {
        setStatus(normalizeAgentStatus(await res.json()))
      }
    } catch {
      // ignore
    }
    setLoading(false)
  }, [])

  useEffect(() => {
    fetchStatus()
  }, [fetchStatus])

  const allConfigured = status?.agents.every(a => !a.installed || !a.setup_required || a.configured) ?? false

  const handleThemeChange = async (themeName: string) => {
    applyTheme(themeName, prefs.custom_theme)
    await updatePrefs({ theme: themeName })
  }

  return (
    <div className={fullPage ? "flex items-center justify-center min-h-dvh w-screen bg-background py-8" : "flex-1 flex items-center justify-center overflow-y-auto"}>
      <div className="w-full max-w-md p-8">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-foreground tracking-tight">
            {step === 'agents' ? 'agent setup' : 'preferences'}
          </h1>
          <p className="text-sm text-muted-foreground mt-2">
            {step === 'agents'
              ? 'Configure your agents to report status to TmuxAtlas'
              : 'Pick a theme and enable notifications'}
          </p>
        </div>

        {step === 'agents' && (
          <>
            {loading && !status ? (
              <div className="text-center text-sm text-muted-foreground py-8">Checking agents...</div>
            ) : status ? (
              <div className="space-y-5">
                <AgentStatusList agents={status.agents} />

                {!allConfigured && status.setup_command && (
                  <div className="space-y-2">
                    <p className="text-xs text-muted-foreground">
                      Run this command to configure hooks for all installed agents:
                    </p>
                    <SetupCommandBox command={status.setup_command} />
                  </div>
                )}

                {allConfigured && status.agents.length > 0 && (
                  <p className="text-xs text-success text-center">
                    All installed agents are configured.
                  </p>
                )}

                {status.agents.length === 0 && (
                  <p className="text-xs text-muted-foreground text-center">
                    This Hub does not require local agent setup. Install and pair an Agent on each tmux host.
                  </p>
                )}

                <div className="flex gap-3">
                  <button
                    onClick={fetchStatus}
                    disabled={loading}
                    className="flex-1 px-3 py-2 rounded text-sm border border-border text-foreground hover:bg-muted transition-colors disabled:opacity-50"
                  >
                    {loading ? 'Checking...' : 'Refresh'}
                  </button>
                  <button
                    onClick={() => setStep('preferences')}
                    className="flex-1 px-3 py-2 bg-primary text-primary-foreground rounded text-sm font-medium hover:opacity-90 transition-opacity"
                  >
                    Next
                  </button>
                </div>

                <p className="text-xs text-muted-foreground text-center">
                  Multi-host? Run the setup command on each machine where you use agents.
                </p>
              </div>
            ) : (
              <div className="text-center">
                <p className="text-sm text-muted-foreground mb-4">Could not check agent status.</p>
                <button
                  onClick={() => setStep('preferences')}
                  className="px-4 py-2 bg-primary text-primary-foreground rounded text-sm font-medium hover:opacity-90 transition-opacity"
                >
                  Next
                </button>
              </div>
            )}
          </>
        )}

        {step === 'preferences' && (
          <div className="space-y-5">
            {/* Theme picker */}
            <div>
              <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Theme</h3>
              <div className="grid grid-cols-2 gap-2">
                {Object.values(themePresets).map(theme => (
                  <button
                    key={theme.name}
                    onClick={() => handleThemeChange(theme.name)}
                    className={cn(
                      'p-3 rounded-lg border text-left transition-colors',
                      prefs.theme === theme.name
                        ? 'border-primary bg-primary/10 text-primary'
                        : 'border-border bg-background text-foreground hover:border-primary/40',
                    )}
                  >
                    <div className="flex items-center gap-1.5 mb-1.5">
                      <div className="w-3 h-3 rounded-full border border-border" style={{ background: theme.xterm.background }} />
                      <div className="w-3 h-3 rounded-full border border-border" style={{ background: theme.xterm.foreground }} />
                      <div className="w-3 h-3 rounded-full border border-border" style={{ background: theme.xterm.blue }} />
                      <div className="w-3 h-3 rounded-full border border-border" style={{ background: theme.xterm.green }} />
                    </div>
                    <div className="text-sm font-semibold">{theme.label}</div>
                  </button>
                ))}
              </div>
            </div>

            {/* Push notifications */}
            <div>
              <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Notifications</h3>
              <div className="rounded border border-border bg-background p-3">
                {pushState === 'unsupported' ? (
                  <p className="text-sm text-muted-foreground">
                    Push notifications require HTTPS or localhost with a supported browser.
                  </p>
                ) : pushState === 'denied' ? (
                  <p className="text-sm text-muted-foreground">
                    Push notifications are blocked by your browser. You can reset this in your browser's site settings.
                  </p>
                ) : pushState === 'subscribed' ? (
                  <div className="flex items-center gap-2">
                    <span className="text-success text-sm">Push notifications enabled</span>
                  </div>
                ) : (
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm text-foreground">Push notifications</p>
                      <p className="text-xs text-muted-foreground mt-0.5">Get alerted when agents need attention</p>
                    </div>
                    <button
                      onClick={pushSubscribe}
                      className="px-3 py-1.5 rounded text-sm border border-primary text-primary hover:bg-primary hover:text-primary-foreground transition-colors"
                    >
                      Enable
                    </button>
                  </div>
                )}
              </div>
            </div>

            {/* Keyboard shortcuts hint */}
            <p className="text-xs text-muted-foreground text-center">
              Press <kbd className="inline-flex items-center justify-center min-w-[20px] h-5 px-1 rounded border border-border bg-muted text-foreground text-xs font-mono">{/Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl'}</kbd>+<kbd className="inline-flex items-center justify-center min-w-[20px] h-5 px-1 rounded border border-border bg-muted text-foreground text-xs font-mono">/</kbd> anytime to see all keyboard shortcuts
            </p>

            {/* Navigation */}
            <div className="flex gap-3">
              <button
                onClick={() => setStep('agents')}
                className="flex-1 px-3 py-2 rounded text-sm border border-border text-foreground hover:bg-muted transition-colors"
              >
                Back
              </button>
              <button
                onClick={onComplete}
                className="flex-1 px-3 py-2 bg-primary text-primary-foreground rounded text-sm font-medium hover:opacity-90 transition-opacity"
              >
                Continue to Dashboard
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
