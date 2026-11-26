# CodeQL lokal: scan ulang sesudah perbaikan

Commit: `d8c9abf`. CodeQL CLI 2.27.6, suite `security-extended` (`codeql/go-queries` 1.6.16, `codeql/javascript-queries` 2.5.2).

## Go: 10 temuan

| Rule | Level | Security severity | Lokasi | Pesan |
| --- | --- | --- | --- | --- |
| `go/log-injection` | error | 6.1 | `backend/internal/admin/service.go:375` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/admin/service.go:461` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/auth/auth.go:58` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/auth/auth.go:189` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/explorer/querylog.go:38` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/explorer/querylog.go:41` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/httpapi/middleware.go:47` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/httpapi/middleware.go:62` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/httpapi/router.go:172` | This log entry depends on a [user-provided value](1). |
| `go/log-injection` | error | 6.1 | `backend/internal/migration/submit.go:83` | This log entry depends on a [user-provided value](1). |

## JavaScript/TypeScript dan GitHub Actions: 0 temuan

