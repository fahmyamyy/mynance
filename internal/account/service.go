package account

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/pkg/timeutil"
)

// LedgerClient is the consumer-side interface for cross-module ledger access.
// Satisfied by ledger.Service — never import ledger's Store directly.
type LedgerClient interface {
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type Service interface {
	CreateAccount(ctx context.Context, userID uuid.UUID, asset string) (*Account, error)
	GetAccount(ctx context.Context, id uuid.UUID) (*Account, error)
	GetAccountByID(ctx context.Context, id uuid.UUID) (*Account, error)
	ListAccounts(ctx context.Context, userID *uuid.UUID, limit, offset int) ([]*Account, error)
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (string, error)
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

type accountService struct {
	db     *pgxpool.Pool
	store  Store
	ledger LedgerClient
}

func NewService(
	db *pgxpool.Pool,
	store Store,
	ledger LedgerClient,
) Service {
	return &accountService{
		db:     db,
		store:  store,
		ledger: ledger,
	}
}

func (s *accountService) CreateAccount(ctx context.Context, userID uuid.UUID, asset string) (*Account, error) {
	id, err := NewAccountID()
	if err != nil {
		return nil, fmt.Errorf("CreateAccount generate id: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateAccount begin: %w", err)
	}
	defer tx.Rollback(ctx)

	now := timeutil.Now()
	acct := &Account{
		ID:        id,
		UserID:    userID,
		Asset:     asset,
		CreatedAt: &now,
	}
	if err := s.store.Create(ctx, tx, acct); err != nil {
		return nil, fmt.Errorf("CreateAccount: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateAccount commit: %w", err)
	}
	return acct, nil
}

func (s *accountService) GetAccount(ctx context.Context, id uuid.UUID) (*Account, error) {
	acct, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetAccount: %w", err)
	}
	return acct, nil
}

func (s *accountService) GetAccountByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	return s.GetAccount(ctx, id)
}

func (s *accountService) ListAccounts(ctx context.Context, userID *uuid.UUID, limit, offset int) ([]*Account, error) {
	if userID != nil {
		accounts, err := s.store.ListByUser(ctx, *userID, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("ListAccounts by user: %w", err)
		}
		return accounts, nil
	}
	accounts, err := s.store.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ListAccounts: %w", err)
	}
	return accounts, nil
}

func (s *accountService) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("DeleteAccount begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.store.Delete(ctx, tx, id); err != nil {
		return fmt.Errorf("DeleteAccount: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteAccount commit: %w", err)
	}
	return nil
}

func (s *accountService) AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error) {
	acct, err := s.store.GetByUserAndAsset(ctx, userID, asset)
	if err != nil {
		return uuid.Nil, fmt.Errorf("AccountID: %w", err)
	}
	return acct.ID, nil
}

func (s *accountService) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	balance, err := s.ledger.SumByUserAsset(ctx, userID, asset)
	if err != nil {
		return "", fmt.Errorf("GetBalance: %w", err)
	}
	return balance, nil
}
