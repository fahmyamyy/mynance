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

type SettlementSubscriber struct {
	trades trade.Service
}

func NewSettlementSubscriber(trades trade.Service) *SettlementSubscriber {
	return &SettlementSubscriber{trades: trades}
}

func (s *SettlementSubscriber) OnTradeMatched(e eventbus.Event) {
	evt, ok := e.(eventbus.TradeMatchedEvent)
	if !ok {
		return
	}

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

	_, err = s.trades.ExecuteTrade(context.Background(), trade.ExecuteTradeCommand{
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
