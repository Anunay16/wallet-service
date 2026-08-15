# Wallet Service — Happy Path Testing Guide

This document outlines the complete step-by-step Happy Path testing flow for the Wallet Service.

---

## Environment Prerequisites

Start the containerized stack before testing:
```bash
docker compose up --build -d
```

Service Base URL: `http://localhost:8080`

---

## Step-by-Step Happy Path Test Execution

### Step 1: Health Check
Verify that the service is running and the database connection pool is healthy.

* **HTTP Method**: `GET`
* **Path**: `/health`
* **Headers**: None
* **Expected Response Code**: `200 OK`
* **Expected Body**:
```json
{
  "status": "ok"
}
```

---

### Step 2: User Registration (Alice & Bob)
Create two user accounts to test peer-to-peer fund transfers.

#### 2a. Register User "Alice"
* **HTTP Method**: `POST`
* **Path**: `/auth/register`
* **Headers**: `Content-Type: application/json`
* **Body**:
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "password123"
}
```
* **Expected Response Code**: `201 Created`
* **Expected Body**:
```json
{
  "id": "<ALICE_USER_UUID>",
  "username": "alice",
  "email": "alice@example.com"
}
```

#### 2b. Register User "Bob"
* **HTTP Method**: `POST`
* **Path**: `/auth/register`
* **Headers**: `Content-Type: application/json`
* **Body**:
```json
{
  "username": "bob",
  "email": "bob@example.com",
  "password": "password123"
}
```
* **Expected Response Code**: `201 Created`
* **Expected Body**:
```json
{
  "id": "<BOB_USER_UUID>",
  "username": "bob",
  "email": "bob@example.com"
}
```

---

### Step 3: User Login & JWT Token Retrieval
Authenticate each user to obtain JWT bearer tokens for protected endpoints.

#### 3a. Login as Alice
* **HTTP Method**: `POST`
* **Path**: `/auth/login`
* **Headers**: `Content-Type: application/json`
* **Body**:
```json
{
  "username": "alice",
  "password": "password123"
}
```
* **Expected Response Code**: `200 OK`
* **Expected Body**:
```json
{
  "token": "<ALICE_JWT_TOKEN>"
}
```

#### 3b. Login as Bob
* **HTTP Method**: `POST`
* **Path**: `/auth/login`
* **Headers**: `Content-Type: application/json`
* **Body**:
```json
{
  "username": "bob",
  "password": "password123"
}
```
* **Expected Response Code**: `200 OK`
* **Expected Body**:
```json
{
  "token": "<BOB_JWT_TOKEN>"
}
```

---

### Step 4: Wallet Initialization
Get or create wallets for both users. Each new wallet is seeded with 1,000,000 paise (₹10,000.00).

#### 4a. Get/Create Alice's Wallet
* **HTTP Method**: `POST`
* **Path**: `/wallets`
* **Headers**: `Authorization: Bearer <ALICE_JWT_TOKEN>`
* **Expected Response Code**: `200 OK` or `201 Created`
* **Expected Body**:
```json
{
  "id": "<ALICE_WALLET_UUID>",
  "user_id": "<ALICE_USER_UUID>",
  "balance": 1000000,
  "created_at": "...",
  "updated_at": "..."
}
```

#### 4b. Get/Create Bob's Wallet
* **HTTP Method**: `POST`
* **Path**: `/wallets`
* **Headers**: `Authorization: Bearer <BOB_JWT_TOKEN>`
* **Expected Response Code**: `200 OK` or `201 Created`
* **Expected Body**:
```json
{
  "id": "<BOB_WALLET_UUID>",
  "user_id": "<BOB_USER_UUID>",
  "balance": 1000000,
  "created_at": "...",
  "updated_at": "..."
}
```

---

### Step 5: Execute Peer-to-Peer Transfer
Alice initiates a transfer of 50,000 paise (₹500.00) to Bob.

* **HTTP Method**: `POST`
* **Path**: `/transfers`
* **Headers**:
  * `Authorization: Bearer <ALICE_JWT_TOKEN>`
  * `Content-Type: application/json`
* **Body**:
```json
{
  "from": "alice",
  "to": "bob",
  "amount": 50000,
  "idempotency_key": "txn-happy-001"
}
```
* **Expected Response Code**: `201 Created`
* **Expected Body**:
```json
{
  "id": "<TRANSFER_UUID>",
  "from_wallet_id": "<ALICE_WALLET_UUID>",
  "to_wallet_id": "<BOB_WALLET_UUID>",
  "amount": 50000,
  "status": "completed",
  "created_at": "...",
  "updated_at": "..."
}
```

---

### Step 6: Verify Exactly-Once Idempotency Replay
Re-send the exact same transfer request with the identical `idempotency_key` (`txn-happy-001`).

* **HTTP Method**: `POST`
* **Path**: `/transfers`
* **Headers**:
  * `Authorization: Bearer <ALICE_JWT_TOKEN>`
  * `Content-Type: application/json`
* **Body**:
```json
{
  "from": "alice",
  "to": "bob",
  "amount": 50000,
  "idempotency_key": "txn-happy-001"
}
```
* **Expected Response Code**: `200 OK` (Replay cached response)
* **Expected Body**: Identical to Step 5 (`<TRANSFER_UUID>`). No second debit occurs.

---

### Step 7: Verify Post-Transfer Balances
Confirm that balances were accurately updated for both sender and receiver.

#### 7a. Query Alice's Wallet Balance
* **HTTP Method**: `GET`
* **Path**: `/wallets/<ALICE_WALLET_UUID>`
* **Headers**: `Authorization: Bearer <ALICE_JWT_TOKEN>`
* **Expected Response Code**: `200 OK`
* **Expected Balance**: `950000` paise (1,000,000 − 50,000 = ₹9,500.00)

#### 7b. Query Bob's Wallet Balance
* **HTTP Method**: `GET`
* **Path**: `/wallets/<BOB_WALLET_UUID>`
* **Headers**: `Authorization: Bearer <BOB_JWT_TOKEN>`
* **Expected Response Code**: `200 OK`
* **Expected Balance**: `1050000` paise (1,000,000 + 50,000 = ₹10,500.00)

---

### Step 8: Query Transfer Record by ID
Fetch the final transaction status record using the generated transfer ID.

* **HTTP Method**: `GET`
* **Path**: `/transfers/<TRANSFER_UUID>`
* **Headers**: `Authorization: Bearer <ALICE_JWT_TOKEN>`
* **Expected Response Code**: `200 OK`
* **Expected Body**:
```json
{
  "id": "<TRANSFER_UUID>",
  "from_wallet_id": "<ALICE_WALLET_UUID>",
  "to_wallet_id": "<BOB_WALLET_UUID>",
  "amount": 50000,
  "status": "completed",
  "failure_reason": null,
  "created_at": "...",
  "updated_at": "..."
}
```

---

## Automated Script Alternative

To run this complete test suite automatically, execute:
```bash
./test_happy_path.sh
```
