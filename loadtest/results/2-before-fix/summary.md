## Hasil per tahap

| Tahap | VU API | Job/menit per skenario | API p50 | API p95 | API req/s | API error | Submit p95 | Job selesai (same / spread) | Job sukses | Queue wait p95 (same / spread) | Durasi job p95 (same / spread) | Submit sampai selesai p95 (same / spread) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| normal | 10 | 6 | 5 ms | 27 ms | 10.0 | 0.00% | 16 ms | 6 / 6 | 100.0% | 8 ms / 5 ms | 672 ms / 597 ms | 1052 ms / 1052 ms |
| high | 30 | 18 | 5 ms | 30 ms | 29.8 | 0.00% | 26 ms | 18 / 18 | 100.0% | 9 ms / 15 ms | 753 ms / 774 ms | 1076 ms / 1088 ms |
| peak | 100 | 60 | 18 ms | 248 ms | 94.4 | 0.00% | 170 ms | 34 / 60 | 84.7% | 42069 ms / 65 ms | 5320 ms / 9193 ms | 43709 ms / 9804 ms |
| stress | 300 | 180 | 596 ms | 5525 ms | 107.0 | 37.86% | 1819 ms | 4 / 23 | 11.5% | 163338 ms / 160428 ms | 27376 ms / 30816 ms | 170906 ms / 170575 ms |
| recovery | 10 | 6 | 713 ms | 5024 ms | 4.4 | 3.83% | 5551 ms | 0 / 3 | 16.7% | - / 172201 ms | - / 1144 ms | - / 175587 ms |

## Threshold

| Metrik | Syarat | Hasil |
| --- | --- | --- |
| `http_req_duration{type:api}` | `p(95)<500` | gagal |
| `http_req_failed{type:api}` | `rate<0.01` | gagal |
| `http_req_duration{type:submit}` | `p(95)<1000` | gagal |
| `http_req_failed{type:submit}` | `rate<0.01` | gagal |
| `job_success` | `rate>0.95` | gagal |

Total request HTTP: 53876, iterasi: 21990, VU maksimum: 475. Iterasi job yang tidak sempat dimulai (dropped): same 88, spread 67.
