package withdrawal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, w *Withdrawal) error
}

type pgxStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgxStore{db: db} }

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, w *Withdrawal) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO withdrawals (id, user_id, account_id, asset, amount, status, idempotency_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		w.ID, w.UserID, w.AccountID, w.Asset, w.Amount, w.Status, w.IdempotencyKey, w.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("withdrawalStore.Insert: %w", err)
	}
	return nil
}
