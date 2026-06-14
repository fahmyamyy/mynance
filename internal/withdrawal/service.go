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
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

// AssetCatalog lets the withdrawal service resolve a (asset, network) pair
// to a network row and validate the destination address against that
// network's pattern. Wired to internal/asset.Service in main.go.
type AssetCatalog interface {
	NetworkID(ctx context.Context, assetSymbol, networkName string) (uuid.UUID, error)
	ValidateAddress(ctx context.Context, networkID uuid.UUID, address string) error
}

type WithdrawCommand struct {
	UserID             uuid.UUID
	Asset              string
	NetworkName        string
	DestinationAddress string
	Amount             pgtype.Numeric
	IdempotencyKey     string
}

type WithdrawResult struct {
	Withdrawal *Withdrawal
	NewBalance string
}

type Service interface {
	Withdraw(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error)
	Simulate(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error)
	ListMine(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Withdrawal, error)
}

// MaxSandboxWithdrawalAmount caps a single sandbox-simulated withdrawal so
// the FE cannot drain the ledger with absurd numbers during testing.
const MaxSandboxWithdrawalAmount = "100000"

type withdrawalService struct {
	db       *pgxpool.Pool
	store    Store
	idemp    IdempotencyStore
	ledger   LedgerStore
	outbox   OutboxStore
	accounts AccountLookup
	catalog  AssetCatalog
}

func NewService(
	db *pgxpool.Pool,
	store Store,
	idemp IdempotencyStore,
	ledgerStore LedgerStore,
	outboxStore OutboxStore,
	accounts AccountLookup,
	catalog AssetCatalog,
) Service {
	return &withdrawalService{
		db:       db,
		store:    store,
		idemp:    idemp,
		ledger:   ledgerStore,
		outbox:   outboxStore,
		accounts: accounts,
		catalog:  catalog,
	}
}

func (s *withdrawalService) Withdraw(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error) {
	if numeric.Cmp(cmd.Amount, numeric.Zero()) <= 0 {
		return nil, shared.ErrBadRequest
	}
	networkID, err := s.catalog.NetworkID(ctx, cmd.Asset, cmd.NetworkName)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrValidation
		}
		return nil, fmt.Errorf("Withdraw network: %w", err)
	}
	if err := s.catalog.ValidateAddress(ctx, networkID, cmd.DestinationAddress); err != nil {
		return nil, err
	}
	accountID, err := s.accounts.AccountID(ctx, cmd.UserID, cmd.Asset)
	if err != nil {
		return nil, fmt.Errorf("Withdraw account: %w", err)
	}
	balanceStr, err := s.ledger.SumByUserAsset(ctx, cmd.UserID, cmd.Asset)
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
		ID: entryID, UserID: cmd.UserID, AccountID: accountID, Asset: cmd.Asset,
		Amount: numeric.Neg(cmd.Amount), EntryType: ledger.EntryTypeWithdraw,
		RefType: ledger.RefTypeWithdraw, RefID: id, CreatedAt: &now,
	}); err != nil {
		return nil, fmt.Errorf("Withdraw ledger: %w", err)
	}
	w := &Withdrawal{
		ID: id, UserID: cmd.UserID, AccountID: accountID, Asset: cmd.Asset,
		NetworkID: networkID, DestinationAddress: cmd.DestinationAddress,
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
	newBalance, _ := s.ledger.SumByUserAsset(ctx, cmd.UserID, cmd.Asset)
	return &WithdrawResult{Withdrawal: w, NewBalance: newBalance}, nil
}

func (s *withdrawalService) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Withdrawal, error) {
	return s.store.ListByUser(ctx, userID, limit, offset)
}

// Simulate auto-fills an idempotency key, applies the sandbox amount cap, and
// delegates to the real Withdraw flow so the ledger debit and outbox event
// are identical to a production withdrawal. Gated to sandbox via route
// registration in main.go.
func (s *withdrawalService) Simulate(ctx context.Context, cmd WithdrawCommand) (*WithdrawResult, error) {
	cmp, err := numeric.CmpString(cmd.Amount, MaxSandboxWithdrawalAmount)
	if err != nil {
		return nil, shared.ErrValidation
	}
	if cmp > 0 {
		return nil, shared.ErrValidation
	}
	if cmd.IdempotencyKey == "" {
		key, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("Simulate idempotency: %w", err)
		}
		cmd.IdempotencyKey = "sandbox-" + key.String()
	}
	return s.Withdraw(ctx, cmd)
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
