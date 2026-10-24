import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { read, write } from '../../api/admin'
import type { ConnectionInput, ConnectionTest, Database } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, PlugIcon, TrashIcon } from '../../components/icons'
import { Alert, Empty, Field, Loading, PageHeader, Pill } from '../../components/kit'
import { ui } from '../../components/ui'
import { useCrumbs } from '../../context/crumbs'

const initial: ConnectionInput = { name: '', host: 'localhost', port: 15433, dbName: '', username: 'platform_app', password: '', sslMode: 'disable' }
const sslModes = ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full']
const fields = [
  { key: 'name', label: 'Nama', span: true },
  { key: 'host', label: 'Host', span: true },
  { key: 'port', label: 'Port' },
  { key: 'dbName', label: 'Nama database' },
  { key: 'username', label: 'Username' },
] as const

export default function Databases() {
  useCrumbs([{ label: 'Administrasi' }, { label: 'Koneksi database' }])
  const load = useCallback(() => read<Database[]>('/admin/databases'), [])
  const s = useAdmin(load)
  const [id, setId] = useState<number | null>(null)
  const [form, setForm] = useState<ConnectionInput>({ ...initial })
  const [result, setResult] = useState<ConnectionTest | null>(null)

  function reset() {
    setId(null)
    setForm({ ...initial })
    setResult(null)
  }
  function field(k: keyof ConnectionInput, v: string) {
    setForm((f) => ({ ...f, [k]: k === 'port' ? Number(v) : v }))
    setResult(null)
  }
  async function save(e: FormEvent) {
    e.preventDefault()
    if (await s.act(() => write(id === null ? 'POST' : 'PATCH', id === null ? '/admin/databases' : `/admin/databases/${id}`, form))) reset()
  }
  async function test(saved?: number) {
    setResult(null)
    await s.act(async () => {
      const v = await write<ConnectionTest>('POST', saved ? `/admin/databases/${saved}/test` : '/admin/databases/test', saved ? undefined : form)
      setResult(v)
    })
  }

  return (
    <>
      <PageHeader
        title="Koneksi database"
        subtitle={s.data && `${s.data.length} database PostgreSQL target. Password disimpan terenkripsi dan tidak pernah ditampilkan.`}
      />
      {s.error && <Alert>{s.error}</Alert>}
      {result && (
        <Alert tone={result.warning ? 'warning' : 'success'}>
          <strong className="font-medium">Koneksi berhasil.</strong> {result.warning}
        </Alert>
      )}
      <div className="grid gap-6 lg:grid-cols-[352px_minmax(0,1fr)] lg:items-start">
        <form onSubmit={save} className={`${ui.well} space-y-4`}>
          <h2 className="text-base font-semibold">{id === null ? 'Tambah koneksi' : `Edit ${form.name || 'koneksi'}`}</h2>
          <div className="grid grid-cols-2 gap-4">
            {fields.map((f) => (
              <div key={f.key} className={'span' in f ? 'col-span-2' : ''}>
                <Field label={f.label}>
                  <input
                    className={ui.input}
                    type={f.key === 'port' ? 'number' : 'text'}
                    required
                    autoComplete="off"
                    min={f.key === 'port' ? 1 : undefined}
                    max={f.key === 'port' ? 65535 : undefined}
                    value={form[f.key]}
                    onChange={(e) => field(f.key, e.target.value)}
                  />
                </Field>
              </div>
            ))}
            <Field label="Password" hint={id !== null ? 'Kosong = tetap.' : undefined}>
              <input className={ui.input} type="password" autoComplete="new-password" value={form.password} onChange={(e) => field('password', e.target.value)} />
            </Field>
            <div className="col-span-2">
              <Field label="SSL mode">
                <select className={ui.selectForm} value={form.sslMode} onChange={(e) => field('sslMode', e.target.value)}>
                  {sslModes.map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            </div>
          </div>
          <p className="text-xs text-muted">Test form memakai password yang diisi; untuk kredensial tersimpan gunakan tombol Test pada daftar.</p>
          <div className="flex flex-wrap gap-2">
            <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
            <button type="button" className={ui.button} disabled={s.busy} onClick={() => void test()}>
              <PlugIcon /> Test form
            </button>
            <button type="button" className={ui.buttonGhost} onClick={reset}>Batal</button>
          </div>
        </form>

        <div className="min-w-0 overflow-x-auto">
          {!s.data ? (
            <Loading rows={3} />
          ) : s.data.length === 0 ? (
            <Empty>Belum ada koneksi. Tambahkan database target lewat form di samping.</Empty>
          ) : (
            <table className={`${ui.table} min-w-[600px]`}>
              <thead>
                <tr>
                  <th className={ui.th}>Koneksi</th>
                  <th className={ui.th}>User</th>
                  <th className={ui.th}>Password</th>
                  <th className={ui.th}><span className="sr-only">Aksi</span></th>
                </tr>
              </thead>
              <tbody>
                {s.data.map((d) => (
                  <tr key={d.id} className={d.id === id ? 'bg-raised' : ''}>
                    <td className={`${ui.td} h-16`}>
                      <p className="font-medium">{d.name}</p>
                      <p className={`${ui.ident} text-muted`}>{d.host}:{d.port}/{d.dbName} <span className="font-sans text-xs">· ssl {d.sslMode}</span></p>
                    </td>
                    <td className={`${ui.td} ${ui.ident}`}>{d.username}</td>
                    <td className={ui.td}>{d.hasPassword ? <Pill tone="positive">Tersimpan</Pill> : <span className="text-muted">Tidak ada</span>}</td>
                    <td className={ui.td}>
                      <div className="flex justify-end gap-1.5">
                        <button
                          className={ui.buttonSm}
                          disabled={s.busy}
                          onClick={() => {
                            setId(d.id)
                            setResult(null)
                            setForm({ name: d.name, host: d.host, port: d.port, dbName: d.dbName, username: d.username, password: '', sslMode: d.sslMode })
                          }}
                        >
                          <PencilIcon size={13} /> Edit
                        </button>
                        <button className={ui.buttonSm} disabled={s.busy} onClick={() => void test(d.id)}>
                          <PlugIcon size={13} /> Test
                        </button>
                        <button
                          className={ui.buttonSmDanger}
                          disabled={s.busy}
                          onClick={() => {
                            if (confirm(`Hapus ${d.name}? Grant dan seluruh riwayat migration ikut dihapus.`)) void s.act(() => write('DELETE', `/admin/databases/${d.id}`))
                          }}
                        >
                          <TrashIcon size={13} /> Hapus
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </>
  )
}
