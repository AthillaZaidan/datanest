# loadtest

Stress test k6 untuk bonus Milestone 2. Beban dinaikkan dari normal ke 3x, 10x, dan 30x, lalu diturunkan lagi untuk melihat
pemulihan, sambil mengirim job migration yang sudah disetujui ke satu database yang sama dan ke beberapa database berbeda.
Desain, threshold, hasil per tahap, dan analisisnya ada di
[19. Bonus: stress test dengan k6](../README.md#19-bonus-stress-test-dengan-k6).

## Isi

| File | Fungsi |
| --- | --- |
| `stress.js` | Script k6 dengan tiga skenario: `api` (pengguna yang membaca API), `jobs_same_db` (job ke `loadtest_a`), dan `jobs_spread` (job bergiliran ke `loadtest_b` sampai `loadtest_d`). `setup()` mendaftarkan database, memberi grant, dan membuat ulang table `lt_items` lewat migration yang disetujui. `handleSummary` menulis ringkasan per tahap. |
| `prepare.sh` | Membuat database `loadtest_a` sampai `loadtest_d` di `target-pg` bila belum ada (milik `platform_app`). |
| `results/1-light/` | Run ringan (job 20.000 row, puncak 10x). Lulus semua threshold. |
| `results/2-before-fix/` | Run berat sebelum perbaikan pool koneksi dan penanganan gangguan database di auth. |
| `results/3-after-fix/` | Run berat dengan beban yang sama sesudah perbaikan. |

Setiap folder hasil berisi `summary.md` (tabel per tahap dan threshold dari k6), `summary.json` (semua metrik k6), dan untuk run
berat `server-metrics.md` (antrean, durasi job, status HTTP, pool koneksi, dan CPU per tahap dari Prometheus).

## Menjalankan

Butuh k6 (`brew install k6`), stack Docker yang menyala, server dan dua worker seperti di
[18. Bonus: observability](../README.md#18-bonus-observability-collector-prometheus-dan-grafana), dan akun demo (`go run ./cmd/seed`).
Tes menulis ratusan migration dan eksekusi ke database platform, jadi arahkan `PLATFORM_DSN` server, worker, dan seed ke database
platform terpisah, misalnya `datanest_loadtest`.

```bash
./loadtest/prepare.sh
SUMMARY_DIR=loadtest/results/baru k6 run loadtest/stress.js
QUICK=1 k6 run loadtest/stress.js
```

Perintah terakhir menjalankan versi 10 kali lebih singkat, berguna untuk memeriksa script sebelum run penuh (sekitar 10 menit).

| Variabel | Default | Fungsi |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:8080` | Alamat backend. |
| `NORMAL_VUS` | `10` | Jumlah pengguna API pada beban normal; tahap lain dikalikan 3, 10, dan 30. |
| `NORMAL_JOBS_PER_MIN` | `6` | Job per menit per skenario job pada beban normal. |
| `ROWS` | `1000000` | Jumlah row `lt_items`; setiap job mengubah seperempatnya. |
| `JOB_TIMEOUT_S` | `180` | Batas waktu polling satu job sebelum dianggap tidak selesai. |
| `QUICK` | kosong | `1` memperpendek semua tahap 10 kali. |
| `SUMMARY_DIR` | kosong | Folder tujuan `summary.md` dan `summary.json`. Tanpa ini ringkasan hanya dicetak ke layar. |

Script login sebagai `demo-admin` untuk menyiapkan koneksi dan grant, lalu sebagai `developer` dan `reviewer` (leader kedua group)
untuk mengajukan dan menyetujui job. Job dianggap berhasil hanya bila statusnya menjadi `success`, bukan saat pengajuannya diterima.
Untuk membersihkan setelah tes, hapus database `loadtest_a` sampai `loadtest_d` di `target-pg` dan database platform terpisahnya.
