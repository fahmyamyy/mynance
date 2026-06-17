package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mynance/internal/eventbus"
	"mynance/internal/order"
	"mynance/internal/trade"
	"mynance/pkg/numeric"
	"mynance/pkg/timeutil"
)

// OrderInserter is the subset of order.Store the sandbox processor needs to
// persist a synthetic order row owned by the MARKET user. Declared on the
// consumer side per project convention so a test fake stays trivial.
type OrderInserter interface {
	Create(ctx context.Context, tx pgx.Tx, o *order.Order) error
}

// sandboxSettlementProcessor settles user↔partner trades by first inserting a
// synthetic order row owned by the MARKET user and then delegating to the
// regular ExecuteTrade path. Net effect: from the real user's perspective the
// fill looks identical to a user↔user match; the counterparty is just the
// partner (booked against the house). Both-partner trades are dropped — they
// would only pollute DB.
type sandboxSettlementProcessor struct {
	db           *pgxpool.Pool
	trades       trade.Service
	orders       OrderInserter
	marketUserID uuid.UUID
}

func NewSandboxProcessor(
	db *pgxpool.Pool,
	trades trade.Service,
	orders OrderInserter,
	marketUserID uuid.UUID,
) SettlementProcessor {
	return &sandboxSettlementProcessor{
		db:           db,
		trades:       trades,
		orders:       orders,
		marketUserID: marketUserID,
	}
}

func (p *sandboxSettlementProcessor) OnTradeMatched(e eventbus.Event) {
	evt, ok := e.(eventbus.TradeMatchedEvent)
	if !ok {
		return
	}

	buyIsPartner := isPartner(evt.BuyUserID)
	sellIsPartner := isPartner(evt.SellUserID)

	switch {
	case buyIsPartner && sellIsPartner:
		// Both sides are partner levels crossing inside the engine — nothing
		// to persist; ignore.
		return

	case !buyIsPartner && !sellIsPartner:
		settle(p.trades, evt)

	case buyIsPartner:
		marketID, err := p.persistMarketOrder(order.SideBuy, evt.Symbol, evt.Price, evt.Quantity)
		if err != nil {
			slog.Error("sandbox settlement: persist market buy", "err", err)
			return
		}
		evt.BuyOrderID = marketID.String()
		evt.BuyUserID = p.marketUserID.String()
		settle(p.trades, evt)

	case sellIsPartner:
		marketID, err := p.persistMarketOrder(order.SideSell, evt.Symbol, evt.Price, evt.Quantity)
		if err != nil {
			slog.Error("sandbox settlement: persist market sell", "err", err)
			return
		}
		evt.SellOrderID = marketID.String()
		evt.SellUserID = p.marketUserID.String()
		settle(p.trades, evt)
	}
}

// persistMarketOrder inserts a one-shot order row owned by MARKET. The row
// starts OPEN with filled_quantity=0 so the downstream ExecuteTrade call can
// run its normal increment-and-finalize logic. Inserted in its own tx and
// committed before ExecuteTrade so the trade tx can lock the row with
// SELECT ... FOR UPDATE.
func (p *sandboxSettlementProcessor) persistMarketOrder(side order.Side, symbol string, price, qty float64) (uuid.UUID, error) {
	id, err := order.NewID()
	if err != nil {
		return uuid.Nil, fmt.Errorf("new id: %w", err)
	}
	priceN, err := numeric.Parse(strconv.FormatFloat(price, 'f', -1, 64))
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse price: %w", err)
	}
	qtyN, err := numeric.Parse(strconv.FormatFloat(qty, 'f', -1, 64))
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse qty: %w", err)
	}

	now := timeutil.Now()
	o := &order.Order{
		ID:             id,
		UserID:         p.marketUserID,
		Symbol:         symbol,
		Side:           side,
		Price:          priceN,
		Quantity:       qtyN,
		FilledQuantity: numeric.Zero(),
		Status:         order.StatusOpen,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	ctx := context.Background()
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := p.orders.Create(ctx, tx, o); err != nil {
		return uuid.Nil, fmt.Errorf("create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}
