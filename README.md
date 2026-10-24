# datanest

Database management platform: access control, database explorer, and controlled migrations for PostgreSQL.

## Requirements

- Go 1.25+
- Bun 1.2+
- Docker

## Setup

```bash
cp .env.example .env          # fill ADMIN_PASSWORD and CREDENTIALS_KEY (openssl rand -base64 32)
docker compose up -d --wait   # platform-pg :15432, target-pg :15433, test-pg :15434
```

## Run

```bash
cd backend && go run ./cmd/server     # http://localhost:8080
cd frontend && bun install && bun run dev   # http://localhost:5173, /api proxied to the backend
```

## Conventions

- Frontend uses Bun (`bun add`, `bun run ...`), not npm.
- Styling uses Tailwind CSS v4 utility classes; avoid new hand-written CSS files.

## Test

```bash
cd backend && go test -race ./...                                   # unit tests
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...   # + integration (needs test-pg)
```

## Database explorer

- Shows schemas, tables, columns, constraints, indexes, and table data of a target database. Requires at least **VIEW**.
- Every explorer query runs in a `READ ONLY` transaction, so this path cannot change a target database even if a bug slipped through.
- Schema and table names from the URL are matched against the catalog before use; unknown names return 404.
- System schemas (`pg_catalog`, `information_schema`, `pg_toast`, temp schemas) and the `_platform_migrations` table are hidden.
- Data is paginated: page size 10 / 25 / 50 / 100 (default 25, values above 100 are clamped). Rows are ordered by the primary key; tables without one are ordered by `ctid` (stable while the table is not modified).

## Migrations

- **View** users can read the history and details; **Edit** users can create, edit, delete, duplicate, and execute.
- Lifecycle: `draft → running → success | failed`. Drafts can be edited by any Edit user of that database; executed migrations are immutable. Fix a failed one with **Duplicate**, which copies its SQL into a new draft.
- Concurrent edits of a draft use a `revision` number; a stale save returns `409 REVISION_CONFLICT`.
- **Versions** are assigned when execution starts, per database, including failed runs. Version order equals the order changes really happened, and history is never rewritten. The current schema version is the highest successful version.
- **Execution**: the SQL runs in one transaction with `SET LOCAL lock_timeout` (`MIGRATION_LOCK_TIMEOUT`) and `statement_timeout` (`MIGRATION_STATEMENT_TIMEOUT`), bounded by `MIGRATION_EXEC_TIMEOUT`. Any error rolls everything back and the migration is recorded as `failed` with the PostgreSQL message and SQLSTATE. Execution finishes even if the browser is closed.
- **One migration per database at a time**: a second execute on the same database returns `409 MIGRATION_RUNNING`; different databases run in parallel. Executing the same draft twice runs it once.
- **Consistency**: each successful run also inserts its version into `public._platform_migrations` on the target, in the same transaction. On startup, migrations left `running` by a crash are settled from that table (`success` if the row exists, otherwise `failed`).

### Limitations

- Statements that cannot run inside a transaction (`CREATE INDEX CONCURRENTLY`, `VACUUM`, `CREATE DATABASE`, ...) fail with SQLSTATE `25001`; this is intentional so every migration can be rolled back.
- `_platform_migrations` is managed by the platform; do not modify it in a migration.
- The per-database lock is in memory, so the backend must run as a single instance.
