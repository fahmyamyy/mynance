# SETUP — How to Run mynance Locally

Local dev setup for the mynance CEX backend.

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.25.7+ | `go.mod` pins 1.25.7 |
| PostgreSQL | 14+ | Reachable on `localhost:5432` |
| goose | latest | DB migrations |
| Docker | optional | Swagger UI only |

Install Go: https://go.dev/dl/
Install Postgres (macOS): `brew install postgresql@16 && brew services start postgresql@16`

Install `goose`:
```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
# ensure $(go env GOPATH)/bin is on PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

---

## Ports

| Service | Port |
|---------|------|
| API server | `18080` |
| Swagger UI | `18081` |
| Frontend (CORS allowed) | `13000` |
| PostgreSQL | `5432` |

---

## 1. Database

Create the `mynance` DB with `postgres/postgres`:

```bash
PGPASSWORD=postgres createdb -h localhost -p 5432 -U postgres mynance
```

If `postgres` role missing, create it first:
```bash
psql -h localhost -p 5432 -d postgres -c "CREATE ROLE postgres WITH LOGIN SUPERUSER PASSWORD 'postgres';"
```

Verify:
```bash
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d mynance -c "SELECT 1;"
```

---

## 2. Run Migrations

```bash
make migrate
```

Or directly:
```bash
goose -dir migrations postgres "postgres://postgres:postgres@localhost:5432/mynance?sslmode=disable" up
```

Expected: `successfully migrated database to version: 11`.

Rollback one step:
```bash
goose -dir migrations postgres "postgres://postgres:postgres@localhost:5432/mynance?sslmode=disable" down
```

---

## 3. Configuration

Default config: `config/application.yaml`. Already wired for:
- DB: `postgres://postgres:postgres@localhost:5432/mynance`
- Server port: `18080`
- JWT secret: dev-only placeholder (≥32 chars)

Override via env vars (production):
```bash
export DATABASE_URL="postgres://USER:PASS@HOST:5432/DB?sslmode=require"
export JWT_SECRET="<at-least-32-character-secret>"
export BINANCE_SYMBOLS="BTC-USDT,ETH-USDT,SOL-USDT"
export MARKETFEED_ENABLED=true
export CORS_ALLOWED_ORIGINS="http://localhost:13000"
```

---

## 4. Run

API only:
```bash
make run
# → http://localhost:18080
```

API + Swagger UI (needs Docker):
```bash
make dev
# → API:     http://localhost:18080
# → Swagger: http://localhost:18081
```

Health check:
```bash
curl http://localhost:18080/health
# ok
```

---

## 5. Frontend CORS

Backend allows requests from `http://localhost:13000` with:
- Credentials: `true` (cookies / `Authorization` header allowed)
- Methods: `GET POST PUT DELETE PATCH OPTIONS`
- Headers: `Accept, Authorization, Content-Type, X-Requested-With, Idempotency-Key`

Frontend `fetch` example:
```ts
fetch("http://localhost:18080/me", {
  credentials: "include",
  headers: { Authorization: `Bearer ${token}` },
});
```

**Configured in `config/application.yaml`:**
```yaml
cors:
  allowed_origins: "http://localhost:13000"
```

Multiple origins → comma-separated:
```yaml
cors:
  allowed_origins: "http://localhost:13000,https://app.example.com"
```

Env override:
```bash
export CORS_ALLOWED_ORIGINS="http://localhost:13000,https://app.example.com"
```

---

## 6. Tests

```bash
make test                 # unit + race detector
go test -tags=integration ./internal/repository/...   # integration (needs DB)
```

---

## 7. Common Tasks

| Task | Command |
|------|---------|
| Build binary | `make build` |
| Run server | `make run` |
| Run + Swagger | `make dev` |
| Lint | `make lint` |
| Migrate up | `make migrate` |
| Reset DB | `dropdb mynance && createdb mynance && make migrate` |

---

## Troubleshooting

**`ping database` fails** — Postgres not running or wrong creds. Verify with `pg_isready -h localhost -p 5432`.

**`JWT_SECRET is required and must be at least 32 characters`** — secret too short. Override via `export JWT_SECRET=...`.

**`port 18080: bind: address already in use`** — another process holds the port. `lsof -i :18080` then kill it.

**CORS error in browser** — frontend not on `http://localhost:13000`. Either move FE to that port or update `AllowedOrigins` in `cmd/server/main.go`.

**Marketfeed connection errors** — disable for offline dev: `export MARKETFEED_ENABLED=false`.
