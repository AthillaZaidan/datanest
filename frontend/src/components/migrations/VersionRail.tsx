import { Link } from 'react-router'
import type { Migration } from '../../api/migrations'

const MAX_STEPS = 16

const dot = (m: Migration, current: number) => {
  if (m.status === 'failed') return 'size-2.5 border-2 border-negative bg-sheet'
  if (m.status === 'running') return 'size-2.5 border-2 border-warning bg-sheet'
  if (m.version === current) return 'size-3 bg-accent'
  return 'size-2.5 bg-[#8d9398]'
}

const describe = (m: Migration, current: number) =>
  m.status === 'failed' ? 'gagal' : m.status === 'running' ? 'berjalan' : m.version === current ? 'versi saat ini' : 'sukses'

// The schema's history as one line of executed versions; the current version is the one mark in the data hue.
export default function VersionRail({ migrations, currentVersion }: { migrations: Migration[]; currentVersion: number }) {
  const executed = migrations.filter((m) => m.version !== null).sort((a, b) => (a.version ?? 0) - (b.version ?? 0))
  const shown = executed.slice(-MAX_STEPS)
  const hidden = executed.length - shown.length
  const drafts = migrations.filter((m) => m.status === 'draft').length

  if (executed.length === 0) {
    return <p className="text-sm text-muted">Belum ada migration yang dijalankan{drafts > 0 && `, ${drafts} draft menunggu`}.</p>
  }

  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      {hidden > 0 && <span className="text-xs text-muted">+{hidden} sebelumnya</span>}
      <ol className="flex items-start overflow-x-auto" aria-label="Versi schema">
        {shown.map((m, i) => (
          <li key={m.id} className="relative flex w-11 flex-none flex-col items-center">
            {shown.length > 1 && (
              <span
                aria-hidden="true"
                className={`absolute top-[6px] h-px bg-control ${i === 0 ? 'right-0 left-1/2' : i === shown.length - 1 ? 'right-1/2 left-0' : 'inset-x-0'}`}
              />
            )}
            <Link
              to={`/migrations/${m.id}`}
              className="relative z-10 grid size-3.5 place-items-center rounded-full focus-visible:outline-2 focus-visible:outline-accent"
              title={`v${m.version} ${describe(m, currentVersion)}`}
              aria-label={`v${m.version}, ${describe(m, currentVersion)}`}
            >
              <span className={`block rounded-full ${dot(m, currentVersion)}`} />
            </Link>
            <span className={`mt-1.5 font-mono text-xs ${m.version === currentVersion ? 'font-medium text-ink' : 'text-muted'}`}>v{m.version}</span>
          </li>
        ))}
      </ol>
      {drafts > 0 && <span className="text-xs text-muted">{drafts} draft belum dijalankan</span>}
    </div>
  )
}
