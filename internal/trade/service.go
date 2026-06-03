package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/idempotency"
	"mynance/internal/ledger"
	"mynance/internal/order"
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
}

type OutboxStore interface {
	Insert(ctx context.Context, tx pgx.Tx, event *outbox.OutboxEvent) error
}

type OrderStore interface {
	GetByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*order.Order, error)
	IncrementFilled(ctx context.Context, tx pgx.Tx, id uuid.UUID, qty pgtype.Numeric) error
	UpdateStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status order.Status) error
}

type AccountLookup interface {
	AccountID(ctx context.Context, userID uuid.UUID, asset string) (uuid.UUID, error)
}

type ExecuteTradeCommand struct {
	Symbol         string
	BuyOrderID     uuid.UUID
	SellOrderID    uuid.UUID
	Price          pgtype.Numeric
	Quantity       pgtype.Numeric
	IdempotencyKey string
}

type Service interface {
	ExecuteTrade(ctx context.Context, cmd ExecuteTradeCommand) (*Trade, error)
}

type tradeService struct {
	db          *pgxpool.Pool
	store       Store
	idempotency IdempotencyStore
	ledger      LedgerStore
	orders      OrderStore
	outbox      OutboxStore
	accounts    AccountLookup
}

func NewService(
	db *pgxpool.Pool,
	store Store,
	idempotencyStore IdempotencyStore,
	ledgerStore LedgerStore,
	orderStore OrderStore,
	outboxStore OutboxStore,
	accounts AccountLookup,
) Service {
	return &tradeService{
		db:          db,
		store:       store,
		idempotency: idempotencyStore,
		ledger:      ledgerStore,
		orders:      orderStore,
		outbox:      outboxStore,
		accounts:    accounts,
	}
}

func (s *tradeService) ExecuteTrade(ctx context.Context, cmd ExecuteTradeCommand) (*Trade, error) {
	base, quote, err := order.SplitSymbol(cmd.Symbol)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.idempotency.Insert(ctx, tx, cmd.IdempotencyKey, idempotency.ScopeTrade); err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			return nil, err
		}
		return nil, fmt.Errorf("ExecuteTrade idempotency: %w", err)
	}

	buyOrder, err := s.orders.GetByIDTx(ctx, tx, cmd.BuyOrderID)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade buy lookup: %w", err)
	}
	sellOrder, err := s.orders.GetByIDTx(ctx, tx, cmd.SellOrderID)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade sell lookup: %w", err)
	}
	if buyOrder.Side != order.SideBuy {
		return nil, fmt.Errorf("ExecuteTrade: buy_order_id is not BUY")
	}
	if sellOrder.Side != order.SideSell {
		return nil, fmt.Errorf("ExecuteTrade: sell_order_id is not SELL")
	}
	if order.IsTerminal(buyOrder.Status) || order.IsTerminal(sellOrder.Status) {
		return nil, shared.ErrInvalidStateTransition
	}

	quoteAmount := numeric.Mul(cmd.Price, cmd.Quantity)
	baseAmount := cmd.Quantity

	buyBaseAcct, err := s.accounts.AccountID(ctx, buyOrder.UserID, base)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade buy base acct: %w", err)
	}
	buyQuoteAcct, err := s.accounts.AccountID(ctx, buyOrder.UserID, quote)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade buy quote acct: %w", err)
	}
	sellBaseAcct, err := s.accounts.AccountID(ctx, sellOrder.UserID, base)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade sell base acct: %w", err)
	}
	sellQuoteAcct, err := s.accounts.AccountID(ctx, sellOrder.UserID, quote)
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade sell quote acct: %w", err)
	}

	tradeID, err := NewID()
	if err != nil {
		return nil, fmt.Errorf("ExecuteTrade id: %w", err)
	}
	now := timeutil.Now()

	entries := []ledger.LedgerEntry{
		// Buyer: +base, -quote
		newEntry(buyOrder.UserID, buyBaseAcct, base, baseAmount, tradeID, &now),
		newEntry(buyOrder.UserID, buyQuoteAcct, quote, numeric.Neg(quoteAmount), tradeID, &now),
		// Seller: -base, +quote
		newEntry(sellOrder.UserID, sellBaseAcct, base, numeric.Neg(baseAmount), tradeID, &now),
		newEntry(sellOrder.UserID, sellQuoteAcct, quote, quoteAmount, tradeID, &now),
	}

	sum := numeric.Zero()
	for i := range entries {
		sum = numeric.Add(sum, entries[i].Amount)
	}
	if numeric.Cmp(sum, numeric.Zero()) != 0 {
		return nil, fmt.Errorf("ExecuteTrade: ledger entries do not sum to zero")
	}

	tr := &Trade{
		ID:          tradeID,
		Symbol:      cmd.Symbol,
		BuyOrderID:  cmd.BuyOrderID,
		SellOrderID: cmd.SellOrderID,
		BuyUserID:   buyOrder.UserID,
		SellUserID:  sellOrder.UserID,
		Price:       cmd.Price,
		Quantity:    cmd.Quantity,
		CreatedAt:   &now,
	}
	if err := s.store.Create(ctx, tx, tr); err != nil {
		return nil, fmt.Errorf("ExecuteTrade create: %w", err)
	}

	for i := range entries {
		id, err := ledger.NewLedgerEntryID()
		if err != nil {
			return nil, fmt.Errorf("ExecuteTrade entry id: %w", err)
		}
		entries[i].ID = id
		if err := s.ledger.Insert(ctx, tx, &entries[i]); err != nil {
			return nil, fmt.Errorf("ExecuteTrade ledger: %w", err)
		}
	}

	if err := s.applyOrderFill(ctx, tx, buyOrder, cmd.Quantity); err != nil {
		return nil, fmt.Errorf("ExecuteTrade buy fill: %w", err)
	}
	if err := s.applyOrderFill(ctx, tx, sellOrder, cmd.Quantity); err != nil {
		return nil, fmt.Errorf("ExecuteTrade sell fill: %w", err)
	}

	if err := s.insertOutbox(ctx, tx, outbox.EventTypeTradeExecuted, tradeExecutedPayload(tr)); err != nil {
		return nil, fmt.Errorf("ExecuteTrade outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ExecuteTrade commit: %w", err)
	}
	return tr, nil
}

func (s *tradeService) applyOrderFill(ctx context.Context, tx pgx.Tx, o *order.Order, qty pgtype.Numeric) error {
	if err := s.orders.IncrementFilled(ctx, tx, o.ID, qty); err != nil {
		return err
	}
	newFilled := numeric.Add(o.FilledQuantity, qty)
	cmp := numeric.Cmp(newFilled, o.Quantity)
	if cmp >= 0 {
		return s.orders.UpdateStatus(ctx, tx, o.ID, order.StatusFilled)
	}
	if o.Status == order.StatusOpen {
		return s.orders.UpdateStatus(ctx, tx, o.ID, order.StatusPartial)
	}
	return nil
}

func newEntry(userID, accountID uuid.UUID, asset string, amount pgtype.Numeric, tradeID uuid.UUID, now *time.Time) ledger.LedgerEntry {
	return ledger.LedgerEntry{
		UserID:    userID,
		AccountID: accountID,
		Asset:     asset,
		Amount:    amount,
		EntryType: ledger.EntryTypeTrade,
		RefType:   ledger.RefTypeTrade,
		RefID:     tradeID,
		CreatedAt: now,
	}
}

func (s *tradeService) insertOutbox(ctx context.Context, tx pgx.Tx, eventType outbox.EventType, payload []byte) error {
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

func tradeExecutedPayload(t *Trade) []byte {
	b, _ := json.Marshal(map[string]any{
		"trade_id":      t.ID.String(),
		"symbol":        t.Symbol,
		"buy_order_id":  t.BuyOrderID.String(),
		"sell_order_id": t.SellOrderID.String(),
		"price":         numeric.String(t.Price),
		"quantity":      numeric.String(t.Quantity),
	})
	return b
}
