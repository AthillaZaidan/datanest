| Tahap | Antrean maks | Umur antrean tertua maks | Queue wait p95 | Durasi job p95 | Job sukses | Tertolak lock/held | HTTP 401/5xx | Koneksi platform in use maks | Tunggu koneksi platform rata-rata | CPU proses |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| normal | 0 | 0.0 s | 0.0 s | 1.0 s | 12 | 0 | - | 1 | 0.0 ms | worker 0%, server 5% |
| high | 0 | 0.0 s | 0.0 s | 1.0 s | 36 | 0 | - | 1 | 0.0 ms | worker 1%, server 7% |
| peak | 16 | 52.4 s | 9.6 s | 2.4 s | 88 | 72 | - | 66 | 0.0 ms | worker 1%, server 32% |
| stress | 144 | 142.4 s | 57.0 s | 26.8 s | 14 | 795 | 401:2313, 500:170, 504:1374 | 266 | 0.0 ms | worker 2%, server 76% |
| recovery | 158 | 257.7 s | 281.1 s | 26.7 s | 21 | 1574 | 401:84, 500:23, 504:8 | 149 | 0.0 ms | worker 4%, server 34% |
