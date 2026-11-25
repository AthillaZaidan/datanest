import { type ExecutionKind, type ExecutionStatus, isOpenExecution } from '../../api/executions'
import type { Migration } from '../../api/migrations'

export const MAX_BATCH_SIZE = 1_000_000
export const DEFAULT_BATCH_SIZE = '1000'

export const isDraft = (m: Migration) => m.status === 'draft' && !m.frozenAt

export const openExecutionOf = (m: Migration) => (m.lastExecution && isOpenExecution(m.lastExecution.status) ? m.lastExecution : null)

export const openPhrase: Partial<Record<ExecutionStatus, string>> = {
  pending_review: 'sedang menunggu review',
  approved: 'sudah disetujui dan menunggu giliran',
  queued: 'sedang dalam antrean',
  running: 'sedang berjalan',
  uncertain: 'belum pasti hasilnya',
}

export function parseBatchSize(text: string): number | null {
  const t = text.trim()
  if (!/^\d+$/.test(t)) return null
  const n = Number(t)
  return n >= 1 && n <= MAX_BATCH_SIZE ? n : null
}

export const needsBatchSize = (m: Migration, kind: ExecutionKind) => m.mode === 'batch' && kind !== 'rollback'

export const batchSizeFor = (m: Migration, kind: ExecutionKind, size: number | null) =>
  needsBatchSize(m, kind) && size !== null ? size : undefined

export const canSubmit = (m: Migration, kind: ExecutionKind, size: number | null) =>
  openExecutionOf(m) === null && (!needsBatchSize(m, kind) || size !== null)

export type MigrationActions = {
  run: boolean
  rollback: boolean
  forward: boolean
  resume: 'forward' | 'rollback' | null
  undo: boolean
}

export function migrationActions(m: Migration, currentVersion: number): MigrationActions {
  const partial = m.partial
  const top = m.active && m.version === currentVersion
  return {
    run: isDraft(m) && !partial,
    rollback: !partial && top && !m.irreversible,
    forward: !partial && m.forwardable,
    resume: partial ? (partial.kind === 'rollback' ? 'rollback' : 'forward') : null,
    undo: partial !== null && partial.kind !== 'rollback' && !m.irreversible,
  }
}

export const asksBatchSize = (m: Migration, a: MigrationActions) =>
  m.mode === 'batch' && (a.run || a.forward || a.resume === 'forward')
