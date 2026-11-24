import { ApiError } from '../api/client'
import type { Approver, Attempt, Execution, ExecutionFetcher, ExecutionProgress, ExecutionStatus } from '../api/executions'

const ROWS = 200_000
const BATCH_SIZE = 10_000
const BATCHES = ROWS / BATCH_SIZE
const BATCH_MS = 1_500
const SQL_BATCH = "UPDATE orders SET status = 'archived' WHERE id > $1 AND id <= $2"
const SQL_NORMAL = 'ALTER TABLE orders ADD COLUMN archived_at timestamptz'

const approvers = (reviewer: boolean): Approver[] => [
  { userId: 2, username: 'developer', approved: true },
  { userId: 3, username: 'reviewer', approved: reviewer },
]

const progress = (batches: number): ExecutionProgress => ({
  committedBatches: batches,
  committedRows: batches * BATCH_SIZE,
  lastCursor: String(batches * BATCH_SIZE),
  scopeMax: String(ROWS),
  estimatedRows: ROWS,
})

type AttemptPart = { status: Attempt['status']; startedAtMs: number; finishedAtMs?: number; error?: string }
type Parts = {
  status: ExecutionStatus
  approvers?: Approver[]
  progress?: ExecutionProgress
  attempts?: AttemptPart[]
  error?: { code: string; message: string }
  startedAtMs?: number
  finishedAtMs?: number
}
type Scenario = { id: number; mode: Execution['mode']; at: (t: number) => Parts }

const RUN_FROM = 15_000

const batch: Scenario = {
  id: 9001,
  mode: 'batch',
  at(t) {
    if (t < 10_000) return { status: 'pending_review', approvers: approvers(false) }
    if (t < 12_000) return { status: 'approved' }
    if (t < RUN_FROM) return { status: 'queued' }
    const done = Math.floor((t - RUN_FROM) / BATCH_MS)
    const run: AttemptPart = { status: 'running', startedAtMs: RUN_FROM }
    if (done < BATCHES) return { status: 'running', startedAtMs: RUN_FROM, progress: progress(done), attempts: [run] }
    const end = RUN_FROM + BATCHES * BATCH_MS
    return {
      status: 'success',
      startedAtMs: RUN_FROM,
      finishedAtMs: end,
      progress: progress(BATCHES),
      attempts: [{ ...run, status: 'success', finishedAtMs: end }],
    }
  },
}

const failed: Scenario = {
  id: 9002,
  mode: 'normal',
  at(t) {
    if (t < 4_000) return { status: 'pending_review', approvers: approvers(false) }
    if (t < 5_000) return { status: 'approved' }
    if (t < 6_000) return { status: 'queued' }
    if (t < 9_000) return { status: 'running', startedAtMs: 6_000, attempts: [{ status: 'running', startedAtMs: 6_000 }] }
    const message = 'duplicate key value violates unique constraint "orders_pkey"'
    return {
      status: 'failed',
      startedAtMs: 6_000,
      finishedAtMs: 9_000,
      error: { code: '23505', message },
      attempts: [{ status: 'failed', startedAtMs: 6_000, finishedAtMs: 9_000, error: message }],
    }
  },
}

const partial: Scenario = {
  id: 9003,
  mode: 'batch',
  at(t) {
    if (t < 3_000) return { status: 'queued' }
    const done = Math.floor((t - 3_000) / BATCH_MS)
    if (done < 8) return { status: 'running', startedAtMs: 3_000, progress: progress(done), attempts: [{ status: 'running', startedAtMs: 3_000 }] }
    const message = 'koneksi ke database target terputus'
    return {
      status: 'partial',
      startedAtMs: 3_000,
      finishedAtMs: 15_000,
      progress: progress(8),
      error: { code: 'TARGET_UNREACHABLE', message },
      attempts: [
        { status: 'failed', startedAtMs: 3_000, finishedAtMs: 15_000, error: message },
        { status: 'failed', startedAtMs: 20_000, finishedAtMs: 21_000, error: message },
        { status: 'failed', startedAtMs: 36_000, finishedAtMs: 37_000, error: message },
      ],
    }
  },
}

const uncertain: Scenario = {
  id: 9004,
  mode: 'batch',
  at(t) {
    if (t < 2_000) return { status: 'queued' }
    const done = Math.min(5, Math.floor((t - 2_000) / BATCH_MS))
    if (done < 5) return { status: 'running', startedAtMs: 2_000, progress: progress(done), attempts: [{ status: 'running', startedAtMs: 2_000 }] }
    return { status: 'uncertain', startedAtMs: 2_000, progress: progress(5), attempts: [{ status: 'uncertain', startedAtMs: 2_000 }] }
  },
}

const rejected: Scenario = {
  id: 9005,
  mode: 'normal',
  at(t) {
    if (t < 5_000) return { status: 'pending_review', approvers: approvers(false) }
    return { status: 'rejected', finishedAtMs: 5_000, approvers: approvers(false) }
  },
}

const flaky: Scenario = {
  id: 9006,
  mode: 'batch',
  at(t) {
    if (Math.floor(t / 5_000) % 2 === 1) throw new TypeError('Failed to fetch')
    const done = Math.min(BATCHES - 1, Math.floor(t / BATCH_MS))
    return { status: 'running', startedAtMs: 0, progress: progress(done), attempts: [{ status: 'running', startedAtMs: 0 }] }
  },
}

const scenarios: Record<string, Scenario> = { batch, failed, partial, uncertain, rejected, flaky }

export function snapshot(name: string, elapsedMs: number, startedAt = 0): Execution {
  const s = scenarios[name]
  if (!s) throw new ApiError(404, 'EXECUTION_NOT_FOUND', 'Eksekusi tidak ditemukan.')
  const p = s.at(elapsedMs)
  const iso = (ms?: number) => (ms === undefined ? null : new Date(startedAt + ms).toISOString())
  return {
    id: s.id,
    migrationId: 7,
    databaseId: 1,
    kind: 'run',
    status: p.status,
    mode: s.mode,
    batchSize: s.mode === 'batch' ? BATCH_SIZE : null,
    progress: p.progress ?? null,
    attempts: (p.attempts ?? []).map((a, i) => ({
      number: i + 1,
      status: a.status,
      startedAt: iso(a.startedAtMs) ?? '',
      finishedAt: iso(a.finishedAtMs),
      errorMessage: a.error,
    })),
    errorCode: p.error?.code,
    errorMessage: p.error?.message,
    sqlExecuted: p.startedAtMs === undefined ? null : s.mode === 'batch' ? SQL_BATCH : SQL_NORMAL,
    approvers: p.approvers ?? approvers(true),
    requestedBy: 'developer',
    createdAt: new Date(startedAt).toISOString(),
    startedAt: iso(p.startedAtMs),
    finishedAt: iso(p.finishedAtMs),
  }
}

const started = new Map<string, number>()

const wait = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    const timer = setTimeout(resolve, ms)
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer)
        reject(new DOMException('Aborted', 'AbortError'))
      },
      { once: true },
    )
  })

export const dummyFetcher: ExecutionFetcher = async (id, signal) => {
  const name = id.replace(/^dummy-/, '')
  await wait(150, signal)
  const now = Date.now()
  if (!started.has(name)) started.set(name, now)
  const start = started.get(name) ?? now
  return snapshot(name, now - start, start)
}
