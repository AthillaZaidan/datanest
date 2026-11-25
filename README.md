# datanest

Platform manajemen database PostgreSQL: kontrol akses lewat group, database explorer, dan migration schema berversi. Backend Go menyajikan JSON API; antarmukanya berupa Single-Page Application (React + Vite).

## Isi

- [Kebutuhan](#kebutuhan)
- [Menjalankan dari awal](#menjalankan-dari-awal)
- [Akun demo](#akun-demo)
- [Isi file .env](#isi-file-env)
- [Menjalankan lagi di hari berikutnya](#menjalankan-lagi-di-hari-berikutnya)
- [Test](#test)
- [Reset dan troubleshooting](#reset-dan-troubleshooting)
- [Konvensi](#konvensi)
- [Keputusan desain](#keputusan-desain)

## Kebutuhan

- Go 1.25+
- Bun 1.2+ (membangun dan menjalankan frontend)
- Docker (Docker Desktop di macOS/Windows), dengan port 15432, 15433, 15434, dan 8080 kosong

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

**4. Bangun frontend, lalu jalankan backend.**

```bash
cd frontend && bun install && VITE_SHOW_DEMO_ACCOUNTS=true bun run build
```

```bash
cd backend && go run ./cmd/server
```

Backend menyajikan hasil build SPA (`frontend/dist`) sekaligus JSON API di http://localhost:8080, dan membuat atau memperbarui
table platform secara otomatis. `VITE_SHOW_DEMO_ACCOUNTS=true` menampilkan daftar akun demo di halaman login; hilangkan untuk
menyembunyikannya. Bila `frontend/dist` belum ada, server tetap menyala dan halaman memberi tahu cara membangunnya.

Untuk mengembangkan frontend dengan hot reload, jalankan backend seperti di atas lalu `cd frontend && bun run dev`, dan buka
http://localhost:5173. Daftar akun demo selalu tampil di mode ini.

**5. Buka http://localhost:8080.** Pakai alamat ini persis: request dari origin lain (misalnya `127.0.0.1:8080`) ditolak
oleh pemeriksaan `Origin`, sehingga login, simpan, dan logout gagal dengan 403. Di halaman login, klik salah satu **Akun demo**
untuk mengisi form, lalu **Masuk**.

## Akun demo

Dibuat oleh `go run ./cmd/seed`:

| Username | Password | Akses |
| --- | --- | --- |
| `demo-admin` | `demoadmin123` | Platform Admin. Tidak masuk group mana pun, jadi tidak bisa membuka database: akses database selalu lewat grant group. |
| `developer` | `developer123` | Group Tim Developer: EDIT `toko`, VIEW `kampus`. Bisa membuat dan mengajukan migration di `toko`. Leader Tim Developer. |
| `reviewer` | `reviewer123` | Group Tim Reviewer: VIEW `toko` dan `kampus`. Membaca schema, data, dan riwayat, dan menyetujui pengajuan sebagai leader Tim Reviewer. |

Daftar akun di halaman login hanya tampil di `bun run dev` atau pada build dengan `VITE_SHOW_DEMO_ACCOUNTS=true` (daftarnya ada di `frontend/src/dev/demoAccounts.ts` dan harus sama dengan `backend/internal/demo/demo.go`). Jangan jalankan seed ke database platform sungguhan, dan jangan membangun frontend dengan flag itu di luar demo lokal.

Alur demo singkat:

1. Masuk sebagai `reviewer`: buka `toko`, lihat struktur dan data table. Tombol **Migration baru** tidak muncul.
2. Masuk sebagai `developer`: buka `toko`, **Migration baru**, tulis SQL (misalnya `ALTER TABLE products ADD COLUMN sku text;`), **Simpan draft**, lalu **Jalankan**. Versi schema naik dan kolom baru terlihat di tab Struktur. SQL yang salah tercatat sebagai Gagal dan di-rollback.
3. Alur review: `developer` mengajukan migration (status **Menunggu review**), lalu `developer` dan `reviewer` sama-sama membuka
   **Antrean review** dan menekan **Setujui** (keduanya leader, jadi keduanya harus setuju). Setelah yang terakhir menyetujui, status
   berubah **Disetujui**, **Dalam antrean**, lalu worker (`go run ./cmd/worker`) menjalankannya. `reviewer` juga bisa **Tolak**.
4. Masuk sebagai `demo-admin`: buka **Groups** untuk mengatur anggota dan grant, **Koneksi database** untuk menambah atau menguji koneksi. Membuka `toko` ditolak sampai `demo-admin` dimasukkan ke group yang punya grant.

## Isi file .env

Backend membaca `backend/.env` lalu `.env` di root repo; variabel environment yang sudah diset menang atas keduanya. `.env` tidak boleh di-commit.

| Variabel | Default lokal | Keterangan |
| --- | --- | --- |
| `APP_PORT` | `8080` | Port aplikasi. Jika diubah, sesuaikan `ALLOWED_ORIGIN`. |
| `ALLOWED_ORIGIN` | `http://localhost:8080` | Satu-satunya origin yang boleh mengirim request pengubah data. Harus sama persis dengan alamat yang dibuka di browser. |
| `COOKIE_SECURE` | `false` | `true` hanya bila disajikan lewat HTTPS. |
| `PLATFORM_DSN` | `postgres://datanest:datanest@localhost:15432/datanest_platform?sslmode=disable` | Database platform (`platform-pg`). Wajib. |
| `ADMIN_USERNAME` | `admin` | Username Platform Admin pertama, dibuat hanya bila belum ada admin sama sekali. |
| `ADMIN_PASSWORD` | kosong | Wajib diisi bila belum ada admin (tidak perlu jika seed sudah dijalankan). Admin yang sudah ada tidak pernah di-reset dari nilai ini. |
| `CREDENTIALS_KEY` | kosong | Kunci AES-256 (32 byte, base64) untuk mengenkripsi password koneksi database target. Wajib. Jika diganti, password koneksi yang tersimpan tidak bisa dibuka lagi: isi ulang di halaman Koneksi database, atau jalankan seed lagi untuk koneksi demo. |
| `SESSION_IDLE_TIMEOUT` | `30m` | Session berakhir setelah tidak aktif selama ini. |
| `SESSION_ABSOLUTE_TIMEOUT` | `8h` | Batas umur session, apa pun aktivitasnya. |
| `SESSION_ROTATION_INTERVAL` | `15m` | Identifier session diganti secara berkala. |
| `SESSION_GRACE_PERIOD` | `30s` | Identifier lama masih diterima sesaat setelah rotasi. Urutannya harus grace < rotation < idle < absolute. |
| `SESSION_CLEANUP_INTERVAL` | `1m` | Seberapa sering session kedaluwarsa dihapus dari database. |
| `FRONTEND_DIR` | `../frontend/dist` | Folder hasil build SPA yang disajikan backend, relatif terhadap folder tempat server dijalankan (`backend/`). |
| `TARGET_QUERY_TIMEOUT` | `5s` | Batas waktu koneksi dan query explorer ke database target. |
| `MIGRATION_LOCK_TIMEOUT` | `5s` | `lock_timeout` saat migration berjalan. |
| `MIGRATION_STATEMENT_TIMEOUT` | `60s` | `statement_timeout` per statement migration. |
| `MIGRATION_EXEC_TIMEOUT` | `2m` | Batas total satu eksekusi migration. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, atau `error`. Log berbentuk JSON di stdout. |
| `TRACE_EXPORTER` | `console` | `console` menulis span ke stderr; `none` mematikan tracing. |
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

Eksekusi migration asynchronous butuh proses worker terpisah dan RabbitMQ (ikut `docker compose up`); isi `AMQP_URL` di `.env`:

```bash
cd backend && go run ./cmd/worker
```

Setelah mengubah kode frontend, bangun ulang dengan `cd frontend && bun run build`. Session tersimpan di database platform, jadi pengguna tetap masuk setelah backend di-restart (selama belum melewati timeout).

## Test

```bash
cd backend && go test -race ./...
```

```bash
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...
```

Perintah pertama menjalankan unit test. Perintah kedua juga menjalankan integration test ke `test-pg` (container harus menyala); `-p 1` wajib karena semua package memakai database test yang sama. Test yang butuh RabbitMQ (`internal/queue` dan `internal/worker`) dilewati bila `TEST_AMQP_URL` kosong, jadi pastikan variabel itu ada di `.env` (lihat `.env.example`); tanpanya perintah tetap berakhir `ok`.

Data uji besar (satu `UPDATE` tanpa batch harus lebih dari 1 detik, lalu dataset yang sama diuji per batch) dijalankan terpisah:

```bash
cd backend && set -a && . ../.env && set +a && go test -tags=integration,bigdata -p 1 -run BigData -v ./internal/worker/
```

Jumlah row diatur `BIGDATA_ROWS` (default 2.000.000). Test gagal dengan saran menaikkannya bila `UPDATE` tanpa batch selesai dalam 1 detik atau kurang. Hasil di mesin pengembang (Postgres 17 di Docker): 2.000.000 row, satu `UPDATE` 8,6 detik; per batch 100.000 row (20 batch) 6,9 detik, dengan hasil akhir yang sama.

## Reset dan troubleshooting

| Gejala | Penyebab dan solusi |
| --- | --- |
| Server berhenti dengan pesan `CREDENTIALS_KEY` | Kunci kosong atau bukan 32 byte base64. Buat dengan `openssl rand -base64 32`. |
| Server berhenti: `ADMIN_USERNAME and ADMIN_PASSWORD are required` | Belum ada Platform Admin. Jalankan seed, atau isi `ADMIN_PASSWORD`. |
| Login, simpan, atau logout gagal 403 | Aplikasi dibuka dari origin selain `ALLOWED_ORIGIN`. Buka http://localhost:8080. |
| `address already in use` | Port dipakai proses lain. Cari dengan `lsof -i :8080` (atau port lain), hentikan prosesnya. |
| Koneksi database gagal saat Test | `target-pg` belum menyala, atau password koneksi dienkripsi dengan kunci lama. Nyalakan Docker; isi ulang password atau jalankan seed. |
| Database target tidak berisi table contoh | Volume dibuat sebelum `docker/target-init.sql` diperbarui. Reset volume (di bawah). |

Mengosongkan semua data (user, koneksi, riwayat migration, dan isi database target) lalu mulai ulang dari langkah 2:

```bash
docker compose down -v
```

## Konvensi

- Frontend ada di `frontend/src`: `api/` (satu file per area API, semua lewat `api/client.ts`), `pages/`, `components/`,
  `context/` (sesi dan breadcrumb), dan `hooks/`. Routing memakai React Router.
- Styling memakai utility class Tailwind CSS v4; class bersama didefinisikan di `frontend/src/components/ui.ts`.
- Ikon berupa SVG inline (`components/icons.tsx`), jadi tidak ada ketergantungan CDN untuk ikon.
- Tooling frontend memakai Bun, bukan npm. Lint: `cd frontend && bun run lint`.

## Keputusan desain

Bagian ini mencatat keputusan desain yang diambil tim beserta alasannya. Keputusan yang berasal langsung dari spesifikasi
ditandai **(spek)**; selebihnya adalah pilihan tim.

### 1. Teknologi

| Aspek | Pilihan | Alasan |
| --- | --- | --- |
| Backend | Go **(spek)** dengan Gin | Router ringan dengan middleware yang mudah disusun per kelompok route. |
| Database platform | PostgreSQL melalui GORM | Data platform (user, group, grant, koneksi, riwayat) berupa relasi biasa; ORM mempercepat CRUD dan migrasi schema platform. |
| Database target | PostgreSQL **(spek)** melalui `database/sql` + pgx | Introspeksi katalog dan eksekusi SQL pengguna ditulis manual; ini bukan pekerjaan ORM. |
| Tampilan | SPA React + Vite + TypeScript, React Router, Tailwind CSS | Milestone 2 **(spek)** mewajibkan frontend SPA yang responsif dan hanya berbicara dengan backend lewat API. Halaman Go template dari Milestone 1 dihapus; logika dan otorisasi tetap hanya ada di backend. |
| Origin | Satu origin | Di produksi, backend menyajikan hasil build SPA dan JSON API (`/api`) dari server yang sama, sehingga cookie dapat memakai `SameSite=Strict` tanpa CORS. Di development, dev server Vite mem-proxy `/api` ke backend dan menulis ulang header `Origin`, sehingga browser tetap hanya berbicara ke satu origin dan `.env` tidak perlu diubah. |

Database platform dan database target selalu berada di server terpisah, sehingga migration yang dijalankan pengguna tidak
dapat menyentuh table internal platform.

### 2. Arsitektur

```
Request → middleware (Origin + Content-Type → RequireAuth + CSRF → RequireAdmin)
        → handler (parse input, panggil service, petakan error)
        → service (load resource → authz.Can → logika) → repository / database target
```

| Package | Tanggung jawab |
| --- | --- |
| `config` | Membaca dan memvalidasi environment; server menolak start bila konfigurasi wajib tidak valid. |
| `auth`, `session` | Login, hashing password, seed admin, session di database platform, rotasi, CSRF. |
| `authz` | Policy Decision Point: daftar aksi, level minimum, dan rumus akses efektif. |
| `admin` | User, role, group, keanggotaan, dan grant. |
| `dbconn` | Koneksi database target: enkripsi kredensial, pool, uji koneksi. |
| `explorer` | Schema, table, column, constraint, index, dan data (read-only). |
| `migration` | Draft, eksekusi transaksional, `sqlguard`, lock per database, rekonsiliasi. |
| `httpapi` | Router JSON API, handler, dan pemetaan error ke JSON. |
| `httpapi/spa.go` | Menyajikan build SPA; GET non-`/api` yang bukan file jatuh ke `index.html` agar route sisi klien tetap hidup saat reload. |
| `apperr` | Tipe error aplikasi dan daftar kode error. |

**Pemisahan keputusan dan penegakan akses (spek).** Aturan "siapa boleh melakukan apa" hanya ada di package `authz`.
Middleware memeriksa identitas (sudah login atau belum) dan status Platform Admin. Pemeriksaan akses per database dilakukan
di service **setelah** resource dimuat, sehingga `databaseId` yang diperiksa adalah milik resource sebenarnya, bukan nilai
dari URL. Ini mencegah IDOR, misalnya mengeksekusi migration milik database lain lewat `/migrations/:id`. Halaman
menyembunyikan tombol hanya untuk kenyamanan; seluruh penegakan akses ada di backend **(spek)**. Prinsipnya *deny by
default*: aksi yang tidak dikenal dan kegagalan lookup akses selalu ditolak.

### 3. Autentikasi

- Login memakai username dan password **(spek)**; tidak ada registrasi mandiri. User dibuat oleh Platform Admin.
- Password disimpan sebagai hash **bcrypt** dengan salt bawaan **(spek)**. Password minimal 8 karakter dan maksimal 72 byte (batas input bcrypt).
- Login yang gagal selalu menjawab `401 INVALID_CREDENTIALS` dengan pesan yang sama, baik username tidak dikenal maupun
  password salah. Untuk username yang tidak dikenal, bcrypt tetap dijalankan terhadap hash tiruan agar waktu respons tidak
  membedakan kedua kasus.
- Password dan session identifier tidak pernah muncul di URL, body response, maupun log **(spek)**. Login hanya melalui
  `POST` JSON, session identifier hanya dikirim lewat `Set-Cookie`, logger request hanya mencatat method, path, status,
  durasi, dan user ID, dan logger GORM tidak mencetak nilai parameter query.
- Platform Admin pertama dibuat saat startup dari `ADMIN_USERNAME`/`ADMIN_PASSWORD` bila belum ada admin. Proses ini
  idempotent dan tidak pernah me-reset password admin yang sudah ada. Bila belum ada admin dan kredensial kosong, server
  menolak start.

### 4. Session

Session disimpan di table `sessions` pada database platform, satu baris per login, sehingga tetap berlaku setelah backend
di-restart dan tidak terikat pada satu proses. Kolomnya hanya berisi hash SHA-256 dari identifier (bukan identifier mentah),
`user_id` dengan `ON DELETE CASCADE`, token CSRF, dan waktu. Goroutine pembersih menghapus baris kedaluwarsa setiap
`SESSION_CLEANUP_INTERVAL`.

- **Identifier (spek):** 32 byte dari `crypto/rand`, di-encode base64url, opaque. Server hanya menyimpan hash-nya; hash
  tanpa salt cukup karena identifier sudah berentropi tinggi, dan salt akan membuat lookup mustahil.
- **Cookie (spek):** `sid` dengan `HttpOnly`, `SameSite=Strict`, `Path=/`, dan `Secure` saat `COOKIE_SECURE=true`
  (HTTPS/production).
- **Data session** hanya berisi identitas (user ID, waktu, status rotasi, token CSRF). Role, group, dan level akses tidak
  disimpan di session; semuanya dibaca dari database setiap request, sehingga perubahan hak berlaku seketika.

**Timeout (spek: ditentukan dan didokumentasikan tim).** Semua nilai dapat diubah lewat environment.

| Parameter | Nilai | Alasan |
| --- | --- | --- |
| Idle timeout | 30 menit | Batas atas rentang OWASP (15-30 menit) untuk aplikasi berisiko rendah. |
| Absolute expiration | 8 jam | Satu hari kerja; batas atas rentang OWASP (4-8 jam). Tidak diperpanjang oleh rotasi **(spek)**. |
| Interval rotasi | 15 menit | Lebih pendek dari idle timeout, sehingga identifier yang bocor cepat kedaluwarsa. |
| Grace period | 30 detik | Cukup untuk request yang sedang berjalan atau response yang tidak sampai saat rotasi. |

Hubungan yang divalidasi saat startup: grace < rotasi < idle < absolute. Untuk demo, nilai dapat diperpendek, misalnya
`SESSION_IDLE_TIMEOUT=2m`, `SESSION_ABSOLUTE_TIMEOUT=10m`, `SESSION_ROTATION_INTERVAL=30s`, `SESSION_GRACE_PERIOD=10s`.

**Rotasi identifier (spek).** Nama cookie tetap; nilainya diganti pada tiga kejadian:

1. **Login**: selalu membuat session baru (mencegah session fixation).
2. **Berkala**: pada request pertama setelah interval rotasi terlewati.
3. **Privilege naik**, yaitu user diberi role Platform Admin, user ditambahkan ke group, atau group user mendapat grant baru
   atau grant-nya dinaikkan dari VIEW ke EDIT. Karena kejadian ini dilakukan admin terhadap user lain, semua session user
   tersebut ditandai dan dirotasi pada request berikutnya. Hak baru sendiri berlaku seketika.

Saat rotasi, identifier lama tetap diterima selama grace period dan setiap response untuknya mengirim ulang identifier baru;
setelah grace period identifier lama ditolak **(spek)**.

- **Identifier pengganti tidak bisa disimpan sebagai hash saja**, karena response untuk cookie lama harus mengirim ulang nilai
  mentahnya. Selama grace period nilai itu disimpan terenkripsi AES-GCM (kunci `CREDENTIALS_KEY`) di baris session dan
  dikosongkan setelah grace berakhir.
- **Rotasi atomik lewat compare-and-set:** `UPDATE ... WHERE id = ? AND current_hash = <hash yang dibaca>`. Hanya satu request
  paralel yang berhasil (`RowsAffected = 1`); yang kalah membaca ulang dan mengambil jalur grace, sehingga semua menerima
  pengganti yang sama.
- **Satu baris menyimpan satu identifier pendahulu.** Rotasi yang terjadi di dalam grace period (misalnya karena perubahan hak)
  tetap dijalankan pada request berikutnya, dan identifier dari dua rotasi sebelumnya langsung tidak berlaku.
- **`last_seen` ditulis paling sering sekali per menit** (atau `SESSION_IDLE_TIMEOUT / 10` bila lebih kecil), bukan di setiap
  request, supaya polling dari SPA tidak menjadi satu tulis per request. Akibatnya session dapat kedaluwarsa paling lama
  selisih itu lebih cepat, tidak pernah lebih lambat.
- Pencabutan menghapus baris, sehingga identifier yang masih dalam grace ikut tidak berlaku.

**Pencabutan session (spek).**

| Kejadian | Tindakan |
| --- | --- |
| Logout | Session dan identifier lamanya yang masih dalam grace langsung tidak berlaku. |
| User dihapus | Semua session user dicabut. |
| Role Platform Admin dicabut | Semua session user dicabut; user harus login ulang. |
| Dikeluarkan dari group, grant diturunkan atau dicabut | Session tidak dicabut. Akses sudah hilang pada request berikutnya karena otorisasi selalu membaca data terbaru. |
| Absolute expiration tercapai | Session tidak berlaku; user harus login ulang. |

### 5. Perlindungan CSRF

- **Synchronizer token (spek: perlindungan CSRF).** Token 32 byte acak, berbeda dari session identifier, dibuat saat login
  dan disimpan di session. Setiap form di halaman membawanya sebagai field tersembunyi `csrf_token`; klien JSON API
  menerimanya dari `POST /api/auth/login` dan `GET /api/auth/me` lalu mengirimnya di header `X-CSRF-Token`. Setiap
  `POST`, `PUT`, `PATCH`, dan `DELETE` diperiksa dengan perbandingan constant-time; token tidak berubah saat rotasi agar
  request dalam grace period tetap valid.
- **Pemeriksaan `Origin`:** request pengubah state dari origin selain `ALLOWED_ORIGIN` ditolak `403 ORIGIN_NOT_ALLOWED`.
  Bersama kewajiban JSON, ini juga menangkal login CSRF, saat token belum tersedia.
- **Jenis body dibatasi:** JSON API hanya menerima `application/json` (`415` bila tidak), sehingga form lintas situs tidak dapat
  memanggilnya. Halaman menerima form `application/x-www-form-urlencoded`, yang selalu wajib membawa token CSRF.
- Semua `GET` bebas efek samping; logout, hapus, dan eksekusi migration selalu `POST`, dan setiap form mengikuti pola
  post/redirect/get.

### 6. Otorisasi dan akses efektif

- Akses database diberikan **ke group, bukan ke user (spek)**. User dapat menjadi anggota beberapa group, dan satu group
  dapat memiliki level berbeda di setiap database **(spek)**.
- Level akses: `VIEW` (melihat schema, data, serta riwayat dan detail migration) dan `EDIT` (seluruh `VIEW` ditambah
  membuat, mengubah, menghapus, menduplikasi, dan menjalankan migration) **(spek)**.
- **Rumus akses efektif (spek: ditentukan dan didokumentasikan tim):** akses efektif user terhadap sebuah database adalah
  **level tertinggi dari semua group user yang memiliki grant ke database tersebut**; tanpa grant berarti tanpa akses.
  Rumus ini dipilih karena bergabung ke group baru tidak pernah mengurangi akses, dan hasilnya mudah ditelusuri dari data
  keanggotaan. Level disimpan sebagai integer (`1 = VIEW`, `2 = EDIT`) agar `MAX()` benar; API mengeksposnya sebagai
  `"VIEW"`/`"EDIT"`.
- **Platform Admin bukan tingkatan akses database (spek).** Admin mengelola user, group, koneksi, dan grant, tetapi tidak
  otomatis dapat membuka schema, data, atau migration database mana pun. Admin yang membutuhkannya menjadi anggota group
  seperti user lain, sehingga setiap akses tercatat di data keanggotaan. Uji koneksi tidak dianggap akses database: hanya
  `SELECT 1` dan pemeriksaan superuser.
- Platform harus selalu memiliki minimal satu Platform Admin: mencabut role atau menghapus admin terakhir ditolak
  `409 LAST_ADMIN`, karena tanpa admin platform tidak dapat dikelola dan server tidak dapat membuat admin baru dari env.
- **403, bukan 404:** user yang sudah login tetapi tidak berhak mendapat `403 FORBIDDEN`, termasuk untuk database yang sama
  sekali tidak dapat ia akses. Pilihan ini membuat penolakan akses jelas saat diuji dan didemokan, dan memisahkan "tidak
  berhak" dari "tidak ada". Database yang tidak dapat diakses tetap tidak muncul di daftar `GET /api/databases`.

### 7. Koneksi database target

- Dikelola oleh Platform Admin **(spek)**: nama, host, port, nama database, username, password, dan `sslMode`.
- Password koneksi dienkripsi **AES-256-GCM** dengan `CREDENTIALS_KEY` (nonce acak per enkripsi). Password tidak dapat
  di-hash karena nilai aslinya dibutuhkan untuk terhubung. Password bersifat write-only: response hanya memuat
  `hasPassword`, dan saat edit field kosong berarti password lama dipertahankan.
- Konfigurasi koneksi dibangun dari field terstruktur, bukan connection string berisi password, dan error driver tidak
  diteruskan mentah. Kegagalan dilaporkan sebagai `502 TARGET_DB_UNAVAILABLE` atau `504 TARGET_DB_TIMEOUT` **(spek:
  penanganan kegagalan koneksi)**.
- **Role koneksi sebaiknya bukan superuser.** Semua aksi pengguna dijalankan atas nama role koneksi; superuser dapat
  menghapus database lain, membaca `pg_authid`, dan menjalankan perintah shell lewat `COPY ... TO PROGRAM`. Uji koneksi
  memeriksa `rolsuper` dan menampilkan peringatan. Role yang disarankan adalah owner database target, seperti
  `platform_app` di `docker/target-init.sql`.
- Satu pool per database target, dibuat saat pertama dibutuhkan: maksimal 5 koneksi, koneksi idle ditutup setelah 5 menit,
  dan pool ditutup saat koneksi diubah atau dihapus. Koneksi, uji koneksi, dan query explorer dibatasi
  `TARGET_QUERY_TIMEOUT`.

### 8. Database explorer

- Menampilkan schema, table, column (tipe, nullability, default), constraint (primary key, foreign key beserta rujukannya,
  unique, check), index, dan data **(spek)**, langsung dari `pg_catalog`.
- Schema sistem (`pg_catalog`, `information_schema`, `pg_toast`, schema temporer) dan table internal
  `_platform_migrations` disembunyikan.
- Setiap query berjalan di dalam transaction `READ ONLY`, sebagai pertahanan kedua setelah otorisasi: PostgreSQL menolak
  perubahan apa pun di dalamnya.
- Nama schema dan table dari URL harus ada di katalog, lalu di-quote dengan sanitasi identifier pgx. Identifier tidak dapat
  dijadikan parameter query, sehingga validasi ini menutup celah SQL injection.
- Data dipaginasi: 10, 25, 50, atau 100 baris per halaman (default 25). Backend membatasi `pageSize` ke 1-100 dan `page`
  minimal 1. Urutan stabil berdasarkan primary key; table tanpa primary key diurutkan berdasarkan `ctid`.

### 9. Migration

**Lifecycle.** `draft → running → success | failed`.

| Status | Dapat diubah | Memiliki versi | Aksi |
| --- | --- | --- | --- |
| `draft` | Ya, oleh user EDIT di database tersebut | Tidak | Ubah, hapus, jalankan |
| `running` | Tidak | Ya | Menunggu |
| `success` | Tidak | Ya | Lihat, duplikat jadi draft |
| `failed` | Tidak | Ya | Lihat, duplikat jadi draft |

- Draft memungkinkan SQL disusun dan ditinjau sebelum dijalankan **(spek: editor dan peninjauan)**. Semua user EDIT pada
  database yang sama boleh mengubah draft, bukan hanya pembuatnya. Perubahan yang bersamaan ditangani dengan optimistic
  locking: setiap simpan mengirim `revision` terakhir, dan revisi yang usang ditolak `409 REVISION_CONFLICT`.
- Migration yang sudah dijalankan, baik sukses maupun gagal, bersifat immutable. Koreksi selalu melalui migration baru,
  misalnya dengan **Duplikat jadi draft baru**.
- **Versi dan riwayat linear (spek).** Versi diberikan saat eksekusi dimulai (`MAX(version) + 1` per database), termasuk
  untuk eksekusi yang akhirnya gagal, sehingga urutan versi sama dengan urutan kejadian di database. Versi schema saat ini
  adalah versi `success` terbesar. Riwayat tidak bercabang dan tidak pernah ditulis ulang.
- Setiap migration mencatat versi, SQL, pembuat, pengubah terakhir, pelaksana, waktu mulai dan selesai, status, serta pesan
  error dan SQLSTATE bila gagal **(spek)**. Referensi user memakai `ON DELETE SET NULL` ditambah salinan username, sehingga
  riwayat tetap utuh saat user dihapus; pelaksana yang sudah dihapus ditampilkan sebagai `nama (deleted)`.
- `base_version` dicatat saat draft dibuat. Bila database sudah berubah sejak itu, editor menampilkan peringatan agar SQL
  ditinjau ulang (peringatan, bukan penolakan).

**Eksekusi.**

1. Akses `EDIT` diperiksa terhadap database milik migration.
2. `sqlguard` menolak perintah kontrol transaction (`400 TRANSACTION_CONTROL_NOT_ALLOWED`).
3. Lock per database diambil secara non-blocking; bila database sedang menjalankan migration lain, request ditolak
   `409 MIGRATION_RUNNING`. Lock dipegang sampai status final tercatat, sehingga urutan versi sama dengan urutan commit.
4. Status diubah `draft → running` dengan update bersyarat; bila migration sudah tidak berstatus draft (klik ganda atau dua
   user bersamaan), request ditolak `409 MIGRATION_NOT_DRAFT`.
5. Di database target, dalam **satu transaction (spek)**: `SET LOCAL lock_timeout` dan `statement_timeout`, SQL pengguna
   dalam satu `Exec` tanpa parameter (protokol simple query menerima banyak statement, termasuk body function), lalu satu
   baris di `_platform_migrations`. Seluruhnya di-commit bila berhasil dan di-rollback bila gagal **(spek)**.
6. Status akhir `success` atau `failed` dicatat beserta pesan error PostgreSQL.

Keputusan terkait:

- SQL tidak dipecah per `;`, karena `;` juga muncul di string dan body function.
- `sqlguard` memakai tokenizer kecil yang melewati komentar, string literal, quoted identifier, dan dollar-quoted string,
  lalu memeriksa kata pertama setiap statement terhadap `BEGIN`, `START TRANSACTION`, `COMMIT`, `END`, `ROLLBACK`, `ABORT`,
  `SAVEPOINT`, `RELEASE`, dan `PREPARE TRANSACTION`. `COMMIT` di tengah SQL pengguna akan meng-commit transaction platform
  lebih awal dan merusak jaminan rollback, sehingga harus dicegah sebelum SQL dikirim. Pemeriksaan dilakukan saat eksekusi;
  draft boleh disimpan apa adanya.
- Eksekusi memakai context yang terlepas dari request HTTP dengan batas total `MIGRATION_EXEC_TIMEOUT`: menutup browser
  tidak membatalkan migration yang sedang berjalan, dan hasilnya tetap tercatat.
- Migration yang gagal bukan error HTTP: endpoint menjawab `200` dengan status `failed`. Error HTTP hanya dipakai bila
  eksekusi tidak dimulai.
- Default timeout: `lock_timeout` 5 detik, `statement_timeout` 60 detik, total 2 menit.
- Migration di database berbeda dapat berjalan paralel. Constraint `UNIQUE(database_id, version)` di database platform
  menjadi jaring pengaman terakhir.

**Konsistensi antar database.** Database target dan database platform tidak dapat di-commit dalam satu transaction. Baris
`_platform_migrations` ditulis di transaction yang sama dengan SQL pengguna, sehingga baris itu ada jika dan hanya jika
perubahan ter-commit. Saat startup, setiap migration yang masih `running` (misalnya karena server mati di tengah eksekusi)
direkonsiliasi: bila versinya ada di `_platform_migrations` statusnya menjadi `success`, bila tidak menjadi `failed`. Bila
database target tidak dapat dihubungi, migration tetap `running`, kegagalannya dicatat di log, server tetap berjalan, dan
rekonsiliasi diulang pada startup berikutnya.

### 10. Bonus: analisis execution plan

Halaman migration menyediakan **Analisis execution plan** (`POST /api/migrations/:id/explain`) untuk SQL yang tersimpan, sebelum maupun sesudah dijalankan.

- SQL dipecah per statement dengan tokenizer `sqlguard`, sehingga `;` di dalam string, komentar, dan body dollar-quoted
  tidak memotong statement.
- Seluruh statement dijalankan berurutan dalam **satu transaction yang selalu di-rollback**. Statement yang dapat
  dianalisis (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `WITH`, `VALUES`, `TABLE`, `MERGE`) dijalankan sebagai
  `EXPLAIN (FORMAT JSON)`. Statement lain, misalnya DDL, dijalankan apa adanya agar statement berikutnya dapat dianalisis;
  contohnya `INSERT` ke table yang dibuat di migration yang sama.
- Hasil setiap statement ditampilkan sebagai pohon plan: jenis node dan scan, relation dan index yang dipakai, jenis join,
  estimasi cost (startup..total), dan estimasi jumlah baris.
- **ANALYZE** (opsional) menambahkan jumlah baris aktual, waktu aktual per node, loops, serta waktu planning dan eksekusi.
  Karena ANALYZE benar-benar menjalankan query (lalu di-rollback), opsi ini membutuhkan akses **EDIT**; `EXPLAIN` biasa
  cukup akses **VIEW**.
- Statement yang gagal menampilkan pesan error PostgreSQL beserta SQLSTATE, dan statement sesudahnya dilewati. `sqlguard`
  menolak perintah kontrol transaction sebelum analisis, dan batas `lock_timeout`, `statement_timeout`, serta
  `MIGRATION_EXEC_TIMEOUT` yang sama dengan eksekusi tetap berlaku.
- Batasan: efek di luar transaction tidak ikut di-rollback, misalnya nilai sequence yang terpakai oleh `INSERT` saat ANALYZE.

### 11. Penghapusan data

Semua penghapusan adalah hard delete dengan `ON DELETE CASCADE`, kecuali referensi user di riwayat migration.

| Dihapus | Ikut terhapus |
| --- | --- |
| User | Role, keanggotaan group, dan seluruh session-nya. Riwayat migration tetap ada dengan salinan username. |
| Group | Keanggotaan dan grant; anggotanya langsung kehilangan akses lewat group tersebut. |
| Koneksi database | Grant dan seluruh riwayat migration database itu; pool koneksinya ditutup. |

### 12. Kontrak API dan format error

Semua endpoint berada di bawah `/api` dan memakai JSON `camelCase`; waktu dalam RFC 3339. Route admin berada di
`/api/admin/*`, dan grant serta keanggotaan memakai `PUT` agar idempotent.

| Area | Endpoint |
| --- | --- |
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` |
| Admin | `/admin/users`, `/admin/users/:userId`, `/admin/users/:userId/roles/:role`, `/admin/groups`, `/admin/groups/:groupId`, `/admin/groups/:groupId/members/:userId`, `/admin/groups/:groupId/grants/:databaseId`, `/admin/databases`, `/admin/databases/:databaseId`, `/admin/databases/test`, `/admin/databases/:databaseId/test` |
| Explorer | `GET /databases`, `GET /databases/:databaseId`, `.../schemas`, `.../schemas/:schema/tables`, `.../tables/:table`, `.../tables/:table/rows?page=&pageSize=` |
| Review | `POST /migrations/:migrationId/executions` (ajukan), `GET /executions/:executionId`, `POST /executions/:executionId/approve`, `/reject`, `/cancel`, `/withdraw`, `GET /approvals/pending`, `PUT/DELETE /admin/groups/:groupId/leader` |
| Migration | `GET/POST /databases/:databaseId/migrations?status=`, `GET/PATCH/DELETE /migrations/:migrationId`, `POST /migrations/:migrationId/execute`, `POST /migrations/:migrationId/duplicate`, `POST /migrations/:migrationId/explain?analyze=` |

Setiap error memakai bentuk yang sama:

```json
{ "error": { "code": "MIGRATION_RUNNING", "message": "Sedang ada migration yang berjalan di database ini.", "details": {} } }
```

| HTTP | Kode |
| --- | --- |
| 400 | `VALIDATION_ERROR` (dengan `details` per field), `TRANSACTION_CONTROL_NOT_ALLOWED` |
| 401 | `UNAUTHENTICATED`, `INVALID_CREDENTIALS` |
| 403 | `FORBIDDEN`, `NOT_AN_APPROVER`, `CSRF_INVALID`, `ORIGIN_NOT_ALLOWED` |
| 404 | `NOT_FOUND` |
| 409 | `USERNAME_TAKEN`, `CONFLICT`, `LAST_ADMIN`, `MIGRATION_RUNNING`, `MIGRATION_NOT_DRAFT`, `REVISION_CONFLICT`, `EXECUTION_STATE_CONFLICT`, `GROUP_WITHOUT_LEADER` |
| 415 | `UNSUPPORTED_MEDIA_TYPE` |
| 500 | `INTERNAL_ERROR` (tanpa detail internal; detailnya hanya di log) |
| 502 / 504 | `TARGET_DB_UNAVAILABLE` / `TARGET_DB_TIMEOUT` |

### 13. Strategi pengujian

- **Unit test** di samping kode yang diuji, untuk logika padat aturan: `authz`, `sqlguard`, enkripsi kredensial, pagination, pemetaan error, dan konfigurasi.
- **Integration test** (build tag `integration`) terhadap PostgreSQL sungguhan di `test-pg`, sebagian besar melalui router
  Gin dengan `httptest` tanpa menyalakan server, sesuai contoh di spesifikasi. Cakupannya meliputi login, logout, session store
  (jam yang dapat diinjeksi; rotasi, grace, rotasi paralel, dan session yang tetap berlaku setelah store dibuat ulang), timeout
  dan rotasi session, CSRF dan `Origin`, seed admin, CRUD admin dan penolakan non-admin, akses efektif multi-group, IDOR,
  uji koneksi (termasuk kredensial salah dan peringatan superuser), explorer read-only, eksekusi sukses, multi-statement dan
  function, rollback, statement non-transaksional, concurrency, lock timeout, rekonsiliasi, dan analisis execution plan
  (EXPLAIN dan ANALYZE di-rollback, ANALYZE hanya untuk EDIT).
- Setiap fitur diuji untuk alur berhasil, gagal, dan penolakan akses **(spek)**. Semua test dijalankan dengan race detector.

### 14. Batasan yang disengaja

- **Satu query database per request terautentikasi:** session dibaca dari database platform. Bila database platform tidak
  dapat dihubungi, request terautentikasi ditolak `401`.
- **Backend harus berjalan sebagai satu instance:** lock migration per database masih berada di memori proses. Session sendiri
  sudah tidak terikat pada satu instance.
- **Statement non-transaksional** seperti `CREATE INDEX CONCURRENTLY`, `VACUUM`, `CREATE DATABASE`, dan `ALTER SYSTEM`
  tidak didukung, karena setiap migration dijalankan dalam transaction agar dapat di-rollback. PostgreSQL menolaknya dengan
  SQLSTATE `25001`, dan platform menampilkan penjelasan yang mudah dipahami. Body function standar SQL (`BEGIN ATOMIC`) juga
  ditolak `sqlguard`; gunakan body dollar-quoted.
- **Table `_platform_migrations`** dibuat di schema `public` setiap database target dan dikelola platform; table ini tidak
  boleh diubah manual.
- **Table tanpa primary key** diurutkan berdasarkan `ctid` saat dipaginasi. Urutannya stabil selama data tidak berubah, tetapi
  dapat bergeser setelah `UPDATE` atau `VACUUM FULL`.
- **Font** IBM Plex dimuat dari Google Fonts. Tanpa internet, aplikasi tetap berfungsi dengan font sistem.
- **Frontend harus dibangun** (`bun run build`) sebelum backend dapat menyajikannya; tanpa build, hanya JSON API yang tersedia.
- Di luar lingkup milestone ini: DBMS selain PostgreSQL, schema diff dan migration generation, re-authentication, rate
  limiting login, dan pagination di halaman admin.

### 15. Eksekusi asynchronous (Milestone 2)

Setiap eksekusi migration (termasuk rollback dan forward) adalah satu baris `migration_executions` yang dijalankan oleh proses
worker terpisah (`cmd/worker`) lewat antrean RabbitMQ. Alurnya:

1. Baris execution berstatus `queued` di database platform. **Database adalah sumber kebenaran**; RabbitMQ hanya bel pintu.
2. Sweeper (jalan di worker, tiap `WORKER_SWEEP_INTERVAL`) menerbitkan pesan berisi `execution_id`, `traceparent`, dan
   `correlation_id` untuk execution `queued` yang belum diterbitkan. Pesan yang hilang atau ganda tidak merusak data, karena
   baris di database yang menentukan.
3. Worker mengambil pesan, **mengambil lock database**, lalu **mengklaim** execution (`queued` atau `uncertain` menjadi `running`)
   dengan satu `UPDATE ... WHERE status = ...`. Klaim yang menghasilkan 0 baris berarti pesan duplikat: di-ack dan dibuang.
4. Pesan **di-ack setelah klaim berhasil**, bukan setelah job selesai: migration besar bisa lebih lama dari batas waktu ack
   RabbitMQ, dan pemulihan worker yang mati ditangani database lewat lease. Akibatnya prefetch tidak lagi membatasi jumlah job
   yang berjalan, sehingga worker membatasi sendiri lewat `WORKER_CONCURRENCY` slot yang diambil sebelum klaim.
5. Worker menjalankan SQL di database target dalam satu transaction bersama baris **ledger**, lalu menulis hasil
   (`success`, `failed`, `partial`, atau `uncertain`) dan melepas lock. Hasil dan progress dibaca SPA lewat polling
   `GET /api/executions/:id` (10 detik saat `pending_review`, 2 detik saat status lain yang belum final).

**Lock per database dan pemulihannya.** Table `database_locks` punya satu baris per database: pemegang (`owner`), execution,
`token`, dan `lease_until`. Mengambil lock adalah satu statement atomik `INSERT ... ON CONFLICT DO UPDATE ... WHERE lease_until < now()`
sehingga hanya satu worker yang menang; waktu memakai jam database. Sewa 30 detik (`WORKER_LEASE`) diperpanjang tiap 10 detik
(`WORKER_HEARTBEAT`) dengan syarat `owner` dan `token` masih cocok. Lock tetap dipegang di antara commit batch dan baru dilepas
setelah hasil ditulis; melepas hanya mengakhiri sewa, barisnya dipertahankan agar `token` hanya naik.
- **Worker mati atau macet:** heartbeat berhenti, sewa habis sendiri. Sweeper menemukan execution `running` tanpa lock yang masih
  hidup, menandainya `uncertain`, dan menerbitkan pesan pemulihan. Worker lain mengambil lock (token naik) dan memeriksa ledger.
- **Zombie:** worker yang hanya macet lalu bangun mendapat 0 baris saat memperpanjang sewa, langsung berhenti, dan tidak menulis
  apa pun. Semua penulisan hasil bersyarat pada status `running` dan pemilik klaim, jadi zombie tidak dapat menimpa hasil.
  Pagar terakhirnya ledger: bila zombie dan worker pengganti sama-sama commit batch yang sama, primary key ledger membatalkan
  satu transaction beserta perubahan datanya; yang kalah menganggap hasilnya "tidak pasti" lalu diperiksa lewat ledger.
- Satu sumber lease: sweeper hanya memeriksa `database_locks`, execution tidak punya `lease_until` sendiri.

**Ledger idempotency.** Table `_platform_executions` di setiap database target (disembunyikan dari explorer) dengan primary key
`(execution_id, batch_no)`; mode normal memakai `batch_no = 0`. Barisnya ditulis dalam transaction yang sama dengan SQL, sehingga
ada tepat bila perubahannya ter-commit. Karena itu setiap eksekusi **dimulai dengan membaca ledger**: bila bukti commit sudah ada,
hasilnya ditulis tanpa menjalankan SQL lagi.

**Commit tidak pasti.** Kegagalan saat `COMMIT`, pembatalan di tengah commit, atau bentrok primary key ledger berarti
tidak diketahui apakah data ter-commit. Execution menjadi `uncertain` dan **tidak** di-retry atau di-fail begitu saja:
pemulihan membaca ledger (ada baris berarti selesai, tidak ada berarti dijalankan sebagai percobaan berikutnya). Bila target tidak
dapat dihubungi, status tetap `uncertain` dan dicoba lagi tiap 30 detik; tidak ada tombol "paksa selesai".
Selama ada execution `running`, `partial`, atau `uncertain` di sebuah database, migration berikutnya di database itu **ditahan**
(menjadi `failed` dengan alasan `BLOCKED`).

**Retry.** Hanya kegagalan yang pasti terjadi sebelum commit yang diulang (koneksi, `lock_timeout`, deadlock, serialization):
maksimal `WORKER_MAX_ATTEMPTS` (3) percobaan, jeda 5 lalu 15 detik ditambah jitter, dipegang database lewat `not_before` dan
sweeper. Error SQL (syntax, constraint, pembagian nol) langsung `failed`. Tiap percobaan adalah satu baris `execution_attempts`
(append-only) dengan status, SQLSTATE, dan SQL yang dijalankan.

**Mode per-batch.** SQL ditulis user sebagai satu statement dengan `$1` (batas bawah, tidak termasuk) dan `$2` (batas atas,
termasuk), mis. `UPDATE orders SET status = 'x' WHERE id > $1 AND id <= $2`. Kolom batch harus bertipe `smallint`/`integer`/`bigint`,
`NOT NULL`, dan unik (primary key atau unique index tunggal); nama tabel dan kolom divalidasi lewat katalog dan di-quote. Saat mulai,
worker mengambil snapshot nilai terbesar kolom (`scope_max`) sehingga row baru sesudahnya tidak ikut. Tiap batch satu transaction:
SQL, baris ledger (rentang dan jumlah row), `COMMIT`. Progress di baris execution hanya salinan untuk UI; saat melanjutkan,
kebenaran dibaca dari ledger.
- **Gagal di tengah:** `partial` bila minimal satu batch sudah commit, `failed` bila belum.
- **Retry dari checkpoint:** percobaan berikutnya melanjutkan dari batch berikutnya di ledger, tanpa mengulang batch yang selesai.
- **Lanjutkan `partial`:** execution baru dengan `continues_execution_id`, mulai dari cursor terakhir di ledger dan memakai
  `scope_max` yang sama.
- **Undo batch:** rollback yang menunjuk execution yang dibatalkan (`reverts_execution_id`) menjalankan SQL down per batch
  dalam urutan terbalik memakai rentang dari ledger. Gagal di tengah menghasilkan `partial` ("partially reverted") yang dapat
  dilanjutkan; batch yang sudah di-undo dilewati.

**Asumsi dan aturan tambahan** (spek tidak mengatur):
- Sebuah execution `partial` berhenti menahan database setelah ada execution lanjutan atau rollback yang menyelesaikannya
  dengan status `success` atau `partial`; execution lanjutan tidak ditahan oleh `partial` yang ia lanjutkan.
- Revalidasi aturan versi dan posisi aktif (rollback hanya untuk yang teratas, forward linear) dipasang lewat `Worker.Revalidate`;
  worker sendiri hanya menegakkan aturan blokir di atas.

**Batasan:**
- Platform tidak memeriksa apakah data masih aman di-undo. SQL down yang benar adalah tanggung jawab penulis migration,
  dan migration yang tidak dapat dipulihkan harus ditandai irreversible.
- Kolom batch hanya bilangan bulat; satu statement per batch (tanpa multi-statement).
- Worker memakai batas waktu yang sama dengan eksekusi M1 (`MIGRATION_STATEMENT_TIMEOUT` 60 detik dan `MIGRATION_EXEC_TIMEOUT` 2 menit)
  untuk satu transaction. Migration mode normal yang lebih lama dari itu gagal dengan `TIMEOUT` walau dijalankan asynchronous;
  untuk data besar gunakan mode per-batch, yang batas waktunya berlaku per batch.
- Sweeper berjalan di dalam worker: bila semua worker mati, pekerjaan menunggu sampai ada worker yang hidup lagi.
- Tidak ada dead-letter queue; execution yang gagal tetap tersimpan sebagai baris `failed` yang dapat dilihat dan di-forward ulang.
- Tombol "Jalankan" M1 (`POST /api/migrations/:id/execute`) masih sinkron sampai alur pengajuan dan approval menggantikannya;
  eksekusi M1 tidak lewat antrean.

### 16. Leader per group dan alur review (Milestone 2)

Setiap pengajuan (run, forward, atau rollback) adalah satu baris `migration_executions` yang melewati
`pending_review` → `approved` → `queued` sebelum worker menjalankannya. Status milik execution, bukan milik migration, supaya
rollback dan forward juga wajib melewati review yang baru.

**Leader.** Kolom `groups.leader_id` (boleh kosong): satu leader per group, harus anggota group, hanya Platform Admin yang
mengatur (`PUT/DELETE /api/admin/groups/:groupId/leader`, halaman **Groups**). Seorang user boleh memimpin beberapa group. Bila
leader keluar dari group atau user-nya dihapus, group menjadi tanpa leader.

**Daftar penyetuju dihitung langsung** (bukan disalin saat pengajuan): `DISTINCT leader_id` dari semua group yang punya grant
(VIEW atau EDIT) ke database itu. Seseorang yang memimpin dua group dihitung sekali. Karena dihitung ulang terus, berpindahnya
leader langsung mengubah siapa yang harus setuju.

**Persetujuan atas nama orang.** Table `migration_approvals` (`execution_id`, `user_id`, salinan `username`, `created_at`,
`revoked_at`), append-only: mencabut persetujuan mengisi `revoked_at`, tidak menghapus baris. Persetujuan Sari tidak pindah ke
Joko bila leader berganti; persetujuan orang yang sudah keluar dari daftar diabaikan. Syarat lolos: setiap orang di daftar live
punya persetujuan yang belum dicabut.

**Titik kunci.** Menyetujui, menolak, dan menarik berjalan dalam transaction yang mengunci baris execution, dan setiap transisi
memakai `UPDATE ... WHERE status = <status sebelumnya>`, sehingga klik ganda atau dua leader yang menyetujui bersamaan
menghasilkan tepat satu transisi. Saat persetujuan terakhir masuk, execution menjadi `approved`; langkah berikutnya mengecek
daftar penyetuju sekali lagi dalam satu transaction. Bila ada penyetuju baru yang belum setuju, status kembali ke
`pending_review`; bila lengkap, status menjadi `queued` (disimpan `correlation_id` dan `traceparent` request itu) lalu pesan
dipublikasikan. Gagal publish tidak membatalkan apa pun: sweeper worker menerbitkan execution `queued` yang belum terbit, dan
juga menuntaskan execution yang tertinggal di `approved` lebih dari 10 detik. Setelah `queued`, daftar tidak dihitung lagi.

**Aksi dan hak.**

| Aksi | Siapa | Dari status |
| --- | --- | --- |
| `approve` | leader di daftar penyetuju (level VIEW cukup; leader VIEW tetap tidak boleh membuat atau mengedit) | `pending_review` |
| `reject` | leader di daftar penyetuju, alasan opsional | `pending_review`, `approved` |
| `cancel` | pengaju | `pending_review`, `approved` |
| `withdraw` (cabut persetujuan sendiri) | yang sudah menyetujui | `pending_review`, `approved` (kembali ke `pending_review`) |

Yang bukan leader mendapat `403 NOT_AN_APPROVER`; yang tidak punya akses ke database mendapat `403 FORBIDDEN`; aksi pada status
yang tidak sesuai mendapat `409 EXECUTION_STATE_CONFLICT`. Selama ada execution terbuka SQL resep tidak bisa diedit; migration
yang ditolak atau ditarik tetap bisa diperbaiki dan diajukan lagi. SPA menampilkan tombol sesuai `permissions` dari server
(hanya untuk UX), antrean di **Antrean review** (`GET /api/approvals/pending`, diperbarui tiap 10 detik), dan pengaturan leader
di halaman **Groups**.

**Asumsi dan risiko** (spek tidak mengatur):
- **Group tanpa leader gagal tertutup.** Selama ada group dengan grant ke sebuah database yang belum punya leader, perubahan
  di database itu tidak bisa disetujui (`409 GROUP_WITHOUT_LEADER`, dan halaman eksekusi menulis "Group X belum punya leader").
  Dipilih daripada melewati group itu, karena menghapus seorang leader akan diam-diam menghilangkan veto timnya. Seed demo
  menetapkan leader (`developer` untuk Tim Developer, `reviewer` untuk Tim Reviewer); group dari Milestone 1 lain harus diberi
  leader oleh admin.
- **Menyetujui pengajuan sendiri diperbolehkan.** Spek diam soal ini, dan melarangnya membuat database yang seluruh timnya
  dipimpin satu orang tidak bisa diubah sama sekali.
- **Platform Admin dapat mengubah leader dan grant**, sehingga secara teknis dapat mengatur siapa yang menyetujui (admin yang
  menempatkan dirinya ke group dengan grant lalu menjadi leader). Ini tidak dicegah, hanya dicatat: setiap perubahan grant
  dan leader masuk log terstruktur (`grant.changed`, `leader.changed`) dengan pelaku dan database.
- **`POST /api/migrations/:id/execute` dari Milestone 1 masih ada** dan menjalankan migration langsung tanpa review dan tanpa
  antrean. SPA tidak memakainya lagi (tombol di editor mengajukan lewat `POST /migrations/:id/executions`), tetapi pemanggil
  API langsung dengan hak EDIT masih bisa melewati approval sampai route itu dihapus.

### 17. Log terstruktur, masking, dan tracing (Milestone 2)

**Log.** Semua log berbentuk JSON satu baris di stdout (`log/slog`). Atribut tetap: `time`, `level`, `event`, `actor_id`,
`actor`, `database_id`, `status`, `correlation_id`, `trace_id`, `span_id`. Untuk SQL: `query`, `duration_ms`, `rows`, `error`,
`sqlstate`. Untuk eksekusi: `migration_id`, `execution_id`, `job_id` (sama dengan execution ID), `attempt`, `batch_no`; setiap baris worker juga memuat `actor_id` dan `actor` (pengaju). Nama event berbentuk `area.aksi`:
`http.request`, `auth.login`, `authz.denied`, `explorer.query`, `execution.approved`, `execution.rejected`,
`execution.canceled`, `queue.published`, `lock.acquired`, `lock.lost`, `sql.start`, `sql.result`, `sql.commit`, `sql.rollback`,
`recovery.check`, `grant.changed`, `leader.changed`. Mulai, hasil, dan commit atau rollback sebuah SQL dicatat sebagai baris terpisah, masing-masing dengan `status`. Setiap query explorer ke database target dicatat sebagai satu baris `explorer.query` berisi teks query (dimasking), durasi, dan status; jumlah row tidak tersedia di jalur itu, jadi tidak dicatat. Pengajuan eksekusi dicatat sebagai `execution.requested`. Nama event dan atribut ada di satu tempat, `backend/internal/obs`.

**Correlation ID** dibuat server untuk setiap request API (nilai dari klien diabaikan) dan dikirim balik di header
`X-Request-ID`. Saat sebuah execution masuk antrean, ID itu disimpan di baris execution dan ikut di pesan; worker memakai ID
yang sama, sehingga satu pengajuan dapat ditelusuri dari request sampai SQL-nya dengan `correlation_id`.

**Masking** ada di satu tempat (`ReplaceAttr` pada handler `slog`): atribut berkunci `password`, `sid`, `session`, `csrf`,
`token`, `authorization`, dan `cookie` (serta variannya) diganti `[REDACTED]`. Parameter query ikut dicatat sebagai
atribut `params` (batas batch, atau limit dan offset pada explorer), dan teks error yang memuat `password=...` atau
`user:password@` pada URL disamarkan. Pada atribut `query`, literal string
(termasuk body dollar-quoted) diganti `'?'` dan komentar dibuang, memakai tokenizer yang sama dengan `sqlguard`. SQL yang
persis tetap utuh di database (`sql_executed`), hanya log yang dimasking. **Batasan:** literal angka tidak dimasking, dan
`WHERE id = 42` tampil apa adanya. Query string URL tidak pernah dicatat. Pesan dari `log.Printf` lama (misalnya error session
store) ikut keluar sebagai JSON, tetapi sebagai teks bebas di atribut `event`, bukan atribut terstruktur.

**Tracing.** OpenTelemetry SDK dengan exporter console (span ditulis ke stderr; `TRACE_EXPORTER=none` mematikannya). Setiap
request yang mengubah data (`POST`, `PUT`, `PATCH`, `DELETE`) memulai trace dengan satu span bernama `METODE route`; request
`GET` tidak diberi span karena SPA memanggilnya berulang untuk polling dan akan membanjiri keluaran. Trace berlanjut melewati
antrean ke worker:

| Span | Induk |
| --- | --- |
| `POST /api/executions/:executionId/approve` (span request) | (akar) |
| `api.approve` | span request |
| `queue.publish` | `api.approve` |
| `worker.process` | `queue.publish` |
| `lock.acquire` | `worker.process` |
| `execute` | `worker.process` |
| `batch[n]` (mode per-batch) | `execute` |
| `sql.query` | `execute`, atau `batch[n]` |

Setiap pesan membawa `traceparent` milik span `queue.publish`, sehingga worker menyambung ke penerbitan itu. Konteks `api.approve`
juga disimpan di kolom `traceparent` pada execution; dipakai bila pesan tidak membawa konteks, dan oleh sweeper atau pemulihan
yang menerbitkan pesan baru, supaya trace tetap menyambung. Karena itu satu persetujuan bisa menghasilkan lebih dari satu span
`queue.publish` bila sweeper menerbitkan ulang sebelum penanda "sudah terbit" tertulis; pesan ganda aman karena klaim
`queued → running` hanya dimenangkan satu pesan.

Contoh nyata dari satu persetujuan (UPDATE sederhana yang dijalankan worker lewat RabbitMQ; keluaran asli dari integration test):

```json
{"time":"2026-11-25T18:13:58.2552001+07:00","level":"INFO","event":"http.request","method":"POST","path":"/api/executions/1/approve","status":200,"duration_ms":124,"actor_id":2,"trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"9d1a333ed7dd8c8f","correlation_id":"b2235169ada375e7"}
{"time":"2026-11-25T18:13:58.1688993+07:00","level":"INFO","event":"execution.approved","actor_id":2,"actor":"reviewer","migration_id":1,"execution_id":1,"job_id":1,"database_id":1,"status":"approved","trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"6ff8da75d26af419","correlation_id":"b2235169ada375e7"}
{"time":"2026-11-25T18:13:58.2451506+07:00","level":"INFO","event":"lock.acquired","execution_id":1,"job_id":1,"migration_id":1,"database_id":1,"actor_id":1,"actor":"developer","owner":"test/1","lock_token":1,"trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"22ff56b33eabb594","correlation_id":"b2235169ada375e7"}
{"time":"2026-11-25T18:13:58.2758092+07:00","level":"INFO","event":"sql.start","execution_id":1,"job_id":1,"migration_id":1,"database_id":1,"actor_id":1,"actor":"developer","attempt":1,"batch_no":0,"status":"started","query":"UPDATE items SET tag = '?' WHERE qty = 0","trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"c544933e2e334450","correlation_id":"b2235169ada375e7"}
{"time":"2026-11-25T18:13:58.2768433+07:00","level":"INFO","event":"sql.result","execution_id":1,"job_id":1,"migration_id":1,"database_id":1,"actor_id":1,"actor":"developer","attempt":1,"batch_no":0,"status":"ok","duration_ms":1,"rows":95,"trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"c544933e2e334450","correlation_id":"b2235169ada375e7"}
{"time":"2026-11-25T18:13:58.2792848+07:00","level":"INFO","event":"sql.commit","execution_id":1,"job_id":1,"migration_id":1,"database_id":1,"actor_id":1,"actor":"developer","attempt":1,"batch_no":0,"status":"committed","rows":95,"trace_id":"75f9bdfe27653dfb6d240ff0876f34c9","span_id":"6c35be4bc387e1f1","correlation_id":"b2235169ada375e7"}
```

Perhatikan literal `'rahasia'` pada SQL aslinya sudah menjadi `'?'` di log, dan `trace_id` serta `correlation_id` sama dari
request sampai commit (`correlation_id` sama dengan header `X-Request-ID` respons). Baris worker membawa `actor` (pengaju),
`migration_id`, dan `job_id`.

Keluaran exporter console untuk dua span dari trace yang sama (`75f9bdfe27653dfb6d240ff0876f34c9`); span lain tersusun seperti tabel di bawahnya:

```json
{"Name":"POST /api/executions/:executionId/approve","SpanContext":{"TraceID":"75f9bdfe27653dfb6d240ff0876f34c9","SpanID":"9d1a333ed7dd8c8f","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"00000000000000000000000000000000","SpanID":"0000000000000000","TraceFlags":"00","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2026-11-25T18:13:58.1302448+07:00","EndTime":"2026-11-25T18:13:58.2552001+07:00","Attributes":[{"Key":"method","Value":{"Type":"STRING","Value":"POST"}},{"Key":"http.status_code","Value":{"Type":"INT64","Value":200}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":1,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"unknown_service:worker.test.exe"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.47.0"}}],"InstrumentationScope":{"Name":"github.com/AthillaZaidan/datanest","Version":"","SchemaURL":"","Attributes":null},"InstrumentationLibrary":{"Name":"github.com/AthillaZaidan/datanest","Version":"","SchemaURL":"","Attributes":null}}
{"Name":"sql.query","SpanContext":{"TraceID":"75f9bdfe27653dfb6d240ff0876f34c9","SpanID":"c544933e2e334450","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"75f9bdfe27653dfb6d240ff0876f34c9","SpanID":"6c35be4bc387e1f1","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2026-11-25T18:13:58.2758092+07:00","EndTime":"2026-11-25T18:13:58.2768433+07:00","Attributes":null,"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"unknown_service:worker.test.exe"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.47.0"}}],"InstrumentationScope":{"Name":"github.com/AthillaZaidan/datanest","Version":"","SchemaURL":"","Attributes":null},"InstrumentationLibrary":{"Name":"github.com/AthillaZaidan/datanest","Version":"","SchemaURL":"","Attributes":null}}
```

| Span | span_id | induk |
| --- | --- | --- |
| `POST /api/executions/:executionId/approve` | `9d1a333ed7dd8c8f` | - |
| `api.approve` | `6ff8da75d26af419` | `POST /api/executions/:executionId/approve` |
| `queue.publish` | `3283229004820247` | `api.approve` |
| `worker.process` | `22ff56b33eabb594` | `queue.publish` |
| `lock.acquire` | `c7642825056b679c` | `worker.process` |
| `execute` | `6c35be4bc387e1f1` | `worker.process` |
| `sql.query` | `c544933e2e334450` | `execute` |

Span yang gagal (`sql.query`, `execute`, dan `worker.process`) diberi status `Error` beserta pesan errornya, dan span request
berstatus `Error` untuk respons 5xx.
