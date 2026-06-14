package asset

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"

	"mynance/internal/shared"
)

type Service interface {
	ListAssets(ctx context.Context) ([]*Asset, error)
	GetAsset(ctx context.Context, symbol string) (*Asset, error)
	ListNetworks(ctx context.Context, assetSymbol string) ([]*Network, error)
	GetNetwork(ctx context.Context, assetSymbol, name string) (*Network, error)
	GetNetworkByID(ctx context.Context, id uuid.UUID) (*Network, error)
	ValidateAddress(ctx context.Context, networkID uuid.UUID, address string) error
	EnabledSymbols(ctx context.Context) ([]string, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) ListAssets(ctx context.Context) ([]*Asset, error) {
	return s.store.ListAssets(ctx, true)
}

func (s *service) EnabledSymbols(ctx context.Context) ([]string, error) {
	assets, err := s.store.ListAssets(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(assets))
	for _, a := range assets {
		out = append(out, a.Symbol)
	}
	return out, nil
}

func (s *service) GetAsset(ctx context.Context, symbol string) (*Asset, error) {
	a, err := s.store.GetAsset(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if !a.Enabled {
		return nil, shared.ErrNotFound
	}
	return a, nil
}

func (s *service) ListNetworks(ctx context.Context, assetSymbol string) ([]*Network, error) {
	if _, err := s.GetAsset(ctx, assetSymbol); err != nil {
		return nil, err
	}
	return s.store.ListNetworks(ctx, assetSymbol, true)
}

func (s *service) GetNetwork(ctx context.Context, assetSymbol, name string) (*Network, error) {
	n, err := s.store.GetNetwork(ctx, assetSymbol, name)
	if err != nil {
		return nil, err
	}
	if !n.Enabled {
		return nil, shared.ErrNotFound
	}
	return n, nil
}

func (s *service) GetNetworkByID(ctx context.Context, id uuid.UUID) (*Network, error) {
	return s.store.GetNetworkByID(ctx, id)
}

func (s *service) ValidateAddress(ctx context.Context, networkID uuid.UUID, address string) error {
	n, err := s.store.GetNetworkByID(ctx, networkID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrValidation
		}
		return fmt.Errorf("ValidateAddress lookup: %w", err)
	}
	if n.AddressPattern == "" {
		return nil
	}
	re, err := regexp.Compile(n.AddressPattern)
	if err != nil {
		return fmt.Errorf("ValidateAddress compile pattern: %w", err)
	}
	if !re.MatchString(address) {
		return shared.ErrValidation
	}
	return nil
}
