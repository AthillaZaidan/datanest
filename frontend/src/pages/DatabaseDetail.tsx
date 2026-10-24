import { Link, useParams, useSearchParams } from 'react-router'
import { listSchemas, listTables } from '../api/explorer'
import TableData from '../components/explorer/TableData'
import TableStructure from '../components/explorer/TableStructure'
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

  if (schemas.error) return <p className="error">{schemas.error.message}</p>

  return (
    <>
      <h1>
        Database #{databaseId} <Link to={`/databases/${databaseId}/migrations`}>Riwayat migration →</Link>
      </h1>

      <div className="explorer">
        <aside>
          <label>
            Schema{' '}
            <select value={schema} onChange={(e) => update({ schema: e.target.value, table: null, page: null })}>
              {(schemas.data ?? [{ name: schema }]).map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
          </label>
          {tables.error && <p className="error">{tables.error.message}</p>}
          <ul>
            {tables.data?.map((t) => (
              <li key={t.name}>
                <button
                  className={t.name === table ? 'active' : ''}
                  onClick={() => update({ table: t.name, page: null })}
                >
                  {t.name}
                </button>
              </li>
            ))}
          </ul>
          {tables.data?.length === 0 && <p>Tidak ada table.</p>}
        </aside>

        <section>
          {!table ? (
            <p>Pilih table di sebelah kiri.</p>
          ) : (
            <>
              <h2>
                {schema}.{table}
              </h2>
              <div className="tabs">
                <button className={tab === 'structure' ? 'active' : ''} onClick={() => update({ tab: null })}>
                  Struktur
                </button>
                <button className={tab === 'data' ? 'active' : ''} onClick={() => update({ tab: 'data' })}>
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
