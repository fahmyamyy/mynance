package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
)

type Store interface {
	Create(ctx context.Context, tx pgx.Tx, account *Account) error
	GetByID(ctx context.Context, id uuid.UUID) (*Account, error)
	GetByUserAndAsset(ctx context.Context, userID uuid.UUID, asset string) (*Account, error)
	List(ctx context.Context, limit, offset int) ([]*Account, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Account, error)
	Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
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

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, acct *Account) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO accounts (id, user_id, asset, created_at) VALUES ($1, $2, $3, $4)",
		acct.ID, acct.UserID, acct.Asset, acct.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return shared.ErrConflict
		}
		return fmt.Errorf("accountStore.Create: %w", err)
	}
	return nil
}

func (r *pgxStore) GetByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	acct := &Account{}
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE id = $1",
		id,
	).Scan(&acct.ID, &acct.UserID, &acct.Asset, &acct.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("accountStore.GetByID: %w", err)
	}
	return acct, nil
}

func (r *pgxStore) GetByUserAndAsset(ctx context.Context, userID uuid.UUID, asset string) (*Account, error) {
	acct := &Account{}
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE user_id = $1 AND asset = $2",
		userID, asset,
	).Scan(&acct.ID, &acct.UserID, &acct.Asset, &acct.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("accountStore.GetByUserAndAsset: %w", err)
	}
	return acct, nil
}

func (r *pgxStore) List(ctx context.Context, limit, offset int) ([]*Account, error) {
	rows, err := r.db.Query(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts ORDER BY created_at DESC LIMIT $1 OFFSET $2",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("accountStore.List: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		acct := &Account{}
		if err := rows.Scan(&acct.ID, &acct.UserID, &acct.Asset, &acct.CreatedAt); err != nil {
			return nil, fmt.Errorf("accountStore.List scan: %w", err)
		}
		accounts = append(accounts, acct)
	}
	return accounts, rows.Err()
}

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Account, error) {
	rows, err := r.db.Query(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("accountStore.ListByUser: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		acct := &Account{}
		if err := rows.Scan(&acct.ID, &acct.UserID, &acct.Asset, &acct.CreatedAt); err != nil {
			return nil, fmt.Errorf("accountStore.ListByUser scan: %w", err)
		}
		accounts = append(accounts, acct)
	}
	return accounts, rows.Err()
}

func (r *pgxStore) Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		"DELETE FROM accounts WHERE id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("accountStore.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}
