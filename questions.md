# System Design Interview: Wallet Service (SDE2)

This document contains a curated list of system design interview questions, expected answers, and follow-up questions tailored for an SDE2 role, based on the architecture and design decisions of the `wallet-service` repository.

---

## 1. High-Level Architecture & Data Modeling

**Question:** 
*Design a peer-to-peer digital wallet service where users can register, view their balances, and transfer money to each other. What database would you choose, and what would your core schema look like?*

**Expected Answer:**
- **Database Choice:** A relational database like **PostgreSQL** is the best choice because financial systems require strict ACID properties, strong consistency, and complex transactional guarantees.
- **Core Schema:**
  - `users`: `id`, `username`, `password_hash`, `created_at`
  - `wallets`: `id`, `user_id` (FK to users), `balance` (BIGINT in lowest denomination like paise/cents), `updated_at`. Include a `CHECK (balance >= 0)` constraint.
  - `transfers`: `id`, `from_wallet_id`, `to_wallet_id`, `amount`, `status` (pending, completed, failed), `idempotency_key`, `created_at`.
  - `idempotency_keys`: `idempotency_key`, `user_id`, `request_hash`, `response_status`, `expires_at`.

**Follow-up Question:** 
*Why store the amount as `BIGINT` instead of `FLOAT` or `DECIMAL`?*
**Follow-up Answer:** 
Floating-point numbers can introduce precision errors during arithmetic operations due to how they are represented in memory (IEEE 754). For financial transactions, storing the value in its lowest indivisible unit (e.g., paise, cents) as an integer (`BIGINT`) guarantees perfect precision and avoids rounding errors.

---

## 2. Concurrency & Deadlock Prevention (Deep Dive)

**Question:** 
*Imagine User A and User B are transferring money to each other at the exact same time. How do you ensure the balances update correctly without race conditions, and how do you prevent the database from deadlocking?*

**Expected Answer:**
- **Race Conditions:** To prevent race conditions, we use pessimistic locking via `SELECT ... FOR UPDATE` on the wallet rows within a single database transaction. This ensures that only one transaction can modify a wallet's balance at a time.
- **Deadlock Prevention:** If Tx1 locks Wallet A then Wallet B, and Tx2 concurrently locks Wallet B then Wallet A, a deadlock occurs. To prevent this, the application must **sort the lock acquisition order deterministically**. For instance, always lock the wallet with the smaller UUID string first (`MIN(A, B)` then `MAX(A, B)`). This ensures all concurrent transactions traverse the locks in the exact same global order, making circular dependencies (and thus deadlocks) mathematically impossible.

**Follow-up Question:** 
*What happens to latency in this design if thousands of people are sending money to a single "hot wallet" (e.g., a popular merchant)?*
**Follow-up Answer:** 
Because of row-level locking, transfers to a hot wallet will be strictly serialized, leading to lock contention, queuing in the database, and increased API latency. 

**Follow-up (SDE2 Level):** *How would you solve the hot wallet problem?*
**Follow-up Answer:** 
To mitigate this, we could use an **Event Sourcing / Async Ledger** pattern. Instead of locking the merchant's wallet synchronously on every purchase, we validate the sender's balance, deduct it, and push a "Credit Merchant" event to a message broker (like Kafka). A background consumer then batches these credits and updates the merchant's wallet asynchronously. Alternatively, the merchant's balance could be sharded into multiple sub-wallets.

---

## 3. Idempotency & Network Failures

**Question:** 
*A user initiates a transfer, but their mobile network drops before they receive the HTTP response. The app automatically retries the request. How do you ensure the user isn't charged twice?*

**Expected Answer:**
- We implement **Idempotency** using an `Idempotency-Key` header provided by the client.
- We create an `idempotency_keys` table with a composite primary key `(idempotency_key, user_id)`.
- When a transfer request arrives, we check this table.
  - If the key **does not exist**, we proceed with the transfer, record the result (success/failure) in the `idempotency_keys` table, and return the response. All of this happens in the same ACID transaction.
  - If the key **exists**, we short-circuit and return the cached HTTP response and status code.

**Follow-up Question:** 
*What if a malicious user captures a successful idempotency key and replays it, but modifies the request body to send 10x the amount?*
**Follow-up Answer:** 
The `idempotency_keys` table must store a **cryptographic hash (e.g., SHA-256) of the request payload** (e.g., `from:to:amount`). When a retry comes in, we hash the new payload and compare it to the stored `request_hash`. If they mismatch, we reject the request with a `409 Conflict`.

---

## 4. Consistency vs. Availability (CAP Theorem)

**Question:** 
*This system favors strong consistency (CP). During a database failover, the system might reject writes. Why did you choose this over a highly available, eventually consistent approach (AP)?*

**Expected Answer:**
- Financial systems typically prioritize strict correctness over 100% availability. 
- In an AP system (e.g., Cassandra, or using async Sagas without locks), a user might be able to spend the same money twice (double-spending) during a partition or replication delay.
- By choosing a single PostgreSQL database with row-level locks (CP), we ensure the **Conservation of Money** invariant is never violated, and no overdrafts occur. Rejecting a write (downtime) is vastly preferable to losing money or creating inconsistent financial states.

**Follow-up Question:** 
*You mentioned alternatives like Redis Distributed Locks (Redlock) or Two-Phase Commit (2PC). Why were those rejected?*
**Follow-up Answer:** 
Redis distributed locks add unnecessary network latency and introduce split-brain risks. If a Redis node fails or GC pauses occur, a lock might expire prematurely, leading to race conditions. Since all data resides in a single PostgreSQL database, native RDBMS row-level locks are the simplest, safest, and most performant solution. Sagas/2PC are overkill for a single database and are only necessary when coordinating across multiple microservices/databases.

---

## 5. Scaling the Database (SDE2 Level)

**Question:** 
*The service has become incredibly popular. The `transfers` and `idempotency_keys` tables are growing by millions of rows a day, and the primary database disk is filling up. Query performance is degrading. How do you scale this architecture?*

**Expected Answer:**
1. **Time-Based Table Partitioning:** The `transfers` and `idempotency_keys` tables are append-only time-series data. I would partition them in PostgreSQL by time (e.g., monthly partitions for transfers, daily for idempotency keys). This keeps active indexes small and allows for efficient archiving/dropping of old data.
2. **TTL / Expiry:** For `idempotency_keys`, we only need them for a short window (e.g., 24-48 hours) to handle network retries. We can implement a background cron job to prune expired keys, or use Postgres partitioning to drop old partitions entirely.
3. **Read Replicas:** The system is likely read-heavy (users checking their balances frequently). I would spin up asynchronous Read Replicas. All `GET /wallets/:id` requests would be routed to the replicas, offloading CPU and IO from the primary writer node.
4. **Caching (Optional):** We could introduce a Redis cache for wallet balances. The cache would be updated transactionally (or via Change Data Capture like Debezium) to serve read queries instantly. However, we must accept eventual consistency for the UI if we do this.

---

## 6. Financial Invariants at the Database Level

**Question:** 
*Application bugs happen. How do you guarantee at the database level that a wallet's balance can never drop below zero, even if the application code accidentally tries to subtract too much?*

**Expected Answer:**
- During schema creation, apply a `CHECK` constraint: `CHECK (balance >= 0)`.
- If the application attempts an `UPDATE` that would result in a negative balance, PostgreSQL will instantly reject the transaction with a constraint violation error. This acts as a hard backstop against overdrafts regardless of application-level bugs.
- Furthermore, the application should do an atomic update: `UPDATE wallets SET balance = balance - amount WHERE id = ? AND balance >= amount`. This uses the database's concurrency control to ensure the balance is sufficient at the exact moment of the update.

---

## 7. Distributed Scaling & High Throughput Architecture (SDE3 / Advanced SDE2)

**Question:** 
*The single PostgreSQL node is reaching its CPU/Disk IO limits due to millions of daily transfers. Explain your roadmap for scaling this monolithic database architecture to a distributed, high-throughput system.*

**Expected Answer:**
To scale beyond a single database node, we would implement the following progression:

1. **Read/Write Split (Read Replicas):** 
   - Route all read queries (`GET /wallets/:id`, `GET /transfers/:id`) to asynchronous read replicas. This drastically reduces the load on the primary writer node.
2. **Caching Layer (Redis):**
   - Introduce Redis to cache wallet balances and transfer statuses. We would implement a cache-aside pattern or rely on Change Data Capture (CDC via Debezium) to invalidate/update cache entries. Note that the UI might experience slight eventual consistency.
3. **Database Sharding:**
   - Shard the `wallets` table by `user_id` (e.g., using consistent hashing).
   - **The Trade-off:** Cross-shard transfers (User A on Shard 1 transferring to User B on Shard 2) can no longer use a simple single-database ACID transaction. We would need to implement a **Two-Phase Commit (2PC)** or a **Saga Pattern** to coordinate the distributed transaction, which drastically increases complexity.
4. **Event-Driven Architecture (Kafka):**
   - For extreme throughput and solving the "hot wallet" (merchant) problem, we shift from synchronous row locking to an asynchronous event stream.
   - When a transfer initiates, synchronously validate and debit the sender's wallet.
   - Publish a `TransferInitiated` event to a message broker (e.g., Kafka / RabbitMQ).
   - A background consumer reliably processes this event to credit the receiver's wallet asynchronously. 

---

## 8. Fault Tolerance & Disaster Recovery Scenarios

**Question:** 
*In a distributed financial system, components will eventually fail. Discuss how your system handles the following fault scenarios without losing money or violating financial invariants.*

**Expected Answer:**

- **Scenario 1: Application Node Crashes Mid-Transaction**
  - *Resolution:* Because we use database-level ACID transactions, if the Go application panics or is killed (OOM) *before* issuing the `COMMIT`, PostgreSQL automatically rolls back the entire transaction. No partial debits occur.
  - *Client Impact:* The client receives a 5xx error or timeout. When they retry, the idempotency layer ensures the transaction is safely re-executed exactly once.

- **Scenario 2: Primary Database Node Fails (Hardware Crash)**
  - *Resolution:* We would run PostgreSQL in a High Availability (HA) cluster (e.g., using Patroni). If the primary goes down, a replica is automatically promoted to primary.
  - *Data Loss Consideration:* If replication is asynchronous, a failover might result in losing the last few milliseconds of transactions. For strict zero-data-loss, we must configure **synchronous replication**, which increases write latency but guarantees durability.

- **Scenario 3: Network Partition (Split-Brain) Between App and Database**
  - *Resolution:* Connection pools (like `pgxpool`) will timeout. We must implement **Circuit Breakers** in the app layer. If the DB is unreachable, the circuit trips, fast-failing incoming requests with `503 Service Unavailable` rather than holding HTTP connections open and cascading the failure to upstream load balancers.

- **Scenario 4: Cache (Redis) Node Fails (If introduced for scaling)**
  - *Resolution:* The application must fall back to querying the primary/replica database directly (Cache-Aside pattern). To prevent a "Cache Stampede" (where thousands of requests hit the DB simultaneously), we must implement request coalescing or singleflight mechanisms in the Go app.

- **Scenario 5: Message Broker Fails (If using Kafka for Async Ledger)**
  - *Resolution:* If Kafka is down, the system cannot publish the "Credit" event after deducting the sender. To fix this, we implement the **Transactional Outbox Pattern**. We write the event to an `outbox` table in PostgreSQL in the *same ACID transaction* as the sender's debit. A separate background worker (or CDC system) continuously polling the outbox table and guarantees at-least-once delivery to Kafka when it recovers.

- **Scenario 6: Concurrent Idempotency Race Condition (Current Implementation)**
  - *The Scenario:* A client's network hangs, so they retry the transfer. Both requests hit the backend at the exact same millisecond. Because neither transaction has committed yet, both `SELECT` checks for the idempotency key return "not found", and both attempt to execute the transfer.
  - *Resolution:* Our PostgreSQL database enforces a Unique Constraint on `(idempotency_key, user_id)`. The first transaction commits successfully. The second transaction will trigger a **PG Duplicate Key Error (SQLSTATE 23505)** upon insertion. The Go application catches this specific error, rolls back its redundant transaction, and retrieves the newly cached response from the first transaction to return to the user.

- **Scenario 7: Traffic Spike & Connection Exhaustion (Current Implementation)**
  - *The Scenario:* A massive spike in traffic causes hundreds of concurrent transfer requests. However, the database is configured with `max_open_conns: 25` to protect its memory.
  - *Resolution:* The Go `pgxpool` driver queues the requests in the application layer. If a request waits in the queue longer than the configured timeout, the application drops the request and returns a `503 Service Unavailable`. This "load shedding" prevents the database from crashing under pressure, sacrificing availability for a subset of users to maintain overall system stability and correctness.

---

## 9. Industry Standard Terminologies & Core Flows

**Question:** 
*Familiarity with financial domain concepts is crucial for building fintech systems. Define the core terminologies and lifecycle flows involved in a production-grade wallet service.*

**Expected Answer:**
A strong candidate should understand the following industry-standard concepts:

### Core Terminologies
1. **Ledger:** The append-only, immutable record of all financial transactions (debits and credits). In our system, the `transfers` table acts as a simplified ledger.
2. **Double-Entry Accounting:** A fundamental accounting principle where every transaction involves at least two accounts—a debit to one and an equal credit to another—ensuring the sum of all balances remains constant (Conservation of Money).
3. **Reconciliation:** The automated background process of comparing internal digital wallet balances against the actual fiat funds sitting in external banking partners/payment gateways to detect discrepancies or missing funds.
4. **KYC / AML:** Know Your Customer and Anti-Money Laundering. Regulatory requirements for verifying user identities before allowing large transfers or withdrawals.
5. **Escrow / Suspense Account:** A temporary holding wallet where funds are locked until certain conditions (like goods delivery) are met.
6. **Maker-Checker:** An authorization workflow (often used in B2B or high-value transfers) requiring one user to initiate a transaction ("maker") and a different authorized user to approve it ("checker").

### Core Financial Flows
1. **Pay-In / On-Ramping (Top-up):**
   - The flow of moving external fiat money (via credit card, ACH, or UPI) into the digital wallet ecosystem. This usually involves integrating with a Payment Gateway (Stripe, Razorpay).
2. **Pay-Out / Off-Ramping (Withdrawal):**
   - The flow of users withdrawing their digital wallet funds back to their traditional bank accounts. This often runs asynchronously in batches to optimize banking fees.
3. **P2P (Peer-to-Peer) Transfer:**
   - The internal movement of funds between two users strictly within our platform (the core feature of this repository). Because no external banks are queried mid-transaction, P2P transfers can be resolved instantly.
4. **Auth & Capture:**
   - A two-step transaction flow where funds are first temporarily locked/reserved ("Authorization") and later permanently deducted ("Capture") once a service is fulfilled (e.g., an Uber ride completion).

---

## 10. Observability, API Security, & Production Readiness

**Question:** 
*Once this wallet service is deployed to production, how do you secure it against abuse, monitor its health, and ensure your deployments don't break financial logic?*

**Expected Answer:**
A production-grade system requires robust security, observability, and testing strategies.

### A. API Security & Abuse Prevention
- **Authentication:** We use stateless JWTs (JSON Web Tokens) with a symmetric signature (HS256). Passwords are cryptographically hashed using `bcrypt` before database storage.
- **Why JWT over Sessions?** JWTs allow the backend to remain stateless, saving database/Redis lookups on every request, which is crucial for high-throughput APIs. 
- **Rate Limiting (Future Improvement):** To prevent brute-force attacks on `/auth/login` and spam on `/transfers`, we would implement a Token Bucket or Leaky Bucket rate limiter using Redis (e.g., max 10 transfers per minute per user).
- **Security Headers & Validation:** Strict input validation (e.g., checking for negative amounts before the DB does) and standard security headers (CORS, Helmet) mitigate common web vulnerabilities.

### B. Observability (Logging, Tracing, & Metrics)
- **Structured Logging:** We use high-performance structured JSON logging (Uber's Zap). Logs include key context like `transfer_id`, `user_id`, and `latency_ms`.
- **Distributed Tracing:** Every incoming request should be assigned an `X-Correlation-ID`. This ID is passed through middleware, injected into the logger context, and sent to downstream services. If a user complains about a failed transfer, we can search the logs using this ID to trace the exact lifecycle of their request.
- **Metrics (Prometheus):** We expose custom application metrics, such as:
  - `wallet_transfers_total` (Counter, partitioned by status: completed, declined, failed)
  - `wallet_http_request_duration_seconds` (Histogram for latency percentiles - P95, P99)
  - `idempotency_cache_hits_total` (To monitor network retry frequency)

### C. Testing Strategy
- **Unit Testing (Mocking):** Business logic (e.g., transfer validation) is unit tested by injecting mock implementations of the Repository interfaces. This allows for fast, isolated execution without a database.
- **Integration/E2E Testing (Ephemeral DBs):** Because our core logic relies on complex PostgreSQL features (row locks, constraints), mocking the DB is dangerous. We use `docker-compose.test.yml` to spin up an ephemeral, isolated PostgreSQL container, run migrations, execute real HTTP flows against the API, and tear down the container. This guarantees our locking logic actually works under concurrent load.
