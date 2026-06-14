package asset

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
)

type Store interface {
	ListAssets(ctx context.Context, enabledOnly bool) ([]*Asset, error)
	GetAsset(ctx context.Context, symbol string) (*Asset, error)
	ListNetworks(ctx context.Context, assetSymbol string, enabledOnly bool) ([]*Network, error)
	GetNetwork(ctx context.Context, assetSymbol, name string) (*Network, error)
	GetNetworkByID(ctx context.Context, id uuid.UUID) (*Network, error)
}

type pgxStore struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) Store {
	return &pgxStore{db: db}
}

const assetCols = `symbol, name, decimals,
                   min_deposit::text, min_withdrawal::text,
                   enabled, created_at`

const networkCols = `id, asset_symbol, name,
                     COALESCE(chain_id, ''), COALESCE(address_pattern, ''),
                     withdrawal_fee::text, min_confirmations,
                     enabled, created_at`

func scanAsset(row pgx.Row) (*Asset, error) {
	a := &Asset{}
	if err := row.Scan(&a.Symbol, &a.Name, &a.Decimals,
		&a.MinDeposit, &a.MinWithdrawal, &a.Enabled, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

func scanNetwork(row pgx.Row) (*Network, error) {
	n := &Network{}
	if err := row.Scan(&n.ID, &n.AssetSymbol, &n.Name,
		&n.ChainID, &n.AddressPattern,
		&n.WithdrawalFee, &n.MinConfirmations,
		&n.Enabled, &n.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return n, nil
}

func (s *pgxStore) ListAssets(ctx context.Context, enabledOnly bool) ([]*Asset, error) {
	q := `SELECT ` + assetCols + ` FROM assets`
	if enabledOnly {
		q += ` WHERE enabled = TRUE`
	}
	q += ` ORDER BY symbol`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("assetStore.ListAssets: %w", err)
	}
	defer rows.Close()
	var out []*Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("assetStore.ListAssets scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *pgxStore) GetAsset(ctx context.Context, symbol string) (*Asset, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+assetCols+` FROM assets WHERE symbol = $1`, symbol)
	a, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("assetStore.GetAsset: %w", err)
	}
	return a, nil
}

func (s *pgxStore) ListNetworks(ctx context.Context, assetSymbol string, enabledOnly bool) ([]*Network, error) {
	q := `SELECT ` + networkCols + ` FROM networks WHERE asset_symbol = $1`
	if enabledOnly {
		q += ` AND enabled = TRUE`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(ctx, q, assetSymbol)
	if err != nil {
		return nil, fmt.Errorf("assetStore.ListNetworks: %w", err)
	}
	defer rows.Close()
	var out []*Network
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, fmt.Errorf("assetStore.ListNetworks scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *pgxStore) GetNetwork(ctx context.Context, assetSymbol, name string) (*Network, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+networkCols+` FROM networks WHERE asset_symbol = $1 AND name = $2`,
		assetSymbol, name)
	n, err := scanNetwork(row)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("assetStore.GetNetwork: %w", err)
	}
	return n, nil
}

func (s *pgxStore) GetNetworkByID(ctx context.Context, id uuid.UUID) (*Network, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+networkCols+` FROM networks WHERE id = $1`, id)
	n, err := scanNetwork(row)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("assetStore.GetNetworkByID: %w", err)
	}
	return n, nil
}
