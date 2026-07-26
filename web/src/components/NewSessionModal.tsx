import { useState, useEffect, useRef } from 'react'
import { Host } from '../hooks/useHosts'

interface NewSessionModalProps {
  hosts: Host[]
  onCreateSession: (name: string, hostId: string) => void
  onClose: () => void
}

export function NewSessionModal({ hosts, onCreateSession, onClose }: NewSessionModalProps) {
  const [name, setName] = useState('')
  const onlineHosts = hosts.filter(h => h.online)
  const showHostSelect = onlineHosts.length > 1
  const localHost = onlineHosts.find(h => h.local)
  const [selectedHost, setSelectedHost] = useState<string>(localHost?.id || '')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  useEffect(() => {
    if (!onlineHosts.some(host => host.id === selectedHost)) {
      setSelectedHost(localHost?.id || onlineHosts[0]?.id || '')
    }
  }, [localHost?.id, onlineHosts, selectedHost])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopImmediatePropagation()
        onClose()
      }
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [onClose])

  const handleSubmit = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    if (!selectedHost) return
    onCreateSession(trimmed, selectedHost)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div
      className="fixed inset-0 z-[9999] flex items-start justify-center pt-[max(10vh,env(safe-area-inset-top))] px-[max(1rem,env(safe-area-inset-left))] bg-black/50"
      onClick={onClose}
    >
      <div
        className="w-[400px] bg-card border border-border rounded-xl shadow-2xl flex flex-col overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        <div className="p-4 border-b border-border">
          <div className="text-sm text-foreground font-semibold mb-3">New Session</div>
          <input
            ref={inputRef}
            value={name}
            onChange={e => setName(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Session name..."
            className="w-full text-[15px] text-foreground bg-input border border-border rounded px-3 py-1.5 outline-none font-mono placeholder:text-muted-foreground focus:border-primary"
          />
          {showHostSelect && (
            <div className="mt-3">
              <div className="text-xs text-muted-foreground mb-1.5">Host</div>
              <select
                value={selectedHost}
                onChange={e => setSelectedHost(e.target.value)}
                className="w-full text-sm text-foreground bg-input border border-border rounded px-3 py-1.5 outline-none focus:border-primary"
              >
                {onlineHosts.map(h => (
                  <option key={h.id} value={h.id}>
                    {h.name}{h.local ? ' (local)' : ''}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
        <div className="py-2 px-4 border-t border-border flex justify-between items-center">
          <span className="text-[11px] text-muted-foreground">↵ create &nbsp; esc cancel</span>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="text-xs text-muted-foreground hover:text-foreground px-3 py-1 rounded transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSubmit}
              disabled={!name.trim() || !selectedHost}
              className="text-xs text-foreground bg-primary/20 hover:bg-primary/30 border border-primary/40 px-3 py-1 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              Create
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
