import { useMemo, useState } from 'react'
import { useApplicationState } from '../state/provider'
import type { HealthState } from '../state/types'

interface FleetReason {
  code: string
  severity: number
  message: string
  evidence?: string
  remediation?: string
}

interface FleetRecord {
  facts: {
    host_id: string
    display_name: string
    role?: string
    version?: string
    hub_version?: string
    last_seen?: string
    last_state_sync?: string
    deployment?: string
    last_update?: {
      outcome?: string
      target_version?: string
      restored_version?: string
      error?: string
    }
  }
  summary: string
  reasons: FleetReason[]
  evaluated_at?: string
}

function parseRecord(health: HealthState): FleetRecord | null {
  const candidate = health.facts as unknown as FleetRecord | undefined
  if (!candidate?.facts?.host_id || !candidate.summary || !Array.isArray(candidate.reasons)) {
    return null
  }
  return candidate
}

export function FleetHealth() {
  const { state } = useApplicationState()
  const records = useMemo(() => (
    Object.values(state.projection.health)
      .map(parseRecord)
      .filter((record): record is FleetRecord => record !== null)
      .sort((left, right) => left.facts.host_id.localeCompare(right.facts.host_id))
  ), [state.projection.health])
  return <FleetHealthView records={records} />
}

export function FleetHealthView({ records }: { records: FleetRecord[] }) {
  const [copied, setCopied] = useState<string | null>(null)
  if (records.length === 0) return null

  const copy = async (hostID: string, command: string) => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(hostID + command)
    } catch {
      setCopied(null)
    }
  }

  return (
    <section aria-labelledby="fleet-health-title" className="mb-6">
      <h2 id="fleet-health-title" className="mb-2.5 text-sm font-semibold text-foreground">
        Fleet Health
      </h2>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {records.map((record) => (
          <article
            key={record.facts.host_id}
            data-host-id={record.facts.host_id}
            className="rounded-lg border border-border bg-card p-3.5"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-sm text-foreground">
                  {record.facts.display_name || record.facts.host_id}
                </div>
                <div className="truncate text-[10px] font-normal text-muted-foreground">
                  {record.facts.host_id}
                </div>
              </div>
              <span className={record.summary === 'healthy' ? 'text-success' : record.summary === 'offline' || record.summary === 'incompatible' ? 'text-destructive' : 'text-warning'}>
                {record.summary}
              </span>
            </div>
            <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] font-normal">
              <dt className="text-muted-foreground">Role</dt>
              <dd>{record.facts.role || 'unknown'}</dd>
              <dt className="text-muted-foreground">Version</dt>
              <dd>{record.facts.version || 'unknown'}</dd>
              <dt className="text-muted-foreground">Hub</dt>
              <dd>{record.facts.hub_version || 'unknown'}</dd>
              <dt className="text-muted-foreground">Last sync</dt>
              <dd>{record.facts.last_state_sync || 'unknown'}</dd>
            </dl>
            <details className="mt-3 text-xs font-normal">
              <summary className="cursor-pointer text-muted-foreground">
                {record.reasons.length} reason{record.reasons.length === 1 ? '' : 's'}
              </summary>
              <div className="mt-2 flex flex-col gap-2">
                {record.reasons.map((reason, index) => (
                  <div key={`${reason.code}-${index}`} className="rounded border border-border/70 p-2">
                    <div className="font-semibold">{reason.code}</div>
                    <div>{reason.message}</div>
                    {reason.evidence && <div className="mt-1 text-muted-foreground">{reason.evidence}</div>}
                    {reason.remediation && (
                      <div className="mt-2 flex items-start gap-2">
                        <code className="min-w-0 flex-1 break-all text-[10px]">{reason.remediation}</code>
                        <button
                          type="button"
                          aria-label={`Copy remediation for ${record.facts.host_id}`}
                          onClick={() => void copy(record.facts.host_id, reason.remediation!)}
                          className="min-h-9 rounded border border-border px-2"
                        >
                          {copied === record.facts.host_id + reason.remediation ? 'Copied' : 'Copy'}
                        </button>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </details>
          </article>
        ))}
      </div>
    </section>
  )
}
