import { Link, useParams, useSearchParams } from 'react-router'
import { getDatabase } from '../api/databases'
import { listMigrations } from '../api/migrations'
import { HistoryIcon, PlusIcon } from '../components/icons'
import { Alert, Badge, EmptyState, Loading, PageHeader } from '../components/kit'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { formatTime } from '../utils/format'

const filters = [
  { value: '', label: 'Semua' },
  { value: 'draft', label: 'Draft' },
  { value: 'running', label: 'Berjalan' },
  { value: 'success', label: 'Sukses' },
  { value: 'failed', label: 'Gagal' },
]

export default function Migrations() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const status = params.get('status') ?? ''
  const { data, error, loading } = useAsync(() => listMigrations(databaseId, status), [databaseId, status])
  const database = useAsync(() => getDatabase(databaseId), [databaseId])
  const canEdit = database.data?.myLevel === 'EDIT'

  return (
    <>
      <PageHeader
        back={{ to: `/databases/${databaseId}`, label: database.data?.name ?? `Database #${databaseId}` }}
        title="Riwayat migration"
        badges={data && <Badge tone="indigo">Versi schema: {data.currentVersion ? `v${data.currentVersion}` : '—'}</Badge>}
        subtitle="Perubahan schema yang tercatat untuk database ini."
        actions={
          canEdit && (
            <Link className={ui.buttonPrimary} to={`/databases/${databaseId}/migrations/new`}>
              <PlusIcon /> Migration baru
            </Link>
          )
        }
      />

      <div className="mb-4 flex flex-wrap gap-1" role="tablist" aria-label="Filter status">
        {filters.map((f) => (
          <button
            key={f.value}
            role="tab"
            aria-selected={status === f.value}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              status === f.value
                ? 'bg-zinc-900 font-medium text-white dark:bg-zinc-100 dark:text-zinc-900'
                : 'text-zinc-600 hover:bg-zinc-200/70 dark:text-zinc-400 dark:hover:bg-zinc-800'
            }`}
            onClick={() => setParams(f.value ? { status: f.value } : {})}
          >
            {f.label}
          </button>
        ))}
      </div>

      {error && <Alert>{error.message}</Alert>}
      {loading && !data && <Loading />}
      {data && (
        <div className={`${ui.card} overflow-hidden`}>
          {data.migrations.length === 0 ? (
            <EmptyState icon={<HistoryIcon size={20} />} title="Belum ada migration">
              {canEdit ? 'Buat migration baru untuk mulai mengubah schema.' : 'Migration yang dibuat akan muncul di sini.'}
            </EmptyState>
          ) : (
            <div className={ui.tableWrap}>
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
                    <tr key={m.id} className={ui.tr}>
                      <td className={`${ui.td} font-mono text-xs font-medium whitespace-nowrap`}>{m.version ? `v${m.version}` : '—'}</td>
                      <td className={ui.td}><StatusBadge status={m.status} /></td>
                      <td className={`${ui.td} max-w-md`}>
                        <Link className="block truncate font-mono text-xs hover:text-indigo-600 dark:hover:text-indigo-300" to={`/migrations/${m.id}`}>
                          {m.sql.length > 80 ? m.sql.slice(0, 80) + '…' : m.sql}
                        </Link>
                        {m.status === 'failed' && m.errorMessage && (
                          <div className={`mt-1 truncate text-xs ${ui.error}`}>{m.errorMessage.split('\n')[0]}</div>
                        )}
                      </td>
                      <td className={`${ui.td} whitespace-nowrap`}>{m.executedBy || m.createdBy}</td>
                      <td className={`${ui.td} whitespace-nowrap text-zinc-500 dark:text-zinc-400`}>{formatTime(m.finishedAt ?? m.updatedAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </>
  )
}
