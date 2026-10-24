import type { MigrationStatus } from '../../api/migrations'
import { Badge } from '../kit'

const styles: Record<MigrationStatus, { label: string; tone: 'neutral' | 'amber' | 'green' | 'red'; dot: string }> = {
  draft: { label: 'Draft', tone: 'neutral', dot: 'bg-zinc-400' },
  running: { label: 'Berjalan', tone: 'amber', dot: 'bg-amber-500 animate-pulse' },
  success: { label: 'Sukses', tone: 'green', dot: 'bg-emerald-500' },
  failed: { label: 'Gagal', tone: 'red', dot: 'bg-red-500' },
}

export default function StatusBadge({ status }: { status: MigrationStatus }) {
  const { label, tone, dot } = styles[status]
  return (
    <Badge tone={tone}>
      <span className={`size-1.5 rounded-full ${dot}`} />
      {label}
    </Badge>
  )
}
