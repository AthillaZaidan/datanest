import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { type PendingReview, getPendingReviews } from '../api/executions'
import { Alert, Empty, Loading, PageHeader, Pill } from '../components/kit'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { ago } from '../utils/format'

const kindLabel: Record<PendingReview['kind'], string> = { run: 'Jalankan', forward: 'Forward', rollback: 'Rollback' }

const REFRESH_MS = 10_000

function Votes({ item }: { item: PendingReview }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="num text-sm">{item.approved} dari {item.required}</span>
      {item.approvedByMe && <Pill tone="positive">Anda sudah setuju</Pill>}
      {item.blocked && <Pill tone="warning">Ada group tanpa leader</Pill>}
    </div>
  )
}

export default function Reviews() {
  useCrumbs([{ label: 'Antrean review' }])
  const [tick, setTick] = useState(0)
  useEffect(() => {
    const timer = setInterval(() => setTick((n) => n + 1), REFRESH_MS)
    return () => clearInterval(timer)
  }, [])
  const { data, error, loading } = useAsync(() => getPendingReviews(), [tick])
  const rows = data ?? []

  return (
    <>
      <PageHeader
        title="Antrean review"
        subtitle="Pengajuan yang menunggu persetujuan Anda sebagai leader group. Daftar diperbarui tiap 10 detik."
      />
      {error && <Alert>{error.message}</Alert>}
      {loading && !data && <Loading rows={4} label="Memuat antrean review" />}
      {data && rows.length === 0 && <Empty>Tidak ada pengajuan yang menunggu review Anda.</Empty>}
      {rows.length > 0 && (
        <table className={`${ui.table} ${ui.stack.table}`}>
          <thead className={ui.stack.thead}>
            <tr>
              <th className={`${ui.th} w-24`}>Eksekusi</th>
              <th className={ui.th}>Database</th>
              <th className={`${ui.th} w-28`}>Jenis</th>
              <th className={`${ui.th} w-36`}>Diajukan oleh</th>
              <th className={`${ui.th} w-40`}>Persetujuan</th>
              <th className={`${ui.th} ${ui.num} w-32`}>Diajukan</th>
            </tr>
          </thead>
          <tbody className={ui.stack.tbody}>
            {rows.map((r) => (
              <tr key={r.executionId} className={ui.stack.tr}>
                <td className={`${ui.td} ${ui.stack.td} font-mono text-[13px] font-medium`}>
                  <Link className={ui.link} to={`/executions/${r.executionId}`}>E{r.executionId}</Link>
                </td>
                <td className={`${ui.td} ${ui.stack.td}`}>
                  <Link className={ui.link} to={`/databases/${r.databaseId}`}>{r.databaseName}</Link>
                  <span className="ml-2 text-xs text-muted">migration #{r.migrationId}</span>
                </td>
                <td className={`${ui.td} ${ui.stack.td}`}>{kindLabel[r.kind]}</td>
                <td className={`${ui.td} ${ui.stack.td}`}>{r.requestedBy || '-'}</td>
                <td className={`${ui.td} ${ui.stack.td}`}><Votes item={r} /></td>
                <td className={`${ui.td} ${ui.stack.td} ${ui.num} text-muted`}>{ago(r.createdAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
