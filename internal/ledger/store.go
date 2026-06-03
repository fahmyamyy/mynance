package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
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

func (r *pgxStore) Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO ledger_entries (id, user_id, account_id, asset, amount, entry_type, ref_type, ref_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ID, entry.UserID, entry.AccountID, entry.Asset,
		entry.Amount, entry.EntryType, entry.RefType, entry.RefID, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("ledgerStore.Insert: %w", err)
	}
	return nil
}

func (r *pgxStore) SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	var sum pgtype.Numeric
	err := r.db.QueryRow(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE user_id = $1 AND asset = $2",
		userID, asset,
	).Scan(&sum)
	if err != nil {
		return "", fmt.Errorf("ledgerStore.SumByUserAsset: %w", err)
	}

	if !sum.Valid {
		return "0", nil
	}

	text, err := sum.Value()
	if err != nil {
		return "", fmt.Errorf("ledgerStore.SumByUserAsset numeric value: %w", err)
	}
	if text == nil {
		return "0", nil
	}
	return fmt.Sprintf("%v", text), nil
}
