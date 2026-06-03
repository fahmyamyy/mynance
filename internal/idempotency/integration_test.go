//go:build integration

package idempotency_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/idempotency"
	"mynance/internal/shared"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	return pool
}

func TestIntegration_Idempotency_DuplicateRejected(t *testing.T) {
	ctx := context.Background()
	pool := setupPool(t)
	defer pool.Close()

	store := idempotency.NewStore(pool)
	key := uuid.NewString()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tx, key, idempotency.ScopeOrder))
	require.NoError(t, tx.Commit(ctx))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx)
	err = store.Insert(ctx, tx2, key, idempotency.ScopeOrder)
	require.Error(t, err)
	require.True(t, errors.Is(err, shared.ErrDuplicateIdempotencyKey))
}

func TestIntegration_Idempotency_ScopeIsolation(t *testing.T) {
	ctx := context.Background()
	pool := setupPool(t)
	defer pool.Close()

	store := idempotency.NewStore(pool)
	key := uuid.NewString()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tx, key, idempotency.ScopeOrder))
	require.NoError(t, store.Insert(ctx, tx, key, idempotency.ScopeTrade))
	require.NoError(t, tx.Commit(ctx))
}

func TestIntegration_Idempotency_RollbackAllowsRetry(t *testing.T) {
	ctx := context.Background()
	pool := setupPool(t)
	defer pool.Close()

	store := idempotency.NewStore(pool)
	key := uuid.NewString()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tx, key, idempotency.ScopeOrder))
	require.NoError(t, tx.Rollback(ctx))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tx2, key, idempotency.ScopeOrder))
	require.NoError(t, tx2.Commit(ctx))
}
