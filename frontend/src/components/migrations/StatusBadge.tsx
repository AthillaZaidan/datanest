import type { MigrationStatus } from '../../api/migrations'

const styles: Record<MigrationStatus, { label: string; className: string }> = {
  draft: { label: 'Draft', className: 'bg-neutral-500' },
  running: { label: 'Berjalan', className: 'bg-orange-600' },
  success: { label: 'Sukses', className: 'bg-green-700' },
  failed: { label: 'Gagal', className: 'bg-red-700' },
}

export default function StatusBadge({ status }: { status: MigrationStatus }) {
  const { label, className } = styles[status]
  return <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium text-white ${className}`}>{label}</span>
}
