# frontend

Single-Page Application DataNest: React 19, TypeScript, React Router 7, Vite 8, dan Tailwind CSS 4. Semua halaman, termasuk login
dan admin, berjalan di browser dan bicara dengan backend lewat JSON API di `/api`. Paket dikelola dengan **bun** (jangan pakai
npm atau npx).

## Menjalankan

```bash
bun install
bun run dev
```

`bun run dev` membuka Vite di http://localhost:5173 dan meneruskan `/api` ke backend (default `http://localhost:8080`, bisa diganti
dengan `VITE_API_TARGET`). Proxy itu mengganti header `Origin` menjadi alamat backend, sehingga cookie session (`SameSite=Strict`)
dan pemeriksaan CSRF tetap berjalan tanpa CORS. Backend harus sudah menyala (lihat [backend](../backend/README.md)).

| Perintah | Fungsi |
| --- | --- |
| `bun run dev` | Server pengembangan dengan hot reload. |
| `bun run build` | Type check (`tsc -b`) lalu build ke `dist/`. Backend menyajikan folder ini di http://localhost:8080. |
| `bun run lint` | `oxlint` dengan konfigurasi `.oxlintrc.json`. |
| `bun run preview` | Menyajikan hasil build secara lokal. |

`VITE_SHOW_DEMO_ACCOUNTS=true bun run build` menampilkan daftar akun demo di halaman login. Di `bun run dev` daftar itu selalu
tampil. Jangan membangun dengan flag ini di luar demo lokal.

## Struktur `src/`

| Folder | Isi |
| --- | --- |
| `api/` | Satu modul per area API (`admin`, `databases`, `explorer`, `migrations`, `executions`) di atas `client.ts`. Client mengirim cookie (`credentials: 'same-origin'`), menambahkan header `X-CSRF-Token` pada request pengubah data, dan pada respons 401 mengosongkan user di `AuthContext`, sehingga route yang dilindungi kembali ke `/login`. |
| `pages/` | Satu komponen per route (tabel di bawah). `pages/admin/` khusus Platform Admin. |
| `components/` | Komponen bersama: `Layout`, kit UI (`kit.tsx`, `ui.ts`, `icons.tsx`), explorer (`TableStructure`, `TableData`), editor migration (`RecipeEditor`, `SqlEditor`, `PlanPanel`, `VersionRail`), dan pemantau eksekusi (`ExecutionMonitor`, `ExecutionStatusBadge`). |
| `context/` | `AuthContext` (user yang login dan token CSRF) dan breadcrumb. |
| `hooks/` | `useAsync` untuk request sekali jalan, dan `useExecution` untuk polling status eksekusi. |
| `utils/` | Format tanggal, angka, dan nama pengguna. |
| `dev/` | Daftar akun demo dan data eksekusi palsu, hanya dipakai di mode pengembangan. |

## Route

| Path | Halaman | Akses |
| --- | --- | --- |
| `/login` | `Login` | Publik |
| `/databases` | `Databases`: database yang bisa diakses user beserta levelnya | Login |
| `/databases/:databaseId` | `DatabaseDetail`: explorer schema, table, struktur, dan data | VIEW |
| `/databases/:databaseId/migrations` | `Migrations`: daftar migration dan posisi aktif | VIEW |
| `/databases/:databaseId/migrations/new`, `/migrations/:migrationId` | `MigrationEditor`: SQL up/down atau irreversible, mode normal atau per batch, `EXPLAIN`, dan pengajuan run/forward/rollback | EDIT untuk mengubah |
| `/databases/:databaseId/executions` | `Executions`: riwayat eksekusi dengan filter *Hide rollback* | VIEW |
| `/executions/:executionId` | `ExecutionStatus`: status job, progres batch, attempt, penyetuju, dan aksi review | VIEW |
| `/reviews` | `Reviews`: pengajuan yang menunggu persetujuan user sebagai leader | Login |
| `/admin/users`, `/admin/groups`, `/admin/databases` | Kelola user, group (anggota, leader, grant), dan koneksi database | Platform Admin |
| `/dev/executions/:executionId` | Pratinjau `ExecutionStatus` dengan data palsu (`dummy-...`) | Hanya `bun run dev` |

Otorisasi sebenarnya selalu diperiksa backend; pembatasan di SPA hanya menyembunyikan aksi yang pasti ditolak.

## Polling status eksekusi

`useExecution` memanggil `GET /api/executions/:id` berulang dengan jeda sesuai status (`pollDelayMs` di `api/executions.ts`):
10 detik saat `pending_review`; 2 detik saat `approved`, `queued`, `running`, `partial`, dan `uncertain`. Polling berhenti di
status akhir (`success`, `failed`, `rejected`, `canceled`). Alasan memilih polling dan bentuk API-nya ada di
[15. Eksekusi asynchronous](../README.md#15-eksekusi-asynchronous-milestone-2).
