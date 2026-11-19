export class ApiError extends Error {
  status: number
  code: string
  details?: unknown

  constructor(status: number, code: string, message: string, details?: unknown) {
    super(message)
    this.status = status
    this.code = code
    this.details = details
  }
}

let csrfToken: string | null = null
let unauthenticated: (() => void) | null = null

export function setCSRFToken(token: string | null) { csrfToken = token }
export function onUnauthenticated(handler: (() => void) | null) { unauthenticated = handler }

// Every mutating request carries JSON and the CSRF token, even without a body.
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase()
  const headers = new Headers(init.headers)
  if (method !== 'GET' && method !== 'HEAD') {
    headers.set('Content-Type', 'application/json')
    if (csrfToken) headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(`/api${path}`, { ...init, method, headers, credentials: 'same-origin' })
  if (response.status === 401) unauthenticated?.()
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    const e = body?.error
    throw new ApiError(response.status, e?.code ?? 'NETWORK_ERROR', e?.message ?? 'Request gagal diproses.', e?.details)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const send = <T,>(method: string, path: string, body?: unknown) =>
  api<T>(path, { method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
