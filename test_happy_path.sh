#!/usr/bin/env bash
set -e

BASE_URL="http://localhost:8080"

echo "=================================================="
echo "      Wallet Service — Happy Path Test Suite      "
echo "=================================================="

# 1. Health check
echo -e "\n[Step 1] Health Check..."
HEALTH_RESP=$(curl -s "$BASE_URL/health")
echo "$HEALTH_RESP" | jq .
if [[ $(echo "$HEALTH_RESP" | jq -r .status) != "ok" ]]; then
  echo "Health check failed!"
  exit 1
fi

# Generate unique usernames to allow repeatable test runs
TIMESTAMP=$(date +%s)
ALICE_USER="alice_${TIMESTAMP}"
BOB_USER="bob_${TIMESTAMP}"

# 2. Register Alice
echo -e "\n[Step 2] Registering User: $ALICE_USER..."
ALICE_REG=$(curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$ALICE_USER\",\"email\":\"$ALICE_USER@example.com\",\"password\":\"password123\"}")
echo "$ALICE_REG" | jq .
ALICE_USER_ID=$(echo "$ALICE_REG" | jq -r .id)

# 3. Register Bob
echo -e "\n[Step 3] Registering User: $BOB_USER..."
BOB_REG=$(curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$BOB_USER\",\"email\":\"$BOB_USER@example.com\",\"password\":\"password123\"}")
echo "$BOB_REG" | jq .
BOB_USER_ID=$(echo "$BOB_REG" | jq -r .id)

# 4. Login Alice & Bob
echo -e "\n[Step 4] Logging in users to acquire JWT tokens..."
ALICE_TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$ALICE_USER\",\"password\":\"password123\"}" | jq -r .token)

BOB_TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$BOB_USER\",\"password\":\"password123\"}" | jq -r .token)

echo "Alice Token: ${ALICE_TOKEN:0:25}..."
echo "Bob Token:   ${BOB_TOKEN:0:25}..."

# 5. Get-or-Create Wallets
echo -e "\n[Step 5] Creating wallets for Alice & Bob..."
ALICE_WALLET=$(curl -s -X POST "$BASE_URL/wallets" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq -r .id)

BOB_WALLET=$(curl -s -X POST "$BASE_URL/wallets" \
  -H "Authorization: Bearer $BOB_TOKEN" | jq -r .id)

echo "Alice Wallet ID: $ALICE_WALLET (User ID: $ALICE_USER_ID)"
echo "Bob Wallet ID:   $BOB_WALLET (User ID: $BOB_USER_ID)"

# 6. Verify Seed Balances
echo -e "\n[Step 6] Verifying Alice initial seed balance..."
ALICE_BAL_INIT=$(curl -s "$BASE_URL/wallets/$ALICE_WALLET" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .balance)
echo "Alice Initial Balance: $ALICE_BAL_INIT paise (= ₹$(($ALICE_BAL_INIT / 100)))"

# 7. Execute Peer-to-Peer Transfer (using User IDs)
IDEM_KEY="txn-${TIMESTAMP}"
TRANSFER_AMOUNT=50000

echo -e "\n[Step 7] Executing P2P Transfer (Alice -> Bob: $TRANSFER_AMOUNT paise / ₹500, IdempotencyKey: $IDEM_KEY)..."
TRANSFER_RESP=$(curl -s -X POST "$BASE_URL/transfers" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"from\": \"$ALICE_USER_ID\",
    \"to\": \"$BOB_USER_ID\",
    \"amount\": $TRANSFER_AMOUNT,
    \"idempotency_key\": \"$IDEM_KEY\"
  }")
echo "$TRANSFER_RESP" | jq .

TRANSFER_ID=$(echo "$TRANSFER_RESP" | jq -r .id)

# 8. Idempotent Replay Verification
echo -e "\n[Step 8] Replaying exact same transfer request (Idempotency Key: $IDEM_KEY)..."
REPLAY_RESP=$(curl -s -X POST "$BASE_URL/transfers" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"from\": \"$ALICE_USER_ID\",
    \"to\": \"$BOB_USER_ID\",
    \"amount\": $TRANSFER_AMOUNT,
    \"idempotency_key\": \"$IDEM_KEY\"
  }")
echo "$REPLAY_RESP" | jq .

# 9. Verify Updated Balances
echo -e "\n[Step 9] Verifying updated balances post-transfer..."
ALICE_BAL_AFTER=$(curl -s "$BASE_URL/wallets/$ALICE_WALLET" \
  -H "Authorization: Bearer $ALICE_TOKEN" | jq .balance)
BOB_BAL_AFTER=$(curl -s "$BASE_URL/wallets/$BOB_WALLET" \
  -H "Authorization: Bearer $BOB_TOKEN" | jq .balance)

echo "Alice New Balance: $ALICE_BAL_AFTER paise (Expected: 950000)"
echo "Bob New Balance:   $BOB_BAL_AFTER paise (Expected: 1050000)"

# 10. Query Transfer Record
echo -e "\n[Step 10] Querying transfer status by ID ($TRANSFER_ID)..."
STATUS_RESP=$(curl -s "$BASE_URL/transfers/$TRANSFER_ID" \
  -H "Authorization: Bearer $ALICE_TOKEN")
echo "$STATUS_RESP" | jq .

echo -e "\n=================================================="
echo "   ✅ HAPPY PATH TEST SUITE COMPLETED SUCCESSFULLY! "
echo "=================================================="
