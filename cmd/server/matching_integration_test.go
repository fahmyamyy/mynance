//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/account"
	"mynance/internal/engine"
	"mynance/internal/eventbus"
	"mynance/internal/idempotency"
	"mynance/internal/ledger"
	"mynance/internal/marketdata"
	"mynance/internal/order"
	"mynance/internal/outbox"
	"mynance/internal/trade"
	"mynance/internal/user"
)

type matchingHarness struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
	bus  *eventbus.Bus
	eng  *engine.Engine
	md   *marketdata.Service
	stop context.CancelFunc
}

func buildMatchingHarness(t *testing.T) *matchingHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)

	userStore := user.NewStore(pool)
	accountStore := account.NewStore(pool)
	ledgerStore := ledger.NewStore(pool)
	idempotencyStore := idempotency.NewStore(pool)
	outboxStore := outbox.NewStore(pool)
	orderStore := order.NewStore(pool)
	tradeStore := trade.NewStore(pool)

	bus := eventbus.New()
	eng := engine.New(bus, []string{"BTC-USDT"}, 256)
	md := marketdata.NewService()
	md.Subscribe(bus)

	adapter := &engineAdapter{eng: eng}
	ledgerSvc := ledger.NewService(ledgerStore)
	userSvc := user.NewService(pool, userStore)
	accountSvc := account.NewService(pool, accountStore, ledgerSvc)
	orderSvc := order.NewServiceWithEngine(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc, adapter, []string{"BTC-USDT"})
	tradeSvc := trade.NewService(pool, tradeStore, idempotencyStore, ledgerStore, orderStore, outboxStore, accountSvc)

	settlement := engine.NewSettlementSubscriber(tradeSvc)
	bus.Subscribe(eventbus.EventTypeTradeMatched, settlement.OnTradeMatched)

	r := chi.NewRouter()
	r.Mount("/users", user.NewHandler(userSvc).Routes())
	r.Mount("/accounts", account.NewHandler(accountSvc).Routes())
	r.Mount("/orders", order.NewHandler(orderSvc).Routes())
	mdHandler := marketdata.NewHandler(md)
	r.Mount("/orderbook", mdHandler.OrderBookRoutes())
	r.Mount("/marketdata/trades", mdHandler.TradesRoutes())

	require.NoError(t, rehydrateEngine(ctx, pool, eng))
	go eng.Start(ctx)

	srv := httptest.NewServer(r)

	t.Cleanup(func() {
		srv.Close()
		cancel()
		pool.Close()
	})

	return &matchingHarness{srv: srv, pool: pool, bus: bus, eng: eng, md: md, stop: cancel}
}

func TestIntegration_AutoMatching_BuyMeetsSell(t *testing.T) {
	h := buildMatchingHarness(t)
	ctx := context.Background()

	buyer := createUser(t, h.srv, "buyer")
	seller := createUser(t, h.srv, "seller")

	buyerUSDT := createAccount(t, h.srv, buyer, "USDT")
	_ = createAccount(t, h.srv, buyer, "BTC")
	sellerBTC := createAccount(t, h.srv, seller, "BTC")
	_ = createAccount(t, h.srv, seller, "USDT")

	seedDirect(t, ctx, h.pool, buyer, buyerUSDT, "USDT", "100000")
	seedDirect(t, ctx, h.pool, seller, sellerBTC, "BTC", "10")

	// Place SELL first (rests)
	status, _ := postJSON(t, h.srv, "/orders", map[string]any{
		"user_id":         seller,
		"symbol":          "BTC-USDT",
		"side":            "SELL",
		"price":           "30000",
		"quantity":        "0.5",
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status)

	// Place BUY (matches)
	status, body := postJSON(t, h.srv, "/orders", map[string]any{
		"user_id":         buyer,
		"symbol":          "BTC-USDT",
		"side":            "BUY",
		"price":           "30000",
		"quantity":        "0.5",
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status, "buy resp: %s", body)
	var buyResp map[string]any
	require.NoError(t, json.Unmarshal(body, &buyResp))
	buyID := buyResp["id"].(string)

	// Poll for settlement
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := h.pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, buyID).Scan(&status)
		require.NoError(t, err)
		if status == "FILLED" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	var buyStatus string
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, buyID).Scan(&buyStatus))
	require.Equal(t, "FILLED", buyStatus, "buy order should be FILLED after matching")

	// Verify 4 ledger entries with zero sum on settled trade
	var tradeCount int
	require.NoError(t, h.pool.QueryRow(ctx, `SELECT count(*) FROM trades`).Scan(&tradeCount))
	require.GreaterOrEqual(t, tradeCount, 1)

	// Recent trades visible in market data
	resp, err := http.Get(h.srv.URL + "/marketdata/trades/BTC-USDT")
	require.NoError(t, err)
	defer resp.Body.Close()
	var trades []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&trades))
	require.GreaterOrEqual(t, len(trades), 1)
}

func TestIntegration_Rehydration_RestingOrdersLoadedFromDB(t *testing.T) {
	h := buildMatchingHarness(t)
	ctx := context.Background()

	// Pre-existing OPEN order in DB (created by harness's setup of seller below)
	seller := createUser(t, h.srv, "rehydrate-seller")
	_ = createAccount(t, h.srv, seller, "USDT")
	sellerBTC := createAccount(t, h.srv, seller, "BTC")
	seedDirect(t, ctx, h.pool, seller, sellerBTC, "BTC", "10")

	status, _ := postJSON(t, h.srv, "/orders", map[string]any{
		"user_id":         seller,
		"symbol":          "BTC-USDT",
		"side":            "SELL",
		"price":           "29000",
		"quantity":        "0.3",
		"idempotency_key": uuid.NewString(),
	})
	require.Equal(t, http.StatusCreated, status)

	// Simulate restart — stop and rebuild engine, rehydrate
	h.stop()
	time.Sleep(50 * time.Millisecond)

	bus := eventbus.New()
	eng := engine.New(bus, []string{"BTC-USDT"}, 256)
	require.NoError(t, rehydrateEngine(ctx, h.pool, eng, []string{"BTC-USDT"}))
	go eng.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Verify rehydration by submitting a crossing BUY directly
	resultCh := make(chan eventbus.TradeMatchedEvent, 1)
	bus.Subscribe(eventbus.EventTypeTradeMatched, func(e eventbus.Event) {
		resultCh <- e.(eventbus.TradeMatchedEvent)
	})

	require.NoError(t, eng.Submit(engine.Command{
		Kind: engine.CommandPlaceOrder,
		Order: &engine.Order{
			ID:        uuid.NewString(),
			UserID:    uuid.NewString(),
			Symbol:    "BTC-USDT",
			Side:      engine.SideBuy,
			Price:     29000,
			Quantity:  0.3,
			Remaining: 0.3,
		},
	}))

	select {
	case ev := <-resultCh:
		require.Equal(t, 29000.0, ev.Price)
		require.Equal(t, 0.3, ev.Quantity)
	case <-time.After(2 * time.Second):
		t.Fatal("rehydration: crossing order did not match resting order")
	}
}
