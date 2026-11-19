import type { MigrationStatus } from '../../api/migrations'
import { Pill } from '../kit'

const styles: Record<MigrationStatus, { label: string; tone: 'neutral' | 'warning' | 'positive' | 'negative' }> = {
  draft: { label: 'Draft', tone: 'neutral' },
  running: { label: 'Berjalan', tone: 'warning' },
  success: { label: 'Sukses', tone: 'positive' },
  failed: { label: 'Gagal', tone: 'negative' },
}

export default function StatusBadge({ status }: { status: MigrationStatus }) {
  const { label, tone } = styles[status]
  return <Pill tone={tone}>{label}</Pill>
}
