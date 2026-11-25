import { type Migration, type MigrationState, migrationState } from '../../api/migrations'
import { Pill } from '../kit'

const styles: Record<MigrationState, { label: string; tone: 'neutral' | 'warning' | 'positive' | 'negative' }> = {
  draft: { label: 'Draft', tone: 'neutral' },
  running: { label: 'Berjalan', tone: 'warning' },
  active: { label: 'Aktif', tone: 'positive' },
  inactive: { label: 'Tidak aktif', tone: 'neutral' },
  failed: { label: 'Gagal', tone: 'negative' },
}

export default function StatusBadge({ migration }: { migration: Migration }) {
  const { label, tone } = styles[migrationState(migration)]
  return <Pill tone={tone}>{label}</Pill>
}
