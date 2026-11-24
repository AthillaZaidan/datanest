import { ApiError } from '../../api/client'
import { type Execution, type ExecutionFetcher, getExecution } from '../../api/executions'
import { type ExecutionWatch, useExecution } from '../../hooks/useExecution'
import { formatTime } from '../../utils/format'
import { Alert, Loading, Meta, Pill, Section } from '../kit'
import { ui } from '../ui'
import ExecutionStatusBadge from './ExecutionStatusBadge'

const kindLabel: Record<Execution['kind'], string> = { run: 'Jalankan', forward: 'Forward', rollback: 'Rollback' }
const attemptLabel: Record<Execution['attempts'][number]['status'], string> = {
  running: 'Berjalan',
  success: 'Sukses',
  failed: 'Gagal',
  uncertain: 'Belum pasti',
}
const nf = new Intl.NumberFormat('id-ID')

function fatalMessage(e: Error) {
  if (e instanceof ApiError) {
    if (e.status === 404) return 'Eksekusi tidak ditemukan.'
    if (e.status === 403) return 'Anda tidak punya akses ke eksekusi ini.'
    if (e.status === 401) return 'Sesi berakhir. Masuk lagi untuk melanjutkan.'
  }
  return e.message
}

function barTone(status: Execution['status']) {
  if (status === 'success') return 'bg-positive'
  if (status === 'failed') return 'bg-negative'
  if (status === 'partial' || status === 'uncertain') return 'bg-warning'
  return 'bg-accent'
}

function Progress({ execution: e }: { execution: Execution }) {
  const p = e.progress
  if (!p) return null
  const total = e.batchSize && p.estimatedRows ? Math.ceil(p.estimatedRows / e.batchSize) : null
  const pct = total ? Math.min(100, Math.round((p.committedBatches / total) * 100)) : null
  return (
    <Section title="Progres batch" aside={pct === null ? undefined : `${pct}%`}>
      {pct !== null && (
        <div
          role="progressbar"
          aria-label="Progres batch"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={pct}
          className="mb-3 h-2 overflow-hidden rounded-full bg-raised"
        >
          <div className={`h-full transition-[width] duration-500 ${barTone(e.status)}`} style={{ width: `${pct}%` }} />
        </div>
      )}
      <dl>
        <Meta label="Batch committed">
          <span className="num">{nf.format(p.committedBatches)}{total !== null && ` dari ${nf.format(total)}`}</span>
        </Meta>
        <Meta label="Row committed">
          <span className="num">{nf.format(p.committedRows)}{p.estimatedRows !== null && ` dari ±${nf.format(p.estimatedRows)}`}</span>
        </Meta>
        <Meta label="Cursor terakhir"><span className={ui.ident}>{p.lastCursor ?? '-'}</span></Meta>
        <Meta label="Batas cakupan"><span className={ui.ident}>{p.scopeMax ?? '-'}</span></Meta>
      </dl>
    </Section>
  )
}

function PollNote({ watch }: { watch: ExecutionWatch }) {
  if (watch.stale) {
    return <Alert tone="warning">Koneksi ke server bermasalah. Menampilkan data terakhir dan mencoba lagi.</Alert>
  }
  if (!watch.execution || watch.updatedAt === null) return null
  const at = new Date(watch.updatedAt).toLocaleTimeString('id-ID')
  const next = watch.delayMs === null ? 'Status akhir, pemantauan berhenti.' : `Dicek lagi tiap ${watch.delayMs / 1000} detik.`
  return <p className={ui.small}>Diperbarui {at}. {next}</p>
}

export default function ExecutionMonitor({ executionId, fetcher = getExecution }: { executionId: string; fetcher?: ExecutionFetcher }) {
  const watch = useExecution(executionId, fetcher)
  const { execution: e, error } = watch

  if (error) return <Alert>{fatalMessage(error)}</Alert>
  if (!e) return <Loading rows={3} label="Memuat status eksekusi" />

  return (
    <div className="space-y-6">
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <ExecutionStatusBadge status={e.status} />
          <Pill>{kindLabel[e.kind]}</Pill>
          <Pill>{e.mode === 'batch' ? 'Per batch' : 'Normal'}</Pill>
        </div>
        <div className="mt-2"><PollNote watch={watch} /></div>
      </div>

      {e.errorMessage && (
        <Alert>
          {e.errorCode && <span className="font-mono">{e.errorCode} </span>}
          {e.errorMessage}
        </Alert>
      )}

      <Progress execution={e} />

      <Section title="Penyetuju">
        <dl>
          {e.approvers.map((a) => (
            <Meta key={a.userId} label={a.username}>
              <Pill tone={a.approved ? 'positive' : 'neutral'}>{a.approved ? 'Sudah setuju' : 'Belum setuju'}</Pill>
            </Meta>
          ))}
        </dl>
      </Section>

      <Section title="Percobaan" aside={e.attempts.length || undefined}>
        {e.attempts.length === 0 ? (
          <p className={ui.muted}>Belum ada percobaan.</p>
        ) : (
          <dl>
            {e.attempts.map((a) => (
              <Meta key={a.number} label={`Percobaan ${a.number}`}>
                {attemptLabel[a.status]}, {formatTime(a.startedAt)}
                {a.errorMessage && <span className="block text-xs text-negative">{a.errorMessage}</span>}
              </Meta>
            ))}
          </dl>
        )}
      </Section>

      {e.sqlExecuted && (
        <Section title="SQL yang dijalankan">
          <pre className="overflow-x-auto rounded-lg bg-raised p-3 font-mono text-[13px]">{e.sqlExecuted}</pre>
        </Section>
      )}
    </div>
  )
}
