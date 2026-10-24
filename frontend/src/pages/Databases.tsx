import { useCallback } from 'react'
import { Link } from 'react-router'
import { read } from '../api/admin'
import type { Database } from '../api/admin'
import { useAdmin } from '../components/admin/useAdmin'
import { DatabaseIcon, RefreshIcon } from '../components/icons'
import { Alert, EmptyState, LevelBadge, Loading, PageHeader } from '../components/kit'
import { ui } from '../components/ui'

export default function Databases() {
  const load = useCallback(() => read<Database[]>('/databases'), [])
  const s = useAdmin(load)
  return (
    <>
      <PageHeader
        title="Database saya"
        subtitle="Database yang dapat Anda akses melalui keanggotaan group."
        actions={
          <button className={ui.button} onClick={() => void s.refresh()}>
            <RefreshIcon /> Muat ulang
          </button>
        }
      />
      {s.error && <Alert>{s.error}</Alert>}
      {!s.data ? (
        !s.error && <Loading />
      ) : s.data.length === 0 ? (
        <div className={ui.card}>
          <EmptyState icon={<DatabaseIcon size={20} />} title="Belum ada akses database">
            Hubungi Platform Admin untuk keanggotaan group dan grant.
          </EmptyState>
        </div>
      ) : (
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {s.data.map((d) => (
            <li key={d.id}>
              <Link
                to={`/databases/${d.id}`}
                className={`${ui.card} group flex h-full flex-col gap-4 p-4 transition hover:border-indigo-300 hover:shadow-md dark:hover:border-indigo-500/50`}
              >
                <div className="flex items-start justify-between gap-3">
                  <span className="grid size-10 place-items-center rounded-lg bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-300">
                    <DatabaseIcon size={20} />
                  </span>
                  <LevelBadge level={d.myLevel} />
                </div>
                <div className="min-w-0">
                  <p className="truncate font-semibold group-hover:text-indigo-600 dark:group-hover:text-indigo-300">{d.name}</p>
                  <p className="mt-0.5 truncate font-mono text-xs text-zinc-500 dark:text-zinc-400">{d.dbName}</p>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}
