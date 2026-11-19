import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { read, write } from '../../api/admin'
import type { User } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, TrashIcon } from '../../components/icons'
import { Alert, Avatar, Empty, Field, Loading, PageHeader, Pill } from '../../components/kit'
import { ui } from '../../components/ui'
import { useCrumbs } from '../../context/crumbs'

export default function Users() {
  useCrumbs([{ label: 'Administrasi' }, { label: 'Users' }])
  const load = useCallback(() => read<User[]>('/admin/users'), [])
  const s = useAdmin(load)
  const [id, setId] = useState<number | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const admins = s.data?.filter((u) => u.isAdmin).length ?? 0

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
      <PageHeader
        title="Users"
        subtitle={s.data && `${s.data.length} user, ${admins} Platform Admin. Akses database diatur lewat Groups.`}
      />
      {s.error && <Alert>{s.error}</Alert>}
      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)] lg:items-start">
        <form onSubmit={save} className={`${ui.well} space-y-4`}>
          <h2 className="text-base font-semibold">{id === null ? 'Buat user' : `Edit ${username || 'user'}`}</h2>
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
          <div className="flex gap-2 pt-1">
            <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
            <button type="button" className={ui.button} onClick={reset}>Batal</button>
          </div>
        </form>

        <div className="min-w-0 overflow-x-auto">
          {s.data === null ? (
            <Loading rows={3} />
          ) : s.data.length === 0 ? (
            <Empty>Belum ada user.</Empty>
          ) : (
            <table className={`${ui.table} min-w-[520px]`}>
              <thead>
                <tr>
                  <th className={`${ui.th} w-2/5`}>Username</th>
                  <th className={ui.th}>Role</th>
                  <th className={ui.th}><span className="sr-only">Aksi</span></th>
                </tr>
              </thead>
              <tbody>
                {s.data.map((u) => (
                  <tr key={u.id} className={u.id === id ? 'bg-raised' : ''}>
                    <td className={`${ui.td} h-14`}>
                      <span className="flex items-center gap-2.5">
                        <Avatar id={u.id} name={u.username} />
                        <span className="font-medium">{u.username}</span>
                      </span>
                    </td>
                    <td className={ui.td}>{u.isAdmin ? <Pill>Platform Admin</Pill> : <span className="text-muted">User</span>}</td>
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
          )}
        </div>
      </div>
    </>
  )
}
