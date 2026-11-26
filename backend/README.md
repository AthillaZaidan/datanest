# backend

Backend DataNest dalam Go 1.25: JSON API dan penyaji SPA (`cmd/server`), worker eksekusi migration (`cmd/worker`), dan pengisi
data demo (`cmd/seed`). Semua perintah di bawah dijalankan dari folder `backend/`. Cara menyiapkan `.env`, Docker, dan akun demo
ada di [README utama](../README.md#menjalankan-dari-awal).

## Struktur

```
backend/
├── cmd/
│   ├── server/   API + SPA, metrics di :9100
│   ├── worker/   konsumen antrean RabbitMQ, sweeper, metrics di :9101
│   └── seed/     akun, koneksi, group, dan grant demo
└── internal/     paket aplikasi (tidak bisa di-import dari luar modul)
```

Dependensi utama: Gin (HTTP), GORM + driver Postgres (database platform), pgx v5 (database target), amqp091-go (RabbitMQ),
OpenTelemetry (trace dan log OTLP), dan `prometheus/client_golang` (metrik).

## Binary

| Perintah | Fungsi | Membutuhkan | Port |
| --- | --- | --- | --- |
| `go run ./cmd/server` | Menjalankan migrasi schema platform, membuat Platform Admin pertama bila belum ada, lalu melayani `/api/*` dan file `frontend/dist`. | `PLATFORM_DSN`, `CREDENTIALS_KEY`; `AMQP_URL` agar persetujuan langsung diterbitkan ke antrean | `APP_PORT` (8080), `METRICS_ADDR` (9100) |
| `go run ./cmd/worker` | Mengambil job dari RabbitMQ, mengunci database target, menjalankan SQL migration (normal atau per batch), mencatat hasil, dan menjalankan sweeper (terbit ulang, pemulihan, promosi pengajuan yang sudah disetujui). | `PLATFORM_DSN`, `CREDENTIALS_KEY`, `AMQP_URL` (wajib) | `WORKER_METRICS_ADDR` (9101; worker kedua `:9102`) |
| `go run ./cmd/seed` | Mengisi akun `demo-admin`, `developer`, `reviewer`, koneksi `toko` dan `kampus`, dua group, dan grant. Aman dijalankan berulang. | `PLATFORM_DSN`, `CREDENTIALS_KEY` | - |

Server dan worker berhenti dengan rapi saat menerima `SIGINT` atau `SIGTERM`: request dan job yang sedang berjalan diselesaikan,
lalu log dan span yang tersisa dikirim dengan batas waktu 5 detik. Semua variabel environment dijelaskan di
[Isi file .env](../README.md#isi-file-env).

## Paket `internal/`

| Paket | Tanggung jawab | File utama |
| --- | --- | --- |
| `admin` | Kelola user, role, group, anggota, leader group, dan grant database (khusus Platform Admin). Perubahan grant dan leader dicatat sebagai event log. | `service.go`, `repository.go` |
| `apperr` | Error yang aman ditampilkan ke klien beserta kode standarnya (`NOT_FOUND`, `FORBIDDEN`, `VALIDATION_ERROR`, ...). | `apperr.go` |
| `approval` | Tahap review eksekusi: daftar penyetuju dihitung live dari leader group yang punya akses, approve, reject, cancel, withdraw, dan perpindahan ke `queued`. | `service.go`, `query.go`, `describe.go` |
| `auth` | Login dengan password (bcrypt), cookie session, pemeriksaan CSRF, dan middleware `RequireAuth`/`RequireAdmin`. Gangguan database dijawab 500, bukan 401. | `auth.go` |
| `authz` | Memutuskan level akses efektif user ke database (`None`, `View`, `Edit`; level terkuat dari semua group). Paket lain hanya memanggil `Can`. | `authz.go` |
| `config` | Membaca dan memvalidasi konfigurasi dari environment. | `config.go` |
| `dbconn` | Koneksi database target: CRUD koneksi, enkripsi password (AES-GCM), pool per target, uji koneksi, dan pembukaan database platform dengan pool terbatas (`PLATFORM_MAX_OPEN_CONNS`). | `service.go`, `pools.go`, `crypto.go`, `platform.go` |
| `dblock` | Lock per database target berupa sewa (lease) di tabel `database_locks`: acquire atomik, renew lewat heartbeat, release. | `dblock.go` |
| `demo` | Data demo yang dipakai `cmd/seed`. Jangan dijalankan ke database platform sungguhan. | `demo.go` |
| `execution` | Penyimpanan eksekusi dengan transisi status yang dijaga pemilik (`Claim`, `Finish`, `Requeue`, `MarkUncertain`, ...) dan tampilan `GET /api/executions/:id`. | `store.go`, `service.go` |
| `explorer` | Membaca schema, table, kolom, constraint, index, dan isi table dari database target. Setiap query berjalan dalam transaksi `READ ONLY`. | `service.go`, `catalog.go`, `rows.go`, `querylog.go` |
| `httpapi` | Routing Gin, handler tipis (parse input, panggil service, tulis JSON), format error, middleware log/trace/metrik HTTP, dan penyaji SPA. | `router.go`, `middleware.go`, `helpers.go`, `spa.go` |
| `metrics` | Registry Prometheus: metrik job, lock, migration, HTTP, pool koneksi, dan jumlah eksekusi per status; handler `/metrics`. | `metrics.go`, `pool.go`, `executions.go`, `http.go` |
| `migration` | Resep migration (SQL up/down atau irreversible), pengajuan run/forward/rollback, revalidasi sebelum dijalankan, riwayat, ledger `_platform_executions` di database target, dan `EXPLAIN`. Subpaket `sqlguard` menolak perintah transaction control. | `service.go`, `submit.go`, `revalidate.go`, `history.go`, `ledger.go`, `explain.go` |
| `models` | Model GORM database platform dan migrasi schema-nya. | `models.go`, `execution.go`, `approval.go`, `migrate.go` |
| `obs` | Nama event dan atribut log, masking data sensitif, setup `slog` JSON, OpenTelemetry (console atau OTLP), dan propagasi `traceparent`. | `setup.go`, `log.go`, `mask.go`, `trace.go`, `fanout.go` |
| `position` | Menurunkan posisi migration aktif sebuah database dari riwayat eksekusinya (aturan rollback terakhir dan forward linear). | `state.go`, `decide.go`, `load.go` |
| `queue` | Publisher (dengan publisher confirm) dan consumer (ack manual, prefetch, reconnect) RabbitMQ untuk job eksekusi. | `publisher.go`, `consumer.go`, `message.go` |
| `reqctx` | Menyimpan nilai per request, seperti user yang login, di konteks Gin. | `reqctx.go` |
| `session` | Session HTTP di database platform: hanya hash identifier yang disimpan, idle dan absolute timeout, rotasi dengan grace period. | `store.go` |
| `targeterr` | Memetakan error database target (SQLSTATE, timeout) ke error API yang aman. | `targeterr.go` |
| `testutil` | Helper integration test: database platform dan target kosong di `test-pg`, factory user/group/koneksi/eksekusi. | `testutil.go`, `provider.go` |
| `worker` | Inti eksekusi: klaim di bawah lock, heartbeat, mode normal dan per batch, undo batch, ledger sebagai bukti commit, retry, pemulihan hasil tidak pasti, sweeper, dan metrik job. | `claim.go`, `run.go`, `apply.go`, `batch.go`, `fence.go`, `sweep.go`, `serve.go` |

Penjelasan desain lengkap ada di [Keputusan desain](../README.md#keputusan-desain), terutama
[9. Migration](../README.md#9-migration-rollback-forward-dan-riwayat),
[15. Eksekusi asynchronous](../README.md#15-eksekusi-asynchronous-milestone-2), dan
[16. Alur review](../README.md#16-leader-per-group-dan-alur-review-milestone-2).

## Alur satu migration

```
SPA ──POST /api/migrations/:id/executions──▶ httpapi ─▶ migration.Submit         (status pending_review)
SPA ──POST /api/executions/:id/approve────▶ httpapi ─▶ approval                  (semua leader setuju → queued)
                                                        └─▶ queue.Publisher ──▶ RabbitMQ
cmd/worker: queue.Consumer ─▶ worker.Claim (dblock + execution.Claim) ─▶ migration.Revalidate
          ─▶ worker.Run: SQL + ledger dalam satu transaksi di database target ─▶ success / failed / partial / uncertain
SPA ──GET /api/executions/:id (polling)───▶ execution.Service
```

Database platform adalah sumber kebenaran; RabbitMQ hanya pemberi tahu. Pesan yang hilang diterbitkan ulang oleh sweeper, dan
pesan ganda tidak menjalankan SQL dua kali karena klaim dan ledger.

## Test

```bash
go vet ./...
go test -race ./...
set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...
```

Integration test (`//go:build integration`) memakai container `test-pg` dan RabbitMQ dari `docker compose`; `-p 1` wajib karena
semua paket berbagi database test yang sama. Test yang butuh RabbitMQ dilewati bila `TEST_AMQP_URL` kosong. Data uji besar
dijalankan dengan tag `bigdata` (lihat [Test](../README.md#test)). Strategi pengujian:
[13. Strategi pengujian](../README.md#13-strategi-pengujian).
