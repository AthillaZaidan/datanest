# .github/workflows

Workflow GitHub Actions untuk repository ini.

| Workflow | Kapan berjalan | Isi |
| --- | --- | --- |
| [`codeql.yml`](codeql.yml) | Push ke `main`, pull request ke `main`, setiap Senin 03:17 UTC, dan manual (**Run workflow**) | SAST CodeQL dengan suite `security-extended`. |

`codeql.yml` punya dua job matrix:

| Job | Build | Langkah |
| --- | --- | --- |
| `Analyze (go)` | Manual | `actions/setup-go` memakai versi dari `backend/go.mod`, lalu `go build ./...` di `backend/`. Build manual dipakai karena modul Go tidak berada di root repo. |
| `Analyze (javascript-typescript)` | Tanpa build | Menganalisis `frontend/src`, konfigurasi Vite, `loadtest/stress.js`, dan file workflow. |

Hasilnya diunggah ke tab [Security, Code scanning](https://github.com/AthillaZaidan/datanest/security/code-scanning) dengan kategori
`/language:go` dan `/language:javascript-typescript`. Workflow hanya meminta izin `contents: read`, `actions: read`, dan
`security-events: write`.

Menjalankan manual dari terminal:

```bash
gh workflow run codeql.yml --ref main
gh run list --workflow codeql.yml --limit 3
```

Temuan, keputusannya, dan riwayat run (termasuk run awal yang tertahan kunci billing akun) ada di
[20. Bonus: SAST dengan CodeQL](../../README.md#20-bonus-sast-dengan-codeql) dan [security/](../../security/README.md).
