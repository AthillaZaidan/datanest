// Local demo accounts created by `go run ./cmd/seed`; must match backend/internal/demo.
// Only imported behind import.meta.env.DEV, so production builds drop it.
export const demoAccounts = [
  { username: 'demo-admin', password: 'demoadmin123', role: 'Platform Admin', access: 'Kelola user, group, koneksi; tanpa akses database' },
  { username: 'developer', password: 'developer123', role: 'Edit toko, View kampus', access: 'Menulis dan menjalankan migration' },
  { username: 'reviewer', password: 'reviewer123', role: 'View toko dan kampus', access: 'Membaca schema dan data' },
]
