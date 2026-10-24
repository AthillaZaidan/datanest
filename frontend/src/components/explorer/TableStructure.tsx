import { getTable } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { KeyIcon } from '../icons'
import { Alert, Loading, Pill, Section } from '../kit'
import { ui } from '../ui'

type Props = { databaseId: string; schema: string; table: string }

export default function TableStructure({ databaseId, schema, table }: Props) {
  const { data, error, loading } = useAsync(() => getTable(databaseId, schema, table), [databaseId, schema, table])

  if (loading) return <Loading rows={6} label="Memuat struktur" />
  if (error) return <Alert>{error.message}</Alert>
  if (!data) return null

  const pkColumns = new Set(data.constraints.filter((c) => c.type === 'PRIMARY KEY').flatMap((c) => c.columns))
  const fkColumns = new Set(data.constraints.filter((c) => c.type === 'FOREIGN KEY').flatMap((c) => c.columns))

  return (
    <div className="space-y-10">
      <Section title="Columns" aside={<span className="num">{data.columns.length}</span>}>
        <div className="overflow-x-auto"><table className={`${ui.table} min-w-[560px]`}>
          <thead>
            <tr>
              <th className={`${ui.th} w-[30%]`}>Nama</th>
              <th className={ui.th}>Tipe</th>
              <th className={ui.th}>Nullable</th>
              <th className={ui.th}>Default</th>
            </tr>
          </thead>
          <tbody>
            {data.columns.map((c) => (
              <tr key={c.name}>
                <td className={ui.td}>
                  <span className="inline-flex items-center gap-2">
                    <span className={`${ui.ident} font-medium`}>{c.name}</span>
                    {pkColumns.has(c.name) && <span className="inline-flex items-center gap-1 text-xs text-muted"><KeyIcon size={13} /> PK</span>}
                    {fkColumns.has(c.name) && <span className="text-xs text-muted">FK</span>}
                  </span>
                </td>
                <td className={`${ui.td} ${ui.ident} text-muted`}>{c.dataType}</td>
                <td className={`${ui.td} text-muted`}>{c.nullable ? 'ya' : 'tidak'}</td>
                <td className={`${ui.td} ${ui.ident} text-muted`}>{c.default ?? '-'}</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      </Section>

      <Section title="Constraints" aside={<span className="num">{data.constraints.length}</span>}>
        {data.constraints.length === 0 ? (
          <p className={ui.muted}>Table ini tidak punya constraint.</p>
        ) : (
          <div className="overflow-x-auto"><table className={`${ui.table} min-w-[560px]`}>
            <thead>
              <tr>
                <th className={`${ui.th} w-[30%]`}>Nama</th>
                <th className={ui.th}>Tipe</th>
                <th className={ui.th}>Definisi</th>
              </tr>
            </thead>
            <tbody>
              {data.constraints.map((c) => (
                <tr key={c.name}>
                  <td className={`${ui.td} ${ui.ident}`}>{c.name}</td>
                  <td className={ui.td}><Pill>{c.type}</Pill></td>
                  <td className={`${ui.td} ${ui.ident} text-muted`}>{c.definition}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </Section>

      <Section title="Indexes" aside={<span className="num">{data.indexes.length}</span>}>
        {data.indexes.length === 0 ? (
          <p className={ui.muted}>Table ini tidak punya index.</p>
        ) : (
          <div className="overflow-x-auto"><table className={`${ui.table} min-w-[560px]`}>
            <thead>
              <tr>
                <th className={`${ui.th} w-[30%]`}>Nama</th>
                <th className={ui.th}>Definisi</th>
                <th className={ui.th}>Unique</th>
              </tr>
            </thead>
            <tbody>
              {data.indexes.map((i) => (
                <tr key={i.name}>
                  <td className={ui.td}>
                    <span className="inline-flex items-center gap-2">
                      <span className={ui.ident}>{i.name}</span>
                      {i.primary && <Pill>PK</Pill>}
                    </span>
                  </td>
                  <td className={`${ui.td} ${ui.ident} text-muted`}>{i.definition}</td>
                  <td className={`${ui.td} text-muted`}>{i.unique ? 'ya' : 'tidak'}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </Section>
    </div>
  )
}
