import { useCallback } from 'react'
import { Link } from 'react-router'
import { read } from '../api/admin'
import type { Database } from '../api/admin'
import { listMigrations } from '../api/migrations'
import { useAdmin } from '../components/admin/useAdmin'
import StatusBadge from '../components/migrations/StatusBadge'
import { RefreshIcon } from '../components/icons'
import { Alert, Empty, LevelPill, Loading, PageHeader } from '../components/kit'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { ago } from '../utils/format'

export default function Databases() {
  useCrumbs([{ label: 'Databases' }])
  const load = useCallback(() => read<Database[]>('/databases'), [])
  const s = useAdmin(load)
  const edit = s.data?.filter((d) => d.myLevel === 'EDIT').length ?? 0
  const ids = s.data?.map((d) => d.id).join(',') ?? ''
  const recent = useAsync(async () => {
    if (!s.data) return null
    const lists = await Promise.all(s.data.map(async (d) => ({ d, list: await listMigrations(String(d.id)) })))
    return lists
      .flatMap(({ d, list }) => list.migrations.filter((m) => m.version !== null).map((m) => ({ ...m, dbName: d.name })))
      .sort((a, b) => (b.finishedAt ?? b.updatedAt).localeCompare(a.finishedAt ?? a.updatedAt))
      .slice(0, 6)
  }, [ids])

  return (
    <>
      <PageHeader
        title="Databases"
        subtitle={
          s.data &&
          (s.data.length === 0
            ? 'Anda belum punya akses ke database mana pun.'
            : `${s.data.length} database dapat Anda akses lewat group, ${edit} di antaranya dengan akses edit.`)
        }
        actions={
          <button className={ui.button} onClick={() => void s.refresh()}>
            <RefreshIcon /> Muat ulang
          </button>
        }
      />
      {s.error && <Alert>{s.error}</Alert>}
      {!s.data ? (
        !s.error && <Loading rows={3} />
      ) : s.data.length === 0 ? (
        <Empty>Hubungi Platform Admin untuk keanggotaan group dan grant.</Empty>
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)] gap-x-12 gap-y-8 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
          <div className="self-start overflow-x-auto">
          <table className={ui.table}>
            <thead>
              <tr>
                <th className={`${ui.th} w-1/2`}>Database</th>
                <th className={ui.th}>Akses</th>
                <th className={`${ui.th} ${ui.num}`}>Versi schema</th>
                <th className={`${ui.th} w-16`}><span className="sr-only">Aksi</span></th>
              </tr>
            </thead>
            <tbody>
              {s.data.map((d) => (
                <tr key={d.id}>
                  <td className={`${ui.td} h-16`}>
                    <Link to={`/databases/${d.id}`} className="block font-medium hover:underline">{d.name}</Link>
                    <span className={`${ui.ident} text-muted`}>{d.host}:{d.port}/{d.dbName}</span>
                  </td>
                  <td className={ui.td}><LevelPill level={d.myLevel} /></td>
                  <td className={`${ui.td} ${ui.num} font-mono text-[13px]`}>
                    {d.currentVersion ? `v${d.currentVersion}` : <span className="text-muted">-</span>}
                  </td>
                  <td className={`${ui.td} text-right`}>
                    <Link to={`/databases/${d.id}`} className={ui.buttonSm}>Buka</Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
          <section aria-labelledby="recent" className="min-w-0">
            <h2 id="recent" className="flex h-10 items-center text-xs text-muted">Migration terakhir dijalankan</h2>
            {recent.error ? (
              <p className={ui.error}>{recent.error.message}</p>
            ) : !recent.data ? (
              <Loading rows={3} />
            ) : recent.data.length === 0 ? (
              <Empty>Belum ada migration yang dijalankan di database Anda.</Empty>
            ) : (
              <ol>
                {recent.data.map((m) => (
                  <li key={m.id} className="border-t border-rule">
                    <Link to={`/migrations/${m.id}`} className="flex h-16 items-center gap-3 hover:[&_.sql]:text-ink">
                      <div className="min-w-0 flex-1">
                        <p className="text-sm">
                          <span className="font-medium">{m.dbName}</span> <span className="font-mono text-[13px]">v{m.version}</span>
                        </p>
                        <p className="sql truncate font-mono text-[13px] text-muted">{m.sql}</p>
                      </div>
                      <div className="flex flex-none flex-col items-end gap-1">
                        <StatusBadge status={m.status} />
                        <span className="text-xs text-muted">{m.executedBy || m.createdBy}, {ago(m.finishedAt ?? m.updatedAt)}</span>
                      </div>
                    </Link>
                  </li>
                ))}
              </ol>
            )}
          </section>
        </div>
      )}
    </>
  )
}
