import { getTable } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'

type Props = { databaseId: string; schema: string; table: string }

export default function TableStructure({ databaseId, schema, table }: Props) {
  const { data, error, loading } = useAsync(() => getTable(databaseId, schema, table), [databaseId, schema, table])

  if (loading) return <p>Memuat struktur…</p>
  if (error) return <p className="error">{error.message}</p>
  if (!data) return null

  return (
    <>
      <h3>Columns</h3>
      <table className="grid">
        <thead>
          <tr>
            <th>Nama</th>
            <th>Tipe</th>
            <th>Nullable</th>
            <th>Default</th>
          </tr>
        </thead>
        <tbody>
          {data.columns.map((c) => (
            <tr key={c.name}>
              <td>{c.name}</td>
              <td><code>{c.dataType}</code></td>
              <td>{c.nullable ? 'ya' : 'tidak'}</td>
              <td>{c.default ? <code>{c.default}</code> : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3>Constraints</h3>
      {data.constraints.length === 0 ? (
        <p>Tidak ada constraint.</p>
      ) : (
        <table className="grid">
          <thead>
            <tr>
              <th>Nama</th>
              <th>Tipe</th>
              <th>Kolom</th>
              <th>Definisi</th>
            </tr>
          </thead>
          <tbody>
            {data.constraints.map((c) => (
              <tr key={c.name}>
                <td>{c.name}</td>
                <td>{c.type}</td>
                <td>{c.columns.join(', ')}</td>
                <td><code>{c.definition}</code></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h3>Indexes</h3>
      {data.indexes.length === 0 ? (
        <p>Tidak ada index.</p>
      ) : (
        <table className="grid">
          <thead>
            <tr>
              <th>Nama</th>
              <th>Definisi</th>
              <th>Unique</th>
            </tr>
          </thead>
          <tbody>
            {data.indexes.map((i) => (
              <tr key={i.name}>
                <td>{i.name}{i.primary && ' (PK)'}</td>
                <td><code>{i.definition}</code></td>
                <td>{i.unique ? 'ya' : 'tidak'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}
