import { api } from './client'

export type ExecutionStatus =
  | 'pending_review'
  | 'approved'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'partial'
  | 'uncertain'
  | 'rejected'
  | 'canceled'

export type ExecutionKind = 'run' | 'forward' | 'rollback'

export type ExecutionProgress = {
  committedBatches: number
  committedRows: number
  lastCursor: string | null
  scopeMax: string | null
  estimatedRows: number | null
}

export type Attempt = {
  number: number
  status: 'running' | 'success' | 'failed' | 'uncertain'
  startedAt: string
  finishedAt: string | null
  errorMessage?: string
}

export type Approver = { userId: number; username: string; approved: boolean }

export type Execution = {
  id: number
  migrationId: number
  databaseId: number
  kind: ExecutionKind
  status: ExecutionStatus
  mode: 'normal' | 'batch'
  batchSize: number | null
  progress: ExecutionProgress | null
  attempts: Attempt[]
  errorCode?: string
  errorMessage?: string
  sqlExecuted: string | null
  approvers: Approver[]
  requestedBy: string
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
}

export const pollDelayMs: Record<ExecutionStatus, number | null> = {
  pending_review: 10_000,
  approved: 2_000,
  queued: 2_000,
  running: 2_000,
  partial: 2_000,
  uncertain: 2_000,
  success: null,
  failed: null,
  rejected: null,
  canceled: null,
}

export type ExecutionFetcher = (id: string, signal: AbortSignal) => Promise<Execution>

export const getExecution: ExecutionFetcher = (id, signal) =>
  api<Execution>(`/executions/${encodeURIComponent(id)}`, { signal })
