import { getJSON, request } from './http'

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
  return getJSON<MigrationList>(`/api/databases/${enc(databaseId)}/migrations${q}`)
}

export function createMigration(databaseId: string, sql: string) {
  return request<Migration>('POST', `/api/databases/${enc(databaseId)}/migrations`, { sql })
}

export function getMigration(id: string) {
  return getJSON<Migration>(`/api/migrations/${enc(id)}`)
}

export function updateMigration(id: string, sql: string, revision: number) {
  return request<Migration>('PATCH', `/api/migrations/${enc(id)}`, { sql, revision })
}

export function deleteMigration(id: string) {
  return request<void>('DELETE', `/api/migrations/${enc(id)}`)
}

export function executeMigration(id: string) {
  return request<Migration>('POST', `/api/migrations/${enc(id)}/execute`)
}

export function duplicateMigration(id: string) {
  return request<Migration>('POST', `/api/migrations/${enc(id)}/duplicate`)
}
