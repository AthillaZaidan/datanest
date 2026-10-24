export const formatTime = (iso: string | null) => (iso ? new Date(iso).toLocaleString('id-ID') : '—')
