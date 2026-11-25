import { type Migration, type MigrationState, migrationState } from '../../api/migrations'
import { Pill } from '../kit'

const styles: Record<MigrationState, { label: string; tone: 'neutral' | 'warning' | 'positive' | 'negative' }> = {
  partial: { label: 'Sebagian', tone: 'warning' },
  running: { label: 'Berjalan', tone: 'warning' },
  active: { label: 'Aktif', tone: 'positive' },
  review: { label: 'Menunggu review', tone: 'neutral' },
  draft: { label: 'Draft', tone: 'neutral' },
  failed: { label: 'Gagal', tone: 'negative' },
  inactive: { label: 'Tidak aktif', tone: 'neutral' },
}

export default function StatusBadge({ migration }: { migration: Migration }) {
  const { label, tone } = styles[migrationState(migration)]
  return <Pill tone={tone}>{label}</Pill>
}
