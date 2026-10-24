import { getTable } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { ui } from '../ui'

type Props = { databaseId: string; schema: string; table: string }

export default function TableStructure({ databaseId, schema, table }: Props) {
  const { data, error, loading } = useAsync(() => getTable(databaseId, schema, table), [databaseId, schema, table])

  if (loading) return <p className={ui.muted}>Memuat struktur…</p>
  if (error) return <p className={ui.error}>{error.message}</p>
  if (!data) return null

  return (
    <>
      <h3 className={ui.h3}>Columns</h3>
      <table className={ui.table}>
        <thead>
          <tr>
            <th className={ui.th}>Nama</th>
            <th className={ui.th}>Tipe</th>
            <th className={ui.th}>Nullable</th>
            <th className={ui.th}>Default</th>
          </tr>
        </thead>
        <tbody>
          {data.columns.map((c) => (
            <tr key={c.name}>
              <td className={ui.td}>{c.name}</td>
              <td className={ui.td}><code className={ui.code}>{c.dataType}</code></td>
              <td className={ui.td}>{c.nullable ? 'ya' : 'tidak'}</td>
              <td className={ui.td}>{c.default ? <code className={ui.code}>{c.default}</code> : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3 className={ui.h3}>Constraints</h3>
      {data.constraints.length === 0 ? (
        <p className={ui.muted}>Tidak ada constraint.</p>
      ) : (
        <table className={ui.table}>
          <thead>
            <tr>
              <th className={ui.th}>Nama</th>
              <th className={ui.th}>Tipe</th>
              <th className={ui.th}>Kolom</th>
              <th className={ui.th}>Definisi</th>
            </tr>
          </thead>
          <tbody>
            {data.constraints.map((c) => (
              <tr key={c.name}>
                <td className={ui.td}>{c.name}</td>
                <td className={ui.td}>{c.type}</td>
                <td className={ui.td}>{c.columns.join(', ')}</td>
                <td className={ui.td}><code className={ui.code}>{c.definition}</code></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h3 className={ui.h3}>Indexes</h3>
      {data.indexes.length === 0 ? (
        <p className={ui.muted}>Tidak ada index.</p>
      ) : (
        <table className={ui.table}>
          <thead>
            <tr>
              <th className={ui.th}>Nama</th>
              <th className={ui.th}>Definisi</th>
              <th className={ui.th}>Unique</th>
            </tr>
          </thead>
          <tbody>
            {data.indexes.map((i) => (
              <tr key={i.name}>
                <td className={ui.td}>{i.name}{i.primary && ' (PK)'}</td>
                <td className={ui.td}><code className={ui.code}>{i.definition}</code></td>
                <td className={ui.td}>{i.unique ? 'ya' : 'tidak'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
