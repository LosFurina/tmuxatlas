import { useState } from 'react'
import { usePasskeys, type PasskeyMetadata } from '../hooks/usePasskeys'

function formatDate(value?: string) {
  if (!value) return 'Never'
  return new Date(value).toLocaleString()
}

function PasskeyRow({ item, only, pending, onRename, onDelete }: {
  item: PasskeyMetadata
  only: boolean
  pending: boolean
  onRename: (label: string) => Promise<boolean>
  onDelete: () => Promise<boolean>
}) {
  const [editing, setEditing] = useState(false)
  const [label, setLabel] = useState(item.label)
  const [validation, setValidation] = useState<string | null>(null)

  const save = async () => {
    const normalized = label.trim()
    if (!normalized || [...normalized].length > 80) {
      setValidation('Label must be between 1 and 80 characters')
      return
    }
    if (await onRename(normalized)) {
      setEditing(false)
      setValidation(null)
    }
  }

  const remove = async () => {
    if (only || !window.confirm(`Delete ${item.label || 'this passkey'}?`)) return
    await onDelete()
  }

  return (
    <li className="rounded border border-border bg-background p-3" data-testid="passkey-row">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          {editing ? (
            <div className="flex gap-2">
              <input
                aria-label="Passkey label"
                value={label}
                maxLength={80}
                onChange={(event) => setLabel(event.target.value)}
                className="min-w-0 flex-1 bg-input border border-border rounded px-2 py-1 text-sm text-foreground outline-none focus:border-primary"
              />
              <button onClick={save} disabled={pending} className="px-2 py-1 rounded border border-primary text-primary text-xs disabled:opacity-50">
                Save
              </button>
              <button onClick={() => { setEditing(false); setLabel(item.label); setValidation(null) }} className="px-2 py-1 text-xs text-muted-foreground">
                Cancel
              </button>
            </div>
          ) : (
            <div className="text-sm text-foreground break-all">{item.label || 'Unnamed passkey'}</div>
          )}
          {validation && <p role="alert" className="text-xs text-destructive mt-1">{validation}</p>}
          <div className="text-[11px] font-normal text-muted-foreground mt-1">
            Added {formatDate(item.created_at)} · Last used {formatDate(item.last_used_at)}
          </div>
        </div>
        {!editing && (
          <div className="flex gap-2 shrink-0">
            <button onClick={() => setEditing(true)} disabled={pending} className="text-xs text-primary disabled:opacity-50">
              Rename
            </button>
            <button
              onClick={remove}
              disabled={pending || only}
              title={only ? 'The final passkey cannot be deleted' : 'Delete passkey'}
              className="text-xs text-destructive disabled:opacity-40"
            >
              Delete
            </button>
          </div>
        )}
      </div>
    </li>
  )
}

export function PasskeySettings() {
  const { passkeys, loading, pending, error, add, rename, remove } = usePasskeys()
  const [newLabel, setNewLabel] = useState('')

  const addPasskey = async () => {
    if (await add(newLabel)) setNewLabel('')
  }

  return (
    <div className="flex flex-col gap-3" aria-labelledby="passkeys-heading">
      <div className="flex items-end justify-between gap-3">
        <div>
          <div id="passkeys-heading" className="text-sm text-foreground">Passkeys</div>
          <p className="text-xs font-normal text-muted-foreground mt-0.5">
            Add at least one backup before removing a credential.
          </p>
        </div>
        <div className="flex gap-2">
          <input
            aria-label="New passkey label"
            value={newLabel}
            maxLength={80}
            onChange={(event) => setNewLabel(event.target.value)}
            placeholder="Label (optional)"
            className="w-36 bg-input border border-border rounded px-2 py-1 text-xs text-foreground outline-none focus:border-primary"
          />
          <button
            onClick={addPasskey}
            disabled={pending !== null}
            className="px-3 py-1.5 rounded text-xs border border-primary text-primary hover:bg-primary hover:text-primary-foreground transition-colors disabled:opacity-50"
          >
            {pending === 'add' ? 'Adding...' : 'Add passkey'}
          </button>
        </div>
      </div>
      {error && <p role="alert" className="text-xs font-normal text-destructive">{error}</p>}
      {loading ? (
        <p className="text-xs font-normal text-muted-foreground">Loading passkeys...</p>
      ) : passkeys.length === 0 ? (
        <p className="text-xs font-normal text-muted-foreground">No passkeys found.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {passkeys.map((item) => (
            <PasskeyRow
              key={item.id}
              item={item}
              only={passkeys.length === 1}
              pending={pending === item.id}
              onRename={(label) => rename(item.id, label)}
              onDelete={() => remove(item.id)}
            />
          ))}
        </ul>
      )}
    </div>
  )
}
