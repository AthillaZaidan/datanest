export const formatTime = (iso: string | null) =>
  iso ? new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' }) : '-'

// Relative age for recency, falling back to the date after a week.
export function ago(iso: string | null, now = Date.now()) {
  if (!iso) return '-'
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return 'baru saja'
  if (s < 3600) return `${Math.floor(s / 60)} menit lalu`
  if (s < 86400) return `${Math.floor(s / 3600)} jam lalu`
  if (s < 7 * 86400) return `${Math.floor(s / 86400)} hari lalu`
  return new Date(iso).toLocaleDateString('id-ID', { dateStyle: 'medium' })
}

export function duration(start: string | null, end: string | null) {
  if (!start || !end) return '-'
  const ms = new Date(end).getTime() - new Date(start).getTime()
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}
