import { getTable } from '../../api/explorer'
import { useAsync } from '../../hooks/useAsync'
import { KeyIcon } from '../icons'
import { Alert, Badge, Card, Loading } from '../kit'
import { ui } from '../ui'

type Props = { databaseId: string; schema: string; table: string }

const constraintTone = (type: string) =>
  type === 'PRIMARY KEY' ? 'indigo' : type === 'FOREIGN KEY' ? 'amber' : type === 'UNIQUE' ? 'green' : 'neutral'

export default function TableStructure({ databaseId, schema, table }: Props) {
  const { data, error, loading } = useAsync(() => getTable(databaseId, schema, table), [databaseId, schema, table])

  if (loading) return <Loading label="Memuat struktur…" />
  if (error) return <Alert>{error.message}</Alert>
  if (!data) return null

  const pkColumns = new Set(data.constraints.filter((c) => c.type === 'PRIMARY KEY').flatMap((c) => c.columns))

  return (
    <div className="space-y-6">
      <Card title="Columns" description={`${data.columns.length} kolom`} flush>
        <div className={ui.tableWrap}>
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
                <tr key={c.name} className={ui.tr}>
                  <td className={`${ui.td} font-medium`}>
                    <span className="inline-flex items-center gap-1.5">
                      {c.name}
                      {pkColumns.has(c.name) && <KeyIcon size={13} className="text-amber-500" aria-label="primary key" />}
                    </span>
                  </td>
                  <td className={ui.td}><code className={ui.chip}>{c.dataType}</code></td>
                  <td className={ui.td}>{c.nullable ? <Badge>ya</Badge> : <span className={ui.muted}>tidak</span>}</td>
                  <td className={ui.td}>{c.default ? <code className={ui.chip}>{c.default}</code> : <span className="text-zinc-400">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Card title="Constraints" description={`${data.constraints.length} constraint`} flush>
        {data.constraints.length === 0 ? (
          <p className={`p-4 ${ui.muted}`}>Tidak ada constraint.</p>
        ) : (
          <div className={ui.tableWrap}>
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
                  <tr key={c.name} className={ui.tr}>
                    <td className={`${ui.td} font-medium`}>{c.name}</td>
                    <td className={`${ui.td} whitespace-nowrap`}><Badge tone={constraintTone(c.type)}>{c.type}</Badge></td>
                    <td className={ui.td}>{c.columns.join(', ')}</td>
                    <td className={ui.td}><code className={ui.code}>{c.definition}</code></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card title="Indexes" description={`${data.indexes.length} index`} flush>
        {data.indexes.length === 0 ? (
          <p className={`p-4 ${ui.muted}`}>Tidak ada index.</p>
        ) : (
          <div className={ui.tableWrap}>
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
                  <tr key={i.name} className={ui.tr}>
                    <td className={`${ui.td} font-medium`}>
                      <span className="inline-flex items-center gap-1.5">
                        {i.name}
                        {i.primary && <Badge tone="indigo">PK</Badge>}
                      </span>
                    </td>
                    <td className={ui.td}><code className={ui.code}>{i.definition}</code></td>
                    <td className={ui.td}>{i.unique ? <Badge tone="green">ya</Badge> : <span className={ui.muted}>tidak</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}
