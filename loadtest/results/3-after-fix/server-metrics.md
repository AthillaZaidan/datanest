| Tahap | Antrean maks | Umur antrean tertua maks | Queue wait p95 | Durasi job p95 | Job sukses | Tertolak lock/held | HTTP 401/5xx | Koneksi platform in use maks | Tunggu koneksi platform rata-rata | CPU proses |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| normal | 0 | 0.0 s | 0.0 s | 0.9 s | 12 | 0 | - | 1 | 0.0 ms | worker 0%, server 4% |
| high | 0 | 0.0 s | 0.0 s | 1.0 s | 36 | 0 | - | 1 | 0.0 ms | worker 1%, server 5% |
| peak | 29 | 73.4 s | 12.2 s | 5.8 s | 73 | 153 | - | 4 | 13.6 ms | worker 1%, server 21% |
| stress | 239 | 163.5 s | 98.2 s | 18.3 s | 29 | 1873 | 504:1153 | 23 | 20.6 ms | worker 5%, server 91% |
| recovery | 280 | 263.4 s | 285.9 s | 4.8 s | 104 | 3361 | - | 22 | 10.3 ms | worker 5%, server 36% |
