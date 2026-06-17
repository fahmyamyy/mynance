package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/shared"
	"mynance/pkg/timeutil"
)

type AccountLookup interface {
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

// AssetCatalog supplies the network catalog the wallet service needs.
// Implemented by an adapter over internal/asset.Service in main.go.
type AssetCatalog interface {
	NetworkID(ctx context.Context, assetSymbol, networkName string) (uuid.UUID, error)
}

type Service interface {
	GetOrCreateAddress(ctx context.Context, userID uuid.UUID, asset, networkName string) (*WalletAddress, error)
	ListMyAddresses(ctx context.Context, userID uuid.UUID) ([]*WalletAddress, error)
	GetByAddress(ctx context.Context, address string) (*WalletAddress, error)
}

type walletService struct {
	db       *pgxpool.Pool
	store    Store
	accounts AccountLookup
	catalog  AssetCatalog
}

func NewService(db *pgxpool.Pool, store Store, accounts AccountLookup, catalog AssetCatalog) Service {
	return &walletService{db: db, store: store, accounts: accounts, catalog: catalog}
}

func (s *walletService) GetOrCreateAddress(ctx context.Context, userID uuid.UUID, asset, networkName string) (*WalletAddress, error) {
	networkID, err := s.catalog.NetworkID(ctx, asset, networkName)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrValidation
		}
		return nil, fmt.Errorf("GetOrCreateAddress network: %w", err)
	}

	existing, err := s.store.GetByUserAssetNetwork(ctx, userID, asset, networkID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("GetOrCreateAddress lookup: %w", err)
	}

	if _, err := s.accounts.AccountID(ctx, userID, asset); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrValidation
		}
		return nil, fmt.Errorf("GetOrCreateAddress account check: %w", err)
	}

	for attempt := 0; attempt < 3; attempt++ {
		addr, err := generateMockAddress(networkName)
		if err != nil {
			return nil, fmt.Errorf("GetOrCreateAddress generate: %w", err)
		}
		id, err := NewID()
		if err != nil {
			return nil, err
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("GetOrCreateAddress begin: %w", err)
		}
		now := timeutil.Now()
		w := &WalletAddress{
			ID: id, UserID: userID, Asset: asset, NetworkID: networkID,
			Address: addr, CreatedAt: &now,
		}
		if err := s.store.Create(ctx, tx, w); err != nil {
			tx.Rollback(ctx)
			if errors.Is(err, shared.ErrConflict) {
				continue
			}
			return nil, fmt.Errorf("GetOrCreateAddress create: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("GetOrCreateAddress commit: %w", err)
		}
		return w, nil
	}
	return nil, fmt.Errorf("GetOrCreateAddress: exhausted retries")
}

func (s *walletService) ListMyAddresses(ctx context.Context, userID uuid.UUID) ([]*WalletAddress, error) {
	return s.store.ListByUser(ctx, userID)
}

func (s *walletService) GetByAddress(ctx context.Context, address string) (*WalletAddress, error) {
	return s.store.GetByAddress(ctx, address)
}
