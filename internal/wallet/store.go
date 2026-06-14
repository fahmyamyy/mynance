package wallet

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
	Create(ctx context.Context, tx pgx.Tx, w *WalletAddress) error
	GetByUserAssetNetwork(ctx context.Context, userID uuid.UUID, asset string, networkID uuid.UUID) (*WalletAddress, error)
	GetByAddress(ctx context.Context, address string) (*WalletAddress, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*WalletAddress, error)
}

type pgxStore struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) Store {
	return &pgxStore{db: db}
}

const walletCols = `id, user_id, asset, network_id, address, created_at`

func scanWallet(s interface{ Scan(...any) error }) (*WalletAddress, error) {
	w := &WalletAddress{}
	if err := s.Scan(&w.ID, &w.UserID, &w.Asset, &w.NetworkID, &w.Address, &w.CreatedAt); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *pgxStore) Create(ctx context.Context, tx pgx.Tx, w *WalletAddress) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO wallet_addresses (id, user_id, asset, network_id, address, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		w.ID, w.UserID, w.Asset, w.NetworkID, w.Address, w.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return shared.ErrConflict
		}
		return fmt.Errorf("walletStore.Create: %w", err)
	}
	return nil
}

func (r *pgxStore) GetByUserAssetNetwork(ctx context.Context, userID uuid.UUID, asset string, networkID uuid.UUID) (*WalletAddress, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+walletCols+` FROM wallet_addresses WHERE user_id = $1 AND asset = $2 AND network_id = $3`,
		userID, asset, networkID,
	)
	w, err := scanWallet(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("walletStore.GetByUserAssetNetwork: %w", err)
	}
	return w, nil
}

func (r *pgxStore) GetByAddress(ctx context.Context, address string) (*WalletAddress, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+walletCols+` FROM wallet_addresses WHERE address = $1`,
		address,
	)
	w, err := scanWallet(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("walletStore.GetByAddress: %w", err)
	}
	return w, nil
}

func (r *pgxStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]*WalletAddress, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+walletCols+` FROM wallet_addresses WHERE user_id = $1 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("walletStore.ListByUser: %w", err)
	}
	defer rows.Close()
	var out []*WalletAddress
	for rows.Next() {
		w, err := scanWallet(rows)
		if err != nil {
			return nil, fmt.Errorf("walletStore.ListByUser scan: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
