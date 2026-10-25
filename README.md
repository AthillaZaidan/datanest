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
- [Keputusan desain](#keputusan-desain)

## Kebutuhan

- Go 1.25+
- Bun 1.2+ (hanya untuk membangun ulang CSS setelah mengubah template)
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

**4. Jalankan aplikasi.**

```bash
cd backend && SHOW_DEMO_ACCOUNTS=true go run ./cmd/server
```

Backend menyajikan halaman HTML sekaligus JSON API di http://localhost:8080, dan membuat atau memperbarui table platform secara
otomatis. `SHOW_DEMO_ACCOUNTS=true` menampilkan daftar akun demo di halaman login; hilangkan untuk menyembunyikannya.

**5. Buka http://localhost:8080.** Selalu pakai alamat ini persis: request dari origin lain (misalnya `127.0.0.1:8080`) ditolak
oleh pemeriksaan `Origin`, sehingga login, simpan, dan logout gagal dengan 403. Di halaman login, klik salah satu **Akun demo**
untuk mengisi form, lalu **Masuk**.

## Akun demo

Dibuat oleh `go run ./cmd/seed`:

| Username | Password | Akses |
| --- | --- | --- |
| `demo-admin` | `demoadmin123` | Platform Admin. Tidak masuk group mana pun, jadi tidak bisa membuka database: akses database selalu lewat grant group. |
| `developer` | `developer123` | Group Tim Developer: EDIT `toko`, VIEW `kampus`. Bisa membuat dan menjalankan migration di `toko`. |
| `reviewer` | `reviewer123` | Group Tim Reviewer: VIEW `toko` dan `kampus`. Hanya membaca schema, data, dan riwayat. |

Daftar akun di halaman login hanya tampil bila server dijalankan dengan `SHOW_DEMO_ACCOUNTS=true`. Akun didefinisikan di `backend/internal/demo/demo.go`. Jangan jalankan seed ke database platform sungguhan, dan jangan aktifkan `SHOW_DEMO_ACCOUNTS` di luar demo lokal.

Alur demo singkat:

1. Masuk sebagai `reviewer`: buka `toko`, lihat struktur dan data table. Tombol **Migration baru** tidak muncul.
2. Masuk sebagai `developer`: buka `toko`, **Migration baru**, tulis SQL (misalnya `ALTER TABLE products ADD COLUMN sku text;`), **Simpan draft**, lalu **Jalankan**. Versi schema naik dan kolom baru terlihat di tab Struktur. SQL yang salah tercatat sebagai Gagal dan di-rollback.
3. Masuk sebagai `demo-admin`: buka **Groups** untuk mengatur anggota dan grant, **Koneksi database** untuk menambah atau menguji koneksi. Membuka `toko` ditolak sampai `demo-admin` dimasukkan ke group yang punya grant.

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
| `SESSION_CLEANUP_INTERVAL` | `1m` | Seberapa sering session kedaluwarsa dibersihkan dari memori. |
| `SHOW_DEMO_ACCOUNTS` | kosong | `true` menampilkan akun demo di halaman login. Hanya untuk demo lokal. |
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
cd backend && SHOW_DEMO_ACCOUNTS=true go run ./cmd/server
```

Session hanya ada di memori server: setiap restart backend semua pengguna harus login lagi.

## Test

```bash
cd backend && go test -race ./...
```

```bash
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...
```

Perintah pertama menjalankan unit test. Perintah kedua juga menjalankan integration test ke `test-pg` (container harus menyala); `-p 1` wajib karena semua package memakai database test yang sama.

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

- Halaman ada di `backend/internal/web`: handler di `web.go`, `pages.go`, dan `admin.go`, template di `templates/`.
- Styling memakai utility class Tailwind CSS v4; class bersama (`btn`, `input`, `tbl`, `pill-*`, dll.) didefinisikan di
  `backend/internal/web/tailwind.css`. Setelah mengubah template atau CSS, bangun ulang `static/app.css` (hasilnya di-commit,
  sehingga menjalankan aplikasi tidak membutuhkan Bun):

```bash
cd backend/internal/web && bun install && bun run css
```

- Ikon memakai icon font Lucide (`lucide-static`) melalui CDN, ditulis sebagai `{{icon "database" 16}}` di template.
- Tooling CSS memakai Bun, bukan npm.

## Keputusan desain

Bagian ini mencatat keputusan desain yang diambil tim beserta alasannya. Keputusan yang berasal langsung dari spesifikasi
ditandai **(spek)**; selebihnya adalah pilihan tim.

### 1. Teknologi

| Aspek | Pilihan | Alasan |
| --- | --- | --- |
| Backend | Go **(spek)** dengan Gin | Router ringan dengan middleware yang mudah disusun per kelompok route. |
| Database platform | PostgreSQL melalui GORM | Data platform (user, group, grant, koneksi, riwayat) berupa relasi biasa; ORM mempercepat CRUD dan migrasi schema platform. |
| Database target | PostgreSQL **(spek)** melalui `database/sql` + pgx | Introspeksi katalog dan eksekusi SQL pengguna ditulis manual; ini bukan pekerjaan ORM. |
| Tampilan | Server-side rendering dengan Go `html/template`, Tailwind CSS, icon font Lucide | Spesifikasi dan Q&A asisten: frontend Milestone 1 tidak boleh SPA. Halaman dirender backend dan memanggil service yang sama dengan JSON API, sehingga logika dan otorisasi tidak ditulis dua kali. JavaScript hanya dipakai untuk konfirmasi dan kenyamanan editor; semua aksi tetap berupa form biasa. |
| Origin | Satu origin | Halaman dan JSON API (`/api`) disajikan oleh server yang sama, sehingga cookie dapat memakai `SameSite=Strict` tanpa CORS. |

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
| `auth`, `session` | Login, hashing password, seed admin, session in-memory, rotasi, CSRF. |
| `authz` | Policy Decision Point: daftar aksi, level minimum, dan rumus akses efektif. |
| `admin` | User, role, group, keanggotaan, dan grant. |
| `dbconn` | Koneksi database target: enkripsi kredensial, pool, uji koneksi. |
| `explorer` | Schema, table, column, constraint, index, dan data (read-only). |
| `migration` | Draft, eksekusi transaksional, `sqlguard`, lock per database, rekonsiliasi. |
| `httpapi` | Router JSON API, handler, dan pemetaan error ke JSON. |
| `web` | Halaman HTML server-rendered: login, explorer, migration, dan administrasi. |
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

Session disimpan di memori proses backend. Map berisi hash SHA-256 dari identifier, dengan indeks per user untuk pencabutan
massal, dilindungi satu mutex. Goroutine pembersih menghapus session kedaluwarsa setiap `SESSION_CLEANUP_INTERVAL`.

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
setelah grace period identifier lama ditolak **(spek)**. Pemeriksaan dan pembuatan identifier baru terjadi di bawah satu
lock, sehingga dua request paralel tidak menghasilkan dua penerus.

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

### 10. Penghapusan data

Semua penghapusan adalah hard delete dengan `ON DELETE CASCADE`, kecuali referensi user di riwayat migration.

| Dihapus | Ikut terhapus |
| --- | --- |
| User | Role, keanggotaan group, dan seluruh session-nya. Riwayat migration tetap ada dengan salinan username. |
| Group | Keanggotaan dan grant; anggotanya langsung kehilangan akses lewat group tersebut. |
| Koneksi database | Grant dan seluruh riwayat migration database itu; pool koneksinya ditutup. |

### 11. Kontrak API dan format error

Semua endpoint berada di bawah `/api` dan memakai JSON `camelCase`; waktu dalam RFC 3339. Route admin berada di
`/api/admin/*`, dan grant serta keanggotaan memakai `PUT` agar idempotent.

| Area | Endpoint |
| --- | --- |
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` |
| Admin | `/admin/users`, `/admin/users/:userId`, `/admin/users/:userId/roles/:role`, `/admin/groups`, `/admin/groups/:groupId`, `/admin/groups/:groupId/members/:userId`, `/admin/groups/:groupId/grants/:databaseId`, `/admin/databases`, `/admin/databases/:databaseId`, `/admin/databases/test`, `/admin/databases/:databaseId/test` |
| Explorer | `GET /databases`, `GET /databases/:databaseId`, `.../schemas`, `.../schemas/:schema/tables`, `.../tables/:table`, `.../tables/:table/rows?page=&pageSize=` |
| Migration | `GET/POST /databases/:databaseId/migrations?status=`, `GET/PATCH/DELETE /migrations/:migrationId`, `POST /migrations/:migrationId/execute`, `POST /migrations/:migrationId/duplicate` |

Setiap error memakai bentuk yang sama:

```json
{ "error": { "code": "MIGRATION_RUNNING", "message": "Sedang ada migration yang berjalan di database ini.", "details": {} } }
```

| HTTP | Kode |
| --- | --- |
| 400 | `VALIDATION_ERROR` (dengan `details` per field), `TRANSACTION_CONTROL_NOT_ALLOWED` |
| 401 | `UNAUTHENTICATED`, `INVALID_CREDENTIALS` |
| 403 | `FORBIDDEN`, `CSRF_INVALID`, `ORIGIN_NOT_ALLOWED` |
| 404 | `NOT_FOUND` |
| 409 | `USERNAME_TAKEN`, `CONFLICT`, `LAST_ADMIN`, `MIGRATION_RUNNING`, `MIGRATION_NOT_DRAFT`, `REVISION_CONFLICT` |
| 415 | `UNSUPPORTED_MEDIA_TYPE` |
| 500 | `INTERNAL_ERROR` (tanpa detail internal; detailnya hanya di log) |
| 502 / 504 | `TARGET_DB_UNAVAILABLE` / `TARGET_DB_TIMEOUT` |

### 12. Strategi pengujian

- **Unit test** di samping kode yang diuji, untuk logika padat aturan: `authz`, `sqlguard`, session store (dengan jam yang
  dapat diinjeksi), enkripsi kredensial, pagination, pemetaan error, dan konfigurasi.
- **Integration test** (build tag `integration`) terhadap PostgreSQL sungguhan di `test-pg`, sebagian besar melalui router
  Gin dengan `httptest` tanpa menyalakan server, sesuai contoh di spesifikasi. Cakupannya meliputi halaman HTML (login, redirect pengunjung anonim,
  form tanpa token CSRF, explorer, penolakan VIEW, alur buat-jalankan migration, halaman admin), login, logout, timeout
  dan rotasi session, CSRF dan `Origin`, seed admin, CRUD admin dan penolakan non-admin, akses efektif multi-group, IDOR,
  uji koneksi (termasuk kredensial salah dan peringatan superuser), explorer read-only, eksekusi sukses, multi-statement dan
  function, rollback, statement non-transaksional, concurrency, lock timeout, dan rekonsiliasi.
- Setiap fitur diuji untuk alur berhasil, gagal, dan penolakan akses **(spek)**. Semua test dijalankan dengan race detector.

### 13. Batasan yang disengaja

- **Session in-memory:** restart backend mengeluarkan semua pengguna, dan backend harus berjalan sebagai satu instance
  (lock migration per database juga berada di memori proses).
- **Statement non-transaksional** seperti `CREATE INDEX CONCURRENTLY`, `VACUUM`, `CREATE DATABASE`, dan `ALTER SYSTEM`
  tidak didukung, karena setiap migration dijalankan dalam transaction agar dapat di-rollback. PostgreSQL menolaknya dengan
  SQLSTATE `25001`, dan platform menampilkan penjelasan yang mudah dipahami. Body function standar SQL (`BEGIN ATOMIC`) juga
  ditolak `sqlguard`; gunakan body dollar-quoted.
- **Table `_platform_migrations`** dibuat di schema `public` setiap database target dan dikelola platform; table ini tidak
  boleh diubah manual.
- **Table tanpa primary key** diurutkan berdasarkan `ctid` saat dipaginasi. Urutannya stabil selama data tidak berubah, tetapi
  dapat bergeser setelah `UPDATE` atau `VACUUM FULL`.
- **Ikon dan font** dimuat dari CDN (jsDelivr dan Google Fonts). Tanpa internet, halaman tetap berfungsi dengan font sistem,
  tetapi ikon tidak tampil.
- Di luar lingkup milestone ini: DBMS selain PostgreSQL, schema diff, analisis `EXPLAIN`, re-authentication, rate limiting
  login, dan pagination di halaman admin.
