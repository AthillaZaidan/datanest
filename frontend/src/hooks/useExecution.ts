import { useEffect, useState } from 'react'
import { ApiError } from '../api/client'
import { type Execution, type ExecutionFetcher, pollDelayMs } from '../api/executions'

export type ExecutionWatch = {
  execution: Execution | null
  error: Error | null
  stale: boolean
  delayMs: number | null
  updatedAt: number | null
  refresh: () => void
}

type WatchState = Omit<ExecutionWatch, 'refresh'>

const initial: WatchState = { execution: null, error: null, stale: false, delayMs: null, updatedAt: null }

const stopStatuses = new Set([401, 403, 404])
const RETRY_MS = 10_000

export function useExecution(id: string, fetcher: ExecutionFetcher): ExecutionWatch {
  const [state, setState] = useState<WatchState & { id: string }>({ ...initial, id })
  const [tick, setTick] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    const patch = (next: (w: WatchState) => WatchState) =>
      setState((s) => ({ ...next(s.id === id ? s : initial), id }))

    async function poll() {
      let delayMs: number | null = null
      try {
        const execution = await fetcher(id, controller.signal)
        if (controller.signal.aborted) return
        delayMs = pollDelayMs[execution.status]
        patch(() => ({ execution, error: null, stale: false, delayMs, updatedAt: Date.now() }))
      } catch (e) {
        if (controller.signal.aborted) return
        if (e instanceof ApiError && stopStatuses.has(e.status)) {
          patch((w) => ({ ...w, error: e, stale: false, delayMs: null }))
          return
        }
        delayMs = RETRY_MS
        patch((w) => ({ ...w, stale: true, delayMs: RETRY_MS }))
      }
      if (delayMs !== null) timer = setTimeout(poll, delayMs)
    }

    void poll()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [id, fetcher, tick])

  return { ...(state.id === id ? state : initial), refresh: () => setTick((n) => n + 1) }
}
