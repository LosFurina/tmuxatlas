import { useState, type FormEvent } from 'react'

interface LoginProps {
  mode: 'setup' | 'login'
  error: string | null
  onSubmit: (password: string) => Promise<boolean>
}

export function Login({ mode, error, onSubmit }: LoginProps) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const isSetup = mode === 'setup'
  const displayError = localError || error

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password || submitting) return
    setLocalError(null)

    if (isSetup) {
      if (password.length < 8) {
        setLocalError('Password must be at least 8 characters')
        return
      }
      if (password !== confirm) {
        setLocalError('Passwords do not match')
        return
      }
    }

    setSubmitting(true)
    await onSubmit(password)
    setSubmitting(false)
  }

  return (
    <div className="flex items-center justify-center h-dvh w-screen bg-background font-mono text-sm font-bold">
      <div className="w-full max-w-sm p-8">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-foreground tracking-tight">TmuxAtlas</h1>
          <p className="text-sm text-muted-foreground mt-2 leading-relaxed">
            all your tmux sessions<br />
            all your ai agents<br />
            one interface
          </p>
          {isSetup && (
            <p className="text-xs text-muted-foreground mt-3">set a password to get started</p>
          )}
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={isSetup ? 'Choose a password' : 'Password'}
              autoFocus
              className="w-full px-3 py-2 bg-input border border-border rounded text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          {isSetup && (
            <div>
              <input
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="Confirm password"
                className="w-full px-3 py-2 bg-input border border-border rounded text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
          )}
          {displayError && (
            <p className="text-sm text-destructive">{displayError}</p>
          )}
          <button
            type="submit"
            disabled={submitting || !password || (isSetup && !confirm)}
            className="w-full px-3 py-2 bg-primary text-primary-foreground rounded font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
          >
            {submitting
              ? (isSetup ? 'Setting up...' : 'Signing in...')
              : (isSetup ? 'Set password' : 'Sign in')
            }
          </button>
        </form>
      </div>
    </div>
  )
}
