import { useEffect, useState } from 'react'
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
import { CopyIcon, PlayIcon, SaveIcon, TrashIcon } from '../components/icons'
import { Alert, Loading, Meta, PageHeader } from '../components/kit'
import SqlEditor from '../components/migrations/SqlEditor'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { duration, formatTime } from '../utils/format'

function useDatabase(databaseId: number | string | undefined) {
  const { data } = useAsync(() => (databaseId ? getDatabase(databaseId) : Promise.resolve(null)), [databaseId])
  return data
}

function ViewOnlyNotice() {
  return <Alert tone="info">Anda punya akses View ke database ini: migration bisa dibaca, tetapi tidak bisa diubah atau dijalankan.</Alert>
}

export default function MigrationEditor() {
  const { databaseId, migrationId } = useParams()
  return migrationId ? <ExistingMigration id={migrationId} /> : <NewMigration databaseId={databaseId ?? ''} />
}

function crumbsFor(databaseId: number | string, name: string | undefined, last: string) {
  return [
    { label: 'Databases', to: '/databases' },
    { label: name ?? `Database #${databaseId}`, to: `/databases/${databaseId}` },
    { label: 'Riwayat migration', to: `/databases/${databaseId}/migrations` },
    { label: last },
  ]
}

function NewMigration({ databaseId }: { databaseId: string }) {
  const navigate = useNavigate()
  const db = useDatabase(databaseId)
  const [sql, setSql] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useCrumbs(crumbsFor(databaseId, db?.name, 'Migration baru'))

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
        title="Migration baru"
        subtitle={db && `Draft untuk ${db.name}, dibuat di atas versi schema ${db.currentVersion ? `v${db.currentVersion}` : 'awal'}. Draft bisa ditinjau sebelum dijalankan.`}
      />
      {db?.myLevel === 'VIEW' ? (
        <ViewOnlyNotice />
      ) : (
        <div className="max-w-[960px]">
          <SqlEditor value={sql} onChange={setSql} placeholder="ALTER TABLE products ADD COLUMN sku text;" />
          {error && <div className="mt-4"><Alert>{error}</Alert></div>}
          <div className="mt-4 flex items-center gap-3">
            <button className={ui.buttonPrimary} disabled={busy || !sql.trim()} onClick={save}>
              <SaveIcon /> Simpan draft
            </button>
            <span className={ui.small}>Satu migration dijalankan dalam satu transaction. BEGIN, COMMIT, dan ROLLBACK tidak diperlukan.</span>
          </div>
        </div>
      )}
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
  const db = useDatabase(migration?.databaseId)
  const title = migration ? (migration.version ? `Migration v${migration.version}` : `Draft #${migration.id}`) : 'Migration'
  useCrumbs(crumbsFor(migration?.databaseId ?? '', db?.name, migration?.version ? `v${migration.version}` : `Draft #${id}`))

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

  if (!migration) return error ? <Alert>{error.message}</Alert> : <Loading rows={6} />

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

  const subtitle =
    migration.status === 'success'
      ? `Dijalankan ${migration.executedBy} pada ${formatTime(migration.startedAt)}, selesai dalam ${duration(migration.startedAt, migration.finishedAt)}.`
      : migration.status === 'failed'
        ? `Gagal saat dijalankan ${migration.executedBy} pada ${formatTime(migration.startedAt)}. Perubahan di-rollback, schema tetap di versi sebelumnya.`
        : migration.status === 'running'
          ? `Sedang dijalankan ${migration.executedBy} sejak ${formatTime(migration.startedAt)}.`
          : `Draft dari ${migration.createdBy}, dibuat di atas versi ${migration.baseVersion ? `v${migration.baseVersion}` : 'awal'}.`

  return (
    <>
      <PageHeader title={title} badges={<StatusBadge status={migration.status} />} subtitle={subtitle} />

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

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_288px] lg:items-start">
        <div className="min-w-0 space-y-4">
          <SqlEditor value={sql} readOnly={!isDraft || !canEdit} onChange={setSql} />

          {migration.status === 'failed' && migration.errorMessage && (
            <div className="rounded-lg bg-neg-bg px-4 py-3 text-sm text-neg-ink">
              <p className="font-medium">
                Error dari PostgreSQL{migration.errorCode && <> <span className="font-mono font-normal">SQLSTATE {migration.errorCode}</span></>}
              </p>
              <pre className="mt-1 font-mono text-[13px] whitespace-pre-wrap">{migration.errorMessage}</pre>
            </div>
          )}

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
                  {dirty && <span className={ui.small}>Ada perubahan yang belum disimpan.</span>}
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
        </div>

        <aside>
          <h2 className="mb-1 text-base font-semibold">Detail</h2>
          <dl>
            <Meta label="Database">{db?.name ?? '-'}</Meta>
            <Meta label="Versi">{migration.version ? <span className="font-mono">v{migration.version}</span> : <span className="text-muted">diberi saat dijalankan</span>}</Meta>
            <Meta label="Versi dasar"><span className="font-mono">{migration.baseVersion ? `v${migration.baseVersion}` : '-'}</span></Meta>
            <Meta label="Dibuat">{migration.createdBy}<span className="block text-xs text-muted">{formatTime(migration.createdAt)}</span></Meta>
            {migration.updatedBy && (
              <Meta label="Terakhir diubah">{migration.updatedBy}<span className="block text-xs text-muted">{formatTime(migration.updatedAt)}</span></Meta>
            )}
            {migration.executedBy && (
              <Meta label="Dijalankan">{migration.executedBy}<span className="block text-xs text-muted">{formatTime(migration.startedAt)}</span></Meta>
            )}
            {migration.finishedAt && <Meta label="Durasi"><span className="num">{duration(migration.startedAt, migration.finishedAt)}</span></Meta>}
            <Meta label="Revisi"><span className="num">{migration.revision}</span></Meta>
          </dl>
        </aside>
      </div>
    </>
  )
}
