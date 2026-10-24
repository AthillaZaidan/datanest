import { Link, useParams, useSearchParams } from 'react-router'
import { getDatabase } from '../api/databases'
import { listSchemas, listTables } from '../api/explorer'
import TableData from '../components/explorer/TableData'
import TableStructure from '../components/explorer/TableStructure'
import { HistoryIcon, TableIcon } from '../components/icons'
import { Alert, Badge, EmptyState, LevelBadge, PageHeader } from '../components/kit'
import { ui } from '../components/ui'
import { useAsync } from '../hooks/useAsync'

const tabClass = (active: boolean) =>
  `rounded-md px-3 py-1 text-sm font-medium transition-colors ${
    active
      ? 'bg-white text-zinc-900 shadow-xs dark:bg-zinc-700 dark:text-zinc-100'
      : 'text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'
  }`

// Selection lives in the URL so refresh and shared links keep the same view.
export default function DatabaseDetail() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const schema = params.get('schema') ?? 'public'
  const table = params.get('table')
  const tab = params.get('tab') === 'data' ? 'data' : 'structure'
  const page = Number(params.get('page')) || 1
  const pageSize = Number(params.get('pageSize')) || 25

  const database = useAsync(() => getDatabase(databaseId), [databaseId])
  const schemas = useAsync(() => listSchemas(databaseId), [databaseId])
  const tables = useAsync(() => listTables(databaseId, schema), [databaseId, schema])

  const update = (next: Record<string, string | null>) => {
    const p = new URLSearchParams(params)
    for (const [k, v] of Object.entries(next)) {
      if (v === null) p.delete(k)
      else p.set(k, v)
    }
    setParams(p)
  }

  const db = database.data
  const header = (
    <PageHeader
      back={{ to: '/databases', label: 'Database saya' }}
      title={db?.name ?? `Database #${databaseId}`}
      subtitle={db && <span className="font-mono text-xs">{db.dbName}</span>}
      badges={
        db && (
          <>
            <LevelBadge level={db.myLevel} />
            <Badge>{db.currentVersion ? `v${db.currentVersion}` : 'belum ada migration'}</Badge>
          </>
        )
      }
      actions={
        <Link className={ui.button} to={`/databases/${databaseId}/migrations`}>
          <HistoryIcon /> Riwayat migration
        </Link>
      }
    />
  )

  if (schemas.error) {
    return (
      <>
        {header}
        <Alert>{schemas.error.message}</Alert>
      </>
    )
  }

  return (
    <>
      {header}
      <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
        <aside className={`${ui.card} shrink-0 lg:sticky lg:top-8 lg:w-60`}>
          <div className="border-b border-zinc-200 p-3 dark:border-zinc-800">
            <label className={ui.label} htmlFor="schema">Schema</label>
            <select
              id="schema"
              className={`${ui.select} w-full`}
              value={schema}
              onChange={(e) => update({ schema: e.target.value, table: null, page: null })}
            >
              {(schemas.data ?? [{ name: schema }]).map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
          </div>
          <div className="max-h-[60vh] overflow-y-auto p-2">
            <p className="px-2 pt-1 pb-2 text-[11px] font-semibold tracking-wider text-zinc-400 uppercase dark:text-zinc-500">
              Tables {tables.data && `· ${tables.data.length}`}
            </p>
            {tables.error && <p className={`px-2 ${ui.error}`}>{tables.error.message}</p>}
            <ul className="flex flex-col gap-0.5">
              {tables.data?.map((t) => (
                <li key={t.name}>
                  <button
                    className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors ${
                      t.name === table
                        ? 'bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
                        : 'text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800'
                    }`}
                    onClick={() => update({ table: t.name, page: null })}
                  >
                    <TableIcon size={14} className="shrink-0 opacity-60" />
                    <span className="truncate">{t.name}</span>
                  </button>
                </li>
              ))}
            </ul>
            {tables.data?.length === 0 && <p className={`px-2 pb-2 ${ui.muted}`}>Tidak ada table.</p>}
          </div>
        </aside>

        <section className="min-w-0 flex-1">
          {!table ? (
            <div className={ui.card}>
              <EmptyState icon={<TableIcon size={20} />} title="Pilih table">
                Pilih table di panel kiri untuk melihat struktur dan datanya.
              </EmptyState>
            </div>
          ) : (
            <>
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <h2 className="font-mono text-lg font-semibold">
                  <span className="text-zinc-400 dark:text-zinc-500">{schema}.</span>
                  {table}
                </h2>
                <div className="inline-flex rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800">
                  <button className={tabClass(tab === 'structure')} onClick={() => update({ tab: null })}>
                    Struktur
                  </button>
                  <button className={tabClass(tab === 'data')} onClick={() => update({ tab: 'data' })}>
                    Data
                  </button>
                </div>
              </div>
              {tab === 'structure' ? (
                <TableStructure databaseId={databaseId} schema={schema} table={table} />
              ) : (
                <TableData
                  databaseId={databaseId}
                  schema={schema}
                  table={table}
                  page={page}
                  pageSize={pageSize}
                  onPage={(p, s) => update({ page: String(p), pageSize: String(s) })}
                />
              )}
            </>
          )}
        </section>
      </div>
    </>
  )
}
