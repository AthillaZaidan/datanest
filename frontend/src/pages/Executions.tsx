import { Link, useParams, useSearchParams } from 'react-router'
import { getDatabase } from '../api/databases'
import { type HistoryItem, listExecutions } from '../api/migrations'
import ExecutionStatusBadge from '../components/executions/ExecutionStatusBadge'
import { Alert, Empty, Loading, PageHeader } from '../components/kit'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { formatTime } from '../utils/format'

const what = (e: HistoryItem) =>
  `${e.version ? `v${e.version}` : `#${e.migrationId}`} ${e.kind}${e.revertsExecutionId ? ` (membatalkan E${e.revertsExecutionId})` : ''}`

// Every attempt, oldest first. Hide rollback filters out each successful rollback together with
// the execution it reverted, matched by execution ID on the server; nothing is deleted.
export default function Executions() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const hideRollback = params.get('hideRollback') === 'true'
  const { data, error, loading } = useAsync(() => listExecutions(databaseId, hideRollback), [databaseId, hideRollback])
  const database = useAsync(() => getDatabase(databaseId), [databaseId])
  const name = database.data?.name ?? `Database #${databaseId}`
  useCrumbs([
    { label: 'Databases', to: '/databases' },
    { label: name, to: `/databases/${databaseId}` },
    { label: 'Riwayat migration', to: `/databases/${databaseId}/migrations` },
    { label: 'Riwayat eksekusi' },
  ])
  const rows = data?.executions ?? []

  return (
    <>
      <PageHeader
        title="Riwayat eksekusi"
        subtitle={`Semua attempt run, forward, dan rollback pada ${name}, termasuk yang gagal. Versi aktif: ${database.data?.currentVersion ? `v${database.data.currentVersion}` : '-'}.`}
      />

      <label className="mb-4 flex items-center gap-2 text-sm">
        <input type="checkbox" checked={hideRollback} onChange={(e) => setParams(e.target.checked ? { hideRollback: 'true' } : {})} />
        Hide rollback: sembunyikan rollback sukses beserta eksekusi yang dibatalkannya
      </label>

      {error && <Alert>{error.message}</Alert>}
      {loading && !data && <Loading rows={5} />}
      {data && rows.length === 0 && <Empty>Belum ada eksekusi untuk database ini.</Empty>}
      {rows.length > 0 && (
        <table className={`${ui.table} ${ui.stack.table}`}>
          <thead className={ui.stack.thead}>
            <tr>
              <th className={`${ui.th} w-20`}>Eksekusi</th>
              <th className={ui.th}>Migration</th>
              <th className={`${ui.th} w-32`}>Status</th>
              <th className={`${ui.th} w-32`}>Oleh</th>
              <th className={`${ui.th} ${ui.num} w-44`}>Mulai</th>
            </tr>
          </thead>
          <tbody className={ui.stack.tbody}>
            {rows.map((e) => (
              <tr key={e.id} className={ui.stack.tr}>
                <td className={`${ui.td} ${ui.stack.td} font-mono text-[13px] font-medium`}>
                  <Link className="hover:underline" to={`/executions/${e.id}`}>E{e.id}</Link>
                </td>
                <td className={`${ui.td} ${ui.stack.td} max-w-0 max-sm:max-w-none`}>
                  <Link className="font-mono text-[13px] hover:underline" to={`/migrations/${e.migrationId}`}>{what(e)}</Link>
                  <p className="truncate font-mono text-xs text-muted">{e.sqlExecuted}</p>
                  {e.errorMessage && (
                    <p className="truncate text-xs text-negative">
                      {e.errorCode && <span className="font-mono">{e.errorCode} </span>}
                      {e.errorMessage.split('\n')[0]}
                    </p>
                  )}
                </td>
                <td className={`${ui.td} ${ui.stack.td}`}><ExecutionStatusBadge status={e.status} kind={e.kind} /></td>
                <td className={`${ui.td} ${ui.stack.td} whitespace-nowrap`}>{e.requestedBy}</td>
                <td className={`${ui.td} ${ui.stack.td} ${ui.num} text-muted whitespace-nowrap max-sm:text-left`}>{formatTime(e.startedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
