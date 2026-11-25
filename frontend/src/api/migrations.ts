import { api, send } from './client'

export type MigrationStatus = 'draft' | 'running' | 'success' | 'failed'

export type Migration = {
  id: number
  databaseId: number
  version: number | null
  sql: string
  sqlUp: string
  sqlDown: string
  irreversible: boolean
  irreversibleReason?: string
  // Set when the first execution starts; the recipe can no longer change (spec: immutable SQL).
  frozenAt: string | null
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

export type Recipe = { sqlUp: string; sqlDown: string; irreversible: boolean; irreversibleReason: string }

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
