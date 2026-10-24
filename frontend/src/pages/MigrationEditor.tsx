import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import {
  type Migration,
  createMigration,
  deleteMigration,
  duplicateMigration,
  executeMigration,
  getMigration,
  updateMigration,
} from '../api/migrations'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { formatTime } from '../utils/format'

const sqlBox =
  'my-4 block min-h-56 w-full rounded border border-neutral-500/40 bg-transparent p-3 font-mono text-sm read-only:bg-neutral-500/5'

// TODO(B): hide edit actions when myLevel is VIEW (the backend already rejects them).
export default function MigrationEditor() {
  const { databaseId, migrationId } = useParams()
  return migrationId ? <ExistingMigration id={migrationId} /> : <NewMigration databaseId={databaseId ?? ''} />
}

function NewMigration({ databaseId }: { databaseId: string }) {
  const navigate = useNavigate()
  const [sql, setSql] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const save = async () => {
    setBusy(true)
    setError(null)
    try {
      const m = await createMigration(databaseId, sql)
      navigate(`/migrations/${m.id}`)
    } catch (e) {
      setError((e as Error).message)
      setBusy(false)
    }
  }

  return (
    <>
      <h1 className={ui.h1}>
        Migration baru
        <Link className={ui.link} to={`/databases/${databaseId}/migrations`}>← Riwayat</Link>
      </h1>
      <textarea className={sqlBox} value={sql} onChange={(e) => setSql(e.target.value)} placeholder="ALTER TABLE ..." />
      {error && <p className={ui.error}>{error}</p>}
      <div className="flex gap-2">
        <button className={ui.buttonPrimary} disabled={busy || !sql.trim()} onClick={save}>
          Simpan draft
        </button>
      </div>
    </>
  )
}

function ExistingMigration({ id }: { id: string }) {
  const navigate = useNavigate()
  const [migration, setMigration] = useState<Migration | null>(null)
  const [sql, setSql] = useState('')
  const [error, setError] = useState<ApiError | Error | null>(null)
  const [busy, setBusy] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let active = true
    getMigration(id).then(
      (m) => {
        if (!active) return
        setMigration(m)
        setSql(m.sql)
        setError(null)
      },
      (e: Error) => active && setError(e),
    )
    return () => {
      active = false
    }
  }, [id, reloadKey])

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (e) {
      setError(e as Error)
    } finally {
      setBusy(false)
    }
  }

  if (!migration) return error ? <p className={ui.error}>{error.message}</p> : <p className={ui.muted}>Memuat…</p>

  const isDraft = migration.status === 'draft'
  const dirty = sql !== migration.sql
  const show = (m: Migration) => {
    setMigration(m)
    setSql(m.sql)
  }

  const save = () => run(async () => show(await updateMigration(id, sql, migration.revision)))
  const execute = () => {
    if (!confirm('Jalankan migration ini pada database target?')) return
    run(async () => show(await executeMigration(id)))
  }
  const remove = () => {
    if (!confirm('Hapus draft ini?')) return
    run(async () => {
      await deleteMigration(id)
      navigate(`/databases/${migration.databaseId}/migrations`)
    })
  }
  const duplicate = () =>
    run(async () => {
      const m = await duplicateMigration(id)
      navigate(`/migrations/${m.id}`)
    })

  return (
    <>
      <h1 className={ui.h1}>
        Migration {migration.version ? `v${migration.version}` : `#${migration.id}`}
        <StatusBadge status={migration.status} />
        <Link className={ui.link} to={`/databases/${migration.databaseId}/migrations`}>← Riwayat</Link>
      </h1>

      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
        <dt className="text-neutral-500">Dibuat</dt>
        <dd>{migration.createdBy} · {formatTime(migration.createdAt)}</dd>
        {migration.updatedBy && (
          <>
            <dt className="text-neutral-500">Terakhir diubah</dt>
            <dd>{migration.updatedBy} · {formatTime(migration.updatedAt)}</dd>
          </>
        )}
        {migration.executedBy && (
          <>
            <dt className="text-neutral-500">Dijalankan</dt>
            <dd>{migration.executedBy} · {formatTime(migration.startedAt)} → {formatTime(migration.finishedAt)}</dd>
          </>
        )}
      </dl>

      {isDraft && migration.baseOutdated && (
        <p className="mt-4 rounded border border-orange-500/50 bg-orange-500/10 px-3 py-2 text-sm text-orange-700 dark:text-orange-300">
          Database sudah berubah sejak draft ini dibuat (dibuat saat versi {migration.baseVersion ?? 0}). Tinjau ulang SQL
          sebelum dijalankan.
        </p>
      )}

      <textarea className={sqlBox} value={sql} readOnly={!isDraft} onChange={(e) => setSql(e.target.value)} />

      {migration.status === 'failed' && migration.errorMessage && (
        <pre className="mb-4 rounded border border-red-600/40 bg-red-600/10 p-3 text-sm whitespace-pre-wrap text-red-700 dark:text-red-300">
          {migration.errorCode && `[${migration.errorCode}] `}
          {migration.errorMessage}
        </pre>
      )}
      {migration.status === 'running' && <p className={ui.muted}>Migration sedang berjalan…</p>}

      {error && (
        <p className={`${ui.error} mb-4 flex items-center gap-2`}>
          {error.message}
          {error instanceof ApiError && error.code === 'REVISION_CONFLICT' && (
            <button className={ui.button} onClick={() => setReloadKey((k) => k + 1)}>Muat ulang</button>
          )}
        </p>
      )}

      <div className="flex gap-2">
        {isDraft && (
          <>
            <button className={ui.button} disabled={busy || !dirty || !sql.trim()} onClick={save}>
              Simpan
            </button>
            <button
              className={ui.buttonPrimary}
              disabled={busy || dirty}
              onClick={execute}
              title={dirty ? 'Simpan perubahan dulu' : undefined}
            >
              Jalankan
            </button>
            <button className={`${ui.buttonDanger} ml-auto`} disabled={busy} onClick={remove}>
              Hapus
            </button>
          </>
        )}
        {(migration.status === 'failed' || migration.status === 'success') && (
          <button className={ui.button} disabled={busy} onClick={duplicate}>
            Duplikat jadi draft baru
          </button>
        )}
      </div>
    </>
  )
}
