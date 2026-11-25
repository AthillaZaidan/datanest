import { type ReactNode, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError, detailTexts } from '../api/client'
import { getDatabase } from '../api/databases'
import {
  type Migration,
  type Recipe,
  createMigration,
  deleteMigration,
  duplicateMigration,
  getMigration,
  submitExecution,
  updateMigration,
} from '../api/migrations'
import type { ExecutionKind } from '../api/executions'
import { ArrowLeftIcon, CopyIcon, PlayIcon, RefreshIcon, SaveIcon, TrashIcon } from '../components/icons'
import ExecutionStatusBadge from '../components/executions/ExecutionStatusBadge'
import { Alert, Loading, Meta, PageHeader } from '../components/kit'
import {
  DEFAULT_BATCH_SIZE,
  MAX_BATCH_SIZE,
  asksBatchSize,
  batchSizeFor,
  canSubmit,
  isDraft,
  migrationActions,
  openExecutionOf,
  openPhrase,
  parseBatchSize,
} from '../components/migrations/actions'
import PlanPanel from '../components/migrations/PlanPanel'
import RecipeEditor from '../components/migrations/RecipeEditor'
import { emptyRecipe, recipeComplete, recipeOf, sameRecipe } from '../components/migrations/recipe'
import StatusBadge from '../components/migrations/StatusBadge'
import { ui } from '../components/ui'
import { useCrumbs } from '../context/crumbs'
import { useAsync } from '../hooks/useAsync'
import { duration, formatTime } from '../utils/format'

function useDatabase(databaseId: number | string | undefined) {
  const { data } = useAsync(() => (databaseId ? getDatabase(databaseId) : Promise.resolve(null)), [databaseId])
  return data
}

function ErrorNotice({ error, action }: { error: Error; action?: ReactNode }) {
  const details = detailTexts(error)
  return (
    <Alert action={action}>
      {error.message}
      {details.length > 0 && (
        <ul className="mt-1 list-disc pl-5">
          {details.map((d, i) => (
            <li key={i}>{d}</li>
          ))}
        </ul>
      )}
    </Alert>
  )
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
  const [recipe, setRecipe] = useState<Recipe>(emptyRecipe)
  const [error, setError] = useState<Error | null>(null)
  const [busy, setBusy] = useState(false)
  useCrumbs(crumbsFor(databaseId, db?.name, 'Migration baru'))

  const save = async () => {
    setBusy(true)
    setError(null)
    try {
      const m = await createMigration(databaseId, recipe)
      navigate(`/migrations/${m.id}`)
    } catch (e) {
      setError(e as Error)
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
          <RecipeEditor value={recipe} onChange={setRecipe} />
          {error && <div className="mt-4"><ErrorNotice error={error} /></div>}
          <div className="mt-4 flex items-center gap-3">
            <button className={ui.buttonPrimary} disabled={busy || !recipeComplete(recipe)} onClick={save}>
              <SaveIcon /> Simpan draft
            </button>
            <span className={ui.small}>
              {recipe.mode === 'batch' ? 'Setiap batch dijalankan dalam transaction sendiri' : 'Satu migration dijalankan dalam satu transaction'}. BEGIN, COMMIT,
              dan ROLLBACK tidak diperlukan.
            </span>
          </div>
        </div>
      )}
    </>
  )
}

function ExistingMigration({ id }: { id: string }) {
  const navigate = useNavigate()
  const [migration, setMigration] = useState<Migration | null>(null)
  const [recipe, setRecipe] = useState<Recipe>(emptyRecipe)
  const [error, setError] = useState<ApiError | Error | null>(null)
  const [busy, setBusy] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [batchSizeText, setBatchSizeText] = useState(DEFAULT_BATCH_SIZE)
  const db = useDatabase(migration?.databaseId)
  const title = migration ? (migration.version ? `Migration v${migration.version}` : `Draft #${migration.id}`) : 'Migration'
  useCrumbs(crumbsFor(migration?.databaseId ?? '', db?.name, migration?.version ? `v${migration.version}` : `Draft #${id}`))

  useEffect(() => {
    let active = true
    getMigration(id).then(
      (m) => {
        if (!active) return
        setMigration(m)
        setRecipe(recipeOf(m))
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
  // A draft whose first execution started is frozen: its SQL is kept exactly as it ran.
  const draft = isDraft(migration)
  const dirty = !sameRecipe(recipe, recipeOf(migration))
  const show = (m: Migration) => {
    setMigration(m)
    setRecipe(recipeOf(m))
  }

  const save = () => run(async () => show(await updateMigration(id, recipe, migration.revision)))
  const current = db?.currentVersion ?? 0
  // The backend re-checks every rule (design D2); these only decide which buttons make sense.
  const isTop = migration.active && migration.version === current
  const actions = migrationActions(migration, current)
  const resume = actions.resume
  const open = openExecutionOf(migration)
  const batchSize = parseBatchSize(batchSizeText)
  const allowed = (kind: ExecutionKind) => !busy && canSubmit(migration, kind, batchSize)
  const submit = (kind: ExecutionKind, question: string) => {
    const size = batchSizeFor(migration, kind, batchSize)
    const sizeNote = size === undefined ? '' : ` Ukuran batch ${size}.`
    if (!confirm(`${question}${sizeNote} Permintaan masuk ke review sebelum dijalankan.`)) return
    run(async () => {
      const e = await submitExecution(id, kind, size)
      navigate(`/executions/${e.id}`)
    })
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
      ? `Dijalankan ${migration.executedBy} pada ${formatTime(migration.startedAt)}, selesai dalam ${duration(migration.startedAt, migration.finishedAt)}. Saat ini ${migration.active ? 'aktif' : 'tidak aktif'}.`
      : migration.status === 'failed'
        ? `Gagal saat dijalankan ${migration.executedBy} pada ${formatTime(migration.startedAt)}. Perubahan di-rollback, schema tetap di versi sebelumnya.`
        : migration.status === 'running'
          ? `Sedang dijalankan ${migration.executedBy} sejak ${formatTime(migration.startedAt)}.`
          : migration.frozenAt
            ? `v${migration.version} ${migration.active ? 'aktif' : 'tidak aktif'} pada database. Riwayat lengkap ada di halaman riwayat eksekusi.`
            : `Draft dari ${migration.createdBy}, dibuat di atas versi ${migration.baseVersion ? `v${migration.baseVersion}` : 'awal'}.`

  return (
    <>
      <PageHeader title={title} badges={<StatusBadge migration={migration} />} subtitle={subtitle} />

      {db?.myLevel === 'VIEW' && <ViewOnlyNotice />}

      {error && (
        <ErrorNotice
          error={error}
          action={
            error instanceof ApiError && error.code === 'REVISION_CONFLICT' && (
              <button className={ui.buttonSm} onClick={() => setReloadKey((k) => k + 1)}>Muat ulang</button>
            )
          }
        />
      )}

      {migration.partial && (
        <Alert tone="warning">
          {migration.partial.kind === 'rollback' ? 'Rollback ' : 'Eksekusi '}
          <Link className={ui.link} to={`/executions/${migration.partial.executionId}`}>E{migration.partial.executionId}</Link> berhenti di tengah jalan:{' '}
          {migration.partial.kind === 'rollback'
            ? 'sebagian batch sudah dibatalkan. Lanjutkan sampai selesai.'
            : `sebagian batch sudah committed. Lanjutkan dari checkpoint terakhir${actions.undo ? ' atau batalkan batch yang sudah committed' : '. Migration ini irreversible, batch yang sudah committed tidak bisa dibatalkan'}.`}
        </Alert>
      )}

      {open && (
        <Alert tone="info">
          Eksekusi <Link className={ui.link} to={`/executions/${open.id}`}>E{open.id}</Link> {openPhrase[open.status]}. Pengajuan baru bisa dibuat setelah itu selesai.
        </Alert>
      )}

      {draft && migration.baseOutdated && (
        <Alert tone="warning">
          Database sudah berubah sejak draft ini dibuat (dibuat saat versi {migration.baseVersion ?? 0}). Tinjau ulang SQL
          sebelum dijalankan.
        </Alert>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_288px] lg:items-start">
        <div className="min-w-0 space-y-4">
          {migration.frozenAt && (
            <Alert tone="info">
              SQL terkunci sejak {formatTime(migration.frozenAt)} karena sudah pernah dieksekusi. Perbaikan dibuat sebagai migration baru.
            </Alert>
          )}
          <RecipeEditor value={recipe} readOnly={!draft || !canEdit} onChange={setRecipe} />

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
              {asksBatchSize(migration, actions) && (
                <label className="flex items-center gap-2 text-sm">
                  <span className="whitespace-nowrap">Ukuran batch</span>
                  <input
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={MAX_BATCH_SIZE}
                    className={`${ui.inputSm} w-28`}
                    value={batchSizeText}
                    aria-invalid={batchSize === null}
                    onChange={(e) => setBatchSizeText(e.target.value)}
                  />
                </label>
              )}
              {asksBatchSize(migration, actions) && batchSize === null && <span className={ui.error}>Isi 1 sampai 1.000.000.</span>}
              {draft && (
                <>
                  <button className={ui.button} disabled={busy || !dirty || !recipeComplete(recipe)} onClick={save}>
                    <SaveIcon /> Simpan
                  </button>
                  {actions.run && (
                    <button
                      className={ui.buttonPrimary}
                      disabled={!allowed('run') || dirty}
                      onClick={() => submit('run', 'Ajukan migration ini untuk dijalankan pada database target?')}
                      title={dirty ? 'Simpan perubahan dulu' : undefined}
                    >
                      <PlayIcon /> Ajukan run
                    </button>
                  )}
                  {dirty && <span className={ui.small}>Ada perubahan yang belum disimpan.</span>}
                  <button className={`${ui.buttonDanger} ml-auto`} disabled={busy} onClick={remove}>
                    <TrashIcon /> Hapus
                  </button>
                </>
              )}
              {resume && (
                <button
                  className={ui.buttonPrimary}
                  disabled={!allowed(resume)}
                  onClick={() =>
                    submit(resume, resume === 'rollback' ? 'Lanjutkan rollback yang berhenti sebagian?' : 'Lanjutkan eksekusi yang berhenti sebagian dari checkpoint terakhir?')
                  }
                >
                  <PlayIcon /> Lanjutkan
                </button>
              )}
              {actions.undo && (
                <button
                  className={ui.buttonDanger}
                  disabled={!allowed('rollback')}
                  onClick={() => submit('rollback', 'Batalkan batch yang sudah committed memakai SQL down-nya?')}
                >
                  <ArrowLeftIcon /> Batalkan batch
                </button>
              )}
              {actions.rollback && (
                <button className={ui.buttonDanger} disabled={!allowed('rollback')} onClick={() => submit('rollback', `Ajukan rollback v${migration.version} memakai SQL down-nya?`)}>
                  <ArrowLeftIcon /> Rollback
                </button>
              )}
              {actions.forward && (
                <button className={ui.button} disabled={!allowed('forward')} onClick={() => submit('forward', `Ajukan forward v${migration.version} (jalankan ulang SQL up)?`)}>
                  <RefreshIcon /> Forward
                </button>
              )}
              {migration.irreversible && isTop && (
                <span className={ui.small}>Irreversible: {migration.irreversibleReason}. Tidak bisa di-rollback.</span>
              )}
              {(migration.status !== 'draft' || migration.frozenAt) && (
                <button className={ui.button} disabled={busy} onClick={duplicate}>
                  <CopyIcon /> Duplikat jadi draft baru
                </button>
              )}
            </div>
          )}

          {migration.mode !== 'batch' && <PlanPanel migrationId={id} canEdit={canEdit} dirty={dirty} />}
        </div>

        <aside>
          <h2 className="mb-1 text-base font-semibold">Detail</h2>
          <dl>
            <Meta label="Database">{db?.name ?? '-'}</Meta>
            <Meta label="Versi">{migration.version ? <span className="font-mono">v{migration.version}</span> : <span className="text-muted">diberi saat dijalankan</span>}</Meta>
            {migration.mode === 'batch' && (
              <>
                <Meta label="Mode">Per-batch</Meta>
                <Meta label="Tabel"><span className={ui.ident}>{migration.batchTable}</span></Meta>
                <Meta label="Kolom"><span className={ui.ident}>{migration.batchColumn}</span></Meta>
              </>
            )}
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
            {migration.lastExecution && (
              <Meta label="Eksekusi terakhir">
                <span className="flex flex-wrap items-center justify-end gap-2">
                  <Link className={ui.link} to={`/executions/${migration.lastExecution.id}`}>E{migration.lastExecution.id}</Link>
                  <ExecutionStatusBadge status={migration.lastExecution.status} kind={migration.lastExecution.kind} />
                </span>
              </Meta>
            )}
          </dl>
        </aside>
      </div>
    </>
  )
}
