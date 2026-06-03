//go:build integration

package user_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/user"
	"mynance/pkg/timeutil"
)

func TestIntegration_CreateAndGetUser(t *testing.T) {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	store := user.NewStore(pool)

	userID, err := user.NewUserID()
	require.NoError(t, err)

	now := timeutil.Now()
	u := &user.User{
		ID:           userID,
		Email:        "test@example.com",
		Username:     "testuser",
		FullName:     "Test User",
		PasswordHash: "hashed",
		Status:       "ACTIVE",
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Create(ctx, tx, u))
	require.NoError(t, tx.Commit(ctx))

	got, err := store.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, userID, got.ID)
	require.Equal(t, "test@example.com", got.Email)
}
