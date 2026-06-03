package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/account"
	"mynance/internal/idempotency"
	"mynance/internal/ledger"
	"mynance/internal/outbox"
	"mynance/internal/shared"
	"mynance/pkg/numeric"
	"mynance/pkg/timeutil"
)

type IdempotencyStore interface {
	Insert(ctx context.Context, tx pgx.Tx, key string, scope idempotency.Scope) error
}

type LedgerStore interface {
	Insert(ctx context.Context, tx pgx.Tx, entry *ledger.LedgerEntry) error
	SumByUserAsset(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type OutboxStore interface {
	Insert(ctx context.Context, tx pgx.Tx, event *outbox.OutboxEvent) error
}

type AccountLookup interface {
	GetAccount(ctx context.Context, id uuid.UUID) (*account.Account, error)
}

type WithdrawCommand struct {
	UserID         uuid.UUID
	AccountID      uuid.UUID
	Amount         pgtype.Numeric
	IdempotencyKey string
}

type WithdrawResult struct {
	Withdrawal *Withdrawal
	NewBalance string
}

type Service interface {
	Withdraw(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error)
}

type withdrawalService struct {
	db       *pgxpool.Pool
	store    Store
	idemp    IdempotencyStore
	ledger   LedgerStore
	outbox   OutboxStore
	accounts AccountLookup
}

func NewService(db *pgxpool.Pool, store Store, idemp IdempotencyStore, ledgerStore LedgerStore, outboxStore OutboxStore, accounts AccountLookup) Service {
	return &withdrawalService{db: db, store: store, idemp: idemp, ledger: ledgerStore, outbox: outboxStore, accounts: accounts}
}

func (s *withdrawalService) Withdraw(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error) {
	if numeric.Cmp(cmd.Amount, numeric.Zero()) <= 0 {
		return nil, shared.ErrBadRequest
	}
	acct, err := s.accounts.GetAccount(ctx, cmd.AccountID)
	if err != nil {
		return nil, fmt.Errorf("Withdraw account: %w", err)
	}
	if acct.UserID != cmd.UserID {
		return nil, shared.ErrForbidden
	}
	balanceStr, err := s.ledger.SumByUserAsset(ctx, cmd.UserID, acct.Asset)
	if err != nil {
		return nil, fmt.Errorf("Withdraw balance: %w", err)
	}
	cmp, err := numeric.CmpString(cmd.Amount, balanceStr)
	if err != nil {
		return nil, fmt.Errorf("Withdraw balance cmp: %w", err)
	}
	if cmp > 0 {
		return nil, shared.ErrInsufficientFunds
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("Withdraw begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := s.idemp.Insert(ctx, tx, cmd.IdempotencyKey, idempotency.ScopeWithdraw); err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			return nil, err
		}
		return nil, fmt.Errorf("Withdraw idempotency: %w", err)
	}
	id, err := NewID()
	if err != nil {
		return nil, err
	}
	entryID, err := ledger.NewLedgerEntryID()
	if err != nil {
		return nil, err
	}
	now := timeutil.Now()
	if err := s.ledger.Insert(ctx, tx, &ledger.LedgerEntry{
		ID: entryID, UserID: cmd.UserID, AccountID: cmd.AccountID, Asset: acct.Asset,
		Amount: numeric.Neg(cmd.Amount), EntryType: ledger.EntryTypeWithdraw,
		RefType: ledger.RefTypeWithdraw, RefID: id, CreatedAt: &now,
	}); err != nil {
		return nil, fmt.Errorf("Withdraw ledger: %w", err)
	}
	w := &Withdrawal{
		ID: id, UserID: cmd.UserID, AccountID: cmd.AccountID, Asset: acct.Asset,
		Amount: cmd.Amount, Status: StatusCompleted, IdempotencyKey: cmd.IdempotencyKey, CreatedAt: &now,
	}
	if err := s.store.Insert(ctx, tx, w); err != nil {
		return nil, fmt.Errorf("Withdraw insert: %w", err)
	}
	if err := s.insertOutbox(ctx, tx, w); err != nil {
		return nil, fmt.Errorf("Withdraw outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("Withdraw commit: %w", err)
	}
	newBalance, _ := s.ledger.SumByUserAsset(ctx, cmd.UserID, acct.Asset)
	return &WithdrawResult{Withdrawal: w, NewBalance: newBalance}, nil
}

func (s *withdrawalService) insertOutbox(ctx context.Context, tx pgx.Tx, w *Withdrawal) error {
	id, err := outbox.NewID()
	if err != nil {
		return err
	}
	now := timeutil.Now()
	payload, _ := json.Marshal(map[string]any{
		"withdrawal_id": w.ID.String(),
		"user_id":       w.UserID.String(),
		"asset":         w.Asset,
		"amount":        numeric.String(w.Amount),
	})
	return s.outbox.Insert(ctx, tx, &outbox.OutboxEvent{
		ID: id, EventType: "WITHDRAW_REQUESTED", Payload: payload, CreatedAt: &now,
	})
}
