import { useState } from 'react'
import { ApiError } from '../../api/client'
import {
  type Execution,
  type ExecutionFetcher,
  approveExecution,
  cancelExecution,
  getExecution,
  rejectExecution,
  withdrawApproval,
} from '../../api/executions'
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

function ReviewActions({ execution: e, onDone }: { execution: Execution; onDone: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const p = e.permissions
  if (!p.canApprove && !p.canReject && !p.canCancel && !p.canWithdraw) return null

  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    setError('')
    try {
      await action()
      setRejecting(false)
      setReason('')
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Aksi gagal diproses.')
      onDone()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section title="Review">
      {error && <Alert>{error}</Alert>}
      <div className="flex flex-wrap gap-2">
        {p.canApprove && <button className={ui.buttonPrimary} disabled={busy} onClick={() => void run(() => approveExecution(e.id))}>Setujui</button>}
        {p.canReject && !rejecting && <button className={ui.buttonDanger} disabled={busy} onClick={() => setRejecting(true)}>Tolak</button>}
        {p.canWithdraw && <button className={ui.button} disabled={busy} onClick={() => void run(() => withdrawApproval(e.id))}>Cabut persetujuan saya</button>}
        {p.canCancel && (
          <button
            className={ui.button}
            disabled={busy}
            onClick={() => {
              if (confirm('Tarik pengajuan ini? Pengajuan yang ditarik tidak bisa dilanjutkan.')) void run(() => cancelExecution(e.id))
            }}
          >
            Tarik pengajuan
          </button>
        )}
      </div>
      {rejecting && (
        <form
          className="mt-3 space-y-2"
          onSubmit={(ev) => {
            ev.preventDefault()
            void run(() => rejectExecution(e.id, reason))
          }}
        >
          <label className={ui.label} htmlFor="reject-reason">Alasan penolakan (opsional)</label>
          <textarea id="reject-reason" className={`${ui.input} h-20 py-2`} maxLength={500} value={reason} onChange={(ev) => setReason(ev.target.value)} />
          <div className="flex gap-2">
            <button className={ui.buttonDanger} disabled={busy}>Tolak pengajuan</button>
            <button type="button" className={ui.button} disabled={busy} onClick={() => setRejecting(false)}>Batal</button>
          </div>
        </form>
      )}
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

      {e.status === 'pending_review' && e.blockedGroups.length > 0 && (
        <Alert tone="warning">
          {e.blockedGroups.map((g) => `Group ${g.name}`).join(', ')} belum punya leader, sehingga perubahan ini tidak dapat
          disetujui. Minta Platform Admin menetapkan leader di halaman Groups.
        </Alert>
      )}

      <Progress execution={e} />

      <ReviewActions execution={e} onDone={watch.refresh} />

      <Section title="Penyetuju" aside={e.approvers.length || undefined}>
        {e.approvers.length === 0 ? (
          <p className={ui.muted}>Belum ada penyetuju. Group yang punya akses ke database ini belum punya leader.</p>
        ) : (
          <dl>
            {e.approvers.map((a) => (
              <Meta key={`${a.userId}-${a.username}`} label={a.username}>
                <Pill tone={a.approved ? 'positive' : 'neutral'}>{a.approved ? 'Sudah setuju' : 'Belum setuju'}</Pill>
              </Meta>
            ))}
          </dl>
        )}
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
