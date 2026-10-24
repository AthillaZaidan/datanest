import type { MigrationStatus } from '../../api/migrations'

const labels: Record<MigrationStatus, string> = {
  draft: 'Draft',
  running: 'Berjalan',
  success: 'Sukses',
  failed: 'Gagal',
}

export default function StatusBadge({ status }: { status: MigrationStatus }) {
  return <span className={`badge badge-${status}`}>{labels[status]}</span>
}
