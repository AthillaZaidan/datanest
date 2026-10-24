import { useCallback } from 'react'
import { Link } from 'react-router'
import { read } from '../api/admin'
import type { Database } from '../api/admin'
import { useAdmin } from '../components/admin/useAdmin'
export default function Databases(){const load=useCallback(()=>read<Database[]>('/databases'),[]),s=useAdmin(load);return <section><h1>Database saya</h1>{s.error&&<p role="alert">{s.error}</p>}<button onClick={()=>void s.refresh()}>Muat ulang</button>{!s.data?<p>Memuat...</p>:s.data.length===0?<p>Belum ada akses database. Hubungi Platform Admin untuk keanggotaan group dan grant.</p>:<ul>{s.data.map(d=><li key={d.id}><Link to={`/databases/${d.id}`}>{d.name}</Link> <strong>{d.myLevel}</strong> — {d.dbName}</li>)}</ul>}</section>}
