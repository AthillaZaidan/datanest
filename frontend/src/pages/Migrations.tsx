import { Link, useParams, useSearchParams } from 'react-router'
import { getDatabase } from '../api/databases'
import { type Migration, type MigrationStatus, listMigrations } from '../api/migrations'
import { PlusIcon } from '../components/icons'
import { Alert, Empty, Loading, PageHeader, Tabs } from '../components/kit'
import StatusBadge from '../components/migrations/StatusBadge'
import VersionRail from '../components/migrations/VersionRail'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { ago, formatTime } from '../utils/format'

const filters: { key: '' | MigrationStatus; label: string }[] = [
  { key: '', label: 'Semua' },
  { key: 'draft', label: 'Draft' },
  { key: 'running', label: 'Berjalan' },
  { key: 'success', label: 'Sukses' },
  { key: 'failed', label: 'Gagal' },
]

function finding(migrations: Migration[]) {
  const last = migrations.filter((m) => m.version !== null).sort((a, b) => (b.version ?? 0) - (a.version ?? 0))[0]
  const drafts = migrations.filter((m) => m.status === 'draft').length
  const waiting = drafts > 0 ? ` ${drafts} draft menunggu dijalankan.` : ''
  if (!last) return `Belum ada migration yang dijalankan.${waiting}`
  const when = ago(last.finishedAt ?? last.updatedAt)
  if (last.status === 'failed') return `v${last.version} gagal ${when}: ${(last.errorMessage ?? '').split('\n')[0]}.${waiting}`
  if (last.status === 'running') return `v${last.version} sedang berjalan.${waiting}`
  return `v${last.version} dijalankan ${last.executedBy || last.createdBy} ${when}.${waiting}`
}

export default function Migrations() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const status = (params.get('status') ?? '') as '' | MigrationStatus
  const { data, error, loading } = useAsync(() => listMigrations(databaseId), [databaseId])
  const database = useAsync(() => getDatabase(databaseId), [databaseId])
  const canEdit = database.data?.myLevel === 'EDIT'
  const name = database.data?.name ?? `Database #${databaseId}`
  useCrumbs([
    { label: 'Databases', to: '/databases' },
    { label: name, to: `/databases/${databaseId}` },
    { label: 'Riwayat migration' },
  ])

  const all = data?.migrations ?? []
  const rows = status ? all.filter((m) => m.status === status) : all

  return (
    <>
      <PageHeader
        title="Riwayat migration"
        subtitle={data && finding(all)}
        actions={
          canEdit && (
            <Link className={ui.buttonPrimary} to={`/databases/${databaseId}/migrations/new`}>
              <PlusIcon /> Migration baru
            </Link>
          )
        }
      />

      {error && <Alert>{error.message}</Alert>}
      {loading && !data && <Loading rows={5} />}
      {data && (
        <>
          <div className="mb-8 flex flex-wrap items-end gap-x-10 gap-y-4">
            <div>
              <p className="text-sm text-muted">Versi schema {name}</p>
              <p className="font-mono text-[28px] leading-9">{data.currentVersion ? `v${data.currentVersion}` : '-'}</p>
            </div>
            <div className="pb-1">
              <VersionRail migrations={all} currentVersion={data.currentVersion} />
            </div>
          </div>

          <Tabs
            label="Filter status"
            value={status}
            onChange={(k) => setParams(k ? { status: k } : {})}
            items={filters.map((f) => ({ ...f, count: f.key ? all.filter((m) => m.status === f.key).length : all.length }))}
          />

          {rows.length === 0 ? (
            <div className="pt-5">
              <Empty>
                {status
                  ? 'Tidak ada migration dengan status ini.'
                  : canEdit
                    ? 'Belum ada migration. Buat migration baru untuk mulai mengubah schema.'
                    : 'Belum ada migration untuk database ini.'}
              </Empty>
            </div>
          ) : (
            <>
            <ul className="mt-1 sm:hidden">
              {rows.map((m) => (
                <li key={m.id} className="border-t border-rule first:border-t-0">
                  <Link to={`/migrations/${m.id}`} className="block py-3">
                    <span className="flex items-center gap-2">
                      <span className="font-mono text-[13px] font-medium">{m.version ? `v${m.version}` : 'draft'}</span>
                      <StatusBadge status={m.status} />
                      <span className="ml-auto text-xs text-muted">{m.executedBy || m.createdBy}, {ago(m.finishedAt ?? m.updatedAt)}</span>
                    </span>
                    <span className="mt-1 block truncate font-mono text-[13px] text-muted">{m.sql}</span>
                    {m.status === 'failed' && m.errorMessage && (
                      <span className="mt-0.5 block truncate text-xs text-negative">{m.errorCode} {m.errorMessage.split('\n')[0]}</span>
                    )}
                  </Link>
                </li>
              ))}
            </ul>
            <div className="mt-2 hidden sm:block"><table className={ui.table}>
              <thead>
                <tr>
                  <th className={`${ui.th} w-16`}>Versi</th>
                  <th className={`${ui.th} w-24`}>Status</th>
                  <th className={ui.th}>SQL</th>
                  <th className={`${ui.th} w-32`}>Oleh</th>
                  <th className={`${ui.th} ${ui.num} w-44`}>Waktu</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((m) => (
                  <tr key={m.id}>
                    <td className={`${ui.td} h-14 font-mono text-[13px] font-medium`}>{m.version ? `v${m.version}` : <span className="text-muted">-</span>}</td>
                    <td className={ui.td}><StatusBadge status={m.status} /></td>
                    <td className={`${ui.td} max-w-0`}>
                      <Link className="block truncate font-mono text-[13px] hover:underline" to={`/migrations/${m.id}`}>
                        {m.sql}
                      </Link>
                      {m.status === 'failed' && m.errorMessage && (
                        <p className="truncate text-xs text-negative">
                          {m.errorCode && <span className="font-mono">{m.errorCode} </span>}
                          {m.errorMessage.split('\n')[0]}
                        </p>
                      )}
                    </td>
                    <td className={`${ui.td} whitespace-nowrap`}>{m.executedBy || m.createdBy}</td>
                    <td className={`${ui.td} ${ui.num} text-muted whitespace-nowrap`}>{formatTime(m.finishedAt ?? m.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table></div>
            </>
          )}
        </>
      )}
    </>
  )
}
