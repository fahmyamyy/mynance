package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
	"mynance/pkg/timeutil"
)

const pgUniqueViolation = "23505"

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, key string, scope Scope) error
}

type pgxStore struct {
	db *pgxpool.Pool
}

func NewStore(
	db *pgxpool.Pool,
) Store {
	return &pgxStore{
		db: db,
	}
}

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, key string, scope Scope) error {
	id, err := NewID()
	if err != nil {
		return fmt.Errorf("idempotencyStore.Insert id: %w", err)
	}
	now := timeutil.Now()
	_, err = tx.Exec(ctx,
		`INSERT INTO idempotency_keys (id, key, scope, created_at)
		 VALUES ($1, $2, $3, $4)`,
		id, key, scope, now,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return shared.ErrDuplicateIdempotencyKey
		}
		return fmt.Errorf("idempotencyStore.Insert: %w", err)
	}
	return nil
}
