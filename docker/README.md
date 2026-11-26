# docker

Berkas pendukung untuk `docker-compose.yml` di root repo: script inisialisasi Postgres dan konfigurasi stack observability.
Semua perintah `docker compose` dijalankan dari root repo.

## Isi

| File | Dipakai oleh | Fungsi |
| --- | --- | --- |
| `target-init.sql` | `target-pg` | Saat volume pertama kali dibuat: membuat role `platform_app` (`NOSUPERUSER`, tanpa `CREATEDB` dan `CREATEROLE`), database demo `toko` (`customers`, `products`, `orders`) dan `kampus` (`students`, `courses`, `enrollments`) milik role itu, beserta data contohnya. |
| `test-init.sql` | `test-pg` | Membuat `datanest_test_target`, database target untuk integration test. Database platform test dibuat lewat `POSTGRES_DB`. |
| `observability/prometheus.yaml` | `lgtm` | Konfigurasi Prometheus bawaan `grafana/otel-lgtm` ditambah scrape tiap 5 detik ke server (`host.docker.internal:9100`) dan dua worker (`:9101`, `:9102`). |
| `observability/grafana-dashboards.yaml` | `lgtm` | Provider dashboard Grafana yang memuat semua JSON di `observability/dashboards/` ke folder **DataNest**. |
| `observability/dashboards/datanest.json` | `lgtm` | Dashboard queue, migration, pool koneksi, API, log, dan trace. |

Script di `docker-entrypoint-initdb.d` hanya berjalan sekali, saat volume masih kosong. Mengubah `target-init.sql` tidak berpengaruh
pada volume yang sudah ada; jalankan `docker compose down -v` untuk mulai dari awal (semua data lokal ikut terhapus).

## Service compose

| Service | Image | Port host | Fungsi |
| --- | --- | --- | --- |
| `platform-pg` | `postgres:17-alpine` | 15432 | Database platform (user, group, session, migration, eksekusi). Data di volume `platform-data`. |
| `target-pg` | `postgres:17-alpine` | 15433 | Database yang dikelola (`toko`, `kampus`). Data di volume `target-data`. |
| `test-pg` | `postgres:17-alpine` | 15434 | Database platform dan target untuk integration test, di tmpfs (hilang saat container berhenti). |
| `rabbitmq` | `rabbitmq:4-management-alpine` | 15673 (AMQP), 15672 (UI) | Antrean job eksekusi. User dan password `datanest`. |
| `lgtm` | `grafana/otel-lgtm:0.41.1` | 3000 (Grafana), 4317/4318 (OTLP), 9090 (Prometheus) | Bonus observability: OpenTelemetry Collector, Prometheus, Loki, Tempo, Grafana. Hanya menyala dengan `--profile observability`. Data di volume `lgtm-data`. |

```bash
docker compose up -d --wait
docker compose --profile observability up -d --wait
```

Perintah pertama menyalakan database dan RabbitMQ; perintah kedua ikut menyalakan `lgtm`. Port host bisa diganti dengan
`PLATFORM_PG_PORT`, `TARGET_PG_PORT`, `TEST_PG_PORT`, `RABBITMQ_PORT`, `RABBITMQ_UI_PORT`, `GRAFANA_PORT`, `OTLP_GRPC_PORT`,
`OTLP_HTTP_PORT`, dan `PROMETHEUS_PORT`; sesuaikan DSN dan URL di `.env`. Grafana di image `otel-lgtm` membuka akses admin anonim,
jadi jangan membuka port-portnya ke jaringan. Detail observability ada di
[18. Bonus: observability](../README.md#18-bonus-observability-collector-prometheus-dan-grafana).
