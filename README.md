# mynance

Centralized exchange (CEX) backend in Go. Ledger-based, event-driven, financially correct.

Features:
- Append-only ledger as financial source of truth (balances derived via `SUM(amount)`)
- Order state machine (`OPEN` → `PARTIAL` → `FILLED` / `CANCELLED`)
- In-memory price-time priority matching engine with FIFO within a price level
- Atomic trade settlement — exactly 4 double-entry ledger writes, zero-sum validated before commit
- Idempotency enforcement on order creation and trade execution
- Transactional outbox for at-least-once event publishing
- In-memory market data (order book + recent trades) with engine event subscription
- Graceful shutdown for HTTP, outbox publisher, and engine goroutine

---

## Tech stack

| Layer | Choice |
|---|---|
| Language | Go 1.25 |
| HTTP | `chi/v5` |
| Database | PostgreSQL via `pgx/v5` |
| Config | `viper` (env vars + `config/application.yaml`) |
| Migrations | `goose` |
| Validation | `go-playground/validator/v10` |
| Testing | `testing` + `testify` |
| Logging | `log/slog` (structured) |

---

## Quick start

```bash
# Prereqs: Go 1.25+, Postgres 14+, goose
brew install goose

# 1. Start Postgres locally (or set DATABASE_URL to your instance)
createdb mynance

# 2. Apply migrations
make migrate

# 3. Run the server
make run
# → listens on :8080
```

Health check:

```bash
curl http://localhost:8080/health
# → ok
```

---

## Configuration

Config loaded in order: **env vars → `config/application.yaml` → defaults**.

| Env var | YAML key | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | `database.url` | (required) | Postgres DSN |
| `SERVER_PORT` | `server.port` | `8080` | |
| `LOG_LEVEL` | `log.level` | `info` | `debug` / `info` / `warn` / `error` |
| `DB_MAX_CONNS` | `db.max_conns` | `10` | pgx pool size |
| `OUTBOX_POLL_INTERVAL_SECONDS` | `outbox.poll_interval_seconds` | `1` | publisher tick |

---

## Architecture

Vertical-slice layout under `internal/`:

```
cmd/server/            # Binary entry point — wires everything
internal/
  user/                # User CRUD
  account/             # Account per (user, asset)
  ledger/              # Append-only entries, balance via SUM
  idempotency/         # UNIQUE(key, scope) dedup
  outbox/              # Transactional event publisher
  order/               # Order lifecycle, RESERVE / RELEASE
  trade/               # Atomic trade settlement (4 entries, zero-sum)
  engine/              # In-memory matching engine + settlement subscriber
  eventbus/            # Async pub/sub with panic isolation
  marketdata/          # In-memory book view + recent trades
  shared/              # Sentinel errors, HTTP helpers
pkg/
  numeric/             # big.Rat arithmetic over pgtype.Numeric
  timeutil/            # UTC time helper
  validate/            # Validator wrapper
migrations/            # SQL migrations (00001..00007)
config/                # Config schema + YAML defaults
```

Each slice owns its `domain.go`, `store.go`, `service.go`, `handler.go`. Cross-slice access uses consumer-side interfaces — never imports the other slice's `Store` directly.

---

## Trading flow

```
POST /orders (BUY, qty 0.5 @ 30000)
   ↓
order.Service.PlaceOrder
   ↓ (single pgx.Tx)
   1. idempotency.Insert(key, scope=ORDER)
   2. balance check via ledger.SumByUserAsset
   3. INSERT orders (status=OPEN)
   4. INSERT ledger_entries (RESERVE, -15000 USDT)
   5. INSERT outbox_events (ORDER_PLACED)
   6. COMMIT
   ↓ (after commit)
   engine.Submit(PlaceOrder)
   ↓ (engine goroutine)
   match against resting asks
   ↓ (for each match)
   bus.Publish(TradeMatchedEvent)
   ↓ (settlement subscriber)
   trade.Service.ExecuteTrade
   ↓ (single pgx.Tx)
   1. idempotency.Insert(key=match-<buy>-<sell>-<seq>, scope=TRADE)
   2. INSERT trades
   3. INSERT 4 ledger_entries (TRADE) — zero-sum validated before commit
   4. orders.IncrementFilled(buy_order) + UpdateStatus
   5. orders.IncrementFilled(sell_order) + UpdateStatus
   6. INSERT outbox_events (TRADE_EXECUTED)
   7. COMMIT
   ↓ (in parallel)
   marketdata subscriber updates in-mem book + recent trades
```

**Ledger entries per trade** (price 30000, qty 0.5 BTC):

| user | asset | amount | type |
|---|---|---|---|
| buyer | BTC | +0.5 | TRADE |
| buyer | USDT | -15000 | TRADE |
| seller | BTC | -0.5 | TRADE |
| seller | USDT | +15000 | TRADE |

Sum = 0. Always. The transaction is aborted if not.

---

## Domain model

### Sentinel errors → HTTP

| Sentinel | HTTP |
|---|---|
| `ErrNotFound` | 404 |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403 |
| `ErrBadRequest` | 400 |
| `ErrValidation` | 422 |
| `ErrInsufficientFunds` | 422 |
| `ErrConflict` | 409 |
| `ErrInvalidStateTransition` | 409 |
| `ErrDuplicateIdempotencyKey` | 200 (idempotent success) |
| `ErrRateLimitExceeded` | 429 |
| `ErrServiceUnavailable` | 503 |
| (unknown) | 500 |

### Order status state machine

```
OPEN ─→ PARTIAL ─→ FILLED
  │       │
  └───────┴──→ CANCELLED
```

`FILLED` and `CANCELLED` are terminal. Attempting to cancel either returns `409 Conflict`.

### Idempotency scopes

- `ORDER` — placed-order dedup. Key supplied by client per `POST /orders`.
- `TRADE` — settled-trade dedup. Key supplied by client per manual `POST /trades`, or derived as `match-<buyID>-<sellID>-<seq>` for engine-driven settlement.

---

## HTTP API

Base URL: `http://localhost:8080`. All payloads are JSON. Decimal fields are strings (`"30000"`, `"0.5"`) to avoid float precision loss.

### Health

```
GET /health → 200 "ok"
```

### Users

#### `POST /users`

```jsonc
// Request
{
  "email": "alice@example.com",
  "username": "alice",
  "full_name": "Alice Doe",
  "password": "passw0rd!ABC"
}

// 201 Created
{
  "id": "01952c8c-...",
  "email": "alice@example.com",
  "username": "alice",
  "full_name": "Alice Doe",
  "status": "ACTIVE",
  "created_at": "2026-06-03T10:00:00Z",
  "updated_at": "2026-06-03T10:00:00Z"
}
```

Validation: `email` valid format, `username` 3-100 chars, `full_name` ≤255 chars, `password` ≥8 chars.

Behavior:
- Password is hashed with bcrypt before storage; never returned
- `email` and `username` are unique — duplicate returns `409 Conflict`

#### `GET /users`
List users. Query: `?limit=N&offset=N` (default 50/0, max 100). Returns `200` array of `UserResponse`.

#### `GET /users/{id}`
Returns `200 UserResponse` or `404`.

#### `DELETE /users/{id}`
Soft-delete (sets `deleted_at`). Returns `204`.

#### `GET /users/{userID}/orders`
List orders for user. Same pagination as `/users`. Returns `200` array of `OrderResponse`.

---

### Accounts

#### `POST /accounts`

```jsonc
// Request
{
  "user_id": "01952c8c-...",
  "asset": "USDT"
}

// 201 Created
{
  "id": "01952c8d-...",
  "user_id": "01952c8c-...",
  "asset": "USDT",
  "created_at": "2026-06-03T10:00:00Z"
}
```

Behavior:
- One account per `(user_id, asset)` — duplicate returns `409 Conflict`
- Account has no balance column. Balance is derived from `ledger_entries`.

#### `GET /accounts`
List accounts. Query: `?user_id=<uuid>` to filter, `?limit&offset` for pagination.

#### `GET /accounts/{id}`
Returns `200 AccountResponse` or `404`.

#### `GET /accounts/{id}/balance`

```jsonc
// 200 OK
{ "balance": "100000.0000000000" }
```

Computed as `SUM(amount) FROM ledger_entries WHERE user_id = ? AND asset = ?`. Returns `"0"` if no entries.

#### `DELETE /accounts/{id}`
Returns `204`. Does not delete ledger entries — historical records preserved.

---

### Orders

#### `POST /orders`

```jsonc
// Request
{
  "user_id": "01952c8c-...",
  "symbol": "BTC-USDT",
  "side": "BUY",
  "price": "30000",
  "quantity": "0.5",
  "idempotency_key": "client-generated-uuid"
}

// 201 Created
{
  "id": "01952c8e-...",
  "user_id": "01952c8c-...",
  "symbol": "BTC-USDT",
  "side": "BUY",
  "price": "30000.0000000000",
  "quantity": "0.5000000000",
  "filled_quantity": "0.0000000000",
  "status": "OPEN",
  "created_at": "2026-06-03T10:00:00Z",
  "updated_at": "2026-06-03T10:00:00Z"
}
```

Validation:
- `symbol` format: `BASE-QUOTE` (e.g. `BTC-USDT`); split on `-`
- `side`: `BUY` or `SELL`
- `idempotency_key`: 1-100 chars, unique per scope=ORDER

Behavior:
- **BUY** reserves quote asset: `price × quantity` of QUOTE (e.g. 15000 USDT)
- **SELL** reserves base asset: `quantity` of BASE (e.g. 0.5 BTC)
- Insufficient balance → `422 ErrInsufficientFunds`, no rows written
- Duplicate `idempotency_key` in scope=ORDER → `200 OK` empty body, no re-execution
- After DB commit, order is submitted to the matching engine for automatic crossing
- If engine submit fails (channel full), order remains `OPEN`; placement still succeeds. Operator can retry via admin route.

#### `GET /orders/{id}`
Returns `200 OrderResponse` or `404`.

#### `DELETE /orders/{id}`

Cancel an order. Returns `200 OrderResponse` (status=CANCELLED).

Behavior:
- Validates state: only `OPEN` or `PARTIAL` cancellable; otherwise `409 ErrInvalidStateTransition`
- Releases unreserved portion via RELEASE ledger entry: `(quantity - filled_quantity)` worth of reserve asset
- Inserts `ORDER_CANCELLED` outbox event
- Submits cancel to engine (removes from book)

---

### Trades

#### `POST /trades`

Admin/manual settlement path. The engine uses the same `trade.Service.ExecuteTrade` internally — this endpoint exposes it directly.

```jsonc
// Request
{
  "symbol": "BTC-USDT",
  "buy_order_id": "01952c8e-...",
  "sell_order_id": "01952c8f-...",
  "price": "30000",
  "quantity": "0.5",
  "idempotency_key": "client-generated-uuid"
}

// 201 Created
{
  "id": "01952c90-...",
  "symbol": "BTC-USDT",
  "buy_order_id": "01952c8e-...",
  "sell_order_id": "01952c8f-...",
  "buy_user_id": "01952c8c-...",
  "sell_user_id": "01952c8d-...",
  "price": "30000.0000000000",
  "quantity": "0.5000000000",
  "created_at": "2026-06-03T10:00:01Z"
}
```

Behavior:
- Locks both orders `FOR UPDATE` inside tx — prevents double-fill
- Rejects if either order is `FILLED` or `CANCELLED` → `409`
- Validates buy_order.side == BUY and sell_order.side == SELL
- Writes 4 ledger entries (TRADE type, ref_type=TRADE, ref_id=trade.id)
- Validates zero-sum across the 4 amounts before commit; aborts if non-zero
- Increments `filled_quantity` on both orders; sets status to `FILLED` if fully filled, else `PARTIAL`
- Inserts `TRADE_EXECUTED` outbox event
- Duplicate `idempotency_key` in scope=TRADE → `200 OK`, no re-execution

---

### Market Data (in-memory, read-only)

State is rebuilt from engine events. Lost on restart but engine rehydrates the order book from DB on startup.

#### `GET /orderbook/{symbol}`

```jsonc
// 200 OK
{
  "symbol": "BTC-USDT",
  "bids": [
    { "price": 30000, "quantity": 0.5 },
    { "price": 29950, "quantity": 1.2 }
  ],
  "asks": [
    { "price": 30100, "quantity": 0.3 }
  ]
}
```

- `bids` sorted descending by price (best bid first)
- `asks` sorted ascending by price (best ask first)
- Quantities aggregated across orders at the same price level

#### `GET /marketdata/trades/{symbol}`

```jsonc
// 200 OK — most recent first, capped at 100
[
  { "price": 30000, "quantity": 0.5, "timestamp": "2026-06-03T10:00:01Z" },
  { "price": 30000, "quantity": 0.3, "timestamp": "2026-06-03T09:59:58Z" }
]
```

Empty array if no trades for the symbol.

---

## Matching engine

Single-goroutine, in-memory, per-symbol order book. Algorithm per `matching.md`:

- **Price-time priority** — best price first; FIFO within price level
- **Matching price** = resting order's price (not incoming)
- **Partial fills supported** — `tradeQty = min(incoming.Remaining, resting.Remaining)`
- **No overfill** — once one side hits zero, that order pops from its price level
- **Symbols isolated** — `BTC-USDT` and `ETH-USDT` never cross
- **float64 internally** — converted to `NUMERIC(30,10)` at the settlement boundary via `pkg/numeric`

### Rehydration on startup

On server start, before the engine goroutine begins consuming commands:

```sql
SELECT id, user_id, symbol, side, price, quantity - filled_quantity AS remaining
FROM orders
WHERE status IN ('OPEN', 'PARTIAL')
ORDER BY created_at ASC
```

Each row is `engine.LoadOrder`-ed back into the book at its correct price level, preserving time priority.

### Settlement subscriber

Listens for `TradeMatchedEvent`. For each event:
- Parses order IDs to UUID
- Converts `float64` price/quantity to `pgtype.Numeric` via `pkg/numeric.Parse(strconv.FormatFloat(...))`
- Derives idempotency key: `match-{buyID}-{sellID}-{seq}`
- Calls `trade.Service.ExecuteTrade(...)` — same path as `POST /trades`

If `ExecuteTrade` fails (e.g. DB error, validation), the engine has already updated its in-memory book. A confirmation event from settlement back to the engine is **future work**; current behavior is log-and-alert.

---

## Outbox publisher

Background goroutine. Polls `outbox_events WHERE status='PENDING' FOR UPDATE SKIP LOCKED` every second. For each row:
- Invokes the handler (default = no-op + slog)
- On success: `UPDATE status='PROCESSED', processed_at=now()`
- On failure: `UPDATE retries = retries + 1`, status remains PENDING

Processed rows are retained for audit — never deleted. Currently no DLQ or max-retry alerting.

---

## Database

Tables:

| Table | Purpose | Mutability |
|---|---|---|
| `users` | Identity, hashed password, status | UPDATE/DELETE (soft) |
| `accounts` | (user_id, asset) registration | UPDATE/DELETE |
| `ledger_entries` | Append-only financial truth | INSERT only |
| `orders` | Order lifecycle | INSERT + status/filled UPDATEs |
| `trades` | Settled trade record | INSERT only |
| `idempotency_keys` | (key, scope) dedup | INSERT only |
| `outbox_events` | Pending events | INSERT + status UPDATE |

All financial amounts: `NUMERIC(30, 10)`. Floats forbidden at DB layer.

Migrations applied with `make migrate` (goose). Files: `migrations/00001_*` through `00007_*`.

---

## Make targets

```bash
make build      # compile ./cmd/server → bin/server
make run        # build + run
make test       # go test -race ./...
make lint       # golangci-lint
make migrate    # goose up
make swagger    # serve docs/swagger.yaml on :8081
make dev        # run server + swagger together
```

---

## Testing

```bash
# Unit tests (no DB required)
go test -race ./...

# Integration tests (require running Postgres with migrations applied)
DATABASE_URL=postgres://... go test -tags integration ./...
```

Unit tests cover:
- Matching algorithm (price-time priority, FIFO, partial fills, cancel, 2-trade matches)
- EventBus (multi-subscriber, panic isolation, race-safe concurrent pub/sub)
- Engine goroutine (Submit, channel backpressure, rehydration, concurrent submits)
- Order state machine (cancel FILLED → error)
- Settlement subscriber (event → ExecuteTrade call)
- Market data (book view updates, trade history cap)
- Trade fill logic (FILLED vs PARTIAL, zero-sum math)

Integration tests cover:
- Idempotency: duplicate rejection, scope isolation, rollback retry
- Outbox: insert → publisher processes → status=PROCESSED
- Orders: place BUY (reserve + ledger + outbox), place-then-cancel net-zero, insufficient funds
- Trades: 4 ledger entries, zero-sum, duplicate key, order status update
- E2E (`cmd/server/integration_test.go`): user → account → order → trade → DB verification
- Matching (`cmd/server/matching_integration_test.go`): auto-match via HTTP, rehydration on restart

---

## Idempotency cheatsheet

| Operation | Scope | Key source |
|---|---|---|
| `POST /orders` | `ORDER` | Client-supplied `idempotency_key` |
| `POST /trades` | `TRADE` | Client-supplied `idempotency_key` |
| Engine-driven trade settlement | `TRADE` | Server-derived `match-{buyID}-{sellID}-{seq}` |

On duplicate: server returns `200 OK` empty body (no re-execution, no new rows).

---

## Operational notes

### Graceful shutdown

`SIGINT` / `SIGTERM` triggers:
1. HTTP server stops accepting new connections; 10-second drain for in-flight requests
2. `workerCtx` cancellation propagates to engine goroutine and outbox publisher
3. Both workers exit cleanly via `ctx.Done()` select
4. Pool closed last

### Known limitations

- Engine uses `float64`; tracked as tech debt per `matching.md`
- In-memory market data and order book lost on crash; rehydrated from DB on next start
- Outbox events never expire — operator must archive old PROCESSED rows
- No automatic retry on settlement failure — alert-only
- Single-instance only — no distributed engine or multi-replica outbox publisher

### Logging

Structured via `log/slog`. Fields include:
- `id`, `user_id`, `order_id`, `trade_id` for entity lookups
- `err` (string) on failures
- `event` (event type) for outbox/eventbus

No secrets logged. Passwords are never logged.
