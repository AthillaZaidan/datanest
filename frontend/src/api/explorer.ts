import { getJSON } from './http'

export type Schema = { name: string }
export type Table = { name: string }

export type Column = {
  name: string
  dataType: string
  nullable: boolean
  default: string | null
}

export type Constraint = {
  name: string
  type: string
  columns: string[]
  definition: string
  references?: { schema: string; table: string; columns: string[] }
}

export type Index = {
  name: string
  definition: string
  unique: boolean
  primary: boolean
}

export type TableDetail = {
  schema: string
  name: string
  columns: Column[]
  constraints: Constraint[]
  indexes: Index[]
}

export type Rows = {
  columns: string[]
  rows: unknown[][]
  page: number
  pageSize: number
  totalRows: number
}

export const PAGE_SIZES = [10, 25, 50, 100]

const enc = encodeURIComponent
const base = (databaseId: string) => `/api/databases/${enc(databaseId)}`

export async function listSchemas(databaseId: string) {
  return (await getJSON<{ schemas: Schema[] }>(`${base(databaseId)}/schemas`)).schemas
}

export async function listTables(databaseId: string, schema: string) {
  return (await getJSON<{ tables: Table[] }>(`${base(databaseId)}/schemas/${enc(schema)}/tables`)).tables
}

export function getTable(databaseId: string, schema: string, table: string) {
  return getJSON<TableDetail>(`${base(databaseId)}/schemas/${enc(schema)}/tables/${enc(table)}`)
}

export function getRows(databaseId: string, schema: string, table: string, page: number, pageSize: number) {
  return getJSON<Rows>(
    `${base(databaseId)}/schemas/${enc(schema)}/tables/${enc(table)}/rows?page=${page}&pageSize=${pageSize}`,
  )
}
