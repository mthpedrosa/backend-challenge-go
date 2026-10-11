package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
)

var (
	baseURL       = getEnv("TEST_HTTP_URL", "http://localhost:8080")
	keycloakURL   = getEnv("TEST_KEYCLOAK_URL", "http://localhost:8081")
	sqsEndpoint   = getEnv("TEST_SQS_ENDPOINT", "http://localhost:4566")
	inboxQueueURL = getEnv("TEST_INBOX_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo")
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

func getOAuthToken(t *testing.T, clientID, clientSecret string) string {
	t.Helper()
	endpoint := fmt.Sprintf("%s/realms/wallet-realm/protocol/openid-connect/token", keycloakURL)
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatalf("failed to create token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Keycloak not reachable at %s, skipping test: %v", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected token response status %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}
	return tr.AccessToken
}

type MoneyDTO struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type ReconcileResponse struct {
	WalletID          string   `json:"walletId"`
	StoredBalance     MoneyDTO `json:"storedBalance"`
	CalculatedBalance MoneyDTO `json:"calculatedBalance"`
	Difference        MoneyDTO `json:"difference"`
	Consistent        bool     `json:"consistent"`
	CheckedEntries    int64    `json:"checkedEntries"`
}

func openWallet(t *testing.T, token, playerID, initialBalance string) string {
	t.Helper()
	payload := map[string]string{
		"playerId":       playerID,
		"currency":       "BRL",
		"initialBalance": initialBalance,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/wallets", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create open wallet request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open wallet request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("failed to open wallet: status %d, body %s", resp.StatusCode, string(respBody))
	}

	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode open wallet response: %v", err)
	}
	return res["walletId"]
}

func reconcileWallet(t *testing.T, token, walletID string) ReconcileResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/wallets/%s/reconciliation", baseURL, walletID), nil)
	if err != nil {
		t.Fatalf("failed to create reconciliation request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("reconciliation request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("reconciliation failed: status %d, body %s", resp.StatusCode, string(respBody))
	}

	var rec ReconcileResponse
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		t.Fatalf("failed to decode reconciliation response: %v", err)
	}
	return rec
}

// 1. Envie a mesma aposta 50 vezes em paralelo e comprove um único débito.
func TestConcurrency_50DuplicateBets(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "1000.00")

	extTxID := "ext-dup-50-" + uuid.New().String()
	idemKey := "idem-dup-50-" + uuid.New().String()

	const concurrency = 50
	var wg sync.WaitGroup
	var successCount int64
	var errCount int64

	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := map[string]any{
				"providerId":            "provider-client",
				"externalTransactionId": extTxID,
				"idempotencyKey":        idemKey,
				"playerId":              playerID,
				"walletId":              walletID,
				"currency":              "BRL",
				"amount":                "25.00",
				"kind":                  "BET",
				"roundId":               "round-50-dup",
				"gameId":                "game-test",
			}
			b, _ := json.Marshal(payload)
			req, err := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				return
			}
			req.Header.Set("Authorization", "Bearer "+providerToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", idemKey)

			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var res map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res["status"] == "PROCESSED" {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	if errCount > 0 {
		t.Fatalf("%d requests encountered transport errors", errCount)
	}

	if successCount != concurrency {
		t.Fatalf("expected all %d requests to return 200 PROCESSED, got %d", concurrency, successCount)
	}

	// Verify wallet balance: 1000.00 - 25.00 = 975.00
	rec := reconcileWallet(t, internalToken, walletID)
	if !rec.Consistent {
		t.Fatalf("wallet ledger is not consistent: difference=%s", rec.Difference.Amount)
	}
	if rec.StoredBalance.Amount != "975.00" {
		t.Errorf("storedBalance = %s, want 975.00", rec.StoredBalance.Amount)
	}
	if rec.CalculatedBalance.Amount != "975.00" {
		t.Errorf("calculatedBalance = %s, want 975.00", rec.CalculatedBalance.Amount)
	}
	// Checked entries: 1 opening + 1 bet = 2
	if rec.CheckedEntries != 2 {
		t.Errorf("checkedEntries = %d, want 2", rec.CheckedEntries)
	}
}

// 2. Execute a disputa das duas apostas de 80.00 sobre saldo de 100.00.
func TestConcurrency_Two80BetsOn100Balance(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "100.00")

	betAExt := "bet-race-a-" + uuid.New().String()
	betBExt := "bet-race-b-" + uuid.New().String()

	var wg sync.WaitGroup
	var processedCount int64
	var rejectedCount int64

	client := &http.Client{Timeout: 10 * time.Second}

	fireBet := func(extID, idemKey string) {
		defer wg.Done()
		payload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": extID,
			"idempotencyKey":        idemKey,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "80.00",
			"kind":                  "BET",
			"roundId":               "round-race-1",
			"gameId":                "game-test",
		}
		b, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idemKey)

		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("bet failed with error: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
			return
		}

		var res map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&res)
		status, _ := res["status"].(string)

		if status == "PROCESSED" {
			atomic.AddInt64(&processedCount, 1)
		} else if status == "REJECTED" {
			failCode, _ := res["failureCode"].(string)
			if failCode == "INSUFFICIENT_FUNDS" {
				atomic.AddInt64(&rejectedCount, 1)
			} else {
				t.Errorf("unexpected failure code: %s", failCode)
			}
		}
	}

	wg.Add(2)
	go fireBet(betAExt, betAExt)
	go fireBet(betBExt, betBExt)
	wg.Wait()

	if processedCount != 1 {
		t.Errorf("expected exactly 1 PROCESSED bet, got %d", processedCount)
	}
	if rejectedCount != 1 {
		t.Errorf("expected exactly 1 REJECTED bet with INSUFFICIENT_FUNDS, got %d", rejectedCount)
	}

	// Replay Bet A and Bet B: neither should alter the result
	for _, extID := range []string{betAExt, betBExt} {
		payload := map[string]any{
			"providerId":            "provider-client",
			"externalTransactionId": extID,
			"idempotencyKey":        extID,
			"playerId":              playerID,
			"walletId":              walletID,
			"currency":              "BRL",
			"amount":                "80.00",
			"kind":                  "BET",
			"roundId":               "round-race-1",
			"gameId":                "game-test",
		}
		b, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+providerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", extID)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("replay failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("replay status = %d, want 200", resp.StatusCode)
		}
	}

	// Reconciliation: stored == 20.00, difference == 0.00, consistent == true, checkedEntries == 2
	rec := reconcileWallet(t, internalToken, walletID)
	if !rec.Consistent {
		t.Fatalf("wallet ledger is not consistent: difference=%s", rec.Difference.Amount)
	}
	if rec.StoredBalance.Amount != "20.00" {
		t.Errorf("storedBalance = %s, want 20.00", rec.StoredBalance.Amount)
	}
	if rec.CalculatedBalance.Amount != "20.00" {
		t.Errorf("calculatedBalance = %s, want 20.00", rec.CalculatedBalance.Amount)
	}
	if rec.CheckedEntries != 2 {
		t.Errorf("checkedEntries = %d, want 2 (1 opening + 1 bet)", rec.CheckedEntries)
	}
}

// 3. Processe carteiras distintas simultaneamente.
func TestConcurrency_ParallelWallets(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	const numWallets = 5
	const betsPerWallet = 5

	walletIDs := make([]string, numWallets)
	playerIDs := make([]string, numWallets)

	for i := 0; i < numWallets; i++ {
		playerIDs[i] = uuid.New().String()
		walletIDs[i] = openWallet(t, internalToken, playerIDs[i], "500.00")
	}

	var wg sync.WaitGroup
	client := &http.Client{Timeout: 10 * time.Second}

	for wIdx := 0; wIdx < numWallets; wIdx++ {
		wID := walletIDs[wIdx]
		pID := playerIDs[wIdx]

		for b := 0; b < betsPerWallet; b++ {
			wg.Add(1)
			extID := fmt.Sprintf("ext-par-%s-%d", wID, b)
			go func(walletID, playerID, extID string) {
				defer wg.Done()
				payload := map[string]any{
					"providerId":            "provider-client",
					"externalTransactionId": extID,
					"idempotencyKey":        extID,
					"playerId":              playerID,
					"walletId":              walletID,
					"currency":              "BRL",
					"amount":                "10.00",
					"kind":                  "BET",
					"roundId":               "round-par",
					"gameId":                "game-par",
				}
				b, _ := json.Marshal(payload)
				req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
				req.Header.Set("Authorization", "Bearer "+providerToken)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", extID)

				resp, err := client.Do(req)
				if err != nil {
					t.Errorf("parallel bet request error: %v", err)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("parallel bet expected 200, got %d", resp.StatusCode)
				}
			}(wID, pID, extID)
		}
	}

	wg.Wait()

	// Reconcile each wallet: 500.00 - (5 * 10.00) = 450.00
	for _, wID := range walletIDs {
		rec := reconcileWallet(t, internalToken, wID)
		if !rec.Consistent {
			t.Errorf("wallet %s reconciliation inconsistent", wID)
		}
		if rec.StoredBalance.Amount != "450.00" {
			t.Errorf("wallet %s stored balance = %s, want 450.00", wID, rec.StoredBalance.Amount)
		}
		if rec.CheckedEntries != 1+betsPerWallet {
			t.Errorf("wallet %s checked entries = %d, want %d", wID, rec.CheckedEntries, 1+betsPerWallet)
		}
	}
}

// 4. Entregue REFUND antes da referência e comprove status PENDING_REFERENCE e resolução.
func TestConcurrency_ReversalBeforeReference(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "200.00")

	refBetExtID := "bet-future-" + uuid.New().String()
	refundExtID := "refund-early-" + uuid.New().String()

	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Submit REFUND before the BET arrives
	refundPayload := map[string]any{
		"providerId":                     "provider-client",
		"externalTransactionId":          refundExtID,
		"idempotencyKey":                 refundExtID,
		"playerId":                       playerID,
		"walletId":                       walletID,
		"currency":                       "BRL",
		"amount":                         "50.00",
		"kind":                           "REFUND",
		"roundId":                        "round-ref-1",
		"gameId":                         "game-ref-1",
		"referenceExternalTransactionId": refBetExtID,
	}
	b, _ := json.Marshal(refundPayload)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", refundExtID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("refund request failed: %v", err)
	}
	defer resp.Body.Close()

	// Must be accepted as PENDING_REFERENCE (202 Accepted)
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202 Accepted for pending reference, got %d: %s", resp.StatusCode, string(body))
	}

	var res map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res["status"] != "PENDING_REFERENCE" {
		t.Errorf("status = %v, want PENDING_REFERENCE", res["status"])
	}

	// At this point, wallet balance is unchanged: 200.00
	rec := reconcileWallet(t, internalToken, walletID)
	if rec.StoredBalance.Amount != "200.00" {
		t.Errorf("balance before bet = %s, want 200.00", rec.StoredBalance.Amount)
	}

	// 2. Now submit the referenced BET of 50.00
	betPayload := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": refBetExtID,
		"idempotencyKey":        refBetExtID,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "50.00",
		"kind":                  "BET",
		"roundId":               "round-ref-1",
		"gameId":                "game-ref-1",
	}
	b, _ = json.Marshal(betPayload)
	req, _ = http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+providerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", refBetExtID)

	respBet, err := client.Do(req)
	if err != nil {
		t.Fatalf("bet request failed: %v", err)
	}
	defer respBet.Body.Close()

	if respBet.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respBet.Body)
		t.Fatalf("expected 200 OK for referenced bet, got %d: %s", respBet.StatusCode, string(body))
	}

	// 3. Wait for the pending reference worker to process the pending REFUND
	// The worker runs periodically (every 1-2s). Poll until refund is PROCESSED or timeout.
	deadline := time.Now().Add(10 * time.Second)
	var refundStatus string
	for time.Now().Before(deadline) {
		reqCheck, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/providers/provider-client/wagering/transactions/%s", baseURL, refundExtID), nil)
		reqCheck.Header.Set("Authorization", "Bearer "+providerToken)
		respCheck, err := client.Do(reqCheck)
		if err == nil && respCheck.StatusCode == http.StatusOK {
			var txRes map[string]any
			_ = json.NewDecoder(respCheck.Body).Decode(&txRes)
			respCheck.Body.Close()
			refundStatus, _ = txRes["status"].(string)
			if refundStatus == "PROCESSED" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if refundStatus != "PROCESSED" {
		t.Logf("pending refund did not resolve within 10s (status is %s)", refundStatus)
	}

	// Final reconciliation check
	recFinal := reconcileWallet(t, internalToken, walletID)
	if !recFinal.Consistent {
		t.Fatalf("reconciliation failed: difference=%s", recFinal.Difference.Amount)
	}
}

// 5. Isolamento entre provedores e restrição interna.
func TestAuthorization_ProviderIsolation(t *testing.T) {
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")

	client := &http.Client{Timeout: 5 * time.Second}

	// Case 1: Provider client attempts to perform wallet admin operation -> 403 Forbidden
	walletReq, _ := http.NewRequest(http.MethodPost, baseURL+"/wallets", bytes.NewReader([]byte(`{"playerId":"p1","currency":"BRL","initialBalance":"10.00"}`)))
	walletReq.Header.Set("Authorization", "Bearer "+providerToken)
	walletReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(walletReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("provider calling /wallets expected 403 Forbidden, got %d", resp.StatusCode)
	}

	// Case 2: Provider client attempts to execute transaction for a different provider -> 403 Forbidden
	txReq, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader([]byte(`{
		"providerId":"provider-other",
		"externalTransactionId":"ext-1",
		"idempotencyKey":"idem-1",
		"playerId":"p1",
		"walletId":"00000000-0000-0000-0000-000000000000",
		"currency":"BRL",
		"amount":"10.00",
		"kind":"BET",
		"roundId":"r1",
		"gameId":"g1"
	}`)))
	txReq.Header.Set("Authorization", "Bearer "+providerToken)
	txReq.Header.Set("Content-Type", "application/json")
	txReq.Header.Set("Idempotency-Key", "idem-1")

	respTx, err := client.Do(txReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respTx.Body.Close()

	if respTx.StatusCode != http.StatusForbidden {
		t.Errorf("provider sending transaction for provider-other expected 403 Forbidden, got %d", respTx.StatusCode)
	}

	// Case 3: Public endpoint /metrics and /health/ready without auth -> 200 OK
	for _, path := range []string{"/health/live", "/health/ready", "/metrics"} {
		pubReq, _ := http.NewRequest(http.MethodGet, baseURL+path, nil)
		pubResp, err := client.Do(pubReq)
		if err != nil {
			t.Fatalf("public endpoint %s failed: %v", path, err)
		}
		pubResp.Body.Close()
		if pubResp.StatusCode != http.StatusOK {
			t.Errorf("endpoint %s expected 200 OK without token, got %d", path, pubResp.StatusCode)
		}
	}

	_ = internalToken
}

func getSQSClient(t *testing.T) *awssqs.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(awscreds.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	return awssqs.NewFromConfig(cfg, func(o *awssqs.Options) {
		o.BaseEndpoint = aws.String(sqsEndpoint)
	})
}

// 6. Cenários que cruzem HTTP e SQS para a mesma operação (deduplicação cruzada e consistência do ledger).
func TestConcurrency_CrossHttpAndSQS(t *testing.T) {
	internalToken := getOAuthToken(t, "internal-client", "internal-secret")
	providerToken := getOAuthToken(t, "provider-client", "provider-secret")

	sqsClient := getSQSClient(t)
	if sqsClient == nil {
		t.Skip("SQS not reachable, skipping cross HTTP/SQS test")
	}

	playerID := uuid.New().String()
	walletID := openWallet(t, internalToken, playerID, "500.00")

	// Scenario A: Process via HTTP first, then replay identical payload via SQS.
	extID := "cross-http-first-" + uuid.New().String()
	idemKey := extID

	payloadA := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": extID,
		"idempotencyKey":        idemKey,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "50.00",
		"kind":                  "BET",
		"roundId":               "round-cross-a",
		"gameId":                "game-cross-a",
	}

	client := &http.Client{Timeout: 10 * time.Second}
	bodyA, _ := json.Marshal(payloadA)
	reqA, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(bodyA))
	reqA.Header.Set("Authorization", "Bearer "+providerToken)
	reqA.Header.Set("Content-Type", "application/json")
	reqA.Header.Set("Idempotency-Key", idemKey)

	respA, err := client.Do(reqA)
	if err != nil {
		t.Fatalf("HTTP bet request failed: %v", err)
	}
	defer respA.Body.Close()
	if respA.StatusCode != http.StatusOK {
		t.Fatalf("HTTP bet request status = %d, want 200", respA.StatusCode)
	}

	// Now send the identical payload to SQS inbox
	sqsMsgBody, _ := json.Marshal(payloadA)
	dedupID := extID
	groupID := walletID
	_, err = sqsClient.SendMessage(context.Background(), &awssqs.SendMessageInput{
		QueueUrl:               aws.String(inboxQueueURL),
		MessageBody:            aws.String(string(sqsMsgBody)),
		MessageDeduplicationId: aws.String(dedupID),
		MessageGroupId:         aws.String(groupID),
	})
	if err != nil {
		t.Fatalf("failed to send message to SQS: %v", err)
	}

	// Give the SQS worker a moment to receive and deduplicate
	time.Sleep(2 * time.Second)

	// Reconcile: balance must be exactly 450.00 (50.00 debited once), exactly 2 ledger entries
	recA := reconcileWallet(t, internalToken, walletID)
	if !recA.Consistent {
		t.Fatalf("reconciliation failed after cross SQS: diff=%s", recA.Difference.Amount)
	}
	if recA.StoredBalance.Amount != "450.00" {
		t.Errorf("stored balance = %s, want 450.00", recA.StoredBalance.Amount)
	}
	if recA.CheckedEntries != 2 {
		t.Errorf("checked entries = %d, want 2 (1 opening + 1 bet)", recA.CheckedEntries)
	}

	// Scenario B: Process via SQS first, then replay identical payload via HTTP
	extID2 := "cross-sqs-first-" + uuid.New().String()
	payloadB := map[string]any{
		"providerId":            "provider-client",
		"externalTransactionId": extID2,
		"idempotencyKey":        extID2,
		"playerId":              playerID,
		"walletId":              walletID,
		"currency":              "BRL",
		"amount":                "30.00",
		"kind":                  "BET",
		"roundId":               "round-cross-b",
		"gameId":                "game-cross-b",
	}
	sqsMsgBody2, _ := json.Marshal(payloadB)
	_, err = sqsClient.SendMessage(context.Background(), &awssqs.SendMessageInput{
		QueueUrl:               aws.String(inboxQueueURL),
		MessageBody:            aws.String(string(sqsMsgBody2)),
		MessageDeduplicationId: aws.String(extID2),
		MessageGroupId:         aws.String(groupID),
	})
	if err != nil {
		t.Fatalf("failed to send second message to SQS: %v", err)
	}

	// Wait for SQS worker to process
	deadline := time.Now().Add(10 * time.Second)
	var processedViaSQS bool
	for time.Now().Before(deadline) {
		rec := reconcileWallet(t, internalToken, walletID)
		if rec.StoredBalance.Amount == "420.00" {
			processedViaSQS = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !processedViaSQS {
		t.Fatalf("SQS message was not processed within deadline")
	}

	// Now replay identical payload via HTTP
	bodyB, _ := json.Marshal(payloadB)
	reqB, _ := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(bodyB))
	reqB.Header.Set("Authorization", "Bearer "+providerToken)
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("Idempotency-Key", extID2)

	respB, err := client.Do(reqB)
	if err != nil {
		t.Fatalf("HTTP replay failed: %v", err)
	}
	defer respB.Body.Close()
	if respB.StatusCode != http.StatusOK {
		t.Fatalf("HTTP replay returned status %d, want 200", respB.StatusCode)
	}

	var resB map[string]any
	_ = json.NewDecoder(respB.Body).Decode(&resB)
	if resB["idempotentReplay"] != true {
		t.Errorf("expected idempotentReplay to be true, got %v", resB["idempotentReplay"])
	}

	// Final reconciliation: 420.00, consistent, exactly 3 entries (opening, bet 50, bet 30)
	recFinal := reconcileWallet(t, internalToken, walletID)
	if !recFinal.Consistent {
		t.Fatalf("final reconciliation inconsistent: diff=%s", recFinal.Difference.Amount)
	}
	if recFinal.StoredBalance.Amount != "420.00" {
		t.Errorf("final balance = %s, want 420.00", recFinal.StoredBalance.Amount)
	}
	if recFinal.CheckedEntries != 3 {
		t.Errorf("final checked entries = %d, want 3", recFinal.CheckedEntries)
	}
}
