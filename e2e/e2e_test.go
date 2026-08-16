// Package e2e contains end-to-end tests for the wallet service.
//
// Run against a live server (default http://localhost:8080):
//
//	go test ./e2e/... -v -timeout 120s
//	BASE_URL=http://localhost:9090 go test ./e2e/... -v
//
// Each top-level Test* function is a self-contained scenario that registers
// fresh users (timestamped to avoid collisions across repeated runs) and
// asserts invariants after every operation.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration & helpers
// ---------------------------------------------------------------------------

func baseURL() string {
	if u := os.Getenv("BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

var client = &http.Client{Timeout: 15 * time.Second}

func mustDo(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP %s %s failed: %v", req.Method, req.URL, err)
	}
	return resp
}

func jsonRequest(t *testing.T, method, url string, body any, token string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func decodeJSON(t *testing.T, resp *http.Response, wantStatus int, dst any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("want HTTP %d, got %d; body: %s", wantStatus, resp.StatusCode, raw)
	}
	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatalf("unmarshal response (%s): %v", raw, err)
		}
	}
}

func rawDecode(t *testing.T, resp *http.Response) (int, []byte) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

// ---------------------------------------------------------------------------
// Test-account fixtures
// ---------------------------------------------------------------------------

func uniqueUser(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func registerAndLogin(t *testing.T, username string) (userID, token string) {
	t.Helper()
	base := baseURL()

	regBody := map[string]string{
		"username": username,
		"email":    username + "@e2e.test",
		"password": "Test@1234!",
	}
	req := jsonRequest(t, http.MethodPost, base+"/auth/register", regBody, "")
	resp := mustDo(t, req)
	var regResp map[string]any
	decodeJSON(t, resp, http.StatusCreated, &regResp)
	userID = regResp["id"].(string)

	loginBody := map[string]string{"username": username, "password": "Test@1234!"}
	req = jsonRequest(t, http.MethodPost, base+"/auth/login", loginBody, "")
	resp = mustDo(t, req)
	var loginResp map[string]string
	decodeJSON(t, resp, http.StatusOK, &loginResp)
	token = loginResp["token"]
	return
}

func createWallet(t *testing.T, token string) (walletID string, balance int64) {
	t.Helper()
	req := jsonRequest(t, http.MethodPost, baseURL()+"/wallets", nil, token)
	resp := mustDo(t, req)
	var w map[string]any
	decodeJSON(t, resp, http.StatusOK, &w)
	walletID = w["id"].(string)
	balance = int64(w["balance"].(float64))
	return
}

func getWalletBalance(t *testing.T, token, walletID string) int64 {
	t.Helper()
	req := jsonRequest(t, http.MethodGet, baseURL()+"/wallets/"+walletID, nil, token)
	resp := mustDo(t, req)
	var w map[string]any
	decodeJSON(t, resp, http.StatusOK, &w)
	return int64(w["balance"].(float64))
}

func transfer(t *testing.T, token, from, to string, amount int64, idempKey string) (int, []byte) {
	t.Helper()
	body := map[string]any{
		"from":            from,
		"to":              to,
		"amount":          amount,
		"idempotency_key": idempKey,
	}
	req := jsonRequest(t, http.MethodPost, baseURL()+"/transfers", body, token)
	resp := mustDo(t, req)
	return rawDecode(t, resp)
}

// ---------------------------------------------------------------------------
// Test 0 – Happy Path
// ---------------------------------------------------------------------------

func TestHappyPath(t *testing.T) {
	t.Log("=== Happy Path: register → wallet → transfer → verify balances ===")

	alice := uniqueUser("alice")
	bob := uniqueUser("bob")

	aliceID, aliceTok := registerAndLogin(t, alice)
	_, bobTok := registerAndLogin(t, bob)
	t.Logf("Alice userID=%s", aliceID)

	aliceWallet, aliceInit := createWallet(t, aliceTok)
	bobWallet, bobInit := createWallet(t, bobTok)
	t.Logf("Alice wallet=%s balance=%d", aliceWallet, aliceInit)
	t.Logf("Bob   wallet=%s balance=%d", bobWallet, bobInit)

	const transferAmount int64 = 10_000
	idemKey := fmt.Sprintf("happy-%d", time.Now().UnixNano())

	status, body := transfer(t, aliceTok, alice, bob, transferAmount, idemKey)
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("transfer want 200/201, got %d; body: %s", status, body)
	}
	t.Logf("Transfer response: %s", body)

	var txResp map[string]any
	if err := json.Unmarshal(body, &txResp); err != nil {
		t.Fatalf("unmarshal transfer response: %v", err)
	}
	if txResp["status"] != "completed" {
		t.Errorf("expected transfer status=completed, got %v", txResp["status"])
	}

	aliceAfter := getWalletBalance(t, aliceTok, aliceWallet)
	bobAfter := getWalletBalance(t, bobTok, bobWallet)
	t.Logf("After transfer — Alice: %d→%d  Bob: %d→%d", aliceInit, aliceAfter, bobInit, bobAfter)

	if aliceAfter != aliceInit-transferAmount {
		t.Errorf("Alice balance mismatch: want %d, got %d", aliceInit-transferAmount, aliceAfter)
	}
	if bobAfter != bobInit+transferAmount {
		t.Errorf("Bob balance mismatch: want %d, got %d", bobInit+transferAmount, bobAfter)
	}

	// Verify transfer record via GET /transfers/:id
	txID, _ := txResp["id"].(string)
	req := jsonRequest(t, http.MethodGet, baseURL()+"/transfers/"+txID, nil, aliceTok)
	resp := mustDo(t, req)
	var getResp map[string]any
	decodeJSON(t, resp, http.StatusOK, &getResp)
	if getResp["status"] != "completed" {
		t.Errorf("GET /transfers/:id status want=completed got=%v", getResp["status"])
	}
	t.Logf("GET /transfers/%s → status=%v", txID, getResp["status"])
}

// ---------------------------------------------------------------------------
// Test 1 – Conservation Law
// ---------------------------------------------------------------------------

func TestConservation(t *testing.T) {
	t.Log("=== Conservation: total balance must be invariant under concurrent transfers ===")

	alice := uniqueUser("cons_a")
	bob := uniqueUser("cons_b")
	carol := uniqueUser("cons_c")

	_, aliceTok := registerAndLogin(t, alice)
	_, bobTok := registerAndLogin(t, bob)
	_, carolTok := registerAndLogin(t, carol)

	aliceWallet, aliceInit := createWallet(t, aliceTok)
	bobWallet, bobInit := createWallet(t, bobTok)
	carolWallet, carolInit := createWallet(t, carolTok)

	totalBefore := aliceInit + bobInit + carolInit
	t.Logf("Initial totals — Alice:%d Bob:%d Carol:%d  Total:%d", aliceInit, bobInit, carolInit, totalBefore)

	const (
		goroutines   = 10
		amountPerTxn = int64(500)
	)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(3)
		idx := i

		go func() {
			defer wg.Done()
			ikey := fmt.Sprintf("cons-ab-%d-%d", time.Now().UnixNano(), idx)
			status, body := transfer(t, aliceTok, alice, bob, amountPerTxn, ikey)
			if status != http.StatusOK && status != http.StatusCreated {
				t.Logf("[A→B #%d] status=%d body=%s", idx, status, body)
			}
		}()

		go func() {
			defer wg.Done()
			ikey := fmt.Sprintf("cons-bc-%d-%d", time.Now().UnixNano(), idx)
			status, body := transfer(t, bobTok, bob, carol, amountPerTxn, ikey)
			if status != http.StatusOK && status != http.StatusCreated {
				t.Logf("[B→C #%d] status=%d body=%s", idx, status, body)
			}
		}()

		go func() {
			defer wg.Done()
			ikey := fmt.Sprintf("cons-ca-%d-%d", time.Now().UnixNano(), idx)
			status, body := transfer(t, carolTok, carol, alice, amountPerTxn, ikey)
			if status != http.StatusOK && status != http.StatusCreated {
				t.Logf("[C→A #%d] status=%d body=%s", idx, status, body)
			}
		}()
	}
	wg.Wait()

	aliceFinal := getWalletBalance(t, aliceTok, aliceWallet)
	bobFinal := getWalletBalance(t, bobTok, bobWallet)
	carolFinal := getWalletBalance(t, carolTok, carolWallet)
	totalAfter := aliceFinal + bobFinal + carolFinal

	t.Logf("Final totals — Alice:%d Bob:%d Carol:%d  Total:%d", aliceFinal, bobFinal, carolFinal, totalAfter)

	if totalAfter != totalBefore {
		t.Errorf("CONSERVATION VIOLATION: total changed from %d to %d (delta=%d)",
			totalBefore, totalAfter, totalAfter-totalBefore)
	}
}

// ---------------------------------------------------------------------------
// Test 2 – No Overdraft
// ---------------------------------------------------------------------------

func TestNoOverdraft(t *testing.T) {
	t.Log("=== No Overdraft: balance must never go negative ===")

	t.Run("single_overdraft_declined", func(t *testing.T) {
		alice := uniqueUser("od_a")
		bob := uniqueUser("od_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)

		aliceWallet, aliceInit := createWallet(t, aliceTok)
		createWallet(t, bobTok)

		overdraftAmount := aliceInit + 1
		ikey := fmt.Sprintf("od-single-%d", time.Now().UnixNano())

		status, body := transfer(t, aliceTok, alice, bob, overdraftAmount, ikey)
		t.Logf("Overdraft attempt: status=%d body=%s", status, body)

		// Expect a declined/failed transfer - check via body status field
		var txResp map[string]any
		_ = json.Unmarshal(body, &txResp)
		txStatus, _ := txResp["status"].(string)
		if txStatus == "completed" {
			t.Fatal("overdraft transfer must NOT complete, got status=completed")
		}

		aliceAfter := getWalletBalance(t, aliceTok, aliceWallet)
		if aliceAfter < 0 {
			t.Errorf("OVERDRAFT: balance went negative: %d", aliceAfter)
		}
		t.Logf("Balance after declined overdraft: %d (unchanged from %d)", aliceAfter, aliceInit)
	})

	t.Run("concurrent_overdraft_storm", func(t *testing.T) {
		alice := uniqueUser("storm_a")
		bob := uniqueUser("storm_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)

		aliceWallet, aliceInit := createWallet(t, aliceTok)
		createWallet(t, bobTok)

		const goroutines = 20
		type result struct {
			status int
			txStat string
		}
		results := make([]result, goroutines)
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ikey := fmt.Sprintf("storm-%d-%d", time.Now().UnixNano(), i)
				status, body := transfer(t, aliceTok, alice, bob, aliceInit, ikey)
				var m map[string]any
				_ = json.Unmarshal(body, &m)
				txStat, _ := m["status"].(string)
				results[i] = result{status: status, txStat: txStat}
			}(i)
		}
		wg.Wait()

		completedCount := 0
		for _, r := range results {
			if r.txStat == "completed" {
				completedCount++
			}
		}
		t.Logf("Concurrent overdraft storm: %d 'completed' out of %d attempts", completedCount, goroutines)

		if completedCount > 1 {
			t.Errorf("more than one overdraft attempt completed (%d); money was created", completedCount)
		}

		aliceAfter := getWalletBalance(t, aliceTok, aliceWallet)
		t.Logf("Alice final balance: %d (init=%d)", aliceAfter, aliceInit)
		if aliceAfter < 0 {
			t.Errorf("OVERDRAFT: Alice balance went negative (%d)", aliceAfter)
		}
	})

	t.Run("zero_amount_rejected", func(t *testing.T) {
		alice := uniqueUser("zero_a")
		bob := uniqueUser("zero_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		createWallet(t, aliceTok)
		createWallet(t, bobTok)

		ikey := fmt.Sprintf("zero-amt-%d", time.Now().UnixNano())
		status, body := transfer(t, aliceTok, alice, bob, 0, ikey)
		t.Logf("Zero-amount transfer: status=%d body=%s", status, body)
		if status == http.StatusOK || status == http.StatusCreated {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if m["status"] == "completed" {
				t.Error("zero-amount transfer must be rejected, but was completed")
			}
		}
	})

	t.Run("negative_amount_rejected", func(t *testing.T) {
		alice := uniqueUser("neg_a")
		bob := uniqueUser("neg_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		createWallet(t, aliceTok)
		createWallet(t, bobTok)

		ikey := fmt.Sprintf("neg-amt-%d", time.Now().UnixNano())
		status, body := transfer(t, aliceTok, alice, bob, -500, ikey)
		t.Logf("Negative-amount transfer: status=%d body=%s", status, body)
		if status == http.StatusOK || status == http.StatusCreated {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if m["status"] == "completed" {
				t.Error("negative-amount transfer must be rejected, but was completed")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3 – Exactly-Once Transfer (Idempotency)
// ---------------------------------------------------------------------------

func TestExactlyOnce(t *testing.T) {
	t.Log("=== Exactly-Once: idempotency_key must guarantee single execution ===")

	t.Run("retry_returns_cached_response", func(t *testing.T) {
		alice := uniqueUser("idem_a")
		bob := uniqueUser("idem_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)

		aliceWallet, aliceInit := createWallet(t, aliceTok)
		bobWallet, bobInit := createWallet(t, bobTok)

		ikey := fmt.Sprintf("idem-retry-%d", time.Now().UnixNano())
		const amount int64 = 1000

		// First call
		status1, body1 := transfer(t, aliceTok, alice, bob, amount, ikey)
		t.Logf("First call:  status=%d body=%s", status1, body1)
		if status1 != http.StatusOK && status1 != http.StatusCreated {
			t.Fatalf("first transfer failed: %d %s", status1, body1)
		}

		// Identical retry
		status2, body2 := transfer(t, aliceTok, alice, bob, amount, ikey)
		t.Logf("Retry call:  status=%d body=%s", status2, body2)
		if status2 != http.StatusOK && status2 != http.StatusCreated {
			t.Fatalf("retry should return cached 2xx, got %d %s", status2, body2)
		}

		// Both responses must reference the same transfer ID
		var r1, r2 map[string]any
		if err := json.Unmarshal(body1, &r1); err != nil {
			t.Fatalf("unmarshal r1: %v", err)
		}
		if err := json.Unmarshal(body2, &r2); err != nil {
			t.Fatalf("unmarshal r2: %v", err)
		}
		if r1["id"] != r2["id"] {
			t.Errorf("idempotent retry returned different transfer ID: first=%v retry=%v", r1["id"], r2["id"])
		}
		t.Logf("Both calls returned same transfer ID: %v", r1["id"])

		// Balance must reflect exactly ONE debit
		aliceAfter := getWalletBalance(t, aliceTok, aliceWallet)
		bobAfter := getWalletBalance(t, bobTok, bobWallet)

		if aliceAfter != aliceInit-amount {
			t.Errorf("Alice debited more than once: init=%d after=%d want=%d", aliceInit, aliceAfter, aliceInit-amount)
		}
		if bobAfter != bobInit+amount {
			t.Errorf("Bob credited more than once: init=%d after=%d want=%d", bobInit, bobAfter, bobInit+amount)
		}
	})

	t.Run("different_body_same_key_is_409_conflict", func(t *testing.T) {
		alice := uniqueUser("conf_a")
		bob := uniqueUser("conf_b")
		carol := uniqueUser("conf_c")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		_, carolTok := registerAndLogin(t, carol)

		createWallet(t, aliceTok)
		createWallet(t, bobTok)
		createWallet(t, carolTok)

		ikey := fmt.Sprintf("conflict-%d", time.Now().UnixNano())

		// First call: Alice→Bob, 500 paise
		status1, body1 := transfer(t, aliceTok, alice, bob, 500, ikey)
		t.Logf("First call (A→B 500): status=%d body=%s", status1, body1)
		if status1 != http.StatusOK && status1 != http.StatusCreated {
			t.Fatalf("first transfer should succeed: %d %s", status1, body1)
		}

		// Same key, different amount → 409
		status2, body2 := transfer(t, aliceTok, alice, bob, 999, ikey)
		t.Logf("Conflict (diff amount): status=%d body=%s", status2, body2)
		if status2 != http.StatusConflict {
			t.Errorf("expected 409 Conflict for same key + different amount, got %d", status2)
		}

		// Same key, different recipient → 409
		status3, body3 := transfer(t, aliceTok, alice, carol, 500, ikey)
		t.Logf("Conflict (diff recipient): status=%d body=%s", status3, body3)
		if status3 != http.StatusConflict {
			t.Errorf("expected 409 Conflict for same key + different recipient, got %d", status3)
		}
	})

	t.Run("concurrent_same_key_exactly_one_debit", func(t *testing.T) {
		alice := uniqueUser("cex_a")
		bob := uniqueUser("cex_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)

		aliceWallet, aliceInit := createWallet(t, aliceTok)
		bobWallet, bobInit := createWallet(t, bobTok)

		ikey := fmt.Sprintf("concurrent-idem-%d", time.Now().UnixNano())
		const amount int64 = 2000
		const goroutines = 15

		type result struct {
			status int
			id     string
		}
		results := make([]result, goroutines)
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				status, body := transfer(t, aliceTok, alice, bob, amount, ikey)
				var m map[string]any
				_ = json.Unmarshal(body, &m)
				id, _ := m["id"].(string)
				results[i] = result{status: status, id: id}
			}(i)
		}
		wg.Wait()

		transferIDs := map[string]bool{}
		for i, r := range results {
			if r.status != http.StatusOK && r.status != http.StatusCreated {
				t.Errorf("goroutine %d: expected 200/201 response for concurrent idempotency key, got status %d", i, r.status)
			}
			if r.id != "" {
				transferIDs[r.id] = true
			}
		}
		t.Logf("Concurrent same-key: unique transfer IDs=%v", transferIDs)

		if len(transferIDs) != 1 {
			t.Errorf("expected exactly 1 distinct transfer ID for same idempotency key, got: %v", transferIDs)
		}

		aliceAfter := getWalletBalance(t, aliceTok, aliceWallet)
		bobAfter := getWalletBalance(t, bobTok, bobWallet)

		if aliceAfter != aliceInit-amount {
			t.Errorf("Alice balance wrong: init=%d after=%d expected=%d", aliceInit, aliceAfter, aliceInit-amount)
		}
		if bobAfter != bobInit+amount {
			t.Errorf("Bob balance wrong: init=%d after=%d expected=%d", bobInit, bobAfter, bobInit+amount)
		}
	})

	t.Run("missing_idempotency_key_rejected", func(t *testing.T) {
		alice := uniqueUser("noidem_a")
		bob := uniqueUser("noidem_b")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		createWallet(t, aliceTok)
		createWallet(t, bobTok)

		body := map[string]any{
			"from":            alice,
			"to":              bob,
			"amount":          1000,
			"idempotency_key": "",
		}
		req := jsonRequest(t, http.MethodPost, baseURL()+"/transfers", body, aliceTok)
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Empty idempotency_key: status=%d body=%s", status, raw)
		if status == http.StatusOK || status == http.StatusCreated {
			t.Error("transfer without idempotency_key must be rejected, got 2xx")
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4 – Race-Free Get-or-Create Wallet
// ---------------------------------------------------------------------------

func TestRaceFreeGetOrCreate(t *testing.T) {
	t.Log("=== Race-Free Get-or-Create: concurrent POST /wallets must be idempotent ===")

	t.Run("concurrent_wallet_creation_single_result", func(t *testing.T) {
		user := uniqueUser("race_u")
		_, tok := registerAndLogin(t, user)

		const goroutines = 20
		walletIDs := make([]string, goroutines)
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				req := jsonRequest(t, http.MethodPost, baseURL()+"/wallets", nil, tok)
				resp := mustDo(t, req)
				var w map[string]any
				decodeJSON(t, resp, http.StatusOK, &w)
				walletIDs[i] = w["id"].(string)
			}(i)
		}
		wg.Wait()

		first := walletIDs[0]
		for i, id := range walletIDs {
			if id != first {
				t.Errorf("goroutine %d got different wallet ID: %s vs %s", i, id, first)
			}
		}
		t.Logf("All %d concurrent POST /wallets → same wallet ID: %s", goroutines, first)
	})

	t.Run("sequential_repeated_calls_idempotent", func(t *testing.T) {
		user := uniqueUser("repeat_u")
		_, tok := registerAndLogin(t, user)

		id1, _ := createWallet(t, tok)
		id2, _ := createWallet(t, tok)
		id3, _ := createWallet(t, tok)

		if id1 != id2 || id2 != id3 {
			t.Errorf("repeated POST /wallets returned different IDs: %s, %s, %s", id1, id2, id3)
		}
		t.Logf("Sequential repeated POST /wallets → consistent wallet ID: %s", id1)
	})

	t.Run("different_users_get_different_wallets", func(t *testing.T) {
		userA := uniqueUser("diff_a")
		userB := uniqueUser("diff_b")

		_, tokA := registerAndLogin(t, userA)
		_, tokB := registerAndLogin(t, userB)

		idA, _ := createWallet(t, tokA)
		idB, _ := createWallet(t, tokB)

		if idA == idB {
			t.Errorf("different users got the same wallet ID: %s", idA)
		}
		t.Logf("User A wallet: %s  User B wallet: %s", idA, idB)
	})

	t.Run("wallet_get_by_id_forbids_foreign_access", func(t *testing.T) {
		userA := uniqueUser("priv_a")
		userB := uniqueUser("priv_b")

		_, tokA := registerAndLogin(t, userA)
		_, tokB := registerAndLogin(t, userB)

		idA, _ := createWallet(t, tokA)
		createWallet(t, tokB)

		req := jsonRequest(t, http.MethodGet, baseURL()+"/wallets/"+idA, nil, tokB)
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("User B accessing User A's wallet: status=%d body=%s", status, raw)
		if status != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d %s", status, raw)
		}
	})

	t.Run("unauthenticated_wallet_creation_rejected", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, baseURL()+"/wallets", nil, "")
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Unauthenticated POST /wallets: status=%d body=%s", status, raw)
		if status != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d %s", status, raw)
		}
	})

	t.Run("invalid_jwt_wallet_creation_rejected", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, baseURL()+"/wallets", nil, "Bearer this.is.not.a.valid.jwt")
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Invalid JWT POST /wallets: status=%d body=%s", status, raw)
		if status != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid JWT, got %d %s", status, raw)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 5 – Edge Cases
// ---------------------------------------------------------------------------

func TestEdgeCases(t *testing.T) {
	t.Run("health_check", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL()+"/health", nil)
		resp := mustDo(t, req)
		var h map[string]any
		decodeJSON(t, resp, http.StatusOK, &h)
		if h["status"] != "ok" {
			t.Errorf("health check status want=ok got=%v", h["status"])
		}
	})

	t.Run("self_transfer_rejected", func(t *testing.T) {
		alice := uniqueUser("self_a")
		_, aliceTok := registerAndLogin(t, alice)
		createWallet(t, aliceTok)

		ikey := fmt.Sprintf("self-%d", time.Now().UnixNano())
		status, body := transfer(t, aliceTok, alice, alice, 1000, ikey)
		t.Logf("Self-transfer: status=%d body=%s", status, body)
		if status == http.StatusOK || status == http.StatusCreated {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if m["status"] == "completed" {
				t.Error("self-transfer must be rejected")
			}
		}
	})

	t.Run("transfer_to_nonexistent_user_returns_404", func(t *testing.T) {
		alice := uniqueUser("ne_a")
		_, aliceTok := registerAndLogin(t, alice)
		createWallet(t, aliceTok)

		ikey := fmt.Sprintf("ne-%d", time.Now().UnixNano())
		status, body := transfer(t, aliceTok, alice, "ghost_user_xyz_987654", 1000, ikey)
		t.Logf("Transfer to non-existent user: status=%d body=%s", status, body)
		if status != http.StatusNotFound {
			t.Errorf("expected 404, got %d %s", status, body)
		}
	})

	t.Run("transfer_from_another_user_forbidden", func(t *testing.T) {
		alice := uniqueUser("frb_a")
		bob := uniqueUser("frb_b")
		carol := uniqueUser("frb_c")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		_, carolTok := registerAndLogin(t, carol)

		createWallet(t, aliceTok)
		createWallet(t, bobTok)
		createWallet(t, carolTok)

		// Alice tries to initiate a transfer FROM Bob's wallet
		ikey := fmt.Sprintf("frb-%d", time.Now().UnixNano())
		status, body := transfer(t, aliceTok, bob, carol, 500, ikey)
		t.Logf("Impersonation attempt: status=%d body=%s", status, body)
		if status == http.StatusOK || status == http.StatusCreated {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if m["status"] == "completed" {
				t.Error("impersonation transfer must be rejected")
			}
		}
	})

	t.Run("get_transfer_by_id_forbids_stranger", func(t *testing.T) {
		alice := uniqueUser("gft_a")
		bob := uniqueUser("gft_b")
		eve := uniqueUser("gft_e")

		_, aliceTok := registerAndLogin(t, alice)
		_, bobTok := registerAndLogin(t, bob)
		_, eveTok := registerAndLogin(t, eve)

		createWallet(t, aliceTok)
		createWallet(t, bobTok)
		createWallet(t, eveTok)

		ikey := fmt.Sprintf("gft-%d", time.Now().UnixNano())
		_, body := transfer(t, aliceTok, alice, bob, 500, ikey)
		var txResp map[string]any
		if err := json.Unmarshal(body, &txResp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		txID, _ := txResp["id"].(string)
		if txID == "" {
			t.Fatal("transfer ID missing from response")
		}

		req := jsonRequest(t, http.MethodGet, baseURL()+"/transfers/"+txID, nil, eveTok)
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Eve reading Alice→Bob transfer: status=%d body=%s", status, raw)
		if status != http.StatusForbidden {
			t.Errorf("expected 403, got %d %s", status, raw)
		}
	})

	t.Run("get_wallet_invalid_uuid_returns_400", func(t *testing.T) {
		alice := uniqueUser("uuid_a")
		_, tok := registerAndLogin(t, alice)
		createWallet(t, tok)

		req := jsonRequest(t, http.MethodGet, baseURL()+"/wallets/not-a-valid-uuid", nil, tok)
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Invalid UUID wallet GET: status=%d body=%s", status, raw)
		if status != http.StatusBadRequest {
			t.Errorf("expected 400, got %d %s", status, raw)
		}
	})

	t.Run("get_transfer_invalid_uuid_returns_400", func(t *testing.T) {
		alice := uniqueUser("tuuid_a")
		_, tok := registerAndLogin(t, alice)
		createWallet(t, tok)

		req := jsonRequest(t, http.MethodGet, baseURL()+"/transfers/not-a-valid-uuid", nil, tok)
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Invalid UUID transfer GET: status=%d body=%s", status, raw)
		if status != http.StatusBadRequest {
			t.Errorf("expected 400, got %d %s", status, raw)
		}
	})

	t.Run("register_duplicate_username_returns_409", func(t *testing.T) {
		username := uniqueUser("dup_u")
		_, _ = registerAndLogin(t, username)

		// Try to register the same username again
		regBody := map[string]string{
			"username": username,
			"email":    "different_email_" + username + "@e2e.test",
			"password": "Test@1234!",
		}
		req := jsonRequest(t, http.MethodPost, baseURL()+"/auth/register", regBody, "")
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Duplicate register: status=%d body=%s", status, raw)
		if status != http.StatusConflict {
			t.Errorf("expected 409 Conflict for duplicate username, got %d %s", status, raw)
		}
	})

	t.Run("login_wrong_password_returns_401", func(t *testing.T) {
		username := uniqueUser("wrongpw_u")
		_, _ = registerAndLogin(t, username)

		loginBody := map[string]string{"username": username, "password": "WrongPassword!"}
		req := jsonRequest(t, http.MethodPost, baseURL()+"/auth/login", loginBody, "")
		resp := mustDo(t, req)
		status, raw := rawDecode(t, resp)
		t.Logf("Wrong password login: status=%d body=%s", status, raw)
		if status != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d %s", status, raw)
		}
	})
}
