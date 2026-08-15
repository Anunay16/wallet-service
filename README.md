# Wallet Service 💸

A high-performance, concurrent digital wallet microservice built in Go. The service handles user authentication, wallet creation, and financial transfers with strict guarantees around transactional integrity, idempotency, money conservation, and deadlock prevention under heavy concurrent load.

---

## 🚀 Features

- **User Authentication**: Secure user registration and login with bcrypt password hashing and JWT token authentication.
- **Race-Free Wallet Management**: Atomic wallet provisioning ensuring a single wallet per user under high concurrency.
- **Transactional Money Transfers**: Atomic, cross-wallet funds transfers with strict ACID guarantees.
- **Deadlock Prevention**: Deterministic row locking strategy (`MIN(from_id, to_id)` before `MAX(from_id, to_id)`) to eliminate database deadlocks during concurrent bi-directional transfers.
- **Idempotency Guarantee**: `Idempotency-Key` header handling prevents double debits/credits on network retries or concurrent duplicate requests.
- **Financial Invariants**:
  - **Conservation of Money**: Sum of balances before and after a transfer remains constant ($B_{from} + B_{to} = C$).
  - **No Overdrafts**: Guaranteed balance non-negativity enforced via database `CHECK (balance >= 0)` constraints and application logic.
- **Built-in Web UI**: Embedded web interface served directly by the application for managing accounts and transfers visually.

---

## 🛠️ Tech Stack

- **Language**: [Go 1.22+](https://go.dev/)
- **Web Framework**: [Fiber v2](https://gofiber.io/)
- **Database & ORM**: [PostgreSQL 16](https://www.postgresql.org/) with [GORM](https://gorm.io/)
- **Logging**: [Uber Zap](https://github.com/uber-go/zap)
- **Containerization**: Docker & Docker Compose

---

## 📚 Quick Documentation Links

- 📖 **[Local Setup & Development Guide](./SETUP.md)**: Instructions on prerequisites, running via Docker Compose, running locally, environment variables, and unit tests.
- 🧪 **[End-to-End Testing Guide](./E2E_TESTING.md)**: Details on the E2E test suite, running tests on ephemeral PostgreSQL, and concurrency scenario coverage.

---

## 🔌 API Reference

All protected endpoints require an `Authorization: Bearer <token>` HTTP header obtained via `/auth/login`.

### Public Endpoints

#### 1. Health Check
- **`GET /health`**
- **Response `200 OK`**:
  ```json
  {
    "status": "ok"
  }
  ```

#### 2. Register User
- **`POST /auth/register`**
- **Body**:
  ```json
  {
    "username": "alice",
    "email": "alice@example.com",
    "password": "password123"
  }
  ```
- **Response `201 Created`**:
  ```json
  {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "username": "alice",
    "email": "alice@example.com"
  }
  ```

#### 3. Login
- **`POST /auth/login`**
- **Body**:
  ```json
  {
    "username": "alice",
    "password": "password123"
  }
  ```
- **Response `200 OK`**:
  ```json
  {
    "token": "eyJhbGciOiJIUzI1Ni..."
  }
  ```

---

### Protected Endpoints (Requires `Authorization: Bearer <token>`)

#### 4. Get or Create Wallet
- **`POST /wallets`**
- **Description**: Returns existing wallet or creates one with an initial seed balance (10,000 paise / ₹100.00).
- **Response `200 OK` / `201 Created`**:
  ```json
  {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "username": "alice",
    "balance": 10000,
    "created_at": "2026-08-15T10:00:00Z",
    "updated_at": "2026-08-15T10:00:00Z"
  }
  ```

#### 5. Get Wallet Details
- **`GET /wallets/:id`**
- **Param**: `:id` (UUID string, e.g. `a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11`)
- **Response `200 OK`**:
  ```json
  {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "username": "alice",
    "balance": 8500,
    "created_at": "2026-08-15T10:00:00Z",
    "updated_at": "2026-08-15T10:05:00Z"
  }
  ```

#### 6. Initiate Transfer
- **`POST /transfers`**
- **Headers**:
  - `Authorization: Bearer <JWT_TOKEN>`
- **Body**:
  ```json
  {
    "to": "bob",
    "amount": 1500,
    "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
  }
  ```
  *(Note: `amount` is an integer representing monetary value in paise/cents, e.g., `1500` = ₹15.00)*
- **Response `201 Created`** (New transfer processed):
  ```json
  {
    "id": "c1f72a44-648b-4b4f-8f81-229b41d4a716",
    "from_wallet_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "to_wallet_id": "b2d3c4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
    "amount": 1500,
    "status": "completed",
    "created_at": "2026-08-15T10:05:00Z",
    "updated_at": "2026-08-15T10:05:00Z"
  }
  ```
- **Response `200 OK`** (Idempotent retry returns existing record):
  ```json
  {
    "id": "c1f72a44-648b-4b4f-8f81-229b41d4a716",
    "from_wallet_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "to_wallet_id": "b2d3c4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
    "amount": 1500,
    "status": "completed",
    "created_at": "2026-08-15T10:05:00Z",
    "updated_at": "2026-08-15T10:05:00Z"
  }
  ```

#### 7. Get Transfer Details
- **`GET /transfers/:id`**
- **Response `200 OK`**:
  ```json
  {
    "id": "c1f72a44-648b-4b4f-8f81-229b41d4a716",
    "from_wallet_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "to_wallet_id": "b2d3c4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
    "amount": 1500,
    "status": "completed",
    "created_at": "2026-08-15T10:05:00Z",
    "updated_at": "2026-08-15T10:05:00Z"
  }
  ```

---

## 🏛️ System Architecture & Financial Safety

```
                     ┌──────────────────┐
                     │    Client / UI   │
                     └────────┬─────────┘
                              │ HTTP (JWT + Idempotency Key)
                              ▼
                     ┌──────────────────┐
                     │   Fiber Server   │
                     └────────┬─────────┘
                              │
            ┌─────────────────┼─────────────────┐
            ▼                 ▼                 ▼
     ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
     │ Auth Service│   │Wallet Service│   │Transfer Svc │
     └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
            │                 │                 │
            └─────────────────┼─────────────────┘
                              ▼
                     ┌──────────────────┐
                     │  PostgreSQL DB   │
                     │ (Row-level Lock) │
                     └──────────────────┘
```

### Deterministic Lock Ordering
When transferring funds between Wallet A and Wallet B, the service acquires row locks (`SELECT ... FOR UPDATE`) in ascending order of Wallet IDs:
$$\text{Lock Order: } \min(A, B) \longrightarrow \max(A, B)$$
This prevents circular wait conditions and completely avoids database deadlocks even under heavy concurrent cross-transfers.

---

## 📁 Repository Structure

```
.
├── cmd/
│   └── server/          # Main application entry point
├── config/              # Configuration models & default config.yaml
├── e2e/                 # End-to-end test suite
├── internal/
│   ├── db/              # GORM database connection setup
│   ├── domain/          # Data models and error definitions
│   ├── handler/         # HTTP handlers (Auth, Wallet, Transfer, Health)
│   ├── middleware/      # Fiber middlewares (JWT Auth, Zap Logger)
│   ├── repository/      # GORM repositories with raw SQL/locking logic
│   ├── server/          # Fiber app routing & error handling setup
│   └── service/         # Business logic layer
├── migrations/          # SQL schema migrations
├── ui/                  # Web dashboard UI static files
├── docker-compose.yml   # Production-like docker compose setup
├── docker-compose.test.yml # Ephemeral test database setup
├── E2E_TESTING.md       # Detailed E2E test guide
├── Makefile             # Development & testing task runner
└── SETUP.md             # Local setup & running guide
```
