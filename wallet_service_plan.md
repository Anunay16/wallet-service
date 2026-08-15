# Wallet Service — Implementation Plan

> **Status: Awaiting approval before any code is written.**
> Currency unit: **paise** (₹1 = 100 paise). All amounts are stored as integers (BIGINT).

---

## Decisions Locked In

| Decision | Value |
|---|---|
| Initial wallet balance | **1,000,000 paise** (= ₹10,000) |
| JWT algorithm | **HS256** (symmetric secret) |
| Rate limiting | ❌ Out of scope for now |
| Amount type | `BIGINT` (integer paise — no floats) |
| Token expiry | 24 hours |
| Idempotency key scope | Per user — PK is `(idempotency_key, user_id)` |
| Password hashing | `bcrypt` |

---

## Goal

Build a production-grade peer-to-peer wallet service in Go using GoFiber, backed by PostgreSQL,
with strong financial invariants (conservation, no-overdraft, exactly-once transfers, race-free
wallet creation), JWT authentication, a simple web UI, containerization, and structured logging.

---

## Financial Invariants

| Invariant | How it's enforced |
|---|---|
| **Conservation** | Debit sender + credit receiver in a **single ACID transaction** |
| **No overdraft** | App-level `WHERE balance >= amount` guard + DB `CHECK (balance >= 0)` hard backstop |
| **Exactly-once transfer** | `idempotency_keys` table with PK `(idempotency_key, user_id)`, SHA-256 request hash; hash mismatch → 409 |
| **Race-free wallet creation** | `INSERT ... ON CONFLICT (user_id) DO UPDATE SET updated_at = wallets.updated_at RETURNING *` |

---

## Project Layout

```
wallet-service/
├── cmd/
│   └── server/
│       └── main.go                        # Entry point — wire deps, start server
├── config/
│   └── config.go                          # Viper config loader + Config structs
├── internal/
│   ├── db/
│   │   ├── db.go                          # pgxpool initialization
│   │   └── migrate.go                     # golang-migrate runner (go:embed)
│   ├── domain/
│   │   ├── models.go                      # Domain types: User, Wallet, Transfer, IdempotencyKey
│   │   └── errors.go                      # Sentinel errors: ErrInsufficientFunds, ErrConflict
│   ├── handler/
│   │   ├── auth.go                        # POST /auth/register, POST /auth/login
│   │   ├── wallet.go                      # POST /wallets, GET /wallets/:id
│   │   ├── transfer.go                    # POST /transfers, GET /transfers/:id
│   │   └── health.go                      # GET /health
│   ├── middleware/
│   │   ├── auth.go                        # JWT bearer token validation
│   │   └── logger.go                      # slog request logger middleware
│   ├── repository/
│   │   ├── user.go                        # User CRUD
│   │   ├── wallet.go                      # get-or-create, balance lookup
│   │   ├── transfer.go                    # Atomic transfer execution
│   │   └── idempotency.go                 # Idempotency key persistence
│   ├── server/
│   │   └── server.go                      # Server initialization & route wiring
│   └── service/
│       ├── auth.go                        # Register, Login, token generation
│       ├── wallet.go                      # Wallet business logic
│       └── transfer.go                    # Transfer orchestration + idempotency
├── migrations/
│   ├── 000001_create_users.up.sql
│   ├── 000001_create_users.down.sql
│   ├── 000002_create_wallets.up.sql
│   ├── 000002_create_wallets.down.sql
│   ├── 000003_create_transfers.up.sql
│   ├── 000003_create_transfers.down.sql
│   ├── 000004_create_idempotency_keys.up.sql
│   └── 000004_create_idempotency_keys.down.sql
├── ui/
│   └── index.html                         # Single-page UI (HTML/JS/CSS — no framework)
├── config.yaml                            # Non-secret configuration
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

---

## Database Schema (4 Tables)

### Table 1: `users`

```sql
-- 000001_create_users.up.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email    ON users(email);
```

### Table 2: `wallets`

```sql
-- 000002_create_wallets.up.sql
CREATE TABLE wallets (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    balance    BIGINT      NOT NULL DEFAULT 1000000 CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- DEFAULT 1000000 = 1,000,000 paise = ₹10,000 seed balance
-- UNIQUE (user_id)       → one wallet per user enforced at DB level
-- CHECK (balance >= 0)   → overdraft hard stop even if app logic fails

CREATE INDEX idx_wallets_user_id ON wallets(user_id);
```

### Table 3: `transfers`

```sql
-- 000003_create_transfers.up.sql
CREATE TYPE transfer_status AS ENUM ('pending', 'completed', 'failed', 'declined');

CREATE TABLE transfers (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    from_wallet_id  UUID            NOT NULL REFERENCES wallets(id),
    to_wallet_id    UUID            NOT NULL REFERENCES wallets(id),
    amount          BIGINT          NOT NULL CHECK (amount > 0),
    status          transfer_status NOT NULL DEFAULT 'pending',
    idempotency_key TEXT            NOT NULL,
    initiated_by    UUID            NOT NULL REFERENCES users(id),
    failure_reason  TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_different_wallets CHECK (from_wallet_id <> to_wallet_id)
);

CREATE INDEX idx_transfers_from_wallet  ON transfers(from_wallet_id);
CREATE INDEX idx_transfers_to_wallet    ON transfers(to_wallet_id);
CREATE INDEX idx_transfers_idem_key     ON transfers(idempotency_key, initiated_by);
CREATE INDEX idx_transfers_initiated_by ON transfers(initiated_by);
```

### Table 4: `idempotency_keys`

```sql
-- 000004_create_idempotency_keys.up.sql
CREATE TABLE idempotency_keys (
    idempotency_key TEXT        NOT NULL,
    user_id         UUID        NOT NULL REFERENCES users(id),
    request_hash    TEXT        NOT NULL,   -- SHA-256 of canonicalized request body
    transfer_id     UUID        REFERENCES transfers(id),
    response_status INT         NOT NULL,
    response_body   JSONB       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',

    PRIMARY KEY (idempotency_key, user_id)
);

CREATE INDEX idx_idem_keys_expires ON idempotency_keys(expires_at);
```

---

## Configuration

### `config.yaml` (committed to repo — no secrets)

```yaml
server:
  port: "8080"
  read_timeout: 30s
  write_timeout: 30s

database:
  host: "localhost"
  port: "5432"
  user: "wallet"
  password: "wallet_secret"
  name: "walletdb"
  sslmode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  max_lifetime: "5m"

auth:
  jwt_secret: "change-me-in-production"
  token_expiry_hours: 24

log:
  level: "info"    # debug | info | warn | error
  format: "json"   # json (prod) | text (dev)
```

### Environment variable overrides (Viper)

Prefix: `WALLET_`, separator: `_` replacing `.`

| Env var | Overrides config key |
|---|---|
| `WALLET_DATABASE_HOST` | `database.host` |
| `WALLET_DATABASE_PASSWORD` | `database.password` |
| `WALLET_AUTH_JWT_SECRET` | `auth.jwt_secret` |
| `WALLET_LOG_LEVEL` | `log.level` |

---

## API Reference

### Public Routes (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/register` | Register a new user |
| `POST` | `/auth/login` | Login → returns JWT bearer token |
| `GET`  | `/health` | Health check (used by Docker HEALTHCHECK) |
| `GET`  | `/` | Serves the single-page UI |

### Protected Routes (`Authorization: Bearer <token>` required)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/wallets` | Get-or-create wallet for the authenticated user |
| `GET`  | `/wallets/:id` | Get wallet balance (owner only) |
| `POST` | `/transfers` | Initiate a transfer |
| `GET`  | `/transfers/:id` | Get transfer status |

### Request / Response Shapes

**`POST /auth/register`**
```json
// Request
{ "username": "alice", "email": "alice@example.com", "password": "secret123" }
// Response 201
{ "id": "uuid", "username": "alice", "email": "alice@example.com" }
```

**`POST /auth/login`**
```json
// Request
{ "username": "alice", "password": "secret123" }
// Response 200
{ "token": "eyJhbGci..." }
```

**`POST /wallets`** (no request body)
```json
// Response 200 (existing) or 201 (newly created)
{ "id": "uuid", "user_id": "uuid", "balance": 1000000, "created_at": "..." }
```

**`GET /wallets/:id`**
```json
// Response 200
{ "id": "uuid", "user_id": "uuid", "balance": 950000, "updated_at": "..." }
// Response 403 — caller does not own this wallet
{ "error": "forbidden" }
```

**`POST /transfers`**
```json
// Request
{
  "from": "wallet-uuid",
  "to":   "wallet-uuid",
  "amount": 50000,
  "idempotency_key": "client-generated-unique-key"
}

// 201 Created — transfer completed
{
  "id": "transfer-uuid",
  "from_wallet_id": "...",
  "to_wallet_id": "...",
  "amount": 50000,
  "status": "completed",
  "created_at": "..."
}

// 200 OK — idempotent replay (same key, same body → cached response)
{ /* identical to 201 body */ }

// 402 Payment Required — declined (insufficient funds)
{ "error": "insufficient_funds", "transfer_id": "uuid", "status": "declined" }

// 409 Conflict — same idempotency_key, different body
{ "error": "idempotency_key_conflict" }

// 422 Unprocessable Entity — validation failure
{ "error": "amount must be greater than 0" }
```

**`GET /transfers/:id`**
```json
// Response 200
{
  "id": "transfer-uuid",
  "from_wallet_id": "...",
  "to_wallet_id": "...",
  "amount": 50000,
  "status": "completed",
  "failure_reason": null,
  "created_at": "...",
  "updated_at": "..."
}
```

---

## Transfer Algorithm (Critical Path)

```
POST /transfers
      │
      ▼
┌─────────────────────────────────────────┐
│ 1. Parse + validate request body        │
│    - amount > 0                         │
│    - from ≠ to                          │
│    - idempotency_key not empty          │
│    - caller owns the "from" wallet      │
└──────────────────┬──────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────┐
│ 2. Idempotency lookup                   │
│    SELECT * FROM idempotency_keys       │
│    WHERE idempotency_key=$1             │
│      AND user_id=$2                     │
└──────────────────┬──────────────────────┘
                   │
       ┌───────────┴────────────┐
    FOUND                   NOT FOUND
       │                    (proceed)
       ├─ request_hash matches? ──► 200 (cached response)
       └─ request_hash differs? ──► 409 Conflict
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ BEGIN TRANSACTION (READ COMMITTED)                           │
│                                                             │
│ 3. Acquire row locks in ascending UUID order                │
│    (prevents deadlocks on concurrent A↔B transfers):        │
│                                                             │
│    ids = sort([from_wallet_id, to_wallet_id])  -- ascending │
│    SELECT id, balance FROM wallets                          │
│    WHERE id IN ($1, $2) ORDER BY id FOR UPDATE              │
│                                                             │
│ 4. Overdraft check:                                         │
│    IF sender.balance < amount THEN                          │
│       INSERT INTO transfers (status='declined')             │
│       INSERT INTO idempotency_keys (response=402)           │
│       COMMIT   ← declined is a valid terminal state         │
│       RETURN 402                                            │
│                                                             │
│ 5. Debit sender:                                            │
│    UPDATE wallets                                           │
│    SET balance = balance - $amount, updated_at = NOW()      │
│    WHERE id = $from_id AND balance >= $amount               │
│    → RowsAffected = 0 means race-lost (retry / fail)        │
│                                                             │
│ 6. Credit receiver:                                         │
│    UPDATE wallets                                           │
│    SET balance = balance + $amount, updated_at = NOW()      │
│    WHERE id = $to_id                                        │
│                                                             │
│ 7. Record transfer:                                         │
│    INSERT INTO transfers (status='completed') RETURNING id  │
│                                                             │
│ 8. Record idempotency key:                                  │
│    INSERT INTO idempotency_keys (response_body, status=201) │
│                                                             │
│ COMMIT                                                      │
└─────────────────────────────────────────────────────────────┘
```

**Deadlock prevention explained:**
Two concurrent transfers A→B and B→A both sort their lock targets the same way — they both try to
lock `MIN(A_id, B_id)` first. The second transaction simply waits instead of creating a circular
dependency. No deadlock possible.

---

## Middleware Stack

```
Incoming request
       │
       ├─► Logger middleware
       │     Logs: request_id, method, path, status, latency_ms, ip
       │
       ├─► [Protected routes only] JWT middleware (gofiber/contrib/jwt)
       │     Validates Bearer token (HS256)
       │     Injects user_id into c.Locals("user_id")
       │     On failure → 401 { "error": "unauthorized" }
       │
       └─► Handler → Service → Repository → PostgreSQL
```

---

## Containerization

### `Dockerfile` — multi-stage, non-root user, HEALTHCHECK

```dockerfile
# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependency downloads separately from source code
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# Produce a statically linked binary (no CGO, stripped debug symbols)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o wallet-server ./cmd/server

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.20

# Create non-root user and group
RUN addgroup -S wallet && adduser -S wallet -G wallet

WORKDIR /app

COPY --from=builder /app/wallet-server .
COPY --from=builder /app/config.yaml   .
COPY --from=builder /app/web/          ./web/

RUN chown -R wallet:wallet /app

# Switch to non-root
USER wallet

EXPOSE 8080

# wget is available in alpine — hits the /health endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/wallet-server"]
```

> **Why `alpine:3.20` over `scratch`/`distroless`?**
> Alpine includes `wget` (needed for `HEALTHCHECK CMD`) and is still minimal (~5 MB).
> The `wallet` non-root user provides the security posture without losing debug ergonomics.

### `docker-compose.yml` — one command: `docker compose up --build`

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    container_name: wallet_postgres
    environment:
      POSTGRES_USER: wallet
      POSTGRES_PASSWORD: wallet_secret
      POSTGRES_DB: walletdb
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U wallet -d walletdb"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 10s
    restart: unless-stopped

  migrate:
    image: migrate/migrate:v4
    container_name: wallet_migrate
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ./migrations:/migrations
    command: >
      -path=/migrations
      -database=postgres://wallet:wallet_secret@postgres:5432/walletdb?sslmode=disable
      up
    restart: on-failure

  app:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: wallet_app
    depends_on:
      migrate:
        condition: service_completed_successfully
    ports:
      - "8080:8080"
    environment:
      WALLET_DATABASE_HOST: postgres
      WALLET_DATABASE_PASSWORD: wallet_secret
      WALLET_AUTH_JWT_SECRET: super-secret-change-me
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    restart: unless-stopped

volumes:
  pgdata:
```

**Startup order guaranteed:** `postgres` (healthy) → `migrate` (exit 0) → `app`

---

## Go Dependencies

```
github.com/gofiber/fiber/v2              # HTTP framework
github.com/gofiber/contrib/jwt           # JWT middleware for Fiber
github.com/golang-jwt/jwt/v5             # JWT token generation/parsing
github.com/jackc/pgx/v5                 # PostgreSQL driver + pgxpool
github.com/golang-migrate/migrate/v4     # Database migrations
  └─ database/postgres                   # Postgres driver for migrate
  └─ source/iofs                         # go:embed source for migrate
github.com/spf13/viper                   # Configuration management
github.com/google/uuid                   # UUID generation
golang.org/x/crypto                      # bcrypt for password hashing
go.uber.org/zap                            # High-performance structured logging
gorm.io/gorm                             # ORM framework
gorm.io/driver/postgres                  # Postgres driver for GORM
```

> **Logging:** `go.uber.org/zap` — JSON in prod, text in dev.

---

## UI (`web/index.html`)

Single HTML file with embedded CSS and vanilla JS. Served at `GET /` by GoFiber static handler.

**Sections:**

| Tab | Contents |
|---|---|
| **Auth** | Register form + Login form. JWT stored in `localStorage`. |
| **Wallet** | "Get or Create My Wallet" button. Shows wallet ID + balance in paise and ₹ equivalent. |
| **Transfer** | Form: from wallet ID, to wallet ID, amount (paise), idempotency key. Shows response (success / declined / conflict). |
| **Transfer Status** | Input: transfer ID. Shows full transfer record. |

All API calls use `fetch()` with `Authorization: Bearer <token>`.

---

## Logging Strategy

| Event | Level | Key Fields |
|---|---|---|
| Server start | `INFO` | `port`, `env` |
| DB connected | `INFO` | `host`, `db_name` |
| Migrations applied | `INFO` | `applied_count` |
| HTTP request | `INFO` | `request_id`, `method`, `path`, `status`, `latency_ms`, `ip` |
| Transfer completed | `INFO` | `transfer_id`, `from`, `to`, `amount`, `status` |
| Transfer declined | `WARN` | `transfer_id`, `reason: insufficient_funds` |
| Idempotency conflict | `WARN` | `key`, `user_id` |
| Auth failure | `WARN` | `username`, `ip`, `reason` |
| DB / internal error | `ERROR` | `error`, `stack` |

---

## Implementation Order

```
Step 1 :  go mod init github.com/anunay/wallet-service
Step 2 :  go get <all dependencies>
Step 3 :  config/config.go                 (Viper loader + Config structs)
Step 4 :  migrations/                      (4 × up.sql + 4 × down.sql)
Step 5 :  internal/db/db.go               (pgxpool setup)
Step 6 :  internal/db/migrate.go          (go:embed + golang-migrate runner)
Step 7 :  internal/domain/models.go       (domain types)
Step 8 :  internal/domain/errors.go       (sentinel errors)
Step 9 :  internal/repository/user.go
Step 10:  internal/repository/wallet.go
Step 11:  internal/repository/transfer.go (atomic transfer + idempotency)
Step 12:  internal/service/auth.go
Step 13:  internal/service/wallet.go
Step 14:  internal/service/transfer.go
Step 15:  internal/middleware/logger.go
Step 16:  internal/middleware/auth.go
Step 17:  internal/handler/health.go
Step 18:  internal/handler/auth.go
Step 19:  internal/handler/wallet.go
Step 20:  internal/handler/transfer.go
Step 21:  cmd/server/main.go              (wire all deps, register routes, start Fiber)
Step 22:  web/index.html                  (single-page UI)
Step 23:  Dockerfile
Step 24:  docker-compose.yml
Step 25:  config.yaml + .env.example
Step 26:  docker compose up --build       (smoke test)
```

---

## Verification Plan

### Automated smoke tests (curl)

```bash
# Bring up the full stack
docker compose up --build -d
docker compose ps   # wait for all services healthy

# 1. Register two users
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","email":"alice@test.com","password":"secret123"}' | jq

curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"bob","email":"bob@test.com","password":"secret123"}' | jq

# 2. Login and capture tokens
ALICE_TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}' | jq -r .token)

BOB_TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"bob","password":"secret123"}' | jq -r .token)

# 3. Get-or-create wallets (call twice — must return same wallet ID)
ALICE_WALLET=$(curl -s -X POST http://localhost:8080/wallets \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq -r .id)

BOB_WALLET=$(curl -s -X POST http://localhost:8080/wallets \
  -H "Authorization: Bearer $BOB_TOKEN" | jq -r .id)

# 4. Verify seed balances = 1,000,000 paise
curl -s "http://localhost:8080/wallets/$ALICE_WALLET" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .balance
# Expected: 1000000

# 5. Transfer 50,000 paise from Alice to Bob
curl -s -X POST http://localhost:8080/transfers \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"from\":\"$ALICE_WALLET\",\"to\":\"$BOB_WALLET\",\"amount\":50000,\"idempotency_key\":\"txn-001\"}" | jq
# Expected: 201, status=completed

# 6. Replay same request — must return identical response (no second debit)
curl -s -X POST http://localhost:8080/transfers \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"from\":\"$ALICE_WALLET\",\"to\":\"$BOB_WALLET\",\"amount\":50000,\"idempotency_key\":\"txn-001\"}" | jq
# Expected: 200, same transfer_id

# 7. Same key, different amount — must conflict
curl -s -X POST http://localhost:8080/transfers \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"from\":\"$ALICE_WALLET\",\"to\":\"$BOB_WALLET\",\"amount\":99999,\"idempotency_key\":\"txn-001\"}" | jq
# Expected: 409 { "error": "idempotency_key_conflict" }

# 8. Overdraft — must decline
curl -s -X POST http://localhost:8080/transfers \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"from\":\"$ALICE_WALLET\",\"to\":\"$BOB_WALLET\",\"amount\":9999999,\"idempotency_key\":\"txn-002\"}" | jq
# Expected: 402 { "error": "insufficient_funds" }

# 9. Conservation check — sum of all balances must equal N × 1,000,000
docker exec wallet_postgres psql -U wallet -d walletdb \
  -c "SELECT SUM(balance) FROM wallets;"
# Expected: 2000000 (two wallets × 1,000,000 − 0 net transfer)
```

### Manual UI verification checklist

- [ ] `http://localhost:8080` loads — UI renders with all 4 sections
- [ ] Register two users successfully
- [ ] Login as each user — token stored, subsequent calls authorized
- [ ] Each user gets-or-creates wallet; balance shows 1,000,000 paise (₹10,000.00)
- [ ] Alice transfers 50,000 paise to Bob — both balances update correctly
- [ ] Alice tries to overdraft — UI shows "insufficient funds" cleanly
- [ ] Replay same idempotency key — UI shows cached result (no double-debit)
- [ ] Same key + different amount — UI shows "conflict"
- [ ] `docker compose ps` — all 3 services show `healthy`
