package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/domain"
	"mynance/pkg/timeutil"
)

type AccountStore interface {
	Create(ctx context.Context, tx pgx.Tx, account *domain.Account) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetByUserAndAsset(ctx context.Context, userID uuid.UUID, asset string) (*domain.Account, error)
	List(ctx context.Context, limit, offset int) ([]*domain.Account, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Account, error)
	Delete(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
}

type LedgerStore interface {
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type AccountService interface {
	CreateAccount(ctx context.Context, userID uuid.UUID, asset string) (*domain.Account, error)
	GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetAccountByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	ListAccounts(ctx context.Context, userID *uuid.UUID, limit, offset int) ([]*domain.Account, error)
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type accountService struct {
	db     *pgxpool.Pool
	store  AccountStore
	ledger LedgerStore
}

func NewAccountService(
	db *pgxpool.Pool,
	store AccountStore,
	ledger LedgerStore,
) AccountService {
	return &accountService{
		db:     db,
		store:  store,
		ledger: ledger,
	}
}

func (s *accountService) CreateAccount(ctx context.Context, userID uuid.UUID, asset string) (*domain.Account, error) {
	id, err := domain.AccountID()
	if err != nil {
		return nil, fmt.Errorf("CreateAccount generate id: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateAccount begin: %w", err)
	}
	defer tx.Rollback(ctx)

	now := timeutil.Now()
	account := &domain.Account{
		ID:        id,
		UserID:    userID,
		Asset:     asset,
		CreatedAt: &now,
	}
	if err := s.store.Create(ctx, tx, account); err != nil {
		return nil, fmt.Errorf("CreateAccount: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateAccount commit: %w", err)
	}
	return account, nil
}

func (s *accountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	account, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetAccount: %w", err)
	}
	return account, nil
}

func (s *accountService) GetAccountByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.GetAccount(ctx, id)
}

func (s *accountService) ListAccounts(ctx context.Context, userID *uuid.UUID, limit, offset int) ([]*domain.Account, error) {
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

func (s *accountService) GetBalance(ctx context.Context, userID uuid.UUID, asset string) (string, error) {
	balance, err := s.ledger.SumByUserAsset(ctx, userID, asset)
	if err != nil {
		return "", fmt.Errorf("GetBalance: %w", err)
	}
	return balance, nil
}
