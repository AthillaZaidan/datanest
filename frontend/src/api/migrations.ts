import { api, send } from './client'
import type { Execution, ExecutionKind, ExecutionStatus } from './executions'

export type MigrationStatus = 'draft' | 'running' | 'success' | 'failed'

export type MigrationMode = 'normal' | 'batch'

export type Migration = {
  id: number
  databaseId: number
  version: number | null
  sql: string
  sqlUp: string
  sqlDown: string
  irreversible: boolean
  irreversibleReason?: string
  mode: MigrationMode
  batchTable: string
  batchColumn: string
  lastExecution: { id: number; kind: ExecutionKind; status: ExecutionStatus } | null
  partial: { executionId: number; kind: ExecutionKind } | null
  // Set when the first execution starts; the recipe can no longer change (spec: immutable SQL).
  frozenAt: string | null
  // From the server's derived position, the one source of truth for what is applied.
  active: boolean
  forwardable: boolean
  status: MigrationStatus
  errorMessage?: string
  errorCode?: string
  baseVersion: number | null
  baseOutdated: boolean
  revision: number
  createdBy: string
  updatedBy?: string
  executedBy?: string
  createdAt: string
  updatedAt: string
  startedAt: string | null
  finishedAt: string | null
}

// What a migration is now. M1 status alone is not enough: an M2 migration keeps status
// draft after it runs, and a rolled-back one keeps the status of its old run.
export type MigrationState = 'partial' | 'running' | 'active' | 'review' | 'draft' | 'failed' | 'inactive'

export function migrationState(m: Migration): MigrationState {
  const last = m.lastExecution
  if (m.partial) return 'partial'
  if (last && (last.status === 'queued' || last.status === 'running' || last.status === 'uncertain')) return 'running'
  if (m.active) return 'active'
  if (last && (last.status === 'pending_review' || last.status === 'approved')) return 'review'
  if (m.status === 'draft' && !m.frozenAt && m.version === null) return 'draft'
  if (last && last.status === 'failed' && last.kind !== 'rollback') return 'failed'
  return 'inactive'
}

export type MigrationList = { migrations: Migration[]; currentVersion: number }

const enc = encodeURIComponent

export function listMigrations(databaseId: string, status = '') {
  const q = status ? `?status=${enc(status)}` : ''
  return api<MigrationList>(`/databases/${enc(databaseId)}/migrations${q}`)
}

export type Recipe = {
  sqlUp: string
  sqlDown: string
  irreversible: boolean
  irreversibleReason: string
  mode: MigrationMode
  batchTable: string
  batchColumn: string
}

export function createMigration(databaseId: string, recipe: Recipe) {
  return send<Migration>('POST', `/databases/${enc(databaseId)}/migrations`, recipe)
}

export function getMigration(id: string) {
  return api<Migration>(`/migrations/${enc(id)}`)
}

export function updateMigration(id: string, recipe: Recipe, revision: number) {
  return send<Migration>('PATCH', `/migrations/${enc(id)}`, { ...recipe, revision })
}

export function deleteMigration(id: string) {
  return send<void>('DELETE', `/migrations/${enc(id)}`)
}

// Run, forward and rollback are new executions that go through approval and the queue.
export function submitExecution(id: string, kind: ExecutionKind, batchSize?: number) {
  return send<Execution>('POST', `/migrations/${enc(id)}/executions`, batchSize === undefined ? { kind } : { kind, batchSize })
}

export type HistoryItem = {
  id: number
  migrationId: number
  version: number | null
  revision: number
  kind: ExecutionKind
  status: ExecutionStatus
  sqlExecuted: string
  revertsExecutionId: number | null
  requestedBy: string
  errorCode?: string
  errorMessage?: string
  startedAt: string | null
  finishedAt: string | null
}

export function listExecutions(databaseId: string, hideRollback: boolean) {
  return api<{ executions: HistoryItem[] }>(`/databases/${enc(databaseId)}/executions${hideRollback ? '?hideRollback=true' : ''}`)
}

export function duplicateMigration(id: string) {
  return send<Migration>('POST', `/migrations/${enc(id)}/duplicate`)
}

export type PlanNode = {
  depth: number
  nodeType: string
  operation?: string
  relation?: string
  index?: string
  joinType?: string
  strategy?: string
  startupCost: number
  totalCost: number
  planRows: number
  actualRows: number | null
  actualMs: number | null
  loops: number | null
}

export type StatementPlan = {
  sql: string
  explained: boolean
  note?: string
  error?: string
  nodes: PlanNode[] | null
  planningMs: number | null
  executionMs: number | null
}

// EXPLAIN runs in a rolled-back transaction; analyze also executes the SQL, so it needs EDIT.
export function explainMigration(id: string, analyze: boolean) {
  return send<{ plans: StatementPlan[] | null }>('POST', `/migrations/${enc(id)}/explain${analyze ? '?analyze=true' : ''}`)
}
