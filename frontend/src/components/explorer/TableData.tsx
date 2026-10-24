import { PAGE_SIZES, getRows } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'

type Props = {
  databaseId: string
  schema: string
  table: string
  page: number
  pageSize: number
  onPage: (page: number, pageSize: number) => void
}

function formatCell(v: unknown) {
  if (v === null || v === undefined) return <span className="null">NULL</span>
  if (typeof v === 'object') return <code>{JSON.stringify(v)}</code>
  return String(v)
}

export default function TableData({ databaseId, schema, table, page, pageSize, onPage }: Props) {
  const { data, error, loading } = useAsync(
    () => getRows(databaseId, schema, table, page, pageSize),
    [databaseId, schema, table, page, pageSize],
  )

  if (error) return <p className="error">{error.message}</p>
  if (!data) return <p>Memuat data…</p>

  const lastPage = Math.max(1, Math.ceil(data.totalRows / data.pageSize))

  return (
    <>
      <div className="pager">
        <button disabled={loading || data.page <= 1} onClick={() => onPage(data.page - 1, data.pageSize)}>
          ‹ Sebelumnya
        </button>
        <span>
          Halaman {data.page} / {lastPage} · {data.totalRows} baris
        </span>
        <button disabled={loading || data.page >= lastPage} onClick={() => onPage(data.page + 1, data.pageSize)}>
          Berikutnya ›
        </button>
        <label>
          Baris per halaman{' '}
          <select value={data.pageSize} onChange={(e) => onPage(1, Number(e.target.value))}>
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="scroll">
        <table className="grid">
          <thead>
            <tr>
              {data.columns.map((c) => (
                <th key={c}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.rows.map((row, i) => (
              <tr key={i}>
                {row.map((v, j) => (
                  <td key={j}>{formatCell(v)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {data.rows.length === 0 && <p>Table kosong.</p>}
      </div>
    </>
  )
}
