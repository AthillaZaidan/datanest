import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { read, write } from '../../api/admin'
import type { User } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, TrashIcon, UserIcon } from '../../components/icons'
import { Alert, Badge, Card, EmptyState, Field, Loading, PageHeader } from '../../components/kit'
import { ui } from '../../components/ui'

export default function Users() {
  const load = useCallback(() => read<User[]>('/admin/users'), [])
  const s = useAdmin(load)
  const [id, setId] = useState<number | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  function reset() {
    setId(null)
    setUsername('')
    setPassword('')
  }
  async function save(e: FormEvent) {
    e.preventDefault()
    const body = id === null ? { username, password } : { username, ...(password ? { password } : {}) }
    if (await s.act(() => write(id === null ? 'POST' : 'PATCH', id === null ? '/admin/users' : `/admin/users/${id}`, body))) reset()
  }

  return (
    <>
      <PageHeader title="Users" subtitle="Kelola akun dan role Platform Admin." />
      {s.error && <Alert>{s.error}</Alert>}
      <div className="grid gap-6 lg:grid-cols-[20rem_minmax(0,1fr)] lg:items-start">
        <Card title={id === null ? 'Buat user' : 'Edit user'} className="lg:sticky lg:top-8">
          <form onSubmit={save} className="space-y-4">
            <Field label="Username">
              <input className={ui.input} required maxLength={64} value={username} onChange={(e) => setUsername(e.target.value)} />
            </Field>
            <Field label={id === null ? 'Password' : 'Password baru'} hint={id === null ? 'Minimal 8 karakter.' : 'Kosongkan untuk mempertahankan password lama.'}>
              <input
                className={ui.input}
                type="password"
                autoComplete="new-password"
                required={id === null}
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <div className="flex gap-2">
              <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
              <button type="button" className={ui.button} onClick={reset}>Batal</button>
            </div>
          </form>
        </Card>

        <Card title="Semua user" description={s.data && `${s.data.length} user`} flush>
          {s.data === null ? (
            <div className="px-4"><Loading /></div>
          ) : s.data.length === 0 ? (
            <EmptyState icon={<UserIcon size={20} />} title="Belum ada user" />
          ) : (
            <div className={ui.tableWrap}>
              <table className={ui.table}>
                <thead>
                  <tr>
                    <th className={ui.th}>Username</th>
                    <th className={ui.th}>Role</th>
                    <th className={`${ui.th} text-right`}>Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {s.data.map((u) => (
                    <tr key={u.id} className={`${ui.tr} ${u.id === id ? 'bg-indigo-50/60 dark:bg-indigo-500/5' : ''}`}>
                      <td className={`${ui.td} font-medium`}>
                        <span className="flex items-center gap-2.5">
                          <span className="grid size-7 place-items-center rounded-full bg-zinc-100 text-xs font-semibold uppercase dark:bg-zinc-800">
                            {u.username.slice(0, 1)}
                          </span>
                          {u.username}
                        </span>
                      </td>
                      <td className={ui.td}>{u.isAdmin ? <Badge tone="indigo">Platform Admin</Badge> : <Badge>User</Badge>}</td>
                      <td className={ui.td}>
                        <div className="flex justify-end gap-1.5">
                          <button
                            className={ui.buttonSm}
                            disabled={s.busy}
                            onClick={() => {
                              setId(u.id)
                              setUsername(u.username)
                              setPassword('')
                            }}
                          >
                            <PencilIcon size={13} /> Edit
                          </button>
                          <button
                            className={ui.buttonSm}
                            disabled={s.busy}
                            onClick={() => {
                              if (confirm(`${u.isAdmin ? 'Cabut' : 'Beri'} admin untuk ${u.username}?`))
                                void s.act(() => write(u.isAdmin ? 'DELETE' : 'PUT', `/admin/users/${u.id}/roles/platform_admin`))
                            }}
                          >
                            {u.isAdmin ? 'Cabut admin' : 'Beri admin'}
                          </button>
                          <button
                            className={ui.buttonSmDanger}
                            disabled={s.busy}
                            onClick={() => {
                              if (confirm(`Hapus ${u.username}? Semua session user akan dicabut.`)) void s.act(() => write('DELETE', `/admin/users/${u.id}`))
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
