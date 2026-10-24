import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { message } from '../../api/admin'

export const panel: CSSProperties = { border: '1px solid #8884', borderRadius: 6, padding: 12, margin: '12px 0' }

// Loads data on mount; act runs a mutation, reloads on success, and reports whether it succeeded.
export function useAdmin<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const refresh = useCallback(async () => {
    try {
      setData(await load())
      setError('')
    } catch (e) {
      setError(message(e))
    }
  }, [load])
  useEffect(() => {
    void refresh()
  }, [refresh])
  const act = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      await refresh()
      return true
    } catch (e) {
      setError(message(e))
      return false
    } finally {
      setBusy(false)
    }
  }, [refresh])
  return { data, error, busy, refresh, act, setError }
}
