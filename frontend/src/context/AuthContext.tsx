import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { api, onUnauthenticated, setCSRFToken } from '../api/client'

export type User = { id: number; username: string; isAdmin: boolean }
type AuthResponse = { user: User; csrfToken: string }
type AuthValue = { user: User | null; loading: boolean; login(username: string, password: string): Promise<void>; logout(): Promise<void> }
const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const accept = (result: AuthResponse) => { setCSRFToken(result.csrfToken); setUser(result.user) }
  useEffect(() => {
    onUnauthenticated(() => { setCSRFToken(null); setUser(null) })
    api<AuthResponse>('/auth/me').then(accept).catch(() => undefined).finally(() => setLoading(false))
    return () => onUnauthenticated(null)
  }, [])
  const value = useMemo<AuthValue>(() => ({ user, loading, async login(username, password) { accept(await api<AuthResponse>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) })) }, async logout() { await api<void>('/auth/logout', { method: 'POST' }); setCSRFToken(null); setUser(null) } }), [user, loading])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
export function useAuth() { const value = useContext(AuthContext); if (!value) throw new Error('useAuth must be used inside AuthProvider'); return value }
