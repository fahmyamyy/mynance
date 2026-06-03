package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

type EngineSubmitter interface {
	SubmitPlace(orderID, userID, symbol, side string, price, quantity float64) error
	SubmitCancel(orderID, symbol string) error
}

type noopEngine struct{}

func (noopEngine) SubmitPlace(_, _, _, _ string, _, _ float64) error { return nil }
func (noopEngine) SubmitCancel(_, _ string) error                    { return nil }

type PlaceOrderCommand struct {
	UserID         uuid.UUID
	Symbol         string
	Side           Side
	Price          pgtype.Numeric
	Quantity       pgtype.Numeric
	IdempotencyKey string
}

type Service interface {
	PlaceOrder(ctx context.Context, cmd PlaceOrderCommand) (*Order, error)
	CancelOrder(ctx context.Context, id uuid.UUID) (*Order, error)
	GetOrder(ctx context.Context, id uuid.UUID) (*Order, error)
	ListOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error)
}

type orderService struct {
	db          *pgxpool.Pool
	store       Store
	idempotency IdempotencyStore
	ledger      LedgerStore
	outbox      OutboxStore
	accounts    AccountLookup
	engine      EngineSubmitter
}

func NewService(
	db *pgxpool.Pool,
	store Store,
	idempotencyStore IdempotencyStore,
	ledgerStore LedgerStore,
	outboxStore OutboxStore,
	accounts AccountLookup,
) Service {
	return &orderService{
		db:          db,
		store:       store,
		idempotency: idempotencyStore,
		ledger:      ledgerStore,
		outbox:      outboxStore,
		accounts:    accounts,
		engine:      noopEngine{},
	}
}

func NewServiceWithEngine(
	db *pgxpool.Pool,
	store Store,
	idempotencyStore IdempotencyStore,
	ledgerStore LedgerStore,
	outboxStore OutboxStore,
	accounts AccountLookup,
	engine EngineSubmitter,
) Service {
	if engine == nil {
		engine = noopEngine{}
	}
	return &orderService{
		db:          db,
		store:       store,
		idempotency: idempotencyStore,
		ledger:      ledgerStore,
		outbox:      outboxStore,
		accounts:    accounts,
		engine:      engine,
	}
}

func (s *orderService) PlaceOrder(ctx context.Context, cmd PlaceOrderCommand) (*Order, error) {
	base, quote, err := SplitSymbol(cmd.Symbol)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder: %w", err)
	}

	var reserveAsset string
	var reserveAmount pgtype.Numeric
	switch cmd.Side {
	case SideBuy:
		reserveAsset = quote
		reserveAmount = numeric.Mul(cmd.Price, cmd.Quantity)
	case SideSell:
		reserveAsset = base
		reserveAmount = cmd.Quantity
	default:
		return nil, fmt.Errorf("PlaceOrder: invalid side %q", cmd.Side)
	}

	accountID, err := s.accounts.AccountID(ctx, cmd.UserID, reserveAsset)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder lookup account: %w", err)
	}

	balanceStr, err := s.ledger.SumByUserAsset(ctx, cmd.UserID, reserveAsset)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder balance: %w", err)
	}
	cmpResult, err := numeric.CmpString(reserveAmount, balanceStr)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder balance compare: %w", err)
	}
	if cmpResult > 0 {
		return nil, shared.ErrInsufficientFunds
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.idempotency.Insert(ctx, tx, cmd.IdempotencyKey, idempotency.ScopeOrder); err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			return nil, err
		}
		return nil, fmt.Errorf("PlaceOrder idempotency: %w", err)
	}

	orderID, err := NewID()
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder id: %w", err)
	}
	now := timeutil.Now()
	o := &Order{
		ID:             orderID,
		UserID:         cmd.UserID,
		Symbol:         cmd.Symbol,
		Side:           cmd.Side,
		Price:          cmd.Price,
		Quantity:       cmd.Quantity,
		FilledQuantity: numeric.Zero(),
		Status:         StatusOpen,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}
	if err := s.store.Create(ctx, tx, o); err != nil {
		return nil, fmt.Errorf("PlaceOrder create: %w", err)
	}

	entryID, err := ledger.NewLedgerEntryID()
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder entry id: %w", err)
	}
	negative := numeric.Neg(reserveAmount)
	entry := &ledger.LedgerEntry{
		ID:        entryID,
		UserID:    cmd.UserID,
		AccountID: accountID,
		Asset:     reserveAsset,
		Amount:    negative,
		EntryType: ledger.EntryTypeReserve,
		RefType:   ledger.RefTypeOrder,
		RefID:     orderID,
		CreatedAt: &now,
	}
	if err := s.ledger.Insert(ctx, tx, entry); err != nil {
		return nil, fmt.Errorf("PlaceOrder reserve: %w", err)
	}

	if err := s.insertOutbox(ctx, tx, outbox.EventTypeOrderPlaced, orderPlacedPayload(o)); err != nil {
		return nil, fmt.Errorf("PlaceOrder outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("PlaceOrder commit: %w", err)
	}

	priceF, _ := strconv.ParseFloat(numeric.String(cmd.Price), 64)
	qtyF, _ := strconv.ParseFloat(numeric.String(cmd.Quantity), 64)
	if err := s.engine.SubmitPlace(o.ID.String(), o.UserID.String(), o.Symbol, string(o.Side), priceF, qtyF); err != nil {
		slog.Warn("engine.SubmitPlace failed; order remains OPEN", "id", o.ID, "err", err)
	}
	return o, nil
}

func (s *orderService) CancelOrder(ctx context.Context, id uuid.UUID) (*Order, error) {
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("CancelOrder get: %w", err)
	}
	if !IsCancellable(existing.Status) {
		return nil, shared.ErrInvalidStateTransition
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("CancelOrder begin: %w", err)
	}
	defer tx.Rollback(ctx)

	o, err := s.store.GetByIDTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("CancelOrder lock: %w", err)
	}
	if !IsCancellable(o.Status) {
		return nil, shared.ErrInvalidStateTransition
	}

	base, quote, err := SplitSymbol(o.Symbol)
	if err != nil {
		return nil, fmt.Errorf("CancelOrder: %w", err)
	}

	var reserveAsset string
	var totalReserve pgtype.Numeric
	switch o.Side {
	case SideBuy:
		reserveAsset = quote
		totalReserve = numeric.Mul(o.Price, o.Quantity)
	case SideSell:
		reserveAsset = base
		totalReserve = o.Quantity
	default:
		return nil, fmt.Errorf("CancelOrder: invalid side %q", o.Side)
	}

	var filledReserve pgtype.Numeric
	switch o.Side {
	case SideBuy:
		filledReserve = numeric.Mul(o.Price, o.FilledQuantity)
	case SideSell:
		filledReserve = o.FilledQuantity
	}

	unreserved := numeric.Sub(totalReserve, filledReserve)
	if numeric.Cmp(unreserved, numeric.Zero()) > 0 {
		accountID, err := s.accounts.AccountID(ctx, o.UserID, reserveAsset)
		if err != nil {
			return nil, fmt.Errorf("CancelOrder lookup account: %w", err)
		}
		entryID, err := ledger.NewLedgerEntryID()
		if err != nil {
			return nil, fmt.Errorf("CancelOrder entry id: %w", err)
		}
		now := timeutil.Now()
		entry := &ledger.LedgerEntry{
			ID:        entryID,
			UserID:    o.UserID,
			AccountID: accountID,
			Asset:     reserveAsset,
			Amount:    unreserved,
			EntryType: ledger.EntryTypeRelease,
			RefType:   ledger.RefTypeOrder,
			RefID:     o.ID,
			CreatedAt: &now,
		}
		if err := s.ledger.Insert(ctx, tx, entry); err != nil {
			return nil, fmt.Errorf("CancelOrder release: %w", err)
		}
	}

	if err := s.store.UpdateStatus(ctx, tx, o.ID, StatusCancelled); err != nil {
		return nil, fmt.Errorf("CancelOrder status: %w", err)
	}
	o.Status = StatusCancelled

	if err := s.insertOutbox(ctx, tx, outbox.EventTypeOrderCancelled, orderCancelledPayload(o)); err != nil {
		return nil, fmt.Errorf("CancelOrder outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CancelOrder commit: %w", err)
	}

	if err := s.engine.SubmitCancel(o.ID.String(), o.Symbol); err != nil {
		slog.Warn("engine.SubmitCancel failed", "id", o.ID, "err", err)
	}
	return o, nil
}

func (s *orderService) GetOrder(ctx context.Context, id uuid.UUID) (*Order, error) {
	o, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetOrder: %w", err)
	}
	return o, nil
}

func (s *orderService) ListOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error) {
	orders, err := s.store.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ListOrders: %w", err)
	}
	return orders, nil
}

func (s *orderService) insertOutbox(ctx context.Context, tx pgx.Tx, eventType outbox.EventType, payload []byte) error {
	id, err := outbox.NewID()
	if err != nil {
		return err
	}
	now := timeutil.Now()
	return s.outbox.Insert(ctx, tx, &outbox.OutboxEvent{
		ID:        id,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: &now,
	})
}

func orderPlacedPayload(o *Order) []byte {
	b, _ := json.Marshal(map[string]any{
		"order_id": o.ID.String(),
		"user_id":  o.UserID.String(),
		"symbol":   o.Symbol,
		"side":     o.Side,
		"price":    numeric.String(o.Price),
		"quantity": numeric.String(o.Quantity),
	})
	return b
}

func orderCancelledPayload(o *Order) []byte {
	b, _ := json.Marshal(map[string]any{
		"order_id": o.ID.String(),
		"user_id":  o.UserID.String(),
		"status":   o.Status,
	})
	return b
}
