import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { read, write } from '../../api/admin'
import type { ConnectionInput, ConnectionTest, Database } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, PlugIcon, ServerIcon, TrashIcon } from '../../components/icons'
import { Alert, Badge, Card, EmptyState, Field, Loading, PageHeader } from '../../components/kit'
import { ui } from '../../components/ui'

const initial: ConnectionInput = { name: '', host: 'localhost', port: 15433, dbName: '', username: 'platform_app', password: '', sslMode: 'disable' }
const sslModes = ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full']
const fields = [
  { key: 'name', label: 'Nama' },
  { key: 'host', label: 'Host' },
  { key: 'port', label: 'Port' },
  { key: 'dbName', label: 'Nama database' },
  { key: 'username', label: 'Username' },
] as const

export default function Databases() {
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
      <PageHeader title="Koneksi database" subtitle="Database PostgreSQL target yang dikelola platform." />
      {s.error && <Alert>{s.error}</Alert>}
      {result && (
        <Alert tone={result.warning ? 'warning' : 'success'}>
          <strong className="font-medium">Koneksi berhasil.</strong> {result.warning}
        </Alert>
      )}
      <div className="grid gap-6 lg:grid-cols-[22rem_minmax(0,1fr)] lg:items-start">
        <Card title={id === null ? 'Tambah koneksi' : 'Edit koneksi'} className="lg:sticky lg:top-8">
          <form onSubmit={save} className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              {fields.map((f) => (
                <div key={f.key} className={f.key === 'name' || f.key === 'host' ? 'col-span-2' : ''}>
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
              <Field label={id !== null ? 'Password (kosong = tetap)' : 'Password'}>
                <input className={ui.input} type="password" autoComplete="new-password" value={form.password} onChange={(e) => field('password', e.target.value)} />
              </Field>
              <Field label="SSL mode">
                <select className={`${ui.select} w-full`} value={form.sslMode} onChange={(e) => field('sslMode', e.target.value)}>
                  {sslModes.map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            </div>
            <p className="text-xs text-zinc-500 dark:text-zinc-400">
              Password tersimpan tidak ditampilkan. Test form memakai password yang diisi; untuk kredensial tersimpan gunakan tombol Test pada daftar.
            </p>
            <div className="flex flex-wrap gap-2">
              <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
              <button type="button" className={ui.button} disabled={s.busy} onClick={() => void test()}>
                <PlugIcon /> Test form
              </button>
              <button type="button" className={ui.buttonGhost} onClick={reset}>Batal</button>
            </div>
          </form>
        </Card>

        <Card title="Semua koneksi" description={s.data && `${s.data.length} koneksi`} flush>
          {!s.data ? (
            <div className="px-4"><Loading /></div>
          ) : s.data.length === 0 ? (
            <EmptyState icon={<ServerIcon size={20} />} title="Belum ada koneksi">Tambahkan database target lewat form di samping.</EmptyState>
          ) : (
            <div className={ui.tableWrap}>
              <table className={ui.table}>
                <thead>
                  <tr>
                    <th className={ui.th}>Nama</th>
                    <th className={ui.th}>Target</th>
                    <th className={ui.th}>User</th>
                    <th className={ui.th}>Password</th>
                    <th className={`${ui.th} text-right`}>Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {s.data.map((d) => (
                    <tr key={d.id} className={`${ui.tr} ${d.id === id ? 'bg-indigo-50/60 dark:bg-indigo-500/5' : ''}`}>
                      <td className={`${ui.td} font-medium`}>{d.name}</td>
                      <td className={`${ui.td} font-mono text-xs whitespace-nowrap`}>{d.host}:{d.port}/{d.dbName}</td>
                      <td className={`${ui.td} font-mono text-xs`}>{d.username}</td>
                      <td className={ui.td}>{d.hasPassword ? <Badge tone="green">Tersimpan</Badge> : <Badge>Tidak ada</Badge>}</td>
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
            </div>
          )}
        </Card>
      </div>
    </>
  )
}
