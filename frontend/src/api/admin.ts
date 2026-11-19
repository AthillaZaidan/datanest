import { api, send } from './client'
export type User = { id: number; username: string; isAdmin: boolean }
export type Group = { id: number; name: string; description: string }
export type Grant = { groupId: number; databaseId: number; level: 'VIEW' | 'EDIT' }
export type Database = { id: number; name: string; host: string; port: number; dbName: string; username: string; sslMode: string; hasPassword: boolean; myLevel?: 'VIEW' | 'EDIT'; currentVersion: number }
export type ConnectionInput = { name: string; host: string; port: number; dbName: string; username: string; password: string; sslMode: string }
export type ConnectionTest = { ok: boolean; isSuperuser: boolean; warning?: string }
export const read = <T,>(path: string) => api<T>(path)
export const write = send
export function message(e: unknown): string { return typeof e === 'object' && e !== null && 'message' in e ? String(e.message) : 'Request gagal. Coba lagi.' }
