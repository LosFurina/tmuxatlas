export interface RuntimeAPIErrorBody {
  request_id?: string
  generation?: number
  code?: string
}

export class RuntimeAPIError extends Error {
  requestId?: string
  generation?: number
  code: string

  constructor(status: number, body: RuntimeAPIErrorBody = {}) {
    const code = body.code || `http-${status}`
    super(runtimeErrorMessage(code))
    this.name = 'RuntimeAPIError'
    this.requestId = body.request_id
    this.generation = body.generation
    this.code = code
  }
}

export async function postRuntimeMutation<T>(
  path: string,
  body: Record<string, unknown>,
): Promise<T> {
  const response = await apiFetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    let errorBody: RuntimeAPIErrorBody = {}
    try {
      errorBody = await response.json()
    } catch {}
    throw new RuntimeAPIError(response.status, errorBody)
  }
  return response.json()
}

export function runtimeErrorMessage(code: string): string {
  switch (code) {
    case 'invalid-target': return 'The session target is invalid or incomplete.'
    case 'not-found': return 'The selected session no longer exists.'
    case 'peer-offline': return 'The selected host is offline.'
    case 'peer-revoked': return 'Access to the selected host was revoked.'
    case 'protocol-incompatible': return 'The host must be upgraded before this action can run.'
    case 'capability-unsupported': return 'The host does not support this action.'
    case 'queue-full': return 'The host is busy. Try again shortly.'
    case 'timeout': return 'The host did not finish the action in time.'
    case 'execution-unknown': return 'The result is unknown after the host reconnected. Check the session before retrying.'
    case 'request-conflict': return 'This request conflicts with an earlier action.'
    case 'resource-exhausted': return 'The host cannot retain another action result right now.'
    default: return 'The session action failed.'
  }
}
import { apiFetch } from './api'
