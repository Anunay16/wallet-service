## 1. Conservation + No-Overdraft Mechanism:
- Mechanisms Used: Combines row-level `SELECT ... FOR UPDATE` locks, atomic conditional update (`UPDATE ... WHERE balance >= amount`), and database table `CHECK (balance >= 0)` constraints.
- Deadlock Avoidance: User IDs are sorted in ascending UUID string order prior to locking so all concurrent transactions acquire row locks in the exact same global order, making lock cycle dependencies impossible.
- Why Simplest & Correct: Guarantees strict row-level isolation and atomic balance mutations within a single ACID transaction without application-level retry overhead or serialization failures.
- Alternatives Rejected: Avoided Serializable isolation (frequent SQL 40001 retries), Redis Distributed lock (split-brain risks and external network overhead), and 2PC/Sagas (unnecessary for a single database).

## 2. Idempotency Location & Enforcement:
- Primary Location: Stored directly in the PostgreSQL database within the `idempotency_keys` table using a composite primary key `(idempotency_key, user_id)` and a 24-hour expiration timestamp (`expires_at`).
- Transactional Enforcement: Idempotency records are inserted and committed within the exact same database transaction as the sender debit, receiver credit, and transfer ledger audit record.
- Same-Key / Different-Body Replay: Evaluates a canonical SHA-256 request payload hash (`from:to:amount`); matching hashes return the cached response (200/201 replay), while mismatched hashes return HTTP 409 Conflict.
- Scalability Evaluation:
  * Current State & Trade-offs: Provides 100% atomic race-condition safety on single-node Postgres, but increases write IO load and table growth on the primary database.
  * Future Scaling Strategy: Scalable via periodic TTL deletion background jobs, table range partitioning on `expires_at`, or by offloading key reservation to Redis (SETNX locks + TTL cached response) with DB constraints as a safety net.

## 3. Consistency vs Availability:
- Architectural Choice (CP / Strong Consistency):
  * Strong ACID Guarantees: Prioritized absolute financial correctness, balance conservation, and zero overdrafts over high write availability.
  * Single Source of Truth: Enforced strict database isolation so that every debit/credit operation is immediately visible and consistently serialized across concurrent transactions.
- Trade-offs & Sacrifices Made:
  * Write Availability During Network Partitions: Sacrificed write availability during primary DB failovers or network partitions, choosing to reject writes rather than risk inconsistent balance updates.
  * Hot-Wallet Latency Under Contention: Accepted higher request latency under heavy contention because concurrent transfers targeting the same wallet serially queue via row-level locks.

## 4. Data Model:
- `users`: `id` (UUID PK), `username` (TEXT UNIQUE), `email` (TEXT UNIQUE), `password_hash`, timestamps.
- `wallets`: `id` (UUID PK), `user_id` (UUID UNIQUE FK -> users), `balance` (BIGINT, DEFAULT 1,000,000, CHECK balance >= 0), timestamps.
- `transfers`: `id` (UUID PK), `from_wallet_id` (FK), `to_wallet_id` (FK), `amount` (BIGINT > 0), `status` (ENUM: pending, completed, failed, declined), `idempotency_key`, `initiated_by` (FK), `failure_reason`, timestamps.
- `idempotency_keys`: `(idempotency_key, user_id)` (Composite PK), `request_hash` (TEXT), `transfer_id` (FK), `response_status` (INT), `response_body` (JSONB), `expires_at`, `created_at`.

## 5. Simplest Correct Mechanism & Rejected Alternatives:
- Simplest Mechanism Used:
  * Single Database ACID Transaction: Executes row-level `SELECT ... FOR UPDATE` locks ordered deterministically by ascending user UUID string to eliminate lock cycle deadlocks.
  * Atomic Debit & Database Constraints: Combines conditional update statements (`UPDATE wallets SET balance = balance - amount WHERE id = ? AND balance >= amount`) with table-level `CHECK (balance >= 0)` constraints and idempotency record persistence inside a single `BEGIN...COMMIT` block.
- Rejected Alternatives & Justification:
  * Serializable Isolation Level: Rejected because PostgreSQL serializable mode frequently aborts concurrent transactions with SQL state `40001` (serialization failure) under hot-wallet load, requiring expensive retry loops and increasing tail latency.
  * Distributed Locks (Redis Redlock / ZooKeeper): Rejected because acquiring external locks adds network latency roundtrips, split-brain failure risks, and infrastructure overhead when native RDBMS row locks already guarantee isolation.
  * Distributed Sagas / Two-Phase Commit (2PC): Rejected because all wallet accounts and transaction ledgers reside within a single PostgreSQL database. Implementing a Saga or 2PC adds unnecessary async state machines and eventual consistency windows.

## 6. AI Directed vs Decided:
- User Directed:
  * Technology Stack: Selected Go, PostgreSQL, JWT authentication, Fiber web framework (for HTTP routing and context management), and GORM as core technologies.
  * Username-Based API Contracts: Directed using human-readable `username` parameters for transfers and wallet lookups instead of exposing raw internal UUIDs to clients.
  * Isolated E2E Test Suite & Workflows: Required an isolated `e2e` test package with ephemeral containerized PostgreSQL (`docker-compose.test.yml`) and root `Makefile` targets (`make test-e2e`, `make test-e2e-remote`).
  * Deterministic Lock Order Strategy: Directed sorting user UUIDs in ascending order prior to row locking to eliminate deadlocks.
  * UUID & Auth Constraints: Required strict lowercase UUID normalization and explicit JWT claim structures (username and user_id).
  * Application Resilience & Tracing: Required Fiber recovery middleware to prevent crashes on panics and integrated `X-Correlation-ID` header tracing across middleware and Zap logger.
- LLM Decided:
  * Canonical Request Hashing: Built SHA-256 payload hashing (`from:to:amount`) to verify payload match on idempotent replays.
  * PG Duplicate Key (23505) Handling: Designed transaction fallback logic to recover cached idempotency records during concurrent race inserts.
  * Prometheus Metrics Collector: Implemented custom Prometheus metrics for tracking transfer creation, idempotency hits, and declines.
  * Test Suite & Mock Architecture: Created decoupled repository interfaces and mock implementations for comprehensive unit test coverage.

## 7. Tools & Infrastructure Used:
- Development Tools: Antigravity CLI.
- AI Models:
  * Claude Sonnet (via Antigravity): High-level architectural planning, design alignment, and E2E test setup.
  * Gemini 3.6 Flash: Core Go implementation, repository/service refactoring, bug fixes, and documentation.
- Deployment Platform: Render Free Tier (both Go service and PostgreSQL database deployed in Singapore region).

## 8. Pointers for Improvement:
1. Balance Top-Up Feature: Add a dedicated deposit API (`POST /api/v1/wallets/deposit`) to allow adding funds to a wallet without direct DB manipulation.
2. High-Traffic Distributed Architecture: Introduce event streaming (Kafka / RabbitMQ) for asynchronous audit logging and ledger processing, alongside Redis caching for read-heavy balance checks.
3. Database Partitioning & Sharding: Implement table partitioning on `transfers` and `idempotency_keys` by timestamp/range, and shard wallets by `user_id` with read-replicas for balance queries.
