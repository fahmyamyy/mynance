//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/account"
	"mynance/internal/idempotency"
	"mynance/internal/ledger"
	"mynance/internal/order"
	"mynance/internal/outbox"
	"mynance/internal/trade"
	"mynance/internal/user"
	"mynance/pkg/numeric"
	"mynance/pkg/timeutil"
)

func buildRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	userStore := user.NewStore(pool)
	accountStore := account.NewStore(pool)
	ledgerStore := ledger.NewStore(pool)
	idempotencyStore := idempotency.NewStore(pool)
	outboxStore := outbox.NewStore(pool)
	orderStore := order.NewStore(pool)
	tradeStore := trade.NewStore(pool)

	ledgerSvc := ledger.NewService(ledgerStore)
	userSvc := user.NewService(pool, userStore)
	accountSvc := account.NewService(pool, accountStore, ledgerSvc)
	orderSvc := order.NewService(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc)
	tradeSvc := trade.NewService(pool, tradeStore, idempotencyStore, ledgerStore, orderStore, outboxStore, accountSvc)

	r := chi.NewRouter()
	r.Mount("/users", user.NewHandler(userSvc).Routes())
	r.Mount("/accounts", account.NewHandler(accountSvc).Routes())
	orderHandler := order.NewHandler(orderSvc)
	r.Mount("/orders", orderHandler.Routes())
	r.Mount("/trades", trade.NewHandler(tradeSvc).Routes())
	r.Get("/users/{userID}/orders", orderHandler.ListByUser)
	return r
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

func TestIntegration_EndToEnd_OrderTradeFlow(t *testing.T) {
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	srv := httptest.NewServer(buildRouter(t, pool))
	defer srv.Close()

	// Create buyer user
	buyer := createUser(t, srv, "buyer")
	// Create seller user
	seller := createUser(t, srv, "seller")

	// Accounts
	buyerUSDT := createAccount(t, srv, buyer, "USDT")
	buyerBTC := createAccount(t, srv, buyer, "BTC")
	sellerBTC := createAccount(t, srv, seller, "BTC")
	sellerUSDT := createAccount(t, srv, seller, "USDT")
	_ = buyerBTC
	_ = sellerUSDT

	// Seed balances directly via ledger
	seedDirect(t, ctx, pool, buyer, buyerUSDT, "USDT", "100000")
	seedDirect(t, ctx, pool, seller, sellerBTC, "BTC", "10")

	price := "30000"
	qty := "0.5"

	// Place BUY
	status, body := postJSON(t, srv, "/orders", map[string]any{
		"user_id":         buyer,
		"symbol":          "BTC-USDT",
		"side":            "BUY",
		"price":           price,
		"quantity":        qty,
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status, "buy resp: %s", body)
	var buyResp map[string]any
	require.NoError(t, json.Unmarshal(body, &buyResp))
	buyID := buyResp["id"].(string)

	// Place SELL
	status, body = postJSON(t, srv, "/orders", map[string]any{
		"user_id":         seller,
		"symbol":          "BTC-USDT",
		"side":            "SELL",
		"price":           price,
		"quantity":        qty,
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status, "sell resp: %s", body)
	var sellResp map[string]any
	require.NoError(t, json.Unmarshal(body, &sellResp))
	sellID := sellResp["id"].(string)

	// Execute trade
	status, body = postJSON(t, srv, "/trades", map[string]any{
		"symbol":          "BTC-USDT",
		"buy_order_id":    buyID,
		"sell_order_id":   sellID,
		"price":           price,
		"quantity":        qty,
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status, "trade resp: %s", body)
	var tradeResp map[string]any
	require.NoError(t, json.Unmarshal(body, &tradeResp))
	tradeID := tradeResp["id"].(string)

	// Verify 4 TRADE ledger entries summing to zero
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_entries WHERE ref_type = 'TRADE' AND ref_id = $1`,
		tradeID).Scan(&ledgerCount))
	require.Equal(t, 4, ledgerCount)

	var sumStr string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)::text FROM ledger_entries WHERE ref_id = $1`,
		tradeID).Scan(&sumStr))
	s, err := numeric.Parse(sumStr)
	require.NoError(t, err)
	require.Equal(t, 0, numeric.Cmp(s, numeric.Zero()))

	// Verify both orders FILLED
	for _, id := range []string{buyID, sellID} {
		var status string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status FROM orders WHERE id = $1`, id).Scan(&status))
		require.Equal(t, "FILLED", status, "order %s should be FILLED", id)
	}
}

func createUser(t *testing.T, srv *httptest.Server, label string) string {
	t.Helper()
	status, body := postJSON(t, srv, "/users", map[string]any{
		"email":     uuid.NewString() + "@test.com",
		"username":  label + uuid.NewString()[:8],
		"full_name": label,
		"password":  "passw0rd!ABC",
	})
	require.Equal(t, http.StatusCreated, status, "user resp: %s", body)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp["id"].(string)
}

func createAccount(t *testing.T, srv *httptest.Server, userID, asset string) string {
	t.Helper()
	status, body := postJSON(t, srv, "/accounts", map[string]any{
		"user_id": userID,
		"asset":   asset,
	})
	require.Equal(t, http.StatusCreated, status, "account resp: %s", body)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp["id"].(string)
}

func seedDirect(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userIDStr, accountIDStr, asset, amount string) {
	t.Helper()
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)
	accountID, err := uuid.Parse(accountIDStr)
	require.NoError(t, err)
	store := ledger.NewStore(pool)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	id, _ := ledger.NewLedgerEntryID()
	amt, _ := numeric.Parse(amount)
	now := timeutil.Now()
	require.NoError(t, store.Insert(ctx, tx, &ledger.LedgerEntry{
		ID:        id,
		UserID:    userID,
		AccountID: accountID,
		Asset:     asset,
		Amount:    amt,
		EntryType: ledger.EntryTypeTrade,
		RefType:   ledger.RefTypeTrade,
		RefID:     uuid.New(),
		CreatedAt: &now,
	}))
	require.NoError(t, tx.Commit(ctx))
	fmt.Println("seeded", userIDStr, asset, amount)
}
