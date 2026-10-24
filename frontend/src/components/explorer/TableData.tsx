import { PAGE_SIZES, getRows } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { ChevronLeftIcon, ChevronRightIcon } from '../icons'
import { Alert, EmptyState, Loading } from '../kit'
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
  if (v === null || v === undefined) return <span className="text-zinc-400 italic dark:text-zinc-500">NULL</span>
  if (typeof v === 'object') return <code className={ui.code}>{JSON.stringify(v)}</code>
  return String(v)
}

export default function TableData({ databaseId, schema, table, page, pageSize, onPage }: Props) {
  const { data, error, loading } = useAsync(
    () => getRows(databaseId, schema, table, page, pageSize),
    [databaseId, schema, table, page, pageSize],
  )

  if (error) return <Alert>{error.message}</Alert>
  if (!data) return <Loading label="Memuat data…" />

  const lastPage = Math.max(1, Math.ceil(data.totalRows / data.pageSize))
  const from = data.totalRows === 0 ? 0 : (data.page - 1) * data.pageSize + 1
  const to = Math.min(data.page * data.pageSize, data.totalRows)

  return (
    <section className={`${ui.card} overflow-hidden ${loading ? 'opacity-60' : ''}`}>
      <div className="flex flex-wrap items-center gap-3 border-b border-zinc-200 px-4 py-2.5 text-sm dark:border-zinc-800">
        <span className={ui.muted}>
          <strong className="font-medium text-zinc-900 dark:text-zinc-100">{from}–{to}</strong> dari {data.totalRows} baris
        </span>
        <label className="ml-auto flex items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
          Baris per halaman
          <select className={ui.selectSm} value={data.pageSize} onChange={(e) => onPage(1, Number(e.target.value))}>
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </label>
        <div className="flex items-center gap-1">
          <button
            className={ui.buttonSm}
            disabled={loading || data.page <= 1}
            onClick={() => onPage(data.page - 1, data.pageSize)}
            aria-label="Halaman sebelumnya"
          >
            <ChevronLeftIcon size={14} /> Sebelumnya
          </button>
          <span className="px-2 text-xs text-zinc-500 tabular-nums dark:text-zinc-400">
            {data.page} / {lastPage}
          </span>
          <button
            className={ui.buttonSm}
            disabled={loading || data.page >= lastPage}
            onClick={() => onPage(data.page + 1, data.pageSize)}
            aria-label="Halaman berikutnya"
          >
            Berikutnya <ChevronRightIcon size={14} />
          </button>
        </div>
      </div>

      {data.rows.length === 0 ? (
        <EmptyState title="Table kosong">Belum ada baris di table ini.</EmptyState>
      ) : (
        <div className="max-h-[65vh] overflow-auto">
          <table className={ui.table}>
            <thead className="sticky top-0 z-10">
              <tr>
                {data.columns.map((c) => (
                  <th key={c} className={`${ui.th} whitespace-nowrap normal-case`}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row, i) => (
                <tr key={i} className={ui.tr}>
                  {row.map((v, j) => (
                    <td key={j} className={`${ui.td} max-w-xs truncate font-mono text-xs`} title={typeof v === 'string' ? v : undefined}>
                      {formatCell(v)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
