package withdrawal

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, w *Withdrawal) error
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Withdrawal, error)
}

type pgxStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgxStore{db: db} }

const cols = `id, user_id, account_id, asset, network_id, destination_address,
	amount, status, idempotency_key, created_at`

func scan(s interface{ Scan(...any) error }) (*Withdrawal, error) {
	w := &Withdrawal{}
	if err := s.Scan(&w.ID, &w.UserID, &w.AccountID, &w.Asset, &w.NetworkID,
		&w.DestinationAddress, &w.Amount, &w.Status, &w.IdempotencyKey, &w.CreatedAt); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Withdrawal, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+cols+` FROM withdrawals WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("withdrawalStore.ListByUser: %w", err)
	}
	defer rows.Close()
	var out []*Withdrawal
	for rows.Next() {
		w, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("withdrawalStore.ListByUser scan: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, w *Withdrawal) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO withdrawals (id, user_id, account_id, asset, network_id, destination_address,
		                          amount, status, idempotency_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		w.ID, w.UserID, w.AccountID, w.Asset, w.NetworkID, w.DestinationAddress,
		w.Amount, w.Status, w.IdempotencyKey, w.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("withdrawalStore.Insert: %w", err)
	}
	return nil
}
