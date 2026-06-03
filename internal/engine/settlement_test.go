package engine

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"mynance/internal/eventbus"
	"mynance/internal/trade"
)

type stubTradeService struct {
	calls []trade.ExecuteTradeCommand
}

func (s *stubTradeService) ExecuteTrade(ctx context.Context, cmd trade.ExecuteTradeCommand) (*trade.Trade, error) {
	s.calls = append(s.calls, cmd)
	return &trade.Trade{}, nil
}

func TestSettlement_OnTradeMatched_CallsExecuteTrade(t *testing.T) {
	stub := &stubTradeService{}
	sub := NewSettlementSubscriber(stub)

	buyID := uuid.NewString()
	sellID := uuid.NewString()
	sub.OnTradeMatched(eventbus.TradeMatchedEvent{
		Seq:         1,
		Symbol:      "BTC-USDT",
		BuyOrderID:  buyID,
		SellOrderID: sellID,
		Price:       30000.0,
		Quantity:    0.5,
		Timestamp:   time.Now(),
	})

	require.Len(t, stub.calls, 1)
	require.Equal(t, "BTC-USDT", stub.calls[0].Symbol)
	require.Contains(t, stub.calls[0].IdempotencyKey, "match-")
	require.Contains(t, stub.calls[0].IdempotencyKey, buyID)
	require.Contains(t, stub.calls[0].IdempotencyKey, sellID)
}

func TestSettlement_IgnoresWrongEventType(t *testing.T) {
	stub := &stubTradeService{}
	sub := NewSettlementSubscriber(stub)
	sub.OnTradeMatched(eventbus.OrderRestedEvent{})
	require.Len(t, stub.calls, 0)
}
