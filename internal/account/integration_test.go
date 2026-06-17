//go:build integration

package account_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/account"
	"mynance/pkg/timeutil"
)

func TestIntegration_CreateAndGetAccount(t *testing.T) {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	store := account.NewStore(pool)

	acctID, err := account.NewAccountID()
	require.NoError(t, err)

	now := timeutil.Now()
	acct := &account.Account{
		ID:        acctID,
		UserID:    uuid.New(),
		Asset:     "BTC",
		CreatedAt: &now,
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Create(ctx, tx, acct))
	require.NoError(t, tx.Commit(ctx))

	got, err := store.GetByID(ctx, acctID)
	require.NoError(t, err)
	require.Equal(t, acctID, got.ID)
	require.Equal(t, "BTC", got.Asset)
}
