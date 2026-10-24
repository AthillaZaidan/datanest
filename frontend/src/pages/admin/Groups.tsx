import { useCallback, useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { message, read, write } from '../../api/admin'
import type { Database, Grant, Group, User } from '../../api/admin'
import { useAdmin } from '../../components/admin/useAdmin'
import { PencilIcon, TrashIcon } from '../../components/icons'
import { Alert, Avatar, Empty, Field, LevelPill, Loading, PageHeader, Section } from '../../components/kit'
import { ui } from '../../components/ui'
import { useCrumbs } from '../../context/crumbs'

export default function Groups() {
  useCrumbs([{ label: 'Administrasi' }, { label: 'Groups' }])
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
  // Open the first group until one is picked, so the members and grants show without a click.
  if (selected === null && s.data?.groups.length) setSelected(s.data.groups[0].id)

  return (
    <>
      <PageHeader
        title="Groups"
        subtitle={s.data && `${s.data.groups.length} group. Akses database diberikan ke group; level tertinggi dari semua group user yang berlaku.`}
      />
      {s.error && <Alert>{s.error}</Alert>}
      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)] lg:items-start">
        <div className="space-y-6">
          <form onSubmit={save} className={`${ui.well} space-y-4`}>
            <h2 className="text-base font-semibold">{edit === null ? 'Buat group' : `Edit ${name || 'group'}`}</h2>
            <Field label="Nama">
              <input className={ui.input} required maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label="Deskripsi">
              <input className={ui.input} value={description} onChange={(e) => setDescription(e.target.value)} />
            </Field>
            <div className="flex gap-2 pt-1">
              <button className={ui.buttonPrimary} disabled={s.busy}>Simpan</button>
              <button type="button" className={ui.button} onClick={resetForm}>Batal</button>
            </div>
          </form>

          <Section title="Semua group" aside={s.data && <span className="num">{s.data.groups.length}</span>}>
            {!s.data ? (
              <Loading rows={2} />
            ) : s.data.groups.length === 0 ? (
              <Empty>Belum ada group.</Empty>
            ) : (
              <ul className="space-y-0.5">
                {s.data.groups.map((g) => (
                  <li key={g.id} className={`flex items-center gap-1 rounded-lg pr-1 ${g.id === selected ? 'bg-active' : 'hover:bg-raised'}`}>
                    <button className="min-w-0 flex-1 rounded-lg px-2.5 py-2 text-left focus-visible:outline-2 focus-visible:outline-accent" onClick={() => setSelected(g.id)} aria-current={g.id === selected ? 'true' : undefined}>
                      <span className="block truncate text-sm font-medium">{g.name}</span>
                      {g.description && <span className="block truncate text-xs text-muted">{g.description}</span>}
                    </button>
                    <button
                      className={ui.buttonSmGhost}
                      disabled={s.busy}
                      aria-label={`Edit ${g.name}`}
                      title="Edit"
                      onClick={() => {
                        setEdit(g.id)
                        setName(g.name)
                        setDescription(g.description)
                      }}
                    >
                      <PencilIcon size={14} />
                    </button>
                    <button
                      className={`${ui.buttonSmGhost} hover:text-negative`}
                      disabled={s.busy}
                      aria-label={`Hapus ${g.name}`}
                      title="Hapus"
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
          </Section>
        </div>

        {selected === null || !s.data ? (
          <Empty>Pilih group di sebelah kiri untuk mengatur anggota dan grant database.</Empty>
        ) : (
          <div className="min-w-0 space-y-10">
            <Section title={<>Anggota {group?.name}</>} aside={<span className="num">{members.length}</span>}>
              <form
                className="mb-2 flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  void change(() => write('PUT', `/admin/groups/${selected}/members/${uid}`))
                }}
              >
                <select className={`${ui.select} min-w-48 flex-1 sm:max-w-80`} required value={uid} onChange={(e) => setUid(e.target.value)} aria-label="User">
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
                <Empty>Group ini belum punya anggota.</Empty>
              ) : (
                <ul>
                  {members.map((u) => (
                    <li key={u.id} className="flex h-12 items-center gap-3 border-t border-rule">
                      <Avatar id={u.id} name={u.username} />
                      <span className="flex-1 font-medium">{u.username}</span>
                      {u.isAdmin && <span className="text-xs text-muted">Platform Admin</span>}
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
            </Section>

            <Section title="Grant database" aside={<span className="num">{grants.length}</span>}>
              <form
                className="mb-2 flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  void change(() => write('PUT', `/admin/groups/${selected}/grants/${did}`, { level }))
                }}
              >
                <select className={`${ui.select} min-w-48 flex-1 sm:max-w-80`} required value={did} onChange={(e) => setDid(e.target.value)} aria-label="Database">
                  <option value="">Pilih database</option>
                  {s.data.databases.map((d) => (
                    <option key={d.id} value={d.id}>{d.name}</option>
                  ))}
                </select>
                <select className={ui.select} value={level} onChange={(e) => setLevel(e.target.value as 'VIEW' | 'EDIT')} aria-label="Level">
                  <option>VIEW</option>
                  <option>EDIT</option>
                </select>
                <button className={ui.buttonPrimary} disabled={s.busy}>Simpan grant</button>
              </form>
              {grants.length === 0 ? (
                <Empty>Group ini belum punya grant ke database mana pun.</Empty>
              ) : (
                <ul>
                  {grants.map((g) => {
                    const d = s.data?.databases.find((x) => x.id === g.databaseId)
                    return (
                      <li key={g.databaseId} className="flex h-14 items-center gap-3 border-t border-rule">
                        <div className="min-w-0 flex-1">
                          <p className="font-medium">{d?.name}</p>
                          {d && <p className={`${ui.ident} truncate text-muted`}>{d.host}:{d.port}/{d.dbName}</p>}
                        </div>
                        <LevelPill level={g.level} />
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
                    )
                  })}
                </ul>
              )}
            </Section>
          </div>
        )}
      </div>
    </>
  )
}
