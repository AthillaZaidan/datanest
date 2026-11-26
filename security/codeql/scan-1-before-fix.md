# CodeQL lokal: sebelum perbaikan

Commit: `de61574`. CodeQL CLI 2.27.6, suite `security-extended` (`codeql/go-queries` 1.6.16, `codeql/javascript-queries` 2.5.2).

## Go: 12 temuan

| Rule | Level | Security severity | Lokasi | Pesan |
| --- | --- | --- | --- | --- |
| `go/incorrect-integer-conversion` | warning | 8.1 | `backend/internal/httpapi/helpers.go:28` | Incorrect conversion of an unsigned 64-bit integer from [strconv.ParseUint](1) to a lower bit size type uint without an upper bound check. |
| `go/incorrect-integer-conversion` | warning | 8.1 | `backend/internal/queue/message.go:50` | Incorrect conversion of an unsigned 64-bit integer from [strconv.ParseUint](1) to a lower bit size type uint without an upper bound check. |
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

## JavaScript/TypeScript: 0 temuan

