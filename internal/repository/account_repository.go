package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/domain"
	"mynance/internal/service"
)

type pgxAccountRepository struct {
	db *pgxpool.Pool
}

func NewAccountRepository(
	db *pgxpool.Pool,
) service.AccountStore {
	return &pgxAccountRepository{
		db: db,
	}
}

func (r *pgxAccountRepository) Create(ctx context.Context, tx pgx.Tx, account *domain.Account) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO accounts (id, user_id, asset, created_at) VALUES ($1, $2, $3, $4)",
		account.ID, account.UserID, account.Asset, account.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return fmt.Errorf("accountRepository.Create: %w", err)
	}
	return nil
}

func (r *pgxAccountRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	account := &domain.Account{}
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE id = $1",
		id,
	).Scan(&account.ID, &account.UserID, &account.Asset, &account.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("accountRepository.GetByID: %w", err)
	}
	return account, nil
}

func (r *pgxAccountRepository) GetByUserAndAsset(ctx context.Context, userID uuid.UUID, asset string) (*domain.Account, error) {
	account := &domain.Account{}
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE user_id = $1 AND asset = $2",
		userID, asset,
	).Scan(&account.ID, &account.UserID, &account.Asset, &account.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("accountRepository.GetByUserAndAsset: %w", err)
	}
	return account, nil
}

func (r *pgxAccountRepository) List(ctx context.Context, limit, offset int) ([]*domain.Account, error) {
	rows, err := r.db.Query(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts ORDER BY created_at DESC LIMIT $1 OFFSET $2",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("accountRepository.List: %w", err)
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		account := &domain.Account{}
		if err := rows.Scan(&account.ID, &account.UserID, &account.Asset, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("accountRepository.List scan: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *pgxAccountRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Account, error) {
	rows, err := r.db.Query(ctx,
		"SELECT id, user_id, asset, created_at FROM accounts WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("accountRepository.ListByUser: %w", err)
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		account := &domain.Account{}
		if err := rows.Scan(&account.ID, &account.UserID, &account.Asset, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("accountRepository.ListByUser scan: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *pgxAccountRepository) Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		"DELETE FROM accounts WHERE id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("accountRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
