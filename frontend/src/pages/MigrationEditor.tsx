import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/http'
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
import { formatTime } from '../utils/format'

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
      <h1>
        Migration baru <Link to={`/databases/${databaseId}/migrations`}>← Riwayat</Link>
      </h1>
      <textarea className="sql" value={sql} onChange={(e) => setSql(e.target.value)} placeholder="ALTER TABLE ..." />
      {error && <p className="error">{error}</p>}
      <div className="toolbar">
        <button disabled={busy || !sql.trim()} onClick={save}>Simpan draft</button>
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

  if (!migration) return error ? <p className="error">{error.message}</p> : <p>Memuat…</p>

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
      <h1>
        Migration {migration.version ? `v${migration.version}` : `#${migration.id}`}{' '}
        <StatusBadge status={migration.status} />{' '}
        <Link to={`/databases/${migration.databaseId}/migrations`}>← Riwayat</Link>
      </h1>

      <dl className="meta">
        <dt>Dibuat</dt>
        <dd>{migration.createdBy} · {formatTime(migration.createdAt)}</dd>
        {migration.updatedBy && (
          <>
            <dt>Terakhir diubah</dt>
            <dd>{migration.updatedBy} · {formatTime(migration.updatedAt)}</dd>
          </>
        )}
        {migration.executedBy && (
          <>
            <dt>Dijalankan</dt>
            <dd>{migration.executedBy} · {formatTime(migration.startedAt)} → {formatTime(migration.finishedAt)}</dd>
          </>
        )}
      </dl>

      {isDraft && migration.baseOutdated && (
        <p className="warning">
          Database sudah berubah sejak draft ini dibuat (dibuat saat versi {migration.baseVersion ?? 0}). Tinjau ulang SQL
          sebelum dijalankan.
        </p>
      )}

      <textarea className="sql" value={sql} readOnly={!isDraft} onChange={(e) => setSql(e.target.value)} />

      {migration.status === 'failed' && migration.errorMessage && (
        <pre className="error">
          {migration.errorCode && `[${migration.errorCode}] `}
          {migration.errorMessage}
        </pre>
      )}
      {migration.status === 'running' && <p>Migration sedang berjalan…</p>}

      {error && (
        <p className="error">
          {error.message}{' '}
          {error instanceof ApiError && error.code === 'REVISION_CONFLICT' && (
            <button onClick={() => setReloadKey((k) => k + 1)}>Muat ulang</button>
          )}
        </p>
      )}

      <div className="toolbar">
        {isDraft && (
          <>
            <button disabled={busy || !dirty || !sql.trim()} onClick={save}>Simpan</button>
            <button disabled={busy || dirty} onClick={execute} title={dirty ? 'Simpan perubahan dulu' : undefined}>
              Jalankan
            </button>
            <button disabled={busy} onClick={remove}>Hapus</button>
          </>
        )}
        {(migration.status === 'failed' || migration.status === 'success') && (
          <button disabled={busy} onClick={duplicate}>Duplikat jadi draft baru</button>
        )}
      </div>
    </>
  )
}
