## Hasil per tahap

| Tahap | VU API | Job/menit per skenario | API p50 | API p95 | API req/s | API error | Submit p95 | Job selesai (same / spread) | Job sukses | Queue wait p95 (same / spread) | Durasi job p95 (same / spread) | Submit sampai selesai p95 (same / spread) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| normal | 10 | 6 | 5 ms | 67 ms | 9.9 | 0.00% | 15 ms | 6 / 6 | 100.0% | 9 ms / 10 ms | 671 ms / 621 ms | 1039 ms / 1041 ms |
| high | 30 | 18 | 4 ms | 58 ms | 29.6 | 0.00% | 16 ms | 18 / 18 | 100.0% | 11 ms / 11 ms | 805 ms / 712 ms | 1048 ms / 1047 ms |
| peak | 100 | 60 | 18 ms | 649 ms | 89.5 | 0.00% | 136 ms | 43 / 56 | 93.4% | 150324 ms / 132993 ms | 6813 ms / 7630 ms | 154545 ms / 142114 ms |
| stress | 300 | 180 | 141 ms | 5047 ms | 121.6 | 15.75% | 283 ms | 24 / 75 | 49.7% | 175975 ms / 172050 ms | 10579 ms / 9608 ms | 177838 ms / 174317 ms |
| recovery | 10 | 6 | 11 ms | 356 ms | 9.3 | 0.00% | 39 ms | 3 / 9 | 66.7% | 16 ms / 164932 ms | 2496 ms / 3028 ms | 3076 ms / 166037 ms |

## Threshold

| Metrik | Syarat | Hasil |
| --- | --- | --- |
| `http_req_duration{type:api}` | `p(95)<500` | gagal |
| `http_req_failed{type:api}` | `rate<0.01` | gagal |
| `http_req_duration{type:submit}` | `p(95)<1000` | lulus |
| `http_req_failed{type:submit}` | `rate<0.01` | lulus |
| `job_success` | `rate>0.95` | gagal |

Total request HTTP: 118960, iterasi: 24498, VU maksimum: 588. Iterasi job yang tidak sempat dimulai (dropped): same 143, spread 127.
