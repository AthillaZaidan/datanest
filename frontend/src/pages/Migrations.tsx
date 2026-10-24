import { Link, useParams, useSearchParams } from 'react-router'
import { listMigrations } from '../api/migrations'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { formatTime } from '../utils/format'

const filters = ['', 'draft', 'running', 'success', 'failed']

export default function Migrations() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const status = params.get('status') ?? ''
  const { data, error, loading } = useAsync(() => listMigrations(databaseId, status), [databaseId, status])

  return (
    <>
      <h1 className={ui.h1}>
        Riwayat Migration
        <Link className={ui.link} to={`/databases/${databaseId}`}>← Database #{databaseId}</Link>
      </h1>

      <div className="mb-4 flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2">
          Status
          <select
            className={ui.select}
            value={status}
            onChange={(e) => setParams(e.target.value ? { status: e.target.value } : {})}
          >
            {filters.map((f) => (
              <option key={f} value={f}>{f || 'semua'}</option>
            ))}
          </select>
        </label>
        {data && (
          <span>
            Versi schema saat ini: <strong>{data.currentVersion ? `v${data.currentVersion}` : '—'}</strong>
          </span>
        )}
        <Link className={`${ui.buttonPrimary} ml-auto`} to={`/databases/${databaseId}/migrations/new`}>
          + Migration baru
        </Link>
      </div>

      {error && <p className={ui.error}>{error.message}</p>}
      {loading && !data && <p className={ui.muted}>Memuat…</p>}
      {data && data.migrations.length === 0 && <p className={ui.muted}>Belum ada migration.</p>}
      {data && data.migrations.length > 0 && (
        <table className={ui.table}>
          <thead>
            <tr>
              <th className={ui.th}>Versi</th>
              <th className={ui.th}>Status</th>
              <th className={ui.th}>SQL</th>
              <th className={ui.th}>Pelaksana</th>
              <th className={ui.th}>Waktu</th>
            </tr>
          </thead>
          <tbody>
            {data.migrations.map((m) => (
              <tr key={m.id}>
                <td className={ui.td}>{m.version ? `v${m.version}` : '—'}</td>
                <td className={ui.td}><StatusBadge status={m.status} /></td>
                <td className={ui.td}>
                  <Link className="hover:underline" to={`/migrations/${m.id}`}>
                    <code className={ui.code}>{m.sql.length > 60 ? m.sql.slice(0, 60) + '…' : m.sql}</code>
                  </Link>
                  {m.status === 'failed' && m.errorMessage && (
                    <div className={ui.error}>{m.errorMessage.split('\n')[0]}</div>
                  )}
                </td>
                <td className={ui.td}>{m.executedBy || m.createdBy}</td>
                <td className={`${ui.td} whitespace-nowrap`}>{formatTime(m.finishedAt ?? m.updatedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
