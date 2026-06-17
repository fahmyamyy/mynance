//go:build integration

package trade_test

import (
	"context"
	"os"
	"testing"

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

type tradeFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	tradeSvc   trade.Service
	orderSvc   order.Service
	buyOrder   *order.Order
	sellOrder  *order.Order
	buyUserID  uuid.UUID
	sellUserID uuid.UUID
}

func setupTrade(t *testing.T) *tradeFixture {
	t.Helper()
	ctx := context.Background()
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

	userSvc := user.NewService(pool, userStore)
	accountSvc := account.NewService(pool, accountStore, ledger.NewService(ledgerStore))
	orderSvc := order.NewService(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc)
	tradeSvc := trade.NewService(pool, tradeStore, idempotencyStore, ledgerStore, orderStore, outboxStore, accountSvc)

	buyer := createUserWithAccounts(t, ctx, userSvc, accountSvc, ledgerStore, "100000", "0")
	seller := createUserWithAccounts(t, ctx, userSvc, accountSvc, ledgerStore, "0", "10")

	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")

	buyOrder, err := orderSvc.PlaceOrder(ctx, order.PlaceOrderCommand{
		UserID:         buyer,
		Symbol:         "BTC-USDT",
		Side:           order.SideBuy,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)
	sellOrder, err := orderSvc.PlaceOrder(ctx, order.PlaceOrderCommand{
		UserID:         seller,
		Symbol:         "BTC-USDT",
		Side:           order.SideSell,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)

	return &tradeFixture{
		ctx:        ctx,
		pool:       pool,
		tradeSvc:   tradeSvc,
		orderSvc:   orderSvc,
		buyOrder:   buyOrder,
		sellOrder:  sellOrder,
		buyUserID:  buyer,
		sellUserID: seller,
	}
}

func createUserWithAccounts(t *testing.T, ctx context.Context, userSvc user.Service, accountSvc account.Service, ledgerStore ledger.Store, usdt, btc string) uuid.UUID {
	t.Helper()
	u, err := userSvc.CreateUser(ctx, user.CreateUserCommand{
		Email:    uuid.NewString() + "@test.com",
		Username: "u" + uuid.NewString()[:8],
		FullName: "Test",
		Password: "passw0rd!ABC",
	})
	require.NoError(t, err)
	btcAcct, err := accountSvc.CreateAccount(ctx, u.ID, "BTC")
	require.NoError(t, err)
	usdtAcct, err := accountSvc.CreateAccount(ctx, u.ID, "USDT")
	require.NoError(t, err)

	pool, _ := ctx.Value(ctxPoolKey{}).(*pgxpool.Pool)
	_ = pool
	seedLedgerEntry(t, ctx, ledgerStore, u.ID, usdtAcct.ID, "USDT", usdt)
	seedLedgerEntry(t, ctx, ledgerStore, u.ID, btcAcct.ID, "BTC", btc)
	return u.ID
}

type ctxPoolKey struct{}

func seedLedgerEntry(t *testing.T, ctx context.Context, store ledger.Store, userID, accountID uuid.UUID, asset, amount string) {
	t.Helper()
	if amount == "0" {
		return
	}
	pool := getPoolFromTest(t)
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
}

var sharedPool *pgxpool.Pool

func getPoolFromTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if sharedPool == nil {
		var err error
		sharedPool, err = pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
		require.NoError(t, err)
	}
	return sharedPool
}

func TestIntegration_ExecuteTrade_FourLedgerEntriesZeroSum(t *testing.T) {
	f := setupTrade(t)
	defer f.pool.Close()

	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")

	tr, err := f.tradeSvc.ExecuteTrade(f.ctx, trade.ExecuteTradeCommand{
		Symbol:         "BTC-USDT",
		BuyOrderID:     f.buyOrder.ID,
		SellOrderID:    f.sellOrder.ID,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)

	var count int
	require.NoError(t, f.pool.QueryRow(f.ctx,
		`SELECT count(*) FROM ledger_entries WHERE ref_type = 'TRADE' AND ref_id = $1`,
		tr.ID).Scan(&count))
	require.Equal(t, 4, count)

	var sum string
	require.NoError(t, f.pool.QueryRow(f.ctx,
		`SELECT COALESCE(SUM(amount), 0)::text FROM ledger_entries WHERE ref_id = $1`,
		tr.ID).Scan(&sum))
	s, err := numeric.Parse(sum)
	require.NoError(t, err)
	require.Equal(t, 0, numeric.Cmp(s, numeric.Zero()))
}

func TestIntegration_ExecuteTrade_DuplicateKeyReturns200NoNewRows(t *testing.T) {
	f := setupTrade(t)
	defer f.pool.Close()

	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")
	key := uuid.NewString()

	_, err := f.tradeSvc.ExecuteTrade(f.ctx, trade.ExecuteTradeCommand{
		Symbol: "BTC-USDT", BuyOrderID: f.buyOrder.ID, SellOrderID: f.sellOrder.ID,
		Price: price, Quantity: qty, IdempotencyKey: key,
	})
	require.NoError(t, err)

	var before int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM trades`).Scan(&before))

	_, err = f.tradeSvc.ExecuteTrade(f.ctx, trade.ExecuteTradeCommand{
		Symbol: "BTC-USDT", BuyOrderID: f.buyOrder.ID, SellOrderID: f.sellOrder.ID,
		Price: price, Quantity: qty, IdempotencyKey: key,
	})
	require.Error(t, err)

	var after int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM trades`).Scan(&after))
	require.Equal(t, before, after)
}

func TestIntegration_ExecuteTrade_UpdatesBothOrderFillAndStatus(t *testing.T) {
	f := setupTrade(t)
	defer f.pool.Close()

	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")

	_, err := f.tradeSvc.ExecuteTrade(f.ctx, trade.ExecuteTradeCommand{
		Symbol:         "BTC-USDT",
		BuyOrderID:     f.buyOrder.ID,
		SellOrderID:    f.sellOrder.ID,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)

	buyAfter, err := f.orderSvc.GetOrder(f.ctx, f.buyOrder.ID)
	require.NoError(t, err)
	require.Equal(t, order.StatusFilled, buyAfter.Status)

	sellAfter, err := f.orderSvc.GetOrder(f.ctx, f.sellOrder.ID)
	require.NoError(t, err)
	require.Equal(t, order.StatusFilled, sellAfter.Status)
}
