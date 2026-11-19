import { PAGE_SIZES, getRows } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { ChevronLeftIcon, ChevronRightIcon } from '../icons'
import { Alert, Empty, Loading } from '../kit'
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
  if (v === null || v === undefined) return <span className="text-muted italic">NULL</span>
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

export default function TableData({ databaseId, schema, table, page, pageSize, onPage }: Props) {
  const { data, error, loading } = useAsync(
    () => getRows(databaseId, schema, table, page, pageSize),
    [databaseId, schema, table, page, pageSize],
  )

  if (error) return <Alert>{error.message}</Alert>
  if (!data) return <Loading rows={8} label="Memuat data" />

  const lastPage = Math.max(1, Math.ceil(data.totalRows / data.pageSize))
  const from = data.totalRows === 0 ? 0 : (data.page - 1) * data.pageSize + 1
  const to = Math.min(data.page * data.pageSize, data.totalRows)
  // A column whose every value on this page is a number is set right-aligned in tabular figures.
  const numeric = data.columns.map((_, j) => data.rows.length > 0 && data.rows.every((r) => r[j] === null || typeof r[j] === 'number'))

  return (
    <div className={loading ? 'opacity-60' : ''}>
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <p className="text-sm">
          <span className="num font-medium">{from}–{to}</span> <span className="text-muted">dari</span>{' '}
          <span className="num font-medium">{data.totalRows.toLocaleString('id-ID')}</span> <span className="text-muted">baris</span>
        </p>
        <label className="ml-auto flex items-center gap-2 text-sm text-muted">
          Baris per halaman
          <select className={ui.select} value={data.pageSize} onChange={(e) => onPage(1, Number(e.target.value))}>
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </label>
        <div className="flex items-center gap-1.5">
          <button
            className={ui.button}
            disabled={loading || data.page <= 1}
            onClick={() => onPage(data.page - 1, data.pageSize)}
            aria-label="Halaman sebelumnya"
          >
            <ChevronLeftIcon size={14} /> Sebelumnya
          </button>
          <span className="num px-1 text-sm text-muted">{data.page} / {lastPage}</span>
          <button
            className={ui.button}
            disabled={loading || data.page >= lastPage}
            onClick={() => onPage(data.page + 1, data.pageSize)}
            aria-label="Halaman berikutnya"
          >
            Berikutnya <ChevronRightIcon size={14} />
          </button>
        </div>
      </div>

      {data.rows.length === 0 ? (
        <Empty>Table ini belum punya baris.</Empty>
      ) : (
        <div className="max-h-[calc(100dvh-300px)] min-h-64 overflow-auto rounded-lg border border-line">
          <table className={ui.table}>
            <thead className="sticky top-0 z-10 bg-sheet">
              <tr>
                {data.columns.map((c, j) => (
                  <th key={c} className={`h-10 border-b border-line px-3 font-mono text-xs font-normal whitespace-nowrap text-muted ${numeric[j] ? 'text-right' : ''}`}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody className="[&>tr:first-child>td]:border-t-0">
              {data.rows.map((row, i) => (
                <tr key={i} className="hover:bg-raised">
                  {row.map((v, j) => (
                    <td
                      key={j}
                      className={`h-10 max-w-xs truncate border-t border-rule px-3 font-mono text-[13px] ${numeric[j] ? ui.num : ''}`}
                      title={typeof v === 'string' ? v : undefined}
                    >
                      {formatCell(v)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
