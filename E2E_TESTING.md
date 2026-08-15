# E2E Test Suite — Wallet Service

This document covers the **purpose**, **architecture**, **running procedure**, and **detailed scenario catalogue** for the end-to-end (e2e) test suite located in [`e2e/`](./e2e/).

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Prerequisites](#prerequisites)
4. [Running the Tests](#running-the-tests)
   - [One-command run (recommended)](#one-command-run-recommended)
   - [Manual DB control](#manual-db-control)
   - [Using a custom database](#using-a-custom-database)
   - [Environment variables reference](#environment-variables-reference)
5. [How TestMain Works](#how-testmain-works)
6. [Test Scenarios](#test-scenarios)
   - [TestHappyPath](#testhappypath)
   - [TestConservation](#testconservation)
   - [TestNoOverdraft](#testnoodraft)
   - [TestExactlyOnce](#testexactlyonce)
   - [TestRaceFreeGetOrCreate](#testracefreegeteorcreate)
   - [TestEdgeCases](#testedgecases)
7. [Helper Utilities](#helper-utilities)
8. [Test Isolation Strategy](#test-isolation-strategy)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The e2e suite validates the **four core financial invariants** of the wallet service plus a complete happy-path smoke test:

| Invariant | What can go wrong without it |
|---|---|
| **Conservation** | Money created or destroyed during a transfer |
| **No Overdraft** | Balance goes negative; database `CHECK` constraint bypassed |
| **Exactly-Once** | Same transfer applied twice; duplicate debit |
| **Race-Free Wallet Creation** | Two wallets created for one user under concurrent load |

Tests are written in standard Go (`testing` package), talk to a **real running HTTP server** over localhost, and use a **dedicated, ephemeral Postgres database** that is destroyed at the end of every run.

---

## Architecture

```
┌─────────────────── go test ./e2e/... ───────────────────┐
│                                                          │
│  TestMain (setup_test.go)                             │
│  ├── docker-compose.test.yml → postgres-test :5433       │
│  ├── db.RunMigrations(dsn)   → applies all 4 SQL files   │
│  ├── db.NewGormDB(cfg)       → GORM connection pool      │
│  ├── server.InitializeServer → same Fiber app as prod    │
│  └── App.Listen(:<free port>) → goroutine                │
│                                                          │
│  Test functions (e2e_test.go)                            │
│  └── HTTP client → localhost:<free port>                 │
│                          │                               │
│                          ▼                               │
│               ┌──────────────────┐                       │
│               │  Fiber app       │                       │
│               │  (in-process)    │                       │
│               └────────┬─────────┘                       │
│                        │ GORM / sql                      │
│                        ▼                                 │
│               ┌──────────────────┐                       │
│               │ wallet_postgres   │                       │
│               │ _test  :5433     │                       │
│               │ (tmpfs, ephemeral)│                      │
│               └──────────────────┘                       │
└──────────────────────────────────────────────────────────┘
```

Key design choices:

- **In-process server** — the Fiber app runs inside the same `go test` binary. No Docker build required for the app; stack traces are fully visible in test output.
- **Port 5433 for DB** — never conflicts with a running dev Postgres on the default port 5432.
- **`tmpfs` volume** — the test database lives entirely in RAM; it is wiped the instant the container is removed.
- **Random free app port** — the OS picks an available port, so the suite can run alongside a live dev server on 8080.
- **Fresh users per test** — every test registers uniquely named accounts (`prefix_<nanosecond timestamp>`), so tests are fully independent and the suite can be run repeatedly without any cleanup.

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | ≥ 1.21 | `go test` runner |
| Docker Desktop | any recent | Needed for the Postgres test container |
| `docker compose` | v2 CLI plugin | Ships with Docker Desktop |

Verify your setup:

```bash
go version          # should print go1.21 or later
docker compose version
```

---

## Running the Tests

### One-command run (recommended)

```bash
make test-e2e
```

This single target:
1. Starts the ephemeral Postgres container and waits until it is healthy.
2. Runs the full e2e suite in-process.
3. Tears the container down on exit (pass or fail).

**Run a single named scenario:**

```bash
make test-e2e RUN=TestConservation
make test-e2e RUN=TestExactlyOnce
make test-e2e RUN=TestNoOverdraft/concurrent_overdraft_storm
```

---

### Manual DB control

Useful when iterating quickly — keep the DB up between test runs to avoid the container startup penalty (~3 s).

```bash
# Start the test DB once
make test-e2e-db-up

# Iterate freely
go test ./e2e/... -v -run TestHappyPath
go test ./e2e/... -v -run TestConservation
go test ./e2e/... -v -count=1 -timeout 120s

# Tear it down when done
make test-e2e-db-down
```

---

### Using a custom database

Point the suite at any Postgres instance by setting `E2E_DSN`:

```bash
E2E_DSN=postgres://myuser:mypass@myhost:5432/mydb?sslmode=disable \
  go test ./e2e/... -v -timeout 120s
```

> **Note:** Goose migrations are idempotent — if the target database is already migrated, `RunMigrations` is a safe no-op.

---

### Environment variables reference

| Variable | Default | Description |
|---|---|---|
| `E2E_DSN` | `postgres://wallet_test:wallet_test_secret@localhost:5433/walletdb_test?sslmode=disable` | Full Postgres DSN for the test database |
| `E2E_PORT` | *(random free port)* | Force the in-process Fiber app to listen on a specific port |
| `BASE_URL` | Set automatically by `TestMain` | Override target server (e.g. point at a remote staging server) |

---

## How TestMain Works

`e2e/setup_test.go` implements Go's `TestMain(m *testing.M)` hook, which runs **once** before all tests in the package.

```
TestMain
 │
 ├─ bootServer()
 │   ├─ Read E2E_DSN  (or use default localhost:5433 DSN)
 │   ├─ Read E2E_PORT (or ask OS for a free port)
 │   ├─ Build a silent zap.Logger (WARN level — no noise in test output)
 │   ├─ db.RunMigrations(dsn)         ← Goose applies all 4 SQL migrations
 │   ├─ db.NewGormDB(cfg)             ← open GORM connection pool
 │   ├─ server.InitializeServer(...)  ← wire repos → services → handlers → Fiber
 │   ├─ go App.Listen(":"+port)       ← start in background goroutine
 │   └─ waitForPort(port, 5s)        ← poll until TCP handshake succeeds
 │
 ├─ os.Setenv("BASE_URL", "http://localhost:"+port)
 │   └─ All test helpers call baseURL() which reads this env var
 │
 ├─ m.Run()   ← executes every Test* function
 │
 └─ cleanup()
     ├─ App.ShutdownWithTimeout(5s)
     └─ sqlDB.Close()
```

---

## Test Scenarios

### TestHappyPath

**File:** `e2e/e2e_test.go` · **Tag:** smoke test

A full end-to-end walk-through of the primary user journey.

**Steps:**
1. Register Alice and Bob via `POST /auth/register`.
2. Login both users and acquire JWT tokens via `POST /auth/login`.
3. Create wallets for both users via `POST /wallets`.
4. Record the initial balances.
5. Alice sends Bob `10,000 paise` (₹100) via `POST /transfers`.
6. Assert the transfer response contains `"status": "completed"`.
7. Assert Alice's balance decreased by exactly `10,000`.
8. Assert Bob's balance increased by exactly `10,000`.
9. Fetch the transfer record via `GET /transfers/:id` and assert its status is `"completed"`.

**Pass criteria:**
- All HTTP calls return expected 2xx status codes.
- Balance arithmetic is exact.
- Transfer record is retrievable and reflects the correct state.

---

### TestConservation

**File:** `e2e/e2e_test.go` · **Tag:** financial invariant

> *The sum of all wallet balances never changes across a transfer, even under concurrent transfers touching the same wallets.*

**Setup:** Three users — Alice, Bob, Carol — each with their seed balance.

**Strategy:**
- Snapshot the total: `totalBefore = Alice + Bob + Carol`.
- Fire **10 goroutines**, each launching 3 concurrent transfers simultaneously:
  - Alice → Bob (`500 paise`)
  - Bob → Carol (`500 paise`)
  - Carol → Alice (`500 paise`)
- Wait for all 30 goroutines to finish.
- Re-snapshot: `totalAfter = Alice + Bob + Carol`.

**Pass criteria:**
- `totalAfter == totalBefore` (conservation holds regardless of which transfers succeeded or failed due to insufficient funds).

**What it catches:**
- Partial updates where the debit is applied but the credit is not (or vice versa).
- Non-atomic balance updates susceptible to lost updates under concurrency.

---

### TestNoOverdraft

**File:** `e2e/e2e_test.go` · **Tag:** financial invariant

> *A wallet balance never goes negative. A debit that would overdraw must fail cleanly, not partially apply.*

#### Sub-test: `single_overdraft_declined`

- Alice tries to send `aliceBalance + 1 paise` (one paise more than she has).
- The transfer response must **not** have `"status": "completed"`.
- Alice's balance must remain **unchanged** after the attempt.

#### Sub-test: `concurrent_overdraft_storm`

- **20 goroutines** each attempt to send Alice's **entire balance** to Bob simultaneously.
- Only **at most one** transfer may complete.
- Alice's balance must be **≥ 0** after all goroutines finish.

**What it catches:** Race conditions in the balance-check-then-debit path that could allow two concurrent transactions to both pass the funds check before either commits.

> The database schema reinforces this with a `CHECK (balance >= 0)` constraint on the `wallets` table, providing a second line of defence.

#### Sub-test: `zero_amount_rejected`

- A transfer with `amount: 0` must be rejected (non-2xx or `status ≠ completed`).

#### Sub-test: `negative_amount_rejected`

- A transfer with `amount: -500` must be rejected (non-2xx or `status ≠ completed`).

---

### TestExactlyOnce

**File:** `e2e/e2e_test.go` · **Tag:** financial invariant

> *Re-sending the same `idempotency_key` applies the transfer once. A retry returns the original result. A reused key with a different body is a conflict (409).*

#### Sub-test: `retry_returns_cached_response`

1. Alice sends Bob `1,000 paise` with `idempotency_key = "idem-retry-<nano>"`.
2. The same request is sent **a second time** with identical body and key.
3. Both responses must return `2xx` **and reference the same `transfer.id`**.
4. Alice's balance must reflect exactly **one debit** of `1,000`, not two.
5. Bob's balance must reflect exactly **one credit** of `1,000`, not two.

#### Sub-test: `different_body_same_key_is_409_conflict`

- First call: `Alice → Bob, 500 paise, key="conflict-<nano>"` → succeeds.
- Second call: **same key**, but `amount: 999` → must return **409 Conflict**.
- Third call: **same key**, but different `to` recipient (Carol) → must return **409 Conflict**.

#### Sub-test: `concurrent_same_key_exactly_one_debit`

- **15 goroutines** all send the **identical request** (same body, same key) simultaneously.
- All responses that carry a `transfer.id` must reference **the same ID**.
- Balance arithmetic must show exactly **one debit** from Alice and **one credit** to Bob.

#### Sub-test: `missing_idempotency_key_rejected`

- A transfer submitted with `"idempotency_key": ""` must be rejected (non-2xx).

---

### TestRaceFreeGetOrCreate

**File:** `e2e/e2e_test.go` · **Tag:** financial invariant

> *Two concurrent `POST /wallets` for the same user yield one wallet, not two.*

#### Sub-test: `concurrent_wallet_creation_single_result`

- **20 goroutines** call `POST /wallets` for the same authenticated user simultaneously.
- Every response must contain the **same `wallet.id`**.

**What it catches:** A non-atomic get-or-create pattern that could insert two rows for the same `user_id` under a race condition.

> The database schema enforces `user_id UUID NOT NULL UNIQUE` on the `wallets` table as a hard constraint.

#### Sub-test: `sequential_repeated_calls_idempotent`

- Three sequential `POST /wallets` calls for the same user must all return the **same wallet ID**.

#### Sub-test: `different_users_get_different_wallets`

- Two different users each call `POST /wallets`; they must receive **distinct wallet IDs**.

#### Sub-test: `wallet_get_by_id_forbids_foreign_access`

- User B calls `GET /wallets/<User A's wallet ID>`.
- Must return **403 Forbidden**.

#### Sub-test: `unauthenticated_wallet_creation_rejected`

- `POST /wallets` with no `Authorization` header must return **401 Unauthorized**.

#### Sub-test: `invalid_jwt_wallet_creation_rejected`

- `POST /wallets` with a malformed JWT string must return **401 Unauthorized**.

---

### TestEdgeCases

**File:** `e2e/e2e_test.go` · **Tag:** guard rails

A collection of boundary conditions and access-control checks.

| Sub-test | Scenario | Expected |
|---|---|---|
| `health_check` | `GET /health` | `200 {"status":"ok"}` |
| `self_transfer_rejected` | Alice transfers to herself | Rejected (non-completed) |
| `transfer_to_nonexistent_user_returns_404` | Transfer to `"ghost_user_xyz_987654"` | `404 Not Found` |
| `transfer_from_another_user_forbidden` | Alice initiates transfer with `from: Bob` (impersonation) | `403 Forbidden` |
| `get_transfer_by_id_forbids_stranger` | Eve reads a transfer between Alice and Bob | `403 Forbidden` |
| `get_wallet_invalid_uuid_returns_400` | `GET /wallets/not-a-valid-uuid` | `400 Bad Request` |
| `get_transfer_invalid_uuid_returns_400` | `GET /transfers/not-a-valid-uuid` | `400 Bad Request` |
| `register_duplicate_username_returns_409` | Register the same username twice | `409 Conflict` |
| `login_wrong_password_returns_401` | Login with incorrect password | `401 Unauthorized` |

---

## Helper Utilities

All helpers live at the top of `e2e/e2e_test.go` and are shared across every test function.

| Helper | Signature | Purpose |
|---|---|---|
| `baseURL()` | `→ string` | Returns `BASE_URL` env var or `http://localhost:8080` |
| `uniqueUser(prefix)` | `→ string` | Returns `"prefix_<nanosecond>"` for collision-free usernames |
| `registerAndLogin(t, username)` | `→ userID, token` | `POST /auth/register` then `POST /auth/login`; fatal on error |
| `createWallet(t, token)` | `→ walletID, balance` | `POST /wallets`; fatal on error |
| `getWalletBalance(t, token, walletID)` | `→ int64` | `GET /wallets/:id`; returns current balance in paise |
| `transfer(t, token, from, to, amount, ikey)` | `→ status, []byte` | `POST /transfers`; returns raw status code and body |
| `jsonRequest(t, method, url, body, token)` | `→ *http.Request` | Builds a JSON request with optional Bearer token |
| `decodeJSON(t, resp, wantStatus, dst)` | — | Reads body, asserts status, unmarshals JSON; fatal on mismatch |
| `rawDecode(t, resp)` | `→ status, []byte` | Reads body without asserting status (for negative tests) |

---

## Test Isolation Strategy

Each test function is fully self-contained:

1. **Unique users per test** — usernames include `time.Now().UnixNano()`, guaranteeing no collision even when the full suite runs concurrently (via `go test -parallel`).
2. **Unique idempotency keys per call** — every transfer call generates a fresh key from the current nanosecond timestamp and a loop index.
3. **No shared mutable state** — test functions never read or write to a global variable that another test also modifies.
4. **No manual cleanup required** — the ephemeral Postgres container is destroyed after each full run; there is no "reset database" step between individual tests.

---

## Troubleshooting

**`connection refused` on test start**

The Postgres container may not be ready. Run `make test-e2e-db-up` manually and check:

```bash
docker logs wallet_postgres_test
```

**`migrations failed`**

Usually means the DSN is wrong or the DB user lacks `CREATE TABLE` permission. Verify:

```bash
psql postgres://wallet_test:wallet_test_secret@localhost:5433/walletdb_test
```

**Tests pass locally but fail in CI**

Set `E2E_DSN` explicitly in your CI pipeline and ensure port 5433 is not blocked. Example GitHub Actions step:

```yaml
- name: Start test DB
  run: docker compose -f docker-compose.test.yml up -d --wait

- name: Run e2e tests
  run: go test ./e2e/... -v -count=1 -timeout 120s
```

**Specific test is flaky**

Run it with `-count=5` to detect intermittent failures:

```bash
go test ./e2e/... -v -run TestConservation -count=5
```

**Want verbose SQL logs**

Change the logger level in `setup_test.go`:

```go
zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
```

and set `logger.Default.LogMode(logger.Info)` in `db.NewGormDB`.
