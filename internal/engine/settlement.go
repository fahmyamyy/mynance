package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"mynance/internal/eventbus"
	"mynance/internal/shared"
	"mynance/internal/trade"
	"mynance/pkg/numeric"
)

// SettlementProcessor turns engine TradeMatched events into persisted trades.
// There are two implementations, picked by main.go based on cfg.Environment.
// Cutover from sandbox to production requires no code change — only the env
// flag (and turning the simbot off, which is already env-gated).
type SettlementProcessor interface {
	OnTradeMatched(e eventbus.Event)
}

// isPartner reports whether a trade side belongs to the partner exchange
// rather than a real user. Partner-sourced orders are ownerless (empty
// UserID); user orders always carry a non-empty UUID. This is the single
// point of counterparty classification — no registry or side table.
func isPartner(userID string) bool {
	return userID == ""
}

// realSettlementProcessor is the production-safe implementation. Any trade
// involving a partner side is dropped: in production the partner ingester
// never runs, so every order is a real user and every event settles. In a
// "sandbox-data, no partner liquidity" mode this also stays correct.
type realSettlementProcessor struct {
	trades trade.Service
}

// NewProcessor returns the production settlement processor. Trades with a
// partner (ownerless) side are dropped.
func NewProcessor(trades trade.Service) SettlementProcessor {
	return &realSettlementProcessor{trades: trades}
}

func (p *realSettlementProcessor) OnTradeMatched(e eventbus.Event) {
	evt, ok := e.(eventbus.TradeMatchedEvent)
	if !ok {
		return
	}
	if isPartner(evt.BuyUserID) || isPartner(evt.SellUserID) {
		return
	}
	settle(p.trades, evt)
}

// settle is the shared "happy path": turn the engine event into an
// ExecuteTrade call. Used by both processors once any sim-side rewriting is
// done. Idempotency key encodes (buy, sell, seq) so a duplicate event from
// the same match collapses on retry.
func settle(trades trade.Service, evt eventbus.TradeMatchedEvent) {
	buyID, err := uuid.Parse(evt.BuyOrderID)
	if err != nil {
		slog.Error("settlement: parse buy order id", "err", err)
		return
	}
	sellID, err := uuid.Parse(evt.SellOrderID)
	if err != nil {
		slog.Error("settlement: parse sell order id", "err", err)
		return
	}
	price, err := numeric.Parse(strconv.FormatFloat(evt.Price, 'f', -1, 64))
	if err != nil {
		slog.Error("settlement: parse price", "err", err)
		return
	}
	qty, err := numeric.Parse(strconv.FormatFloat(evt.Quantity, 'f', -1, 64))
	if err != nil {
		slog.Error("settlement: parse quantity", "err", err)
		return
	}

	idempKey := fmt.Sprintf("match-%s-%s-%d", evt.BuyOrderID, evt.SellOrderID, evt.Seq)

	_, err = trades.ExecuteTrade(context.Background(), trade.ExecuteTradeCommand{
		Symbol:         evt.Symbol,
		BuyOrderID:     buyID,
		SellOrderID:    sellID,
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: idempKey,
	})
	if err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			return
		}
		slog.Error("settlement: ExecuteTrade failed",
			"buy", evt.BuyOrderID, "sell", evt.SellOrderID, "err", err)
	}
}
