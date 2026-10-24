import { useCallback, useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { message, read, write } from '../../api/admin'
import type { Database, Grant, Group, User } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, TrashIcon, UsersIcon } from '../../components/icons'
import { Alert, Card, EmptyState, Field, LevelBadge, Loading, PageHeader } from '../../components/kit'
import { ui } from '../../components/ui'

export default function Groups() {
  const load = useCallback(async () => {
    const [groups, users, databases] = await Promise.all([
      read<Group[]>('/admin/groups'),
      read<User[]>('/admin/users'),
      read<Database[]>('/admin/databases'),
    ])
    return { groups, users, databases }
  }, [])
  const s = useAdmin(load)
  const { setError } = s
  const [edit, setEdit] = useState<number | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [selected, setSelected] = useState<number | null>(null)
  const [members, setMembers] = useState<User[]>([])
  const [grants, setGrants] = useState<Grant[]>([])
  const [uid, setUid] = useState('')
  const [did, setDid] = useState('')
  const [level, setLevel] = useState<'VIEW' | 'EDIT'>('VIEW')

  // Ignore responses from a previously selected group.
  const sequence = useRef(0)
  const details = useCallback(async () => {
    const n = ++sequence.current
    setMembers([])
    setGrants([])
    if (selected === null) return
    try {
      const [m, g] = await Promise.all([
        read<User[]>(`/admin/groups/${selected}/members`),
        read<Grant[]>(`/admin/groups/${selected}/grants`),
      ])
      if (n === sequence.current) {
        setMembers(m)
        setGrants(g)
      }
    } catch (e) {
      if (n === sequence.current) setError(message(e))
    }
  }, [selected, setError])
  useEffect(() => {
    void details()
  }, [details])

  async function change(fn: () => Promise<unknown>) {
    if (await s.act(fn)) await details()
  }
  function resetForm() {
    setEdit(null)
    setName('')
    setDescription('')
  }
  async function save(e: FormEvent) {
    e.preventDefault()
    if (await s.act(() => write(edit === null ? 'POST' : 'PATCH', edit === null ? '/admin/groups' : `/admin/groups/${edit}`, { name, description }))) resetForm()
  }

  const group = s.data?.groups.find((g) => g.id === selected)

  return (
    <>
      <PageHeader title="Groups" subtitle="Akses database diberikan ke group, lalu berlaku untuk semua anggotanya." />
      {s.error && <Alert>{s.error}</Alert>}
      <div className="grid gap-6 lg:grid-cols-[20rem_minmax(0,1fr)] lg:items-start">
        <div className="space-y-6">
          <Card title={edit === null ? 'Buat group' : 'Edit group'}>
            <form onSubmit={save} className="space-y-4">
              <Field label="Nama">
                <input className={ui.input} required maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
              </Field>
              <Field label="Deskripsi">
                <input className={ui.input} value={description} onChange={(e) => setDescription(e.target.value)} />
              </Field>
              <div className="flex gap-2">
                <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
                <button type="button" className={ui.button} onClick={resetForm}>Batal</button>
              </div>
            </form>
          </Card>

          <Card title="Semua group" flush>
            {!s.data ? (
              <div className="px-4"><Loading /></div>
            ) : s.data.groups.length === 0 ? (
              <EmptyState title="Belum ada group" />
            ) : (
              <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
                {s.data.groups.map((g) => (
                  <li key={g.id} className={`flex items-center gap-2 px-2 py-1.5 ${g.id === selected ? 'bg-indigo-50/70 dark:bg-indigo-500/10' : ''}`}>
                    <button className="min-w-0 flex-1 rounded-md px-2 py-1 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800" onClick={() => setSelected(g.id)}>
                      <span className={`block truncate text-sm font-medium ${g.id === selected ? 'text-indigo-700 dark:text-indigo-300' : ''}`}>{g.name}</span>
                      {g.description && <span className="block truncate text-xs text-zinc-500 dark:text-zinc-400">{g.description}</span>}
                    </button>
                    <button
                      className={ui.buttonSmGhost}
                      disabled={s.busy}
                      aria-label={`Edit ${g.name}`}
                      onClick={() => {
                        setEdit(g.id)
                        setName(g.name)
                        setDescription(g.description)
                      }}
                    >
                      <PencilIcon size={14} />
                    </button>
                    <button
                      className={`${ui.buttonSmGhost} hover:text-red-600 dark:hover:text-red-400`}
                      disabled={s.busy}
                      aria-label={`Hapus ${g.name}`}
                      onClick={() => {
                        if (confirm(`Hapus group ${g.name} beserta membership dan grant?`))
                          void s.act(async () => {
                            await write('DELETE', `/admin/groups/${g.id}`)
                            if (selected === g.id) setSelected(null)
                          })
                      }}
                    >
                      <TrashIcon size={14} />
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        {selected === null || !s.data ? (
          <div className={ui.card}>
            <EmptyState icon={<UsersIcon size={20} />} title="Pilih group">
              Pilih group di sebelah kiri untuk mengatur anggota dan grant database.
            </EmptyState>
          </div>
        ) : (
          <div className="space-y-6">
            <Card title={<>Anggota · {group?.name}</>} description={`${members.length} anggota`} flush>
              <form
                className="flex flex-wrap gap-2 border-b border-zinc-200 p-4 dark:border-zinc-800"
                onSubmit={(e) => {
                  e.preventDefault()
                  void change(() => write('PUT', `/admin/groups/${selected}/members/${uid}`))
                }}
              >
                <select className={`${ui.select} min-w-48 flex-1`} required value={uid} onChange={(e) => setUid(e.target.value)}>
                  <option value="">Pilih user</option>
                  {s.data.users
                    .filter((u) => !members.some((m) => m.id === u.id))
                    .map((u) => (
                      <option key={u.id} value={u.id}>{u.username}</option>
                    ))}
                </select>
                <button className={ui.buttonPrimary} disabled={s.busy}>Tambah anggota</button>
              </form>
              {members.length === 0 ? (
                <p className={`p-4 ${ui.muted}`}>Belum ada anggota.</p>
              ) : (
                <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
                  {members.map((u) => (
                    <li key={u.id} className="flex items-center gap-3 px-4 py-2">
                      <span className="grid size-7 place-items-center rounded-full bg-zinc-100 text-xs font-semibold uppercase dark:bg-zinc-800">
                        {u.username.slice(0, 1)}
                      </span>
                      <span className="flex-1 text-sm">{u.username}</span>
                      <button
                        className={ui.buttonSmDanger}
                        disabled={s.busy}
                        onClick={() => void change(() => write('DELETE', `/admin/groups/${selected}/members/${u.id}`))}
                      >
                        Keluarkan
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card title="Grant database" description="Level tertinggi dari semua group user yang berlaku." flush>
              <form
                className="flex flex-wrap gap-2 border-b border-zinc-200 p-4 dark:border-zinc-800"
                onSubmit={(e) => {
                  e.preventDefault()
                  void change(() => write('PUT', `/admin/groups/${selected}/grants/${did}`, { level }))
                }}
              >
                <select className={`${ui.select} min-w-48 flex-1`} required value={did} onChange={(e) => setDid(e.target.value)}>
                  <option value="">Pilih database</option>
                  {s.data.databases.map((d) => (
                    <option key={d.id} value={d.id}>{d.name}</option>
                  ))}
                </select>
                <select className={ui.select} value={level} onChange={(e) => setLevel(e.target.value as 'VIEW' | 'EDIT')}>
                  <option>VIEW</option>
                  <option>EDIT</option>
                </select>
                <button className={ui.buttonPrimary} disabled={s.busy}>Simpan grant</button>
              </form>
              {grants.length === 0 ? (
                <p className={`p-4 ${ui.muted}`}>Belum ada grant.</p>
              ) : (
                <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
                  {grants.map((g) => (
                    <li key={g.databaseId} className="flex items-center gap-3 px-4 py-2">
                      <span className="flex-1 text-sm font-medium">{s.data?.databases.find((d) => d.id === g.databaseId)?.name}</span>
                      <LevelBadge level={g.level} />
                      <button
                        className={ui.buttonSm}
                        disabled={s.busy}
                        onClick={() => {
                          setDid(String(g.databaseId))
                          setLevel(g.level)
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className={ui.buttonSmDanger}
                        disabled={s.busy}
                        onClick={() => void change(() => write('DELETE', `/admin/groups/${selected}/grants/${g.databaseId}`))}
                      >
                        Cabut
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        )}
      </div>
    </>
  )
}
