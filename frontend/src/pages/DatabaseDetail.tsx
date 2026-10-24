import { Link, useParams, useSearchParams } from 'react-router'
import { listSchemas, listTables } from '../api/explorer'
import TableData from '../components/explorer/TableData'
import TableStructure from '../components/explorer/TableStructure'
import { ui } from '../components/ui'
import { useAsync } from '../hooks/useAsync'

const tabClass = (active: boolean) =>
  `border-b-2 px-3 py-1 text-sm ${active ? 'border-blue-600 font-semibold' : 'border-transparent text-neutral-500 hover:text-inherit'}`

// Selection lives in the URL so refresh and shared links keep the same view.
export default function DatabaseDetail() {
  const { databaseId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const schema = params.get('schema') ?? 'public'
  const table = params.get('table')
  const tab = params.get('tab') === 'data' ? 'data' : 'structure'
  const page = Number(params.get('page')) || 1
  const pageSize = Number(params.get('pageSize')) || 25

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

  if (schemas.error) return <p className={ui.error}>{schemas.error.message}</p>

  return (
    <>
      <h1 className={ui.h1}>
        Database #{databaseId}
        <Link className={ui.link} to={`/databases/${databaseId}/migrations`}>Riwayat migration →</Link>
      </h1>

      <div className="flex gap-6">
        <aside className="w-56 shrink-0">
          <label className="flex items-center gap-2 text-sm">
            Schema
            <select
              className={ui.select}
              value={schema}
              onChange={(e) => update({ schema: e.target.value, table: null, page: null })}
            >
              {(schemas.data ?? [{ name: schema }]).map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
          </label>
          {tables.error && <p className={ui.error}>{tables.error.message}</p>}
          <ul className="mt-3 flex flex-col gap-0.5">
            {tables.data?.map((t) => (
              <li key={t.name}>
                <button
                  className={`w-full rounded px-2 py-1 text-left text-sm hover:bg-neutral-500/10 ${
                    t.name === table ? 'bg-neutral-500/20 font-semibold' : ''
                  }`}
                  onClick={() => update({ table: t.name, page: null })}
                >
                  {t.name}
                </button>
              </li>
            ))}
          </ul>
          {tables.data?.length === 0 && <p className={ui.muted}>Tidak ada table.</p>}
        </aside>

        <section className="min-w-0 flex-1">
          {!table ? (
            <p className={ui.muted}>Pilih table di sebelah kiri.</p>
          ) : (
            <>
              <h2 className={ui.h2}>
                {schema}.{table}
              </h2>
              <div className="mb-4 flex gap-1 border-b border-neutral-500/30">
                <button className={tabClass(tab === 'structure')} onClick={() => update({ tab: null })}>
                  Struktur
                </button>
                <button className={tabClass(tab === 'data')} onClick={() => update({ tab: 'data' })}>
                  Data
                </button>
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
