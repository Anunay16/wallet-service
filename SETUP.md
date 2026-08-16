# Local Setup & Development Guide

This guide provides instructions for setting up, running, and testing the **Wallet Service** on your local machine.

---

## Prerequisites

Before starting, ensure you have the following installed on your machine:

- **Go**: `1.22` or later (with CGO disabled or race detector support)
- **Docker & Docker Compose**: Docker Desktop / Docker Engine version `20.10+`
- **Make**: (Optional) For executing Makefile target shortcuts
- **PostgreSQL 16**: (Optional) Only required if running the application without Docker

---

## Quick Start (Docker Compose)

The fastest way to get the service and database running is using Docker Compose.

### 1. Launch Service & Database

Run the following command from the repository root:

```bash
make build-and-run
```

*Or manually using Docker Compose:*

```bash
docker compose up --build -d
```

This will:
1. Start a **PostgreSQL 16** container listening on port `5432`.
2. Automatically execute database schema migrations on container initialization.
3. Build and start the **Wallet Service** HTTP container listening on port `8080`.

### 2. Verify Service Health

Check if the application is healthy by running:

```bash
curl http://localhost:8080/health
```

Expected Response:
```json
{
  "status": "ok"
}
```

### 3. Open Web UI

Navigate to `http://localhost:8080` in your web browser to access the built-in web frontend for managing accounts, wallets, and executing transfers.

### 4. Stop Service

To stop and clean up containers:

```bash
docker compose down --remove-orphans
```

---

## Manual Local Setup (Without Docker App Container)

If you prefer running the Go application natively while using Docker for PostgreSQL:

### Step 1: Start Postgres Database

Start only the Postgres database container:

```bash
docker compose up -d postgres
```

### Step 2: Set Environment Variables (Optional)

Default configurations are loaded from `config.yaml`. You can override defaults using environment variables:

| Environment Variable | Description | Default |
|---|---|---|
| `WALLET_SERVER_PORT` | HTTP server port | `8080` |
| `WALLET_DATABASE_HOST` | Postgres hostname | `localhost` |
| `WALLET_DATABASE_PORT` | Postgres port | `5432` |
| `WALLET_DATABASE_USER` | Postgres user | `wallet` |
| `WALLET_DATABASE_PASSWORD` | Postgres password | `wallet_secret` |
| `WALLET_DATABASE_NAME` | Database name | `walletdb` |
| `WALLET_AUTH_JWT_SECRET` | Secret key for signing JWTs | `change-me-in-production` |

### Step 3: Run the Application

```bash
go run ./cmd/server/main.go
```

The server starts on `http://localhost:8080`.

---

## Running Unit Tests

Run unit tests with race condition detection across all internal packages:

```bash
make test-unit
```

*Or directly with Go:*

```bash
go test -race -v -count=1 ./internal/handler/... ./internal/service/... ./internal/repository/...
```

---

## Running End-to-End (E2E) Tests

To run the full E2E suite locally with one command (boots test DB, runs migrations, executes tests, tears down DB):

```bash
make test-e2e
```

To run the E2E suite against the live remote deployment:

```bash
make test-e2e-remote BASE_URL=https://wallet-service-irkk.onrender.com
```

> ⚠️ **Note**: Running tests against the remote server populates the database with test records. On subsequent test runs, the remote database may contain stale test data from previous runs.

For complete details on E2E test scenarios and isolated test runner execution, refer to [E2E_TESTING.md](./E2E_TESTING.md).
