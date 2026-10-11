package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestReversal_SingleSlotAndDuplicateRejection verifies that a bet can only be
// reversed once (single-slot reversal constraint) and subsequent reversal attempts
// return 409 ALREADY_REVERSED without corrupting the ledger.
func TestReversal_SingleSlotAndDuplicateRejection(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "100.00")

	betExtID := "bet-slot-" + uuid.New().String()
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Process initial BET of 40.00
	betPayload := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": betExtID,
		"idempotencyKey":        betExtID,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "40.00",
		"kind":                  "BET",
		"roundId":               "round-slot-1",
		"gameId":                "game-slot-1",
	}
	b, _ := json.Marshal(betPayload)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", betExtID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("bet request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bet failed with %d: %s", resp.StatusCode, string(body))
	}

	// Verify balance after bet: 60.00
	recAfterBet := reconcileWallet(t, internalToken, walletID)
	if recAfterBet.StoredBalance.Amount != "60.00" {
		t.Fatalf("stored balance = %s, want 60.00", recAfterBet.StoredBalance.Amount)
	}

	// 2. First REFUND of 40.00 referencing the bet -> must succeed
	refund1ExtID := "refund1-slot-" + uuid.New().String()
	refund1Payload := map[string]any{
		"providerId":                     "provider-client",
		"externalTransactionId":          refund1ExtID,
		"idempotencyKey":                 refund1ExtID,
		"playerId":                       playerID,
		"walletId":                       walletID,
		"currency":                       "BRL",
		"amount":                         "40.00",
		"kind":                           "REFUND",
		"roundId":                        "round-slot-1",
		"gameId":                         "game-slot-1",
		"referenceExternalTransactionId": betExtID,
	}
	b, _ = json.Marshal(refund1Payload)
	req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", refund1ExtID)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("refund 1 request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("first refund failed with %d: %s", resp.StatusCode, string(body))
	}

	// Balance restored to 100.00
	recAfterRefund := reconcileWallet(t, internalToken, walletID)
	if recAfterRefund.StoredBalance.Amount != "100.00" {
		t.Fatalf("stored balance after first refund = %s, want 100.00", recAfterRefund.StoredBalance.Amount)
	}

	// 3. Second REFUND referencing the SAME bet with different idempotency key -> must be rejected (409 ALREADY_REVERSED)
	refund2ExtID := "refund2-slot-" + uuid.New().String()
	refund2Payload := map[string]any{
		"providerId":                     "provider-client",
		"externalTransactionId":          refund2ExtID,
		"idempotencyKey":                 refund2ExtID,
		"playerId":                       playerID,
		"walletId":                       walletID,
		"currency":                       "BRL",
		"amount":                         "40.00",
		"kind":                           "REFUND",
		"roundId":                        "round-slot-1",
		"gameId":                         "game-slot-1",
		"referenceExternalTransactionId": betExtID,
	}
	b, _ = json.Marshal(refund2Payload)
	req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", refund2ExtID)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("refund 2 request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK for refund 2 response, got %d: %s", resp.StatusCode, string(body))
	}
	var resRef2 map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&resRef2)
	if resRef2["status"] != "REJECTED" || resRef2["failureCode"] != "ALREADY_REVERSED" {
		t.Errorf("expected REJECTED ALREADY_REVERSED, got status=%v code=%v", resRef2["status"], resRef2["failureCode"])
	}

	// 4. ROLLBACK referencing the SAME bet -> must also be rejected (REJECTED ALREADY_REVERSED)
	rollExtID := "roll-slot-" + uuid.New().String()
	rollPayload := map[string]any{
		"providerId":                     "provider-client",
		"externalTransactionId":          rollExtID,
		"idempotencyKey":                 rollExtID,
		"playerId":                       playerID,
		"walletId":                       walletID,
		"currency":                       "BRL",
		"amount":                         "40.00",
		"kind":                           "ROLLBACK",
		"roundId":                        "round-slot-1",
		"gameId":                         "game-slot-1",
		"referenceExternalTransactionId": betExtID,
	}
	b, _ = json.Marshal(rollPayload)
	req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", rollExtID)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("rollback request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK for rollback response, got %d: %s", resp.StatusCode, string(body))
	}
	var resRoll map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&resRoll)
	if resRoll["status"] != "REJECTED" || resRoll["failureCode"] != "ALREADY_REVERSED" {
		t.Errorf("expected REJECTED ALREADY_REVERSED for rollback, got status=%v code=%v", resRoll["status"], resRoll["failureCode"])
	}

	// 5. Final ledger verification: exactly 3 entries (open 100, bet -40, refund +40), balance 100.00
	recFinal := reconcileWallet(t, internalToken, walletID)
	if !recFinal.Consistent {
		t.Fatalf("final ledger inconsistent: diff=%s", recFinal.Difference.Amount)
	}
	if recFinal.StoredBalance.Amount != "100.00" {
		t.Errorf("final balance = %s, want 100.00", recFinal.StoredBalance.Amount)
	}
	if recFinal.CheckedEntries != 3 {
		t.Errorf("checked entries = %d, want 3", recFinal.CheckedEntries)
	}
}

// TestReversal_RollbackDirections verifies that:
// - ROLLBACK of a BET credits the wallet (reverses debit)
// - ROLLBACK of a WIN debits the wallet (reverses credit)
// - ROLLBACK of a WIN fails with INSUFFICIENT_FUNDS if player does not have enough balance
func TestReversal_RollbackDirections(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")
	client := &http.Client{Timeout: 10 * time.Second}

	// --- Part A: ROLLBACK of BET credits the wallet ---
	t.Run("RollbackOfBetCreditsWallet", func(t *testing.T) {
		playerID := uuid.New().String()
		walletID := openWallet(t, internalToken, playerID, "100.00")

		betExtID := "bet-rb-" + uuid.New().String()
		betPayload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": betExtID,
			"idempotencyKey":        betExtID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "30.00",
			"kind":                  "BET",
			"roundId":               "round-rb-bet",
			"gameId":                "game-rb-bet",
		}
		b, _ := json.Marshal(betPayload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", betExtID)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("bet failed: %v", err)
		}
		resp.Body.Close()

		// Rollback of BET
		rbExtID := "rb-bet-" + uuid.New().String()
		rbPayload := map[string]any{
			"providerId":                     "provider-client",
			"externalTransactionId":          rbExtID,
			"idempotencyKey":                 rbExtID,
			"playerId":                       playerID,
			"walletId":                       walletID,
			"currency":                       "BRL",
			"amount":                         "30.00",
			"kind":                           "ROLLBACK",
			"roundId":                        "round-rb-bet",
			"gameId":                         "game-rb-bet",
			"referenceExternalTransactionId": betExtID,
		}
		b, _ = json.Marshal(rbPayload)
		req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", rbExtID)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("rollback of bet request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("rollback of bet status = %d: %s", resp.StatusCode, string(body))
		}

		rec := reconcileWallet(t, internalToken, walletID)
		if !rec.Consistent {
			t.Fatalf("ledger inconsistent: %s", rec.Difference.Amount)
		}
		if rec.StoredBalance.Amount != "100.00" {
			t.Errorf("stored balance after rollback of bet = %s, want 100.00", rec.StoredBalance.Amount)
		}
	})

	// --- Part B: ROLLBACK of WIN debits the wallet ---
	t.Run("RollbackOfWinDebitsWallet", func(t *testing.T) {
		playerID := uuid.New().String()
		walletID := openWallet(t, internalToken, playerID, "100.00")

		winExtID := "win-rb-" + uuid.New().String()
		winPayload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": winExtID,
			"idempotencyKey":        winExtID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "50.00",
			"kind":                  "WIN",
			"roundId":               "round-rb-win",
			"gameId":                "game-rb-win",
		}
		b, _ := json.Marshal(winPayload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", winExtID)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("win failed: %v", err)
		}
		resp.Body.Close()

		// Verify balance is 150.00
		recAfterWin := reconcileWallet(t, internalToken, walletID)
		if recAfterWin.StoredBalance.Amount != "150.00" {
			t.Fatalf("balance after win = %s, want 150.00", recAfterWin.StoredBalance.Amount)
		}

		// Rollback of WIN
		rbExtID := "rb-win-" + uuid.New().String()
		rbPayload := map[string]any{
			"providerId":                     "provider-client",
			"externalTransactionId":          rbExtID,
			"idempotencyKey":                 rbExtID,
			"playerId":                       playerID,
			"walletId":                       walletID,
			"currency":                       "BRL",
			"amount":                         "50.00",
			"kind":                           "ROLLBACK",
			"roundId":                        "round-rb-win",
			"gameId":                         "game-rb-win",
			"referenceExternalTransactionId": winExtID,
		}
		b, _ = json.Marshal(rbPayload)
		req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", rbExtID)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("rollback of win request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("rollback of win status = %d: %s", resp.StatusCode, string(body))
		}

		rec := reconcileWallet(t, internalToken, walletID)
		if !rec.Consistent {
			t.Fatalf("ledger inconsistent: %s", rec.Difference.Amount)
		}
		if rec.StoredBalance.Amount != "100.00" {
			t.Errorf("stored balance after rollback of win = %s, want 100.00", rec.StoredBalance.Amount)
		}
	})

	// --- Part C: ROLLBACK of WIN with Insufficient Funds fails cleanly ---
	t.Run("RollbackOfWinInsufficientFundsFails", func(t *testing.T) {
		playerID := uuid.New().String()
		walletID := openWallet(t, internalToken, playerID, "10.00")

		// 1. Player receives WIN of 50.00 -> balance becomes 60.00
		winExtID := "win-insuf-" + uuid.New().String()
		winPayload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": winExtID,
			"idempotencyKey":        winExtID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "50.00",
			"kind":                  "WIN",
			"roundId":               "round-rb-insuf",
			"gameId":                "game-rb-insuf",
		}
		b, _ := json.Marshal(winPayload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", winExtID)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("win failed: %v", err)
		}
		resp.Body.Close()

		// 2. Player bets 55.00 -> balance becomes 5.00
		betExtID := "bet-insuf-" + uuid.New().String()
		betPayload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": betExtID,
			"idempotencyKey":        betExtID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "55.00",
			"kind":                  "BET",
			"roundId":               "round-rb-insuf",
			"gameId":                "game-rb-insuf",
		}
		b, _ = json.Marshal(betPayload)
		req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", betExtID)
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("bet failed: %v", err)
		}
		resp.Body.Close()

		// 3. Attempt ROLLBACK of the 50.00 WIN -> player only has 5.00, cannot debit 50.00
		rbExtID := "rb-insuf-" + uuid.New().String()
		rbPayload := map[string]any{
			"providerId":                     "provider-client",
			"externalTransactionId":          rbExtID,
			"idempotencyKey":                 rbExtID,
			"playerId":                       playerID,
			"walletId":                       walletID,
			"currency":                       "BRL",
			"amount":                         "50.00",
			"kind":                           "ROLLBACK",
			"roundId":                        "round-rb-insuf",
			"gameId":                         "game-rb-insuf",
			"referenceExternalTransactionId": winExtID,
		}
		b, _ = json.Marshal(rbPayload)
		req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", rbExtID)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("rollback request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 OK for rollback response, got %d: %s", resp.StatusCode, string(body))
		}
		var resRB map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&resRB)
		if resRB["status"] != "REJECTED" || (resRB["failureCode"] != "REVERSAL_INSUFFICIENT_FUNDS" && resRB["failureCode"] != "INSUFFICIENT_FUNDS") {
			t.Errorf("expected REJECTED INSUFFICIENT_FUNDS, got status=%v code=%v", resRB["status"], resRB["failureCode"])
		}

		// Balance remains 5.00
		rec := reconcileWallet(t, internalToken, walletID)
		if !rec.Consistent {
			t.Fatalf("ledger inconsistent: %s", rec.Difference.Amount)
		}
		if rec.StoredBalance.Amount != "5.00" {
			t.Errorf("stored balance = %s, want 5.00", rec.StoredBalance.Amount)
		}
	})
}

// TestIdempotency_ConflictDetection verifies that sending the same external transaction
// ID with a different idempotency key returns 409 Conflict (IDEMPOTENCY_CONFLICT).
func TestIdempotency_ConflictDetection(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")
	client := &http.Client{Timeout: 10 * time.Second}

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "100.00")

	extTxID := "ext-conflict-" + uuid.New().String()
	idemKey1 := "idem-key-1-" + uuid.New().String()
	idemKey2 := "idem-key-2-" + uuid.New().String()

	payload1 := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": extTxID,
		"idempotencyKey":        idemKey1,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "20.00",
		"kind":                  "BET",
		"roundId":               "round-conflict",
		"gameId":                "game-conflict",
	}
	b, _ := json.Marshal(payload1)
	req1, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req1.Header.Set("Authorization", "Bearer "+providerToken)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idemKey1)

	resp1, err := client.Do(req1)
	if err != nil || resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request failed: %v", err)
	}
	resp1.Body.Close()

	// Second request: SAME externalTransactionId, DIFFERENT idempotencyKey
	payload2 := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": extTxID,
		"idempotencyKey":        idemKey2,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "20.00",
		"kind":                  "BET",
		"roundId":               "round-conflict",
		"gameId":                "game-conflict",
	}
	b, _ = json.Marshal(payload2)
	req2, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req2.Header.Set("Authorization", "Bearer "+providerToken)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idemKey2)

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("expected 409 Conflict for idempotency key conflict, got %d: %s", resp2.StatusCode, string(body))
	}
}

// TestConcurrency_ThreeInstances verifies concurrent transaction processing across
// three independent application instances (:8080, :8082, :8083).
func TestConcurrency_ThreeInstances(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	instanceURLs := []string{
		"http://localhost:8080",
		"http://localhost:8082",
		"http://localhost:8083",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Verify all 3 instances are reachable
	for _, instURL := range instanceURLs {
		hResp, err := client.Get(instURL + "/health/live")
		if err != nil || hResp.StatusCode != http.StatusOK {
			t.Skipf("instance at %s not reachable, skipping three-instance test: %v", instURL, err)
		}
		hResp.Body.Close()
	}

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "1000.00")

	const betsPerInstance = 15
	var wg sync.WaitGroup
	var successCount int64
	var errCount int64

	for _, instURL := range instanceURLs {
		url := instURL
		for j := 0; j < betsPerInstance; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				extID := "ext-multi-" + uuid.New().String()
				payload := map[string]any{
					"providerId":            "provider-client",
					"externalTransactionId": extID,
					"idempotencyKey":        extID,
					"playerId":              playerID,
					"walletId":              walletID,
					"currency":              "BRL",
					"amount":                "10.00",
					"kind":                  "BET",
					"roundId":               "round-multi",
					"gameId":                "game-multi",
				}
				b, _ := json.Marshal(payload)
				req, err := http.NewRequest(http.MethodPost, url+"/wagering/transactions", bytes.NewReader(b))
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					return
				}
				req.Header.Set("Authorization", "Bearer "+providerToken)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", extID)

				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&errCount, 1)
				}
			}()
		}
	}

	wg.Wait()

	totalBets := int64(len(instanceURLs) * betsPerInstance)
	if successCount != totalBets {
		t.Fatalf("expected %d successful bets across 3 instances, got %d (errors: %d)", totalBets, successCount, errCount)
	}

	// Initial 1000.00 - (45 * 10.00) = 550.00
	rec := reconcileWallet(t, internalToken, walletID)
	if !rec.Consistent {
		t.Fatalf("wallet ledger inconsistent across 3 instances: diff=%s", rec.Difference.Amount)
	}
	if rec.StoredBalance.Amount != "550.00" {
		t.Errorf("stored balance = %s, want 550.00", rec.StoredBalance.Amount)
	}
	if rec.CheckedEntries != totalBets+1 {
		t.Errorf("checked entries = %d, want %d", rec.CheckedEntries, totalBets+1)
	}
}

// TestLedger_CursorPagination verifies that GET /wallets/:id/ledger supports opaque
// cursor pagination with non-overlapping pages and stable ordering.
func TestLedger_CursorPagination(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")
	client := &http.Client{Timeout: 10 * time.Second}

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "100.00")

	// Create 4 bets: total ledger entries = 1 opening + 4 bets = 5 entries
	for i := 1; i <= 4; i++ {
		extID := fmt.Sprintf("bet-page-%d-%s", i, uuid.New().String())
		payload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": extID,
			"idempotencyKey":        extID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "5.00",
			"kind":                  "BET",
			"roundId":               "round-page",
			"gameId":                "game-page",
		}
		b, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", extID)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("bet %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	// Fetch page 1 (limit=2)
	reqPage1, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/wallets/%s/ledger?limit=2", baseURL, walletID), nil)
	reqPage1.Header.Set("Authorization", "Bearer "+internalToken)
	respPage1, err := client.Do(reqPage1)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	defer respPage1.Body.Close()
	if respPage1.StatusCode != http.StatusOK {
		t.Fatalf("page 1 status = %d, want 200", respPage1.StatusCode)
	}

	var page1 struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	_ = json.NewDecoder(respPage1.Body).Decode(&page1)
	if len(page1.Items) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(page1.Items))
	}
	if page1.NextCursor == nil || *page1.NextCursor == "" {
		t.Fatalf("page 1 expected non-empty nextCursor")
	}

	// Fetch page 2 with cursor (limit=2)
	reqPage2, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/wallets/%s/ledger?limit=2&cursor=%s", baseURL, walletID, *page1.NextCursor), nil)
	reqPage2.Header.Set("Authorization", "Bearer "+internalToken)
	respPage2, err := client.Do(reqPage2)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	defer respPage2.Body.Close()
	if respPage2.StatusCode != http.StatusOK {
		t.Fatalf("page 2 status = %d, want 200", respPage2.StatusCode)
	}

	var page2 struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	_ = json.NewDecoder(respPage2.Body).Decode(&page2)
	if len(page2.Items) != 2 {
		t.Fatalf("page 2 items = %d, want 2", len(page2.Items))
	}

	// Ensure no item IDs overlap between page 1 and page 2
	seen := make(map[string]bool)
	for _, it := range page1.Items {
		seen[it.ID] = true
	}
	for _, it := range page2.Items {
		if seen[it.ID] {
			t.Errorf("duplicate item %s found across pages", it.ID)
		}
	}
}
