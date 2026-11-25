import { api, send } from './client'

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

export type Permissions = { canApprove: boolean; canReject: boolean; canCancel: boolean; canWithdraw: boolean }

export type BlockedGroup = { id: number; name: string }

export type Execution = {
  id: number
  migrationId: number
  databaseId: number
  kind: ExecutionKind
  status: ExecutionStatus
  mode: 'normal' | 'batch'
  batchSize: number | null
  batchTable?: string
  batchColumn?: string
  revertsExecutionId: number | null
  continuesExecutionId: number | null
  version: number | null
  revision: number
  progress: ExecutionProgress | null
  attempts: Attempt[]
  errorCode?: string
  errorMessage?: string
  sqlExecuted: string | null
  approvers: Approver[]
  blockedGroups: BlockedGroup[]
  permissions: Permissions
  requestedBy: string
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
}

const openStatuses: ReadonlySet<ExecutionStatus> = new Set(['pending_review', 'approved', 'queued', 'running', 'uncertain'])

export const isOpenExecution = (status: ExecutionStatus) => openStatuses.has(status)

export const isHeld = (e: Pick<Execution, 'errorCode'>) => e.errorCode === 'HELD'

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

export type PendingReview = {
  executionId: number
  migrationId: number
  databaseId: number
  databaseName: string
  kind: ExecutionKind
  requestedBy: string
  createdAt: string
  approvedByMe: boolean
  approved: number
  required: number
  blocked: boolean
}

export const getPendingReviews = (signal?: AbortSignal) => api<PendingReview[]>('/approvals/pending', { signal })

export const approveExecution = (id: number) => send<Execution>('POST', `/executions/${id}/approve`)
export const rejectExecution = (id: number, reason: string) =>
  send<Execution>('POST', `/executions/${id}/reject`, reason.trim() ? { reason: reason.trim() } : undefined)
export const cancelExecution = (id: number) => send<Execution>('POST', `/executions/${id}/cancel`)
export const withdrawApproval = (id: number) => send<Execution>('POST', `/executions/${id}/withdraw`)

export type ExecutionFetcher = (id: string, signal: AbortSignal) => Promise<Execution>

export const getExecution: ExecutionFetcher = (id, signal) =>
  api<Execution>(`/executions/${encodeURIComponent(id)}`, { signal })
