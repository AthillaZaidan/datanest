export type ApiError = { code: string; message: string; details?: unknown }

let csrfToken: string | null = null
let unauthenticated: (() => void) | null = null

export function setCSRFToken(token: string | null) { csrfToken = token }
export function onUnauthenticated(handler: (() => void) | null) { unauthenticated = handler }

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
    throw (body?.error ?? { code: 'NETWORK_ERROR', message: 'Request gagal diproses.' }) as ApiError
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
