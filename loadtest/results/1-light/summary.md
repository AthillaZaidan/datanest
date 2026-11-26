## Hasil per tahap

| Tahap | VU API | Job/menit per skenario | API p50 | API p95 | API req/s | API error | Submit p95 | Job selesai (same / spread) | Job sukses | Queue wait p95 (same / spread) | Durasi job p95 (same / spread) | Submit sampai selesai p95 (same / spread) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| normal | 10 | 6 | 5 ms | 19 ms | 10.0 | 0.00% | 18 ms | 6 / 6 | 100.0% | 9 ms / 6 ms | 55 ms / 54 ms | 556 ms / 547 ms |
| double | 20 | 12 | 5 ms | 19 ms | 19.8 | 0.00% | 25 ms | 12 / 12 | 100.0% | 9 ms / 10 ms | 59 ms / 60 ms | 567 ms / 566 ms |
| peak | 100 | 60 | 3 ms | 12 ms | 99.3 | 0.00% | 23 ms | 60 / 60 | 100.0% | 9 ms / 9 ms | 63 ms / 62 ms | 560 ms / 566 ms |
| recovery | 10 | 6 | 5 ms | 20 ms | 10.0 | 0.00% | 22 ms | 6 / 6 | 100.0% | 7 ms / 10 ms | 52 ms / 52 ms | 567 ms / 567 ms |

## Threshold

| Metrik | Syarat | Hasil |
| --- | --- | --- |
| `http_req_duration{type:api}` | `p(95)<500` | lulus |
| `http_req_failed{type:api}` | `rate<0.01` | lulus |
| `http_req_duration{type:submit}` | `p(95)<1000` | lulus |
| `http_req_failed{type:submit}` | `rate<0.01` | lulus |
| `job_success` | `rate>0.95` | lulus |

Total request HTTP: 14163, iterasi: 12739.
