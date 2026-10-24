import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import { getDatabase } from '../api/databases'
import {
  type Migration,
  createMigration,
  deleteMigration,
  duplicateMigration,
  executeMigration,
  getMigration,
  updateMigration,
} from '../api/migrations'
import { CopyIcon, LockIcon, PlayIcon, SaveIcon, TrashIcon } from '../components/icons'
import { Alert, Badge, Card, Loading, PageHeader } from '../components/kit'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { formatTime } from '../utils/format'

const sqlBox =
  'block min-h-72 w-full resize-y rounded-b-xl border-0 bg-zinc-950 p-4 font-mono text-[13px] leading-relaxed text-zinc-100 placeholder:text-zinc-500 focus:ring-2 focus:ring-indigo-500/40 focus:outline-none read-only:text-zinc-300'

function useLevel(databaseId: number | string | undefined) {
  const { data } = useAsync(() => (databaseId ? getDatabase(databaseId) : Promise.resolve(null)), [databaseId])
  return data
}

function SqlEditor({ value, onChange, readOnly, placeholder }: { value: string; onChange?: (v: string) => void; readOnly?: boolean; placeholder?: string }) {
  return (
    <div className="overflow-hidden rounded-xl border border-zinc-200 shadow-xs dark:border-zinc-800">
      <div className="flex items-center justify-between bg-zinc-900 px-4 py-2 text-xs text-zinc-400">
        <span className="font-mono">migration.sql</span>
        {readOnly && (
          <span className="inline-flex items-center gap-1">
            <LockIcon size={12} /> read-only
          </span>
        )}
      </div>
      <textarea
        className={sqlBox}
        value={value}
        readOnly={readOnly}
        spellCheck={false}
        placeholder={placeholder}
        onChange={(e) => onChange?.(e.target.value)}
      />
    </div>
  )
}

function ViewOnlyNotice() {
  return <Alert tone="info">Anda hanya memiliki akses VIEW ke database ini, jadi migration tidak dapat diubah atau dijalankan.</Alert>
}

export default function MigrationEditor() {
  const { databaseId, migrationId } = useParams()
  return migrationId ? <ExistingMigration id={migrationId} /> : <NewMigration databaseId={databaseId ?? ''} />
}

function NewMigration({ databaseId }: { databaseId: string }) {
  const navigate = useNavigate()
  const db = useLevel(databaseId)
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
      <PageHeader
        back={{ to: `/databases/${databaseId}/migrations`, label: 'Riwayat migration' }}
        title="Migration baru"
        subtitle={db && <>Draft untuk <strong className="font-medium text-zinc-700 dark:text-zinc-300">{db.name}</strong>. Draft bisa ditinjau dulu sebelum dijalankan.</>}
      />
      {db?.myLevel === 'VIEW' ? (
        <ViewOnlyNotice />
      ) : (
        <>
          <SqlEditor value={sql} onChange={setSql} placeholder="ALTER TABLE ..." />
          {error && <div className="mt-4"><Alert>{error}</Alert></div>}
          <div className="mt-4 flex gap-2">
            <button className={ui.buttonPrimary} disabled={busy || !sql.trim()} onClick={save}>
              <SaveIcon /> Simpan draft
            </button>
          </div>
        </>
      )}
    </>
  )
}

function Meta({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium text-zinc-500 dark:text-zinc-400">{label}</dt>
      <dd className="mt-0.5 text-sm">{children}</dd>
    </div>
  )
}

function ExistingMigration({ id }: { id: string }) {
  const navigate = useNavigate()
  const [migration, setMigration] = useState<Migration | null>(null)
  const [sql, setSql] = useState('')
  const [error, setError] = useState<ApiError | Error | null>(null)
  const [busy, setBusy] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const db = useLevel(migration?.databaseId)

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

  if (!migration) return error ? <Alert>{error.message}</Alert> : <Loading />

  const canEdit = db?.myLevel === 'EDIT'
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
      <PageHeader
        back={{ to: `/databases/${migration.databaseId}/migrations`, label: 'Riwayat migration' }}
        title={`Migration ${migration.version ? `v${migration.version}` : `#${migration.id}`}`}
        badges={
          <>
            <StatusBadge status={migration.status} />
            {db && <Badge>{db.name}</Badge>}
          </>
        }
      />

      {db?.myLevel === 'VIEW' && <ViewOnlyNotice />}

      {error && (
        <Alert
          action={
            error instanceof ApiError && error.code === 'REVISION_CONFLICT' && (
              <button className={ui.buttonSm} onClick={() => setReloadKey((k) => k + 1)}>Muat ulang</button>
            )
          }
        >
          {error.message}
        </Alert>
      )}

      {isDraft && migration.baseOutdated && (
        <Alert tone="warning">
          Database sudah berubah sejak draft ini dibuat (dibuat saat versi {migration.baseVersion ?? 0}). Tinjau ulang SQL
          sebelum dijalankan.
        </Alert>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_16rem]">
        <div className="min-w-0 space-y-4">
          <SqlEditor value={sql} readOnly={!isDraft || !canEdit} onChange={setSql} />
          {canEdit && (
            <div className="flex flex-wrap items-center gap-2">
              {isDraft && (
                <>
                  <button className={ui.button} disabled={busy || !dirty || !sql.trim()} onClick={save}>
                    <SaveIcon /> Simpan
                  </button>
                  <button
                    className={ui.buttonPrimary}
                    disabled={busy || dirty}
                    onClick={execute}
                    title={dirty ? 'Simpan perubahan dulu' : undefined}
                  >
                    <PlayIcon /> Jalankan
                  </button>
                  {dirty && <span className={ui.muted}>Ada perubahan yang belum disimpan.</span>}
                  <button className={`${ui.buttonDanger} ml-auto`} disabled={busy} onClick={remove}>
                    <TrashIcon /> Hapus
                  </button>
                </>
              )}
              {(migration.status === 'failed' || migration.status === 'success') && (
                <button className={ui.button} disabled={busy} onClick={duplicate}>
                  <CopyIcon /> Duplikat jadi draft baru
                </button>
              )}
            </div>
          )}

          {migration.status === 'failed' && migration.errorMessage && (
            <div className="overflow-hidden rounded-xl border border-red-200 dark:border-red-900/60">
              <div className="flex items-center gap-2 bg-red-50 px-4 py-2 text-xs font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300">
                Error dari database {migration.errorCode && <code className="font-mono">[{migration.errorCode}]</code>}
              </div>
              <pre className="px-4 py-3 font-mono text-xs whitespace-pre-wrap text-red-800 dark:text-red-200">{migration.errorMessage}</pre>
            </div>
          )}
          {migration.status === 'running' && <Loading label="Migration sedang berjalan…" />}
        </div>

        <Card title="Detail">
          <dl className="space-y-3">
            <Meta label="Dibuat">
              {migration.createdBy}
              <span className="block text-xs text-zinc-500 dark:text-zinc-400">{formatTime(migration.createdAt)}</span>
            </Meta>
            {migration.updatedBy && (
              <Meta label="Terakhir diubah">
                {migration.updatedBy}
                <span className="block text-xs text-zinc-500 dark:text-zinc-400">{formatTime(migration.updatedAt)}</span>
              </Meta>
            )}
            {migration.executedBy && (
              <Meta label="Dijalankan">
                {migration.executedBy}
                <span className="block text-xs text-zinc-500 dark:text-zinc-400">
                  {formatTime(migration.startedAt)} → {formatTime(migration.finishedAt)}
                </span>
              </Meta>
            )}
            {isDraft && <Meta label="Versi dasar">{migration.baseVersion ? `v${migration.baseVersion}` : '—'}</Meta>}
          </dl>
        </Card>
      </div>
    </>
  )
}
