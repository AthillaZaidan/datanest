# datanest

Platform manajemen database PostgreSQL: kontrol akses lewat group, database explorer, dan migration schema berversi.

## Isi

- [Kebutuhan](#kebutuhan)
- [Menjalankan dari awal](#menjalankan-dari-awal)
- [Akun demo](#akun-demo)
- [Isi file .env](#isi-file-env)
- [Menjalankan lagi di hari berikutnya](#menjalankan-lagi-di-hari-berikutnya)
- [Test](#test)
- [Reset dan troubleshooting](#reset-dan-troubleshooting)
- [Konvensi](#konvensi)

## Kebutuhan

- Go 1.25+
- Bun 1.2+
- Docker (Docker Desktop di macOS/Windows), dengan port 15432, 15433, 15434, 8080, dan 5173 kosong

Ada tiga database PostgreSQL di `docker-compose.yml`:

| Service | Port | Isi |
| --- | --- | --- |
| `platform-pg` | 15432 | Database milik aplikasi: user, group, grant, koneksi, riwayat migration |
| `target-pg` | 15433 | Database yang dikelola: `toko` dan `kampus` dengan data contoh, role `platform_app` |
| `test-pg` | 15434 | Khusus integration test, datanya hilang setiap container berhenti |

## Menjalankan dari awal

Semua perintah dijalankan dari root repo kecuali disebut lain.

**1. Buat file `.env`.**

```bash
cp .env.example .env
```

Isi `CREDENTIALS_KEY` dengan kunci acak (wajib, server menolak start tanpanya):

```bash
openssl rand -base64 32
```

Salin hasilnya ke baris `CREDENTIALS_KEY=` di `.env`. Nilai lain sudah cocok untuk lokal; penjelasan setiap variabel ada di [Isi file .env](#isi-file-env).

**2. Nyalakan database.**

```bash
docker compose up -d --wait
```

Saat pertama kali, `target-pg` menjalankan `docker/target-init.sql` dan membuat database `toko` dan `kampus` beserta table dan datanya.

**3. Isi akun demo.**

```bash
cd backend && go run ./cmd/seed
```

Seed membuat tiga akun (lihat [Akun demo](#akun-demo)), koneksi ke `toko` dan `kampus`, dua group, dan grant-nya. Boleh dijalankan berulang: akun yang sudah ada tidak digandakan, password demo dan level grant dikembalikan ke nilai awal.

Seed bersifat opsional. Tanpa seed, isi `ADMIN_PASSWORD` di `.env`; server membuat satu Platform Admin `ADMIN_USERNAME` dari situ saat pertama start, lalu semua user, group, koneksi, dan grant dibuat lewat halaman admin.

**4. Jalankan backend** (terminal pertama).

```bash
cd backend && go run ./cmd/server
```

Backend berjalan di http://localhost:8080 dan membuat atau memperbarui table platform secara otomatis. Cek dengan `curl localhost:8080/api/healthz`.

**5. Jalankan frontend** (terminal kedua).

```bash
cd frontend && bun install && bun run dev
```

**6. Buka http://localhost:5173.** Selalu pakai alamat ini persis: request dari origin lain (misalnya `127.0.0.1:5173`) ditolak oleh pemeriksaan `Origin`, sehingga login, simpan, dan logout gagal dengan 403. Di halaman login ada daftar **Akun demo**; klik satu untuk mengisi form, lalu **Masuk**.

## Akun demo

Dibuat oleh `go run ./cmd/seed`:

| Username | Password | Akses |
| --- | --- | --- |
| `demo-admin` | `demoadmin123` | Platform Admin. Tidak masuk group mana pun, jadi tidak bisa membuka database: akses database selalu lewat grant group. |
| `developer` | `developer123` | Group Tim Developer: EDIT `toko`, VIEW `kampus`. Bisa membuat dan menjalankan migration di `toko`. |
| `reviewer` | `reviewer123` | Group Tim Reviewer: VIEW `toko` dan `kampus`. Hanya membaca schema, data, dan riwayat. |

Daftar akun di halaman login hanya tampil saat `bun run dev` (`frontend/src/dev/demoAccounts.ts`, di balik `import.meta.env.DEV`), sehingga tidak ikut ke build production. Akun yang sama didefinisikan di `backend/internal/demo/demo.go`; ubah keduanya bersamaan. Jangan jalankan seed ke database platform sungguhan.

Alur demo singkat:

1. Masuk sebagai `reviewer`: buka `toko`, lihat struktur dan data table. Tombol **Migration baru** tidak muncul.
2. Masuk sebagai `developer`: buka `toko`, **Migration baru**, tulis SQL (misalnya `ALTER TABLE products ADD COLUMN sku text;`), **Simpan draft**, lalu **Jalankan**. Versi schema naik dan kolom baru terlihat di tab Struktur. SQL yang salah tercatat sebagai Gagal dan di-rollback.
3. Masuk sebagai `demo-admin`: buka **Groups** untuk mengatur anggota dan grant, **Koneksi database** untuk menambah atau menguji koneksi. Membuka `toko` ditolak sampai `demo-admin` dimasukkan ke group yang punya grant.

## Isi file .env

Backend membaca `backend/.env` lalu `.env` di root repo; variabel environment yang sudah diset menang atas keduanya. `.env` tidak boleh di-commit.

| Variabel | Default lokal | Keterangan |
| --- | --- | --- |
| `APP_PORT` | `8080` | Port backend. Jika diubah, jalankan frontend dengan `VITE_API_TARGET=http://localhost:<port> bun run dev`. |
| `ALLOWED_ORIGIN` | `http://localhost:5173` | Satu-satunya origin yang boleh mengirim request pengubah data. Harus sama persis dengan alamat yang dibuka di browser. |
| `COOKIE_SECURE` | `false` | `true` hanya bila disajikan lewat HTTPS. |
| `PLATFORM_DSN` | `postgres://datanest:datanest@localhost:15432/datanest_platform?sslmode=disable` | Database platform (`platform-pg`). Wajib. |
| `ADMIN_USERNAME` | `admin` | Username Platform Admin pertama, dibuat hanya bila belum ada admin sama sekali. |
| `ADMIN_PASSWORD` | kosong | Wajib diisi bila belum ada admin (tidak perlu jika seed sudah dijalankan). Admin yang sudah ada tidak pernah di-reset dari nilai ini. |
| `CREDENTIALS_KEY` | kosong | Kunci AES-256 (32 byte, base64) untuk mengenkripsi password koneksi database target. Wajib. Jika diganti, password koneksi yang tersimpan tidak bisa dibuka lagi: isi ulang di halaman Koneksi database, atau jalankan seed lagi untuk koneksi demo. |
| `SESSION_IDLE_TIMEOUT` | `30m` | Session berakhir setelah tidak aktif selama ini. |
| `SESSION_ABSOLUTE_TIMEOUT` | `8h` | Batas umur session, apa pun aktivitasnya. |
| `SESSION_ROTATION_INTERVAL` | `15m` | Identifier session diganti secara berkala. |
| `SESSION_GRACE_PERIOD` | `30s` | Identifier lama masih diterima sesaat setelah rotasi. Urutannya harus grace < rotation < idle < absolute. |
| `SESSION_CLEANUP_INTERVAL` | `1m` | Seberapa sering session kedaluwarsa dibersihkan dari memori. |
| `TARGET_QUERY_TIMEOUT` | `5s` | Batas waktu koneksi dan query explorer ke database target. |
| `MIGRATION_LOCK_TIMEOUT` | `5s` | `lock_timeout` saat migration berjalan. |
| `MIGRATION_STATEMENT_TIMEOUT` | `60s` | `statement_timeout` per statement migration. |
| `MIGRATION_EXEC_TIMEOUT` | `2m` | Batas total satu eksekusi migration. |
| `TEST_PLATFORM_DSN`, `TEST_TARGET_DSN` | ke `test-pg` | Hanya untuk integration test. |

Port host database bisa diubah dengan `PLATFORM_PG_PORT`, `TARGET_PG_PORT`, dan `TEST_PG_PORT` saat `docker compose up`; sesuaikan DSN-nya. Koneksi demo dari seed selalu menunjuk `localhost:15433`, jadi bila `TARGET_PG_PORT` diubah, perbarui port koneksi di halaman Koneksi database.

## Menjalankan lagi di hari berikutnya

Data tersimpan di volume Docker, jadi cukup:

```bash
docker compose up -d --wait
```

```bash
cd backend && go run ./cmd/server
```

```bash
cd frontend && bun run dev
```

Session hanya ada di memori server: setiap restart backend semua pengguna harus login lagi.

## Test

```bash
cd backend && go test -race ./...
```

```bash
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...
```

```bash
cd frontend && bun run build && bun run lint
```

Perintah pertama menjalankan unit test. Perintah kedua juga menjalankan integration test ke `test-pg` (container harus menyala); `-p 1` wajib karena semua package memakai database test yang sama. Perintah ketiga memeriksa TypeScript, build production, dan lint frontend.

## Reset dan troubleshooting

| Gejala | Penyebab dan solusi |
| --- | --- |
| Server berhenti dengan pesan `CREDENTIALS_KEY` | Kunci kosong atau bukan 32 byte base64. Buat dengan `openssl rand -base64 32`. |
| Server berhenti: `ADMIN_USERNAME and ADMIN_PASSWORD are required` | Belum ada Platform Admin. Jalankan seed, atau isi `ADMIN_PASSWORD`. |
| Login, simpan, atau logout gagal 403 | Aplikasi dibuka dari origin selain `ALLOWED_ORIGIN`. Buka http://localhost:5173. |
| `address already in use` | Port dipakai proses lain. Cari dengan `lsof -i :8080` (atau port lain), hentikan prosesnya. |
| Koneksi database gagal saat Test | `target-pg` belum menyala, atau password koneksi dienkripsi dengan kunci lama. Nyalakan Docker; isi ulang password atau jalankan seed. |
| Database target tidak berisi table contoh | Volume dibuat sebelum `docker/target-init.sql` diperbarui. Reset volume (di bawah). |

Mengosongkan semua data (user, koneksi, riwayat migration, dan isi database target) lalu mulai ulang dari langkah 2:

```bash
docker compose down -v
```

## Konvensi

- Frontend memakai Bun (`bun add`, `bun run ...`), bukan npm.
- Styling memakai utility class Tailwind CSS v4; class bersama ada di `frontend/src/components/ui.ts` dan komponen bersama di `frontend/src/components/kit.tsx`. Hindari file CSS baru.

## Session dan keamanan request

Session disimpan hanya di memori server: restart server mengeluarkan seluruh pengguna dan deployment harus single-instance. Identifier session berupa 32 byte acak, dikirim hanya melalui cookie `sid` dengan `HttpOnly`, `SameSite=Strict`, `Path=/`, dan `Secure` saat `COOKIE_SECURE=true`. Server hanya menyimpan hash SHA-256 identifier.

- Idle timeout: 30 menit, sesuai rentang OWASP untuk aplikasi berisiko rendah.
- Absolute timeout: 8 jam, satu hari kerja dan tidak diperpanjang oleh rotasi.
- Rotasi: setiap 15 menit atau ketika hak pengguna naik.
- Grace period: 30 detik agar request yang berjalan saat rotasi tetap valid.

Setiap request pengubah state memerlukan `X-CSRF-Token` synchronizer token, JSON, serta `Origin` yang sesuai `ALLOWED_ORIGIN`. Token disimpan di memori frontend dan dikirim oleh API client. Login menggunakan bcrypt; password, token CSRF, serta session identifier tidak dicatat atau dikirim dalam response body. Platform Admin tidak otomatis mendapat akses database target—akses target tetap berasal dari grant group.

## Database explorer dan migrations

Explorer hanya tersedia untuk pengguna dengan akses **VIEW** dan memakai transaction `READ ONLY`. Schema sistem serta tabel `_platform_migrations` disembunyikan; identifier dari URL selalu divalidasi terhadap katalog. Data mendukung pagination 10, 25, 50, atau 100 baris dan diurutkan konsisten dengan primary key (atau `ctid` bila tidak tersedia).

Lifecycle migration adalah `draft → running → success | failed`. Pengguna **EDIT** dapat membuat, mengubah, menghapus, menduplikasi, dan menjalankan draft; riwayat yang sudah dieksekusi tidak dapat diubah. Versi diberi saat eksekusi dimulai sehingga urutannya linear per database. Eksekusi selalu dibungkus satu transaction dengan batas lock, statement, dan total waktu; migration gagal di-rollback serta tetap dicatat. Satu database hanya menjalankan satu migration secara bersamaan.
