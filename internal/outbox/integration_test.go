//go:build integration

package outbox_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/outbox"
	"mynance/pkg/timeutil"
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

func TestIntegration_OutboxPublisher_ProcessesPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := setupPool(t)
	defer pool.Close()

	store := outbox.NewStore(pool)

	id, err := outbox.NewID()
	require.NoError(t, err)
	now := timeutil.Now()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tx, &outbox.OutboxEvent{
		ID:        id,
		EventType: outbox.EventTypeOrderPlaced,
		Payload:   []byte(`{"order_id":"test"}`),
		CreatedAt: &now,
	}))
	require.NoError(t, tx.Commit(ctx))

	publisher := outbox.NewPublisher(pool, store, nil, 100*time.Millisecond)
	pubCtx, pubCancel := context.WithCancel(ctx)
	go publisher.Start(pubCtx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := pool.QueryRow(ctx, `SELECT status FROM outbox_events WHERE id = $1`, id).Scan(&status)
		require.NoError(t, err)
		if status == string(outbox.StatusProcessed) {
			pubCancel()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	pubCancel()
	t.Fatal("event not processed within timeout")
}
