//go:build integration

package ledger_test

import (
	"context"
	"math/big"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/ledger"
	"mynance/pkg/timeutil"
)

func numericFromString(s string) pgtype.Numeric {
	var n pgtype.Numeric
	bi, _ := new(big.Int).SetString(s, 10)
	n.Int = bi
	n.Valid = true
	return n
}

func TestIntegration_LedgerInsertAndSum(t *testing.T) {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	store := ledger.NewStore(pool)
	userID := uuid.New()
	accountID := uuid.New()
	asset := "BTC"
	now := timeutil.Now()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	id1, _ := ledger.NewLedgerEntryID()
	require.NoError(t, store.Insert(ctx, tx, &ledger.LedgerEntry{
		ID:        id1,
		UserID:    userID,
		AccountID: accountID,
		Asset:     asset,
		Amount:    numericFromString("15000000000"),
		EntryType: ledger.EntryTypeTrade,
		RefType:   ledger.RefTypeTrade,
		RefID:     uuid.New(),
		CreatedAt: &now,
	}))

	id2, _ := ledger.NewLedgerEntryID()
	require.NoError(t, store.Insert(ctx, tx, &ledger.LedgerEntry{
		ID:        id2,
		UserID:    userID,
		AccountID: accountID,
		Asset:     asset,
		Amount:    numericFromString("5000000000"),
		EntryType: ledger.EntryTypeTrade,
		RefType:   ledger.RefTypeTrade,
		RefID:     uuid.New(),
		CreatedAt: &now,
	}))

	require.NoError(t, tx.Commit(ctx))

	balance, err := store.SumByUserAsset(ctx, userID, asset)
	require.NoError(t, err)
	require.NotEmpty(t, balance)

	balance, err = store.SumByUserAsset(ctx, uuid.New(), asset)
	require.NoError(t, err)
	require.Equal(t, "0", balance)
}
