import type { ExecutionStatus } from '../../api/executions'
import { Pill } from '../kit'

const styles: Record<ExecutionStatus, { label: string; tone: 'neutral' | 'warning' | 'positive' | 'negative' }> = {
  pending_review: { label: 'Menunggu review', tone: 'neutral' },
  approved: { label: 'Disetujui', tone: 'neutral' },
  queued: { label: 'Dalam antrean', tone: 'neutral' },
  running: { label: 'Berjalan', tone: 'warning' },
  success: { label: 'Sukses', tone: 'positive' },
  failed: { label: 'Gagal', tone: 'negative' },
  partial: { label: 'Sebagian', tone: 'warning' },
  uncertain: { label: 'Belum pasti', tone: 'warning' },
  rejected: { label: 'Ditolak', tone: 'negative' },
  canceled: { label: 'Dibatalkan', tone: 'neutral' },
}

export default function ExecutionStatusBadge({ status }: { status: ExecutionStatus }) {
  const { label, tone } = styles[status]
  return <Pill tone={tone}>{label}</Pill>
}
