import { PAGE_SIZES, getRows } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { ui } from '../ui'

type Props = {
  databaseId: string
  schema: string
  table: string
  page: number
  pageSize: number
  onPage: (page: number, pageSize: number) => void
}

function formatCell(v: unknown) {
  if (v === null || v === undefined) return <span className="text-neutral-500 italic">NULL</span>
  if (typeof v === 'object') return <code className={ui.code}>{JSON.stringify(v)}</code>
  return String(v)
}

export default function TableData({ databaseId, schema, table, page, pageSize, onPage }: Props) {
  const { data, error, loading } = useAsync(
    () => getRows(databaseId, schema, table, page, pageSize),
    [databaseId, schema, table, page, pageSize],
  )

  if (error) return <p className={ui.error}>{error.message}</p>
  if (!data) return <p className={ui.muted}>Memuat data…</p>

  const lastPage = Math.max(1, Math.ceil(data.totalRows / data.pageSize))

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center gap-3 text-sm">
        <button className={ui.button} disabled={loading || data.page <= 1} onClick={() => onPage(data.page - 1, data.pageSize)}>
          ‹ Sebelumnya
        </button>
        <span>
          Halaman {data.page} / {lastPage} · {data.totalRows} baris
        </span>
        <button className={ui.button} disabled={loading || data.page >= lastPage} onClick={() => onPage(data.page + 1, data.pageSize)}>
          Berikutnya ›
        </button>
        <label className="ml-auto flex items-center gap-2">
          Baris per halaman
          <select className={ui.select} value={data.pageSize} onChange={(e) => onPage(1, Number(e.target.value))}>
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="overflow-x-auto">
        <table className={ui.table}>
          <thead>
            <tr>
              {data.columns.map((c) => (
                <th key={c} className={ui.th}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.rows.map((row, i) => (
              <tr key={i}>
                {row.map((v, j) => (
                  <td key={j} className={ui.td}>{formatCell(v)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {data.rows.length === 0 && <p className={ui.muted}>Table kosong.</p>}
      </div>
    </>
  )
}
