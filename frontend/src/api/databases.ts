import type { Database } from './admin'
import { api } from './client'

export const getDatabase = (id: string | number) => api<Database>(`/databases/${encodeURIComponent(id)}`)
