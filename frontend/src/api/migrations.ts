import { api, send } from './client'

export type MigrationStatus = 'draft' | 'running' | 'success' | 'failed'

export type Migration = {
  id: number
  databaseId: number
  version: number | null
  sql: string
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

export type MigrationList = { migrations: Migration[]; currentVersion: number }

const enc = encodeURIComponent

export function listMigrations(databaseId: string, status = '') {
  const q = status ? `?status=${enc(status)}` : ''
  return api<MigrationList>(`/databases/${enc(databaseId)}/migrations${q}`)
}

export function createMigration(databaseId: string, sql: string) {
  return send<Migration>('POST', `/databases/${enc(databaseId)}/migrations`, { sql })
}

export function getMigration(id: string) {
  return api<Migration>(`/migrations/${enc(id)}`)
}

export function updateMigration(id: string, sql: string, revision: number) {
  return send<Migration>('PATCH', `/migrations/${enc(id)}`, { sql, revision })
}

export function deleteMigration(id: string) {
  return send<void>('DELETE', `/migrations/${enc(id)}`)
}

export function executeMigration(id: string) {
  return send<Migration>('POST', `/migrations/${enc(id)}/execute`)
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
