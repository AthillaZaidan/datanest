# security

Bukti dan hasil SAST CodeQL untuk bonus Milestone 2. Workflow-nya ada di
[`.github/workflows/codeql.yml`](../.github/workflows/codeql.yml); daftar setiap temuan beserta dampak dan keputusannya ada di
[20. Bonus: SAST dengan CodeQL](../README.md#20-bonus-sast-dengan-codeql).

## Isi

| File | Isi |
| --- | --- |
| `codeql/scan-1-before-fix.md` | Scan lokal pertama (commit `de61574`): 12 temuan Go (2 `go/incorrect-integer-conversion`, 10 `go/log-injection`), 0 temuan JavaScript/TypeScript. |
| `codeql/scan-2-after-fix.md` | Scan ulang sesudah perbaikan parsing ID (commit `d8c9abf`): 10 temuan Go, semuanya `go/log-injection`; 0 temuan JavaScript/TypeScript dan GitHub Actions. |

Keduanya dibuat dari SARIF CodeQL CLI 2.27.6 dengan suite `security-extended` (`codeql/go-queries` 1.6.16,
`codeql/javascript-queries` 2.5.2), pada checkout bersih tanpa `node_modules` dan `frontend/dist`. SARIF mentahnya tidak disimpan
di repo karena berisi path lokal mesin yang menjalankan scan.

## Status saat ini

| Temuan | Jumlah | Keputusan |
| --- | --- | --- |
| `go/incorrect-integer-conversion` | 2 | Diperbaiki di commit `6a836a0`: ID dari path URL dan dari pesan RabbitMQ di-parse dengan ukuran `uint` platform, dengan test yang menolak nilai di luar jangkauan. |
| `go/log-injection` | 10 | False positive untuk log forging: semua log berupa JSON terstruktur (`log/slog`) atau atribut OTLP, sehingga baris baru di input pengguna di-escape. Dibuktikan oleh `backend/internal/obs/forging_test.go`. |

Hasil yang sama muncul di GitHub Actions ([run 45878168074](https://github.com/AthillaZaidan/datanest/actions/runs/45878168074)) dan
di tab [Security, Code scanning](https://github.com/AthillaZaidan/datanest/security/code-scanning). Hasil bersih tidak berarti
aplikasi pasti aman; batasan cakupan CodeQL di project ini dijelaskan di README utama bagian 20.

## Menjalankan ulang secara lokal

Butuh CodeQL CLI (`brew install --cask codeql`) dan query pack-nya
(`codeql pack download codeql/go-queries codeql/javascript-queries`). Dari root repo:

```bash
codeql database create /tmp/db-go --language=go --source-root=backend --command="go build ./..."
codeql database analyze /tmp/db-go codeql/go-queries:codeql-suites/go-security-extended.qls --format=sarif-latest --output=go.sarif
codeql database create /tmp/db-js --language=javascript-typescript --source-root=. --build-mode=none
codeql database analyze /tmp/db-js codeql/javascript-queries:codeql-suites/javascript-security-extended.qls --format=sarif-latest --output=js.sarif
```

Jalankan dari checkout bersih (misalnya `git worktree add`), supaya `frontend/node_modules` dan `frontend/dist` tidak ikut dianalisis.
