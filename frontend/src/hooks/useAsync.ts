import { useEffect, useState } from 'react'

export type AsyncState<T> = { data: T | null; error: Error | null; loading: boolean }

// Runs load whenever deps change and ignores results from stale runs.
export function useAsync<T>(load: () => Promise<T>, deps: unknown[]): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({ data: null, error: null, loading: true })

  // deps are supplied by the caller, so the linter cannot check them.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    let active = true
    setState((s) => ({ ...s, loading: true, error: null }))
    load().then(
      (data) => active && setState({ data, error: null, loading: false }),
      (error: Error) => active && setState({ data: null, error, loading: false }),
    )
    return () => {
      active = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return state
}
