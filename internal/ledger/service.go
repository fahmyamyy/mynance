package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Service is the public interface other modules use to interact with ledger.
// Cross-module clients depend on this, not Store.
type Service interface {
	Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type ledgerService struct {
	store Store
}

func NewService(
	store Store,
) Service {
	return &ledgerService{
		store: store,
	}
}

func (s *ledgerService) Insert(ctx context.Context, tx pgx.Tx, entry *LedgerEntry) error {
	if err := s.store.Insert(ctx, tx, entry); err != nil {
		return fmt.Errorf("ledgerService.Insert: %w", err)
	}
	return nil
}

func (s *ledgerService) SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	balance, err := s.store.SumByUserAsset(ctx, userID, asset)
	if err != nil {
		return "", fmt.Errorf("ledgerService.SumByUserAsset: %w", err)
	}
	return balance, nil
}
