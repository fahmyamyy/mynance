//go:build integration

package order_test

import (
	"context"
	"errors"
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
	"mynance/internal/shared"
	"mynance/internal/user"
	"mynance/pkg/numeric"
	"mynance/pkg/timeutil"
)

func setup(t *testing.T) (context.Context, *pgxpool.Pool, order.Service, *account.Account, *account.Account) {
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

	userSvc := user.NewService(pool, userStore)
	accountSvc := account.NewService(pool, accountStore, ledger.NewService(ledgerStore))
	orderSvc := order.NewService(pool, orderStore, idempotencyStore, ledgerStore, outboxStore, accountSvc)

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

	// Seed USDT balance for buys
	seedLedger(t, ctx, pool, u.ID, usdtAcct.ID, "USDT", "100000")
	// Seed BTC balance for sells
	seedLedger(t, ctx, pool, u.ID, btcAcct.ID, "BTC", "10")

	return ctx, pool, orderSvc, btcAcct, usdtAcct
}

func seedLedger(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, accountID uuid.UUID, asset, amount string) {
	t.Helper()
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
}

func TestIntegration_PlaceBuyOrder_WritesOrderReserveAndOutbox(t *testing.T) {
	ctx, pool, svc, _, _ := setup(t)
	defer pool.Close()

	userID := getUserFromOrderSvc(t, ctx, pool)
	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")

	o, err := svc.PlaceOrder(ctx, order.PlaceOrderCommand{
		UserID:         userID,
		Symbol:         "BTC-USDT",
		Side:           order.SideBuy,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)
	require.Equal(t, order.StatusOpen, o.Status)

	var orderCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE id = $1`, o.ID).Scan(&orderCount))
	require.Equal(t, 1, orderCount)

	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_entries WHERE ref_type = 'ORDER' AND ref_id = $1 AND entry_type = 'RESERVE'`,
		o.ID).Scan(&ledgerCount))
	require.Equal(t, 1, ledgerCount)

	var outboxCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE event_type = 'ORDER_PLACED' AND payload->>'order_id' = $1`,
		o.ID.String()).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)
}

func TestIntegration_PlaceThenCancel_ReserveReleaseNetZero(t *testing.T) {
	ctx, pool, svc, _, _ := setup(t)
	defer pool.Close()

	userID := getUserFromOrderSvc(t, ctx, pool)
	price, _ := numeric.Parse("30000")
	qty, _ := numeric.Parse("0.5")

	o, err := svc.PlaceOrder(ctx, order.PlaceOrderCommand{
		UserID:         userID,
		Symbol:         "BTC-USDT",
		Side:           order.SideBuy,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)

	cancelled, err := svc.CancelOrder(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, order.StatusCancelled, cancelled.Status)

	var sum string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)::text FROM ledger_entries WHERE ref_id = $1`,
		o.ID).Scan(&sum))
	s, err := numeric.Parse(sum)
	require.NoError(t, err)
	require.Equal(t, 0, numeric.Cmp(s, numeric.Zero()))
}

func TestIntegration_PlaceOrder_InsufficientFunds(t *testing.T) {
	ctx, pool, svc, _, _ := setup(t)
	defer pool.Close()

	userID := getUserFromOrderSvc(t, ctx, pool)
	price, _ := numeric.Parse("9999999")
	qty, _ := numeric.Parse("1000")

	_, err := svc.PlaceOrder(ctx, order.PlaceOrderCommand{
		UserID:         userID,
		Symbol:         "BTC-USDT",
		Side:           order.SideBuy,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: uuid.NewString(),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, shared.ErrInsufficientFunds))

	var orderCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE user_id = $1`, userID).Scan(&orderCount))
	require.Equal(t, 0, orderCount)
}

// helper — fetch the user that setup created via account record
func getUserFromOrderSvc(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT user_id FROM accounts ORDER BY created_at DESC LIMIT 1`).Scan(&id))
	return id
}
