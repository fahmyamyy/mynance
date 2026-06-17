//go:build integration

package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"mynance/internal/domain"
	"mynance/internal/repository"
)

func TestIntegration_CreateUserAndAccount(t *testing.T) {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	userRepo := repository.NewUserRepository(pool)
	accountRepo := repository.NewAccountRepository(pool)

	// Create user
	userID := uuid.New()
	user := &domain.User{ID: userID, CreatedAt: time.Now().UTC()}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, userRepo.Create(ctx, tx, user))
	require.NoError(t, tx.Commit(ctx))

	// Verify user row exists
	got, err := userRepo.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, userID, got.ID)

	// Create account
	account := &domain.Account{
		ID:        uuid.New(),
		UserID:    userID,
		Asset:     "BTC",
		CreatedAt: time.Now().UTC(),
	}

	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, accountRepo.Create(ctx, tx, account))
	require.NoError(t, tx.Commit(ctx))

	// Verify account row exists
	gotAccount, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, account.ID, gotAccount.ID)
	require.Equal(t, userID, gotAccount.UserID)
	require.Equal(t, "BTC", gotAccount.Asset)
}
