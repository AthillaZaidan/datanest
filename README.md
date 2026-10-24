# datanest

Database management platform: access control, database explorer, and controlled migrations for PostgreSQL.

## Requirements

- Go 1.25+
- Bun 1.2+
- Docker

## Setup

```bash
cp .env.example .env          # fill ADMIN_PASSWORD and CREDENTIALS_KEY (openssl rand -base64 32)
docker compose up -d --wait   # platform-pg :15432, target-pg :15433, test-pg :15434
```

## Run

```bash
cd backend && go run ./cmd/server     # http://localhost:8080
cd frontend && bun install && bun run dev   # http://localhost:5173, /api proxied to the backend
```

## Test

```bash
cd backend && go test -race ./...                                   # unit tests
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...   # + integration (needs test-pg)
```

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
