import { useCallback, useEffect, useState } from 'react'
import { message } from '../../api/admin'

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
