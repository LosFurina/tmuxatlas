import { useState, type FormEvent } from 'react'

interface LoginProps {
  mode: 'setup' | 'login'
  error: string | null
  rpId?: string | null
  origin?: string | null
  onSubmit: (setupToken: string, label?: string) => Promise<boolean>
}

export function Login({ mode, error, rpId, origin, onSubmit }: LoginProps) {
  const [setupToken, setSetupToken] = useState('')
  const [label, setLabel] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const isSetup = mode === 'setup'

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (submitting || (isSetup && !setupToken.trim())) return
    setSubmitting(true)
    await onSubmit(setupToken, label)
    setSubmitting(false)
  }

  return (
    <div className="flex items-center justify-center min-h-dvh w-screen bg-background font-mono text-sm font-bold">
      <div className="w-full max-w-md p-8">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-foreground tracking-tight">TmuxAtlas</h1>
          <p className="text-sm text-muted-foreground mt-2 leading-relaxed">
            all your tmux sessions<br />
            all your ai agents<br />
            one interface
          </p>
        </div>

        <div className="border border-border rounded-lg p-5 bg-card">
          <h2 className="text-base text-foreground">
            {isSetup ? 'Create the administrator passkey' : 'Sign in with a passkey'}
          </h2>
          <p className="text-xs font-normal text-muted-foreground mt-2 leading-relaxed">
            Use this device, choose a password manager such as Proton Pass, Bitwarden, or
            1Password, or select another device to scan a QR code with your iPhone.
          </p>

          <form onSubmit={handleSubmit} className="space-y-3 mt-5">
            {isSetup && (
              <>
                <input
                  type="text"
                  value={setupToken}
                  onChange={(event) => setSetupToken(event.target.value)}
                  placeholder="One-time setup token from server logs"
                  autoComplete="off"
                  autoFocus
                  spellCheck={false}
                  className="w-full px-3 py-2 bg-input border border-border rounded text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <input
                  type="text"
                  value={label}
                  onChange={(event) => setLabel(event.target.value)}
                  placeholder="Passkey label (optional)"
                  autoComplete="off"
                  className="w-full px-3 py-2 bg-input border border-border rounded text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </>
            )}

            {error && <p className="text-sm font-normal text-destructive">{error}</p>}

            <button
              type="submit"
              disabled={submitting || (isSetup && !setupToken.trim())}
              className="w-full px-3 py-2 bg-primary text-primary-foreground rounded font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
            >
              {submitting
                ? (isSetup ? 'Creating passkey...' : 'Waiting for passkey...')
                : (isSetup ? 'Create passkey' : 'Sign in with passkey')}
            </button>
          </form>

          {(rpId || origin) && (
            <p className="text-[11px] font-normal text-muted-foreground mt-4 break-all">
              Passkey domain: {rpId || origin}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
