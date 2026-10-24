import { Link, useParams, useSearchParams } from 'react-router'
import { getDatabase } from '../api/databases'
import { listSchemas, listTables } from '../api/explorer'
import { listMigrations } from '../api/migrations'
import TableData from '../components/explorer/TableData'
import TableStructure from '../components/explorer/TableStructure'
import { HistoryIcon, PlusIcon, TableIcon } from '../components/icons'
import { Alert, Empty, LevelPill, PageHeader, Tabs } from '../components/kit'
import VersionRail from '../components/migrations/VersionRail'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'

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
  const history = useAsync(() => listMigrations(databaseId), [databaseId])
  const schemas = useAsync(() => listSchemas(databaseId), [databaseId])
  const tables = useAsync(() => listTables(databaseId, schema), [databaseId, schema])

  const db = database.data
  const name = db?.name ?? `Database #${databaseId}`
  const base = `/databases/${databaseId}`
  useCrumbs([
    { label: 'Databases', to: '/databases' },
    { label: name, to: base },
    ...(table ? [{ label: `${schema}.${table}`, mono: true }] : []),
  ])

  const update = (next: Record<string, string | null>) => {
    const p = new URLSearchParams(params)
    for (const [k, v] of Object.entries(next)) {
      if (v === null) p.delete(k)
      else p.set(k, v)
    }
    setParams(p)
  }

  const header = (
    <>
      <PageHeader
        title={name}
        badges={db && <LevelPill level={db.myLevel} />}
        subtitle={db && <span className="font-mono text-[13px]">{db.host}:{db.port}/{db.dbName}</span>}
        actions={
          <>
            <Link className={ui.button} to={`${base}/migrations`}>
              <HistoryIcon /> Riwayat migration
            </Link>
            {db?.myLevel === 'EDIT' && (
              <Link className={ui.buttonPrimary} to={`${base}/migrations/new`}>
                <PlusIcon /> Migration baru
              </Link>
            )}
          </>
        }
      />
      {history.data && (
        <div className="-mt-2 mb-6 flex flex-wrap items-center gap-x-6 gap-y-2">
          <p className="text-sm">
            <span className="text-muted">Versi schema </span>
            <span className="font-mono font-medium">{history.data.currentVersion ? `v${history.data.currentVersion}` : '-'}</span>
          </p>
          <VersionRail migrations={history.data.migrations} currentVersion={history.data.currentVersion} />
        </div>
      )}
    </>
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
      <div className="flex flex-col gap-6 lg:grid lg:grid-cols-[224px_minmax(0,1fr)] lg:items-start">
        <aside className="lg:sticky lg:top-0">
          <label className="block">
            <span className={ui.label}>Schema</span>
            <select
              className={`${ui.select} w-full`}
              value={schema}
              onChange={(e) => update({ schema: e.target.value, table: null, page: null })}
            >
              {(schemas.data ?? [{ name: schema }]).map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
          </label>
          <p className="mt-5 mb-1.5 text-xs text-muted">
            Tables{tables.data && <span className="num">, {tables.data.length}</span>}
          </p>
          {tables.error && <p className={ui.error}>{tables.error.message}</p>}
          <ul className="flex gap-0.5 overflow-x-auto lg:max-h-[calc(100dvh-280px)] lg:flex-col lg:overflow-y-auto">
            {tables.data?.map((t) => (
              <li key={t.name} className="flex-none">
                <button
                  className={`flex h-8 w-full items-center gap-2 rounded-lg px-2.5 text-left font-mono text-[13px] transition-colors focus-visible:outline-2 focus-visible:outline-accent ${
                    t.name === table ? 'bg-active text-ink' : 'text-muted hover:bg-raised hover:text-ink'
                  }`}
                  onClick={() => update({ table: t.name, page: null })}
                  aria-current={t.name === table ? 'true' : undefined}
                >
                  <TableIcon size={14} className="flex-none" />
                  <span className="truncate">{t.name}</span>
                </button>
              </li>
            ))}
          </ul>
          {tables.data?.length === 0 && <p className={ui.muted}>Tidak ada table di schema ini.</p>}
        </aside>

        <section className="min-w-0">
          {!table ? (
            <Empty>Pilih table di sebelah kiri untuk melihat kolom, constraint, index, dan datanya.</Empty>
          ) : (
            <>
              <h2 className="mb-3 font-mono text-base font-medium">
                <span className="text-muted">{schema}.</span>
                {table}
              </h2>
              <Tabs
                label="Tampilan table"
                value={tab}
                onChange={(k) => update({ tab: k === 'data' ? 'data' : null })}
                items={[
                  { key: 'structure', label: 'Struktur' },
                  { key: 'data', label: 'Data' },
                ]}
              />
              <div className="pt-5">
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
              </div>
            </>
          )}
        </section>
      </div>
    </>
  )
}
