package deposit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/ledger"
	"mynance/internal/outbox"
	"mynance/internal/shared"
	"mynance/internal/wallet"
	"mynance/pkg/numeric"
	"mynance/pkg/timeutil"
)

type WalletLookup interface {
	GetByAddress(ctx context.Context, address string) (*wallet.WalletAddress, error)
}

type LedgerStore interface {
	Insert(ctx context.Context, tx pgx.Tx, entry *ledger.LedgerEntry) error
}

type OutboxStore interface {
	Insert(ctx context.Context, tx pgx.Tx, event *outbox.OutboxEvent) error
}

type AccountLookup interface {
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

type IntakeCommand struct {
	Address string
	Asset   string
	Amount  pgtype.Numeric
	TxHash  string
}

type Service interface {
	Intake(ctx context.Context, cmd IntakeCommand) (*Deposit, error)
	Confirm(ctx context.Context, id uuid.UUID) (*Deposit, error)
	Reject(ctx context.Context, id uuid.UUID) (*Deposit, error)
	ListMine(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Deposit, error)
	Simulate(ctx context.Context, userID uuid.UUID, cmd IntakeCommand) (*Deposit, error)
}

// MaxSandboxDepositAmount caps a single sandbox-simulated deposit so a fat
// finger in the FE cannot mint absurd balances during testing.
const MaxSandboxDepositAmount = "100000"

type depositService struct {
	db       *pgxpool.Pool
	store    Store
	wallets  WalletLookup
	ledger   LedgerStore
	outbox   OutboxStore
	accounts AccountLookup
}

func NewService(db *pgxpool.Pool, store Store, wallets WalletLookup, ledgerStore LedgerStore, outboxStore OutboxStore, accounts AccountLookup) Service {
	return &depositService{db: db, store: store, wallets: wallets, ledger: ledgerStore, outbox: outboxStore, accounts: accounts}
}

func (s *depositService) Intake(ctx context.Context, cmd IntakeCommand) (*Deposit, error) {
	w, err := s.wallets.GetByAddress(ctx, cmd.Address)
	if err != nil {
		return nil, fmt.Errorf("Intake address lookup: %w", err)
	}
	if w.Asset != cmd.Asset {
		return nil, shared.ErrValidation
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("Intake begin: %w", err)
	}
	defer tx.Rollback(ctx)
	id, err := NewID()
	if err != nil {
		return nil, err
	}
	now := timeutil.Now()
	d := &Deposit{
		ID: id, UserID: w.UserID, Asset: cmd.Asset, NetworkID: w.NetworkID, Address: cmd.Address,
		Amount: cmd.Amount, TxHash: cmd.TxHash, Status: StatusPending, CreatedAt: &now,
	}
	if err := s.store.Create(ctx, tx, d); err != nil {
		if errors.Is(err, shared.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("Intake create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("Intake commit: %w", err)
	}
	return d, nil
}

func (s *depositService) Confirm(ctx context.Context, id uuid.UUID) (*Deposit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("Confirm begin: %w", err)
	}
	defer tx.Rollback(ctx)
	d, err := s.store.GetByIDTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("Confirm lookup: %w", err)
	}
	if d.Status != StatusPending {
		return nil, shared.ErrInvalidStateTransition
	}
	acctID, err := s.accounts.AccountID(ctx, d.UserID, d.Asset)
	if err != nil {
		return nil, fmt.Errorf("Confirm account: %w", err)
	}
	entryID, err := ledger.NewLedgerEntryID()
	if err != nil {
		return nil, err
	}
	now := timeutil.Now()
	if err := s.ledger.Insert(ctx, tx, &ledger.LedgerEntry{
		ID: entryID, UserID: d.UserID, AccountID: acctID, Asset: d.Asset,
		Amount: d.Amount, EntryType: ledger.EntryTypeDeposit, RefType: ledger.RefTypeDeposit, RefID: d.ID, CreatedAt: &now,
	}); err != nil {
		return nil, fmt.Errorf("Confirm ledger: %w", err)
	}
	if err := s.store.UpdateStatus(ctx, tx, d.ID, StatusConfirmed, &now); err != nil {
		return nil, fmt.Errorf("Confirm status: %w", err)
	}
	d.Status = StatusConfirmed
	d.ConfirmedAt = &now
	if err := s.insertOutbox(ctx, tx, "DEPOSIT_CONFIRMED", d); err != nil {
		return nil, fmt.Errorf("Confirm outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("Confirm commit: %w", err)
	}
	return d, nil
}

func (s *depositService) Reject(ctx context.Context, id uuid.UUID) (*Deposit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("Reject begin: %w", err)
	}
	defer tx.Rollback(ctx)
	d, err := s.store.GetByIDTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("Reject lookup: %w", err)
	}
	if d.Status != StatusPending {
		return nil, shared.ErrInvalidStateTransition
	}
	if err := s.store.UpdateStatus(ctx, tx, d.ID, StatusRejected, nil); err != nil {
		return nil, fmt.Errorf("Reject status: %w", err)
	}
	d.Status = StatusRejected
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("Reject commit: %w", err)
	}
	return d, nil
}

func (s *depositService) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Deposit, error) {
	return s.store.ListByUser(ctx, userID, limit, offset)
}

// Simulate runs the real Intake + Confirm flow against the caller's own
// wallet address. Gated to sandbox via route registration in main.go — when
// the flag is off the endpoint is not mounted, so this method is unreachable.
func (s *depositService) Simulate(ctx context.Context, userID uuid.UUID, cmd IntakeCommand) (*Deposit, error) {
	w, err := s.wallets.GetByAddress(ctx, cmd.Address)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("Simulate wallet lookup: %w", err)
	}
	if w.UserID != userID {
		return nil, shared.ErrNotFound
	}

	cmp, err := numeric.CmpString(cmd.Amount, MaxSandboxDepositAmount)
	if err != nil {
		return nil, shared.ErrValidation
	}
	if cmp > 0 {
		return nil, shared.ErrValidation
	}

	if cmd.TxHash == "" {
		txID, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("Simulate tx hash: %w", err)
		}
		cmd.TxHash = "sandbox-" + txID.String()
	}

	d, err := s.Intake(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return s.Confirm(ctx, d.ID)
}

func (s *depositService) insertOutbox(ctx context.Context, tx pgx.Tx, eventType string, d *Deposit) error {
	id, err := outbox.NewID()
	if err != nil {
		return err
	}
	now := timeutil.Now()
	payload, _ := json.Marshal(map[string]any{
		"deposit_id": d.ID.String(),
		"user_id":    d.UserID.String(),
		"asset":      d.Asset,
		"amount":     numeric.String(d.Amount),
	})
	return s.outbox.Insert(ctx, tx, &outbox.OutboxEvent{
		ID: id, EventType: outbox.EventType(eventType), Payload: payload, CreatedAt: &now,
	})
}
