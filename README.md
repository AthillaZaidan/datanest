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

## Test

```bash
cd backend && go test -race ./...                                   # unit tests
cd backend && set -a && . ../.env && set +a && go test -race -tags=integration -p 1 ./...   # + integration (needs test-pg)
```
