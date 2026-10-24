import { Link, useParams, useSearchParams } from 'react-router'
import { listMigrations } from '../api/migrations'
import StatusBadge from '../components/migrations/StatusBadge'
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
      <h1>
        Riwayat Migration <Link to={`/databases/${databaseId}`}>← Database #{databaseId}</Link>
      </h1>

      <div className="toolbar">
        <label>
          Status{' '}
          <select value={status} onChange={(e) => setParams(e.target.value ? { status: e.target.value } : {})}>
            {filters.map((f) => (
              <option key={f} value={f}>{f || 'semua'}</option>
            ))}
          </select>
        </label>
        {data && <span>Versi schema saat ini: <strong>{data.currentVersion || '—'}</strong></span>}
        <Link to={`/databases/${databaseId}/migrations/new`}>+ Migration baru</Link>
      </div>

      {error && <p className="error">{error.message}</p>}
      {loading && !data && <p>Memuat…</p>}
      {data && data.migrations.length === 0 && <p>Belum ada migration.</p>}
      {data && data.migrations.length > 0 && (
        <table className="grid">
          <thead>
            <tr>
              <th>Versi</th>
              <th>Status</th>
              <th>SQL</th>
              <th>Pelaksana</th>
              <th>Waktu</th>
            </tr>
          </thead>
          <tbody>
            {data.migrations.map((m) => (
              <tr key={m.id}>
                <td>{m.version ? `v${m.version}` : '—'}</td>
                <td><StatusBadge status={m.status} /></td>
                <td>
                  <Link to={`/migrations/${m.id}`}>
                    <code>{m.sql.length > 60 ? m.sql.slice(0, 60) + '…' : m.sql}</code>
                  </Link>
                  {m.status === 'failed' && m.errorMessage && <div className="error">{m.errorMessage.split('\n')[0]}</div>}
                </td>
                <td>{m.executedBy || m.createdBy}</td>
                <td>{formatTime(m.finishedAt ?? m.updatedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
