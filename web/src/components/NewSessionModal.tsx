import { useState, useEffect, useRef } from 'react'
import type { Host } from '../hooks/useHosts'
import { Button, Dialog } from './ui'

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
    if (!onlineHosts.some(host => host.id === selectedHost)) {
      setSelectedHost(localHost?.id || onlineHosts[0]?.id || '')
    }
  }, [localHost?.id, onlineHosts, selectedHost])

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
    <Dialog
      open
      onOpenChange={open => { if (!open) onClose() }}
      title="New Session"
      description="Create a tmux Session on an online Host."
      initialFocusRef={inputRef}
      footer={(
        <>
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button
            variant="primary"
            onClick={handleSubmit}
            disabled={!name.trim() || !selectedHost}
          >
            Create
          </Button>
        </>
      )}
    >
      <label className="block text-xs text-muted-foreground" htmlFor="new-session-name">
        Session name
      </label>
      <input
        id="new-session-name"
        ref={inputRef}
        value={name}
        onChange={e => setName(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Session name..."
        className="mt-1 min-h-11 w-full rounded border border-border bg-input px-3 font-mono text-[15px] text-foreground outline-none placeholder:text-muted-foreground focus:border-primary"
      />
      {showHostSelect && (
        <div className="mt-4">
          <label className="block text-xs text-muted-foreground" htmlFor="new-session-host">Host</label>
          <select
            id="new-session-host"
            value={selectedHost}
            onChange={e => setSelectedHost(e.target.value)}
            className="mt-1 min-h-11 w-full rounded border border-border bg-input px-3 text-sm text-foreground outline-none focus:border-primary"
          >
            {onlineHosts.map(h => (
              <option key={h.id} value={h.id}>
                {h.name}{h.local ? ' (local)' : ''}
              </option>
            ))}
          </select>
        </div>
      )}
    </Dialog>
  )
}
